package auth

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/config"
	"github.com/migueel15/dory/internal/service"
)

type attempt struct {
	count int
	until time.Time
}
type limiter struct {
	sync.Mutex
	entries map[string]attempt
}

// Reserve before password hashing to also bound concurrent attempts and memory.
func (l *limiter) allow(key string) bool {
	l.Lock()
	defer l.Unlock()
	now := time.Now()
	for k, v := range l.entries {
		if now.After(v.until) {
			delete(l.entries, k)
		}
	}
	v := l.entries[key]
	if v.count >= 5 || v.count == 0 && len(l.entries) >= 4096 {
		return false
	}
	if v.count == 0 {
		v.until = now.Add(15 * time.Minute)
	}
	v.count++
	l.entries[key] = v
	return true
}

func Register(mux *http.ServeMux, svc service.Service, cfg config.Config) {
	attempts := &limiter{entries: map[string]attempt{}}
	// Bound argon2's aggregate memory, and use a dummy hash for unknown users.
	hashing := make(chan struct{}, 2)
	dummy := passwordHash(secret())
	mux.HandleFunc("POST /api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r, cfg) {
			failure(w, 403)
			return
		}
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if !attempts.allow(ip) {
			w.Header().Set("Retry-After", "900")
			failure(w, 429)
			return
		}
		var input struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
		if dec.Decode(&input) != nil || dec.Decode(new(any)) != io.EOF || input.Username == "" || input.Password == "" {
			failure(w, 400)
			return
		}
		select {
		case hashing <- struct{}{}:
			defer func() { <-hashing }()
		default:
			failure(w, 429)
			return
		}
		u, err := svc.FindUser(r.Context(), input.Username)
		if err != nil && !errors.Is(err, service.ErrNotFound) {
			failure(w, 500)
			return
		}
		hash := u.PasswordHash
		if err != nil {
			hash = dummy
		}
		valid := verify(input.Password, hash)
		if !valid || err != nil {
			failure(w, 401)
			return
		}
		value := secret()
		ttl := cfg.SessionTTL
		if ttl <= 0 {
			ttl = 7 * 24 * time.Hour
		}
		expires := time.Now().Add(ttl)
		_, err = svc.Sessions().Put(r.Context(), u.ID, service.Session{UserID: u.ID, SecretHash: digest(value), ExpiresAt: expires})
		if err != nil {
			failure(w, 500)
			return
		}
		http.SetCookie(w, cookie(cfg, value, expires, int(ttl.Seconds())))
		reply(w, 200, publicUser(u))
	})
	mux.HandleFunc("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		v, ok := r.Context().Value(identityKey{}).(identity)
		if !ok {
			failure(w, 401)
			return
		}
		if v.session != "" {
			if err := svc.Sessions().Archive(r.Context(), v.user.ID, v.session); err != nil {
				failure(w, 500)
				return
			}
		}
		http.SetCookie(w, cookie(cfg, "", time.Unix(1, 0), -1))
		reply(w, 204, nil)
	})
	mux.HandleFunc("GET /api/auth/me", func(w http.ResponseWriter, r *http.Request) {
		v, ok := r.Context().Value(identityKey{}).(identity)
		if !ok {
			failure(w, 401)
			return
		}
		reply(w, 200, publicUser(v.user))
	})
}
