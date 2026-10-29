// Package auth owns credentials, route authorization and CLI auth commands.
package auth

import (
	"context"
	"github.com/migueel15/dory/internal/config"
	"github.com/migueel15/dory/internal/service"
	"net/http"
)

// Middleware is deliberately fail-closed until the auth slice implements it.
// The final implementation must allow /api/auth/login and public web assets,
// require sessions or bearer credentials for /api, and bearer credentials for /mcp.
func Middleware(svc service.Service, cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, "authentication implementation pending", http.StatusServiceUnavailable)
		})
	}
}

// Register owns /api/auth/login and /api/auth/logout; API must not register them.
func Register(mux *http.ServeMux, svc service.Service, cfg config.Config) {}

// Command handles user/token subcommands. handled=false means unknown command.
func Command(ctx context.Context, args []string, svc service.Service) (handled bool, err error) {
	return false, nil
}
