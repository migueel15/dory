package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/config"
)

// devicePrefix marks the tokens of paired phones, so they can be listed and
// revoked apart from the tokens made for scripts and the MCP.
const devicePrefix = "device:"

// Device is a paired phone as the interface shows it.
type Device struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// DeviceStore adds what device pairing needs to the account store.
type DeviceStore interface {
	Store
	OwnerID(ctx context.Context) (string, error)
	ListTokens(ctx context.Context, prefix string) ([]Device, error)
	RevokeToken(ctx context.Context, id string) error
}

// IssueDevice creates the token of a newly paired phone for the owner of
// this node and returns its secret, shown only to that phone.
func IssueDevice(ctx context.Context, svc DeviceStore, name string) (string, error) {
	owner, err := svc.OwnerID(ctx)
	if err != nil {
		return "", errors.New("este nodo todavía no tiene cuenta")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "móvil"
	}
	value := secret()
	return value, svc.CreateToken(ctx, Token{UserID: owner, Name: devicePrefix + name, SecretHash: digest(value), Permissions: []string{"admin"}})
}

// DeviceSession trades the token of a phone for a session cookie, so the
// web view of the app works as the browser does.
func DeviceSession(w http.ResponseWriter, r *http.Request, svc Store, cfg config.Config) {
	fields := strings.Fields(r.Header.Get("Authorization"))
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") || len(fields[1]) > 256 {
		failure(w, 401)
		return
	}
	t, err := svc.FindToken(r.Context(), digest(fields[1]))
	if err != nil || !strings.HasPrefix(t.Name, devicePrefix) {
		failure(w, 401)
		return
	}
	u, err := svc.GetUser(r.Context(), t.UserID)
	if err != nil {
		failure(w, 401)
		return
	}
	cfg.CookieSecure = cfg.CookieSecure || r.TLS != nil
	// Short sessions: the app trades its token again on each start, and a
	// revoked phone loses access within this time.
	cfg.SessionTTL = 12 * time.Hour
	if err = startSession(w, r, svc, cfg, u); err != nil {
		failure(w, 500)
		return
	}
	reply(w, 200, publicUser(u))
}

// RegisterDevices adds the routes that list and revoke paired phones.
func RegisterDevices(mux *http.ServeMux, svc DeviceStore) {
	mux.HandleFunc("GET /api/devices", func(w http.ResponseWriter, r *http.Request) {
		list, err := svc.ListTokens(r.Context(), devicePrefix)
		if err != nil {
			failure(w, 500)
			return
		}
		for i := range list {
			list[i].Name = strings.TrimPrefix(list[i].Name, devicePrefix)
		}
		reply(w, 200, list)
	})
	mux.HandleFunc("DELETE /api/devices/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := svc.RevokeToken(r.Context(), r.PathValue("id")); err != nil {
			failure(w, 500)
			return
		}
		reply(w, 204, nil)
	})
}
