// Package auth owns credentials, route authorization and CLI auth commands.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/config"
	"golang.org/x/crypto/argon2"
)

// ErrNotFound is returned by a Store lookup that matches nothing.
var ErrNotFound = errors.New("auth: not found")

// User, Session and Token live only on this node; they never replicate.
type User struct {
	ID           string
	Username     string
	PasswordHash string
}
type Session struct {
	ID, UserID string
	ExpiresAt  time.Time
}
type Token struct {
	ID, UserID, Name, SecretHash string
	Permissions                  []string
	ExpiresAt                    *time.Time
}

// Store keeps the local accounts. Lookups by hash ignore expired and
// revoked entries.
type Store interface {
	FindUser(ctx context.Context, username string) (User, error)
	GetUser(ctx context.Context, id string) (User, error)
	CountUsers(ctx context.Context) (int, error)
	CreateUser(ctx context.Context, username, passwordHash string) (User, error)
	FindSession(ctx context.Context, secretHash string) (Session, error)
	CreateSession(ctx context.Context, userID, secretHash string, expires time.Time) error
	EndSession(ctx context.Context, id string) error
	FindToken(ctx context.Context, secretHash string) (Token, error)
	CreateToken(ctx context.Context, t Token) error
}

type identity struct {
	user    User
	session string
	scopes  []string
}
type identityKey struct{}

// Actor returns the authenticated user ID, or an empty string.
func Actor(ctx context.Context) string { v, _ := ctx.Value(identityKey{}).(identity); return v.user.ID }

// Allowed enforces token scopes. MCP handlers must call this for each tool:
// transport POST requests can contain both read and write operations.
// write includes read; admin includes all permissions. Sessions have admin access.
func Allowed(ctx context.Context, scope string) bool {
	v, ok := ctx.Value(identityKey{}).(identity)
	if !ok {
		return false
	}
	for _, s := range v.scopes {
		if s == "admin" || s == scope || s == "write" && scope == "read" {
			return true
		}
	}
	return false
}

// Secret returns a new random secret.
func Secret() string { return secret() }
func secret() string { return base64.RawURLEncoding.EncodeToString(randBytes(32)) }
func randBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}
func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func passwordHash(password string) string {
	salt := randBytes(16)
	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return "$argon2id$v=19$m=65536,t=3,p=2$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(hash)
}
func verify(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || strings.Join(parts[:4], "$") != "$argon2id$v=19$m=65536,t=3,p=2" {
		return false
	}
	salt, e := base64.RawStdEncoding.DecodeString(parts[4])
	if e != nil || len(salt) != 16 {
		return false
	}
	expected, e := base64.RawStdEncoding.DecodeString(parts[5])
	if e != nil || len(expected) != 32 {
		return false
	}
	return subtle.ConstantTimeCompare(expected, argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)) == 1
}
func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if value != nil {
		_ = json.NewEncoder(w).Encode(value)
	}
}
func failure(w http.ResponseWriter, status int) {
	reply(w, status, map[string]any{"error": map[string]string{"code": http.StatusText(status), "message": http.StatusText(status)}})
}
func publicUser(u User) any {
	return struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	}{u.ID, u.Username}
}

// Middleware authenticates /api and /mcp. agentToken, when set, is the
// secret this node hands to its own agents: it grants write access to /mcp.
func Middleware(svc Store, cfg config.Config, agentToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			mcp := path == "/mcp" || strings.HasPrefix(path, "/mcp/")
			api := path == "/api" || strings.HasPrefix(path, "/api/")
			if !mcp && (!api || path == "/api/auth/login" || path == "/api/auth/setup" || path == "/api/health") {
				next.ServeHTTP(w, r)
				return
			}
			var v identity
			var err error
			if header := r.Header.Get("Authorization"); header != "" {
				fields := strings.Fields(header)
				if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") || len(fields[1]) > 256 {
					failure(w, 401)
					return
				}
				var t Token
				if mcp && agentToken != "" && subtle.ConstantTimeCompare([]byte(fields[1]), []byte(agentToken)) == 1 {
					v.user, v.scopes = User{ID: "agent", Username: "agent"}, []string{"write"}
					ctx := context.WithValue(r.Context(), identityKey{}, v)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				t, err = svc.FindToken(r.Context(), digest(fields[1]))
				if err == nil {
					v.user, err = svc.GetUser(r.Context(), t.UserID)
					v.scopes = t.Permissions
				}
			} else if !mcp {
				var c *http.Cookie
				c, err = r.Cookie("dory_session")
				if err == nil {
					var s Session
					s, err = svc.FindSession(r.Context(), digest(c.Value))
					if err == nil {
						v.user, err = svc.GetUser(r.Context(), s.UserID)
						v.session = s.ID
						v.scopes = []string{"admin"}
					}
				}
			}
			if err != nil || v.user.ID == "" {
				failure(w, 401)
				return
			}
			ctx := context.WithValue(r.Context(), identityKey{}, v)
			scope := "read"
			if !mcp && r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
				scope = "write"
			}
			if !Allowed(ctx, scope) {
				failure(w, 403)
				return
			}
			// Reject cross-origin browser mutations, including requests to logout.
			if v.session != "" && scope == "write" && !sameOrigin(r, cfg) {
				failure(w, 403)
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
func sameOrigin(r *http.Request, cfg config.Config) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	// Behind a TLS-terminating proxy r.TLS is nil, so the configured public origin is the source of truth.
	if cfg.PublicOrigin != "" {
		return origin == cfg.PublicOrigin
	}
	u, err := url.Parse(origin)
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return err == nil && u.Scheme == scheme && u.Host == r.Host
}
func cookie(cfg config.Config, value string, expires time.Time, maxAge int) *http.Cookie {
	return &http.Cookie{Name: "dory_session", Value: value, Path: "/", HttpOnly: true, Secure: cfg.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: maxAge}
}

// AsPeer marks a request forwarded by a paired node: it acts with the rights
// of the user of that node, which is the same person (one owner per mesh).
func AsPeer(ctx context.Context, node string) context.Context {
	return context.WithValue(ctx, identityKey{}, identity{user: User{ID: "peer:" + node, Username: node}, scopes: []string{"admin"}})
}
