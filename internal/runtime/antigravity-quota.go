package runtime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/cmdexec"
)

const (
	antigravityQuotaDaily  = "https://daily-cloudcode-pa.googleapis.com"
	antigravityQuotaProd   = "https://cloudcode-pa.googleapis.com"
	antigravitySecretLimit = 64 << 10
)

type agyToken struct {
	AccessToken string    `json:"access_token"`
	Expiry      time.Time `json:"expiry"`
}

func parseAgyToken(raw []byte) agyToken {
	raw = bytes.TrimSpace(raw)
	if value, ok := bytes.CutPrefix(raw, []byte("go-keyring-base64:")); ok {
		decoded, err := base64.StdEncoding.DecodeString(string(value))
		if err != nil {
			return agyToken{}
		}
		raw = decoded
	}
	var record struct {
		agyToken
		Token *agyToken `json:"token"`
	}
	if json.Unmarshal(raw, &record) != nil {
		return agyToken{}
	}
	if record.Token != nil {
		return *record.Token
	}
	return record.agyToken
}

func loadAgyToken(ctx context.Context) agyToken {
	if token := parseAgyToken(readAgyKeyring(ctx)); token.AccessToken != "" {
		return token
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return agyToken{}
	}
	for _, path := range []string{
		filepath.Join(home, ".gemini", "antigravity-cli", "antigravity-oauth-token"),
		filepath.Join(home, ".gemini", "jetski-standalone-oauth-token"),
	} {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(f, antigravitySecretLimit))
		f.Close()
		if err == nil {
			if token := parseAgyToken(raw); token.AccessToken != "" {
				return token
			}
		}
	}
	return agyToken{}
}

var antigravityQuotaClient = &http.Client{
	Timeout:       8 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// Usage fetches quota for the account signed in to the CLI. Let agy handle
// token renewal so Dory never needs refresh tokens or OAuth client secrets.
func (a Antigravity) Usage(ctx context.Context) ([]Window, error) {
	bin, err := exec.LookPath(a.bin())
	if err != nil {
		return nil, ErrNoUsage
	}
	ctx, cancel := context.WithTimeout(ctx, usageTimeout)
	defer cancel()
	token := loadAgyToken(ctx)
	if token.AccessToken == "" {
		return nil, ErrNoUsage
	}
	if !token.Expiry.IsZero() && time.Until(token.Expiry) < time.Minute {
		refresh, stop := context.WithTimeout(ctx, 12*time.Second)
		_ = cmdexec.CommandContext(refresh, bin, "models").Run()
		stop()
		token = loadAgyToken(ctx)
		if token.AccessToken == "" || (!token.Expiry.IsZero() && time.Until(token.Expiry) <= 0) {
			return nil, ErrNoUsage
		}
	}
	return fetchAgyQuota(ctx, antigravityQuotaClient, token.AccessToken, antigravityQuotaDaily, antigravityQuotaProd)
}

type agyQuotaBucket struct {
	BucketID          string
	DisplayName       string
	Window            string
	RemainingFraction *float64
	ResetTime         string
	Disabled          bool
}

type agyQuotaSummary struct {
	Groups []struct {
		DisplayName string
		Buckets     []agyQuotaBucket
	}
	Buckets []agyQuotaBucket
}

func (summary agyQuotaSummary) windows() []Window {
	windows := map[string]Window{}
	add := func(group string, buckets []agyQuotaBucket) {
		for _, bucket := range buckets {
			if bucket.Disabled || bucket.RemainingFraction == nil || bucket.BucketID == "" {
				continue
			}
			fraction := *bucket.RemainingFraction
			if fraction < 0 || fraction > 1 {
				continue
			}
			reset, _ := time.Parse(time.RFC3339Nano, bucket.ResetTime)
			label := strings.TrimSpace(group)
			if label == "" {
				label = strings.TrimSpace(bucket.DisplayName)
			}
			if label == "" {
				label = bucket.BucketID
			}
			duration := 0
			switch {
			case bucket.Window == "5h" || strings.HasSuffix(bucket.BucketID, "-5h"):
				duration = 5 * 60
				label += " · 5 horas"
			case bucket.Window == "weekly" || strings.HasSuffix(bucket.BucketID, "-weekly"):
				duration = 7 * 24 * 60
				label += " · Semanal"
			}
			if _, exists := windows[bucket.BucketID]; !exists {
				windows[bucket.BucketID] = Window{Kind: bucket.BucketID, Label: label, Minutes: duration, Utilization: 1 - fraction, ResetsAt: reset.UTC()}
			}
		}
	}
	for _, group := range summary.Groups {
		add(group.DisplayName, group.Buckets)
	}
	add("", summary.Buckets)
	out := make([]Window, 0, len(windows))
	for _, w := range windows {
		out = append(out, w)
	}
	return sortWindows(out)
}

func fetchAgyQuota(ctx context.Context, client *http.Client, token string, bases ...string) ([]Window, error) {
	type result struct {
		windows []Window
		err     error
	}
	pending := make([]chan result, len(bases))
	for i, base := range bases {
		ch := make(chan result, 1)
		pending[i] = ch
		go func() {
			windows, err := agyQuotaFromHost(ctx, client, base, token)
			ch <- result{windows, err}
		}()
	}
	seen := map[string]bool{}
	var out []Window
	var lastErr error
	for _, ch := range pending {
		response := <-ch
		if response.err != nil {
			lastErr = response.err
		}
		for _, w := range response.windows {
			if !seen[w.Kind] {
				out = append(out, w)
				seen[w.Kind] = true
			}
		}
	}
	if len(out) > 0 {
		return sortWindows(out), nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrNoUsage
}

func agyQuotaFromHost(ctx context.Context, client *http.Client, base, token string) ([]Window, error) {
	var enrollment struct {
		Project string `json:"cloudaicompanionProject"`
	}
	if err := agyQuotaPost(ctx, client, base, "loadCodeAssist", token, map[string]any{"metadata": map[string]string{"ideType": "ANTIGRAVITY"}}, &enrollment); err != nil {
		return nil, err
	}
	var summary agyQuotaSummary
	params := map[string]string{}
	if enrollment.Project != "" {
		params["project"] = enrollment.Project
	}
	if err := agyQuotaPost(ctx, client, base, "retrieveUserQuotaSummary", token, params, &summary); err != nil {
		return nil, err
	}
	return summary.windows(), nil
}

func agyQuotaPost(ctx context.Context, client *http.Client, base, operation, token string, payload, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1internal:"+operation, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Dory/2 antigravity")
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Antigravity %s: %w", operation, err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized {
		return ErrNoUsage
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Antigravity %s: HTTP %d", operation, response.StatusCode)
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(out) != nil {
		return errors.New("Antigravity: invalid quota response")
	}
	return nil
}
