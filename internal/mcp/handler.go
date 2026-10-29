// Package mcp owns the streamable HTTP transport and MCP tools.
package mcp

import (
	"github.com/migueel15/dory/internal/service"
	"net/http"
)

func Handler(svc service.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "MCP implementation pending", http.StatusNotImplemented)
	})
}
