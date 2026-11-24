package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// ClientID identifies the Dory OAuth App. It is public: the device flow uses
// no client secret. DORY_GITHUB_CLIENT_ID overrides it for forks.
const ClientID = "Ov23liYBASJWXGGZ1IuE"

// Token sources, in priority order.
const (
	SourceEnv  = "env"
	SourceUI   = "ui"
	SourceGh   = "gh"
	SourceNone = "none"
)

const (
	tokenTTL    = 30 * time.Second
	loginBase   = "https://github.com"
	deviceScope = "repo read:org"
)

// Auth resolves the token on demand: DORY_GITHUB_TOKEN, then the token saved
// from the interface, then the gh CLI. The token never leaves this node.
type Auth struct {
	// File stores the token saved from the interface.
	File string
	// Gh returns the token of the gh CLI, empty when not logged in.
	Gh func(context.Context) string
	// LoginBase is github.com; tests replace it.
	LoginBase string
	HTTP      *http.Client
	// OnChange runs when the resolved token changes.
	OnChange func()

	mu       sync.Mutex
	token    string
	source   string
	resolved time.Time
	device   *device
}

// NewAuth stores the interface token in file.
func NewAuth(file string, gh func(context.Context) string) *Auth {
	return &Auth{File: file, Gh: gh, LoginBase: loginBase, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

// Token returns the current token and its source, resolving again when the
// cache expired or no token was found.
func (a *Auth) Token(ctx context.Context) (string, string) {
	if a == nil {
		return "", SourceNone
	}
	a.mu.Lock()
	if a.token != "" && time.Since(a.resolved) < tokenTTL {
		defer a.mu.Unlock()
		return a.token, a.source
	}
	a.mu.Unlock()
	token, source := a.resolve(ctx)
	a.mu.Lock()
	changed := token != a.token
	a.token, a.source, a.resolved = token, source, time.Now()
	a.mu.Unlock()
	if changed && a.OnChange != nil {
		a.OnChange()
	}
	return token, source
}

// Invalidate forces the next request to resolve the token again, after
// GitHub rejected it.
func (a *Auth) Invalidate() {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.resolved = time.Time{}
	a.mu.Unlock()
}

func (a *Auth) resolve(ctx context.Context) (string, string) {
	if t := strings.TrimSpace(os.Getenv("DORY_GITHUB_TOKEN")); t != "" {
		return t, SourceEnv
	}
	if b, err := os.ReadFile(a.File); err == nil {
		if t := strings.TrimSpace(string(b)); t != "" {
			return t, SourceUI
		}
	}
	if a.Gh != nil {
		if t := a.Gh(ctx); t != "" {
			return t, SourceGh
		}
	}
	return "", SourceNone
}

// Logout removes the token saved from the interface.
func (a *Auth) Logout(ctx context.Context) error {
	if err := os.Remove(a.File); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	a.mu.Lock()
	a.device = nil
	a.mu.Unlock()
	a.Invalidate()
	a.Token(ctx)
	return nil
}

func (a *Auth) save(token string) error {
	if err := os.WriteFile(a.File, []byte(token), 0o600); err != nil {
		return err
	}
	a.Invalidate()
	return nil
}

func clientID() string {
	if id := os.Getenv("DORY_GITHUB_CLIENT_ID"); id != "" {
		return id
	}
	return ClientID
}

// Device flow states.
const (
	DevicePending = "pending"
	DeviceDone    = "done"
	DeviceExpired = "expired"
	DeviceDenied  = "denied"
	DeviceFailed  = "failed"
)

// DeviceCode is what the user needs to authorize this node.
type DeviceCode struct {
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
}

// DeviceStatus reports the background polling.
type DeviceStatus struct {
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

type device struct {
	cancel context.CancelFunc
	status DeviceStatus
}

func (a *Auth) post(ctx context.Context, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.LoginBase+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := a.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github: %s returned %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// StartDevice asks GitHub for a code and polls in the background until the
// user authorizes it, saving the token. It replaces any running flow.
func (a *Auth) StartDevice(ctx context.Context) (DeviceCode, error) {
	var code struct {
		DeviceCode
		Device   string `json:"device_code"`
		Interval int    `json:"interval"`
		Error    string `json:"error"`
	}
	if err := a.post(ctx, "/login/device/code", url.Values{"client_id": {clientID()}, "scope": {deviceScope}}, &code); err != nil {
		return DeviceCode{}, err
	}
	if code.Error != "" || code.Device == "" {
		return DeviceCode{}, fmt.Errorf("github: could not start the device flow (%s)", code.Error)
	}
	pollCtx, cancel := context.WithTimeout(context.Background(), time.Duration(code.ExpiresIn)*time.Second)
	d := &device{cancel: cancel, status: DeviceStatus{State: DevicePending}}
	a.mu.Lock()
	if a.device != nil {
		a.device.cancel()
	}
	a.device = d
	a.mu.Unlock()
	go a.poll(pollCtx, d, code.Device, time.Duration(max(code.Interval, 1))*time.Second)
	return code.DeviceCode, nil
}

// Device returns the state of the last flow, or nil when none ran.
func (a *Auth) Device() *DeviceStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.device == nil {
		return nil
	}
	s := a.device.status
	return &s
}

func (a *Auth) finish(d *device, state, msg string) {
	a.mu.Lock()
	d.status = DeviceStatus{State: state, Error: msg}
	a.mu.Unlock()
	d.cancel()
}

func (a *Auth) poll(ctx context.Context, d *device, code string, interval time.Duration) {
	form := url.Values{"client_id": {clientID()}, "device_code": {code}, "grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}}
	for {
		select {
		case <-ctx.Done():
			a.mu.Lock()
			pending := d.status.State == DevicePending
			a.mu.Unlock()
			if pending && errors.Is(ctx.Err(), context.DeadlineExceeded) {
				a.finish(d, DeviceExpired, "")
			}
			return
		case <-time.After(interval):
		}
		var res struct {
			AccessToken string `json:"access_token"`
			Error       string `json:"error"`
			Interval    int    `json:"interval"`
		}
		if err := a.post(ctx, "/login/oauth/access_token", form, &res); err != nil {
			continue
		}
		switch {
		case res.AccessToken != "":
			if err := a.save(res.AccessToken); err != nil {
				a.finish(d, DeviceFailed, "save token: "+err.Error())
				return
			}
			a.finish(d, DeviceDone, "")
			a.Token(context.Background())
			return
		case res.Error == "authorization_pending":
		case res.Error == "slow_down":
			if res.Interval > 0 {
				interval = time.Duration(res.Interval) * time.Second
			} else {
				interval += 5 * time.Second
			}
		case res.Error == "expired_token":
			a.finish(d, DeviceExpired, "")
			return
		case res.Error == "access_denied":
			a.finish(d, DeviceDenied, "")
			return
		default:
			a.finish(d, DeviceFailed, res.Error)
			return
		}
	}
}
