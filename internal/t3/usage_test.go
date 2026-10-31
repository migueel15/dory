package t3

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const configExit = `{"_tag":"Exit","requestId":"1","exit":{"_tag":"Success","value":{"providers":[
{"instanceId":"codex","displayName":"Codex","enabled":true,"usageLimits":{"checkedAt":"2026-10-31T04:00:00Z","windows":[{"kind":"weekly","label":"Weekly","usedPercent":8,"resetsAt":"2026-11-04T00:00:00Z","windowDurationMins":10080}]}},
{"instanceId":"opencode","displayName":"OpenCode","enabled":true},
{"instanceId":"cursor","displayName":"Cursor","enabled":false}]}}}`

func usageServer(t *testing.T, summary string) Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/websocket-ticket", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"ticket":"a+b"}`))
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("wsTicket") != "a+b" || r.URL.Query().Get("orchestrationProtocol") != "2" {
			w.WriteHeader(http.StatusUpgradeRequired)
			return
		}
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer ws.CloseNow()
		for _, reply := range []string{`{"_tag":"Ping"}` + "\n" + configExit, summary} {
			_, raw, err := ws.Read(r.Context())
			if err != nil {
				return
			}
			var req struct {
				Tag string `json:"_tag"`
			}
			json.Unmarshal(raw, &req)
			if req.Tag != "Request" {
				t.Errorf("unexpected message %s", raw)
			}
			ws.Write(r.Context(), websocket.MessageText, []byte(strings.ReplaceAll(reply, "\n{\"instanceId", "{\"instanceId")))
			if strings.Contains(reply, "Ping") {
				ws.Read(r.Context()) // Pong
			}
		}
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return Client{BaseURL: s.URL, Token: "good", HTTP: s.Client()}
}

func TestUsageKeepsLimitsWithoutDiagnosticsScope(t *testing.T) {
	c := usageServer(t, `{"_tag":"Exit","requestId":"2","exit":{"_tag":"Failure","cause":[{"_tag":"Fail","error":{"_tag":"EnvironmentAuthorizationError"}}]}}`)
	u, err := c.Usage(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if u.ModelsError != UsageMissingScope || len(u.Models) != 0 {
		t.Fatalf("models = %q %v", u.ModelsError, u.Models)
	}
	if len(u.Providers) != 2 || u.Providers[0].Windows[0].UsedPercent != 8 || len(u.Providers[1].Windows) != 0 {
		t.Fatalf("providers = %+v", u.Providers)
	}
}

func TestUsageSeparatesEstimatedCost(t *testing.T) {
	bucket := func(source string, cost string) string {
		return `{"provider":"claude","model":"opus","totals":{"uncachedInputTokens":1,"cachedInputTokens":2,"cacheCreationTokens":3,"outputTokens":4},"costUsd":` + cost + `,"costSource":"` + source + `"}`
	}
	c := usageServer(t, `{"_tag":"Exit","requestId":"2","exit":{"_tag":"Success","value":{"buckets":[`+
		bucket("modelPriced", "1")+","+bucket("modelPriced", "2")+","+bucket("providerReported", "5")+`]}}}`)
	u, err := c.Usage(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(u.Models) != 2 || u.Models[0].CostSource != "modelPriced" || u.Models[0].CostUSD != 3 || u.Models[0].InputTokens != 8 || u.Models[1].CostUSD != 5 {
		t.Fatalf("models = %+v", u.Models)
	}
}

func TestUsageRejectedToken(t *testing.T) {
	c := usageServer(t, "")
	c.Token = "bad"
	if _, err := c.Usage(context.Background(), time.Now()); err != errUnauthorized {
		t.Fatalf("err = %v", err)
	}
}

func TestRefreshProviders(t *testing.T) {
	c := usageServer(t, "")
	if err := c.RefreshProviders(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.Token = "bad"
	if err := c.RefreshProviders(context.Background()); err != errUnauthorized {
		t.Fatalf("err = %v", err)
	}
}
