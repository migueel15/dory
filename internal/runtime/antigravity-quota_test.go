package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

type fixtureQuotaProvider struct {
	Antigravity
	windows []Window
}

func (p fixtureQuotaProvider) Usage(context.Context) ([]Window, error) { return p.windows, nil }

func TestAntigravityQuotaStore(t *testing.T) {
	reset := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	makeHost := func(project, buckets string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("User-Agent") != "Dory/2 antigravity" {
				t.Error("unexpected quota request")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			switch r.URL.Path {
			case "/v1internal:loadCodeAssist":
				fmt.Fprintf(w, `{"cloudaicompanionProject":%q}`, project)
			case "/v1internal:retrieveUserQuotaSummary":
				var args struct{ Project string }
				if err := json.NewDecoder(r.Body).Decode(&args); err != nil || args.Project != project {
					t.Errorf("invalid quota project: %+v (%v)", args, err)
				}
				fmt.Fprintf(w, buckets, reset.Format(time.RFC3339))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
	}
	daily := makeHost("daily", `{"buckets":[{"bucketId":"gemini-5h","displayName":"Gemini","remainingFraction":0.25,"resetTime":%q},{"bucketId":"disabled","remainingFraction":0.5,"disabled":true}]}`)
	defer daily.Close()
	prod := makeHost("prod", `{"groups":[{"displayName":"Gemini","buckets":[{"bucketId":"gemini-5h","remainingFraction":1},{"bucketId":"gemini-weekly","window":"weekly","remainingFraction":0.75,"resetTime":%q},{"bucketId":"missing-balance","remainingAmount":12}]}]}`)
	defer prod.Close()

	got, err := fetchAgyQuota(t.Context(), daily.Client(), "test-token", daily.URL, prod.URL)
	if err != nil {
		t.Fatal(err)
	}
	want := []Window{
		{Kind: "gemini-5h", Label: "Gemini · 5 horas", Minutes: 300, Utilization: 0.75, ResetsAt: reset},
		{Kind: "gemini-weekly", Label: "Gemini · Semanal", Minutes: 10080, Utilization: 0.25, ResetsAt: reset},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("quota windows = %+v, want %+v", got, want)
	}
	p := fixtureQuotaProvider{windows: append(got, Window{Kind: "expired", ResetsAt: reset.Add(-2 * time.Hour), Utilization: 0.5})}
	dir := t.TempDir()
	m := NewManager(nil, nil, "node", dir, p)
	m.RefreshUsage(t.Context())
	restarted := NewManager(nil, nil, "node", dir, p)
	if got := restarted.Usage()[antigravityID]; !reflect.DeepEqual(got, want) {
		t.Fatalf("stored and visible windows = %+v, want %+v", got, want)
	}
}

func TestAntigravityTokenFormats(t *testing.T) {
	nested := `{"token":{"access_token":"token","expiry":"2126-01-01T00:00:00Z"},"auth_method":"oauth-personal"}`
	for _, raw := range []string{nested, "go-keyring-base64:" + base64.StdEncoding.EncodeToString([]byte(nested)), `{"access_token":"token"}`} {
		if token := parseAgyToken([]byte(raw)); token.AccessToken != "token" {
			t.Error("failed to read access token")
		}
	}
	if token := parseAgyToken([]byte("go-keyring-base64::corrupt")); token.AccessToken != "" {
		t.Error("accepted a corrupt keyring record")
	}
}
