package mcp

import (
	"context"

	"github.com/migueel15/dory/internal/apperr"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func addServiceTools(srv *sdk.Server, s *Deps) {
	add(srv, "start_service", "Run a long-lived process for this thread (dev server, database, watcher) detached in its folder; the user sees its ports. Use it instead of backgrounding in your shell.", func(ctx context.Context, in struct {
		Command string `json:"command" jsonschema:"Shell command to run, e.g. npm run dev -- --port 5173"`
	}) (any, error) {
		id, _ := ctx.Value(threadKey{}).(string)
		if id == "" {
			return nil, apperr.New(apperr.Invalid, "start_service only works inside a Dory thread")
		}
		pid, log, err := s.Runtime.StartService(ctx, id, in.Command)
		if err != nil {
			return nil, err
		}
		return map[string]any{"pid": pid, "log": log}, nil
	})
	add(srv, "list_services", "List this thread's processes listening on a port.", func(ctx context.Context, _ struct{}) (any, error) {
		id, _ := ctx.Value(threadKey{}).(string)
		if id == "" {
			return nil, apperr.New(apperr.Invalid, "list_services only works inside a Dory thread")
		}
		return s.Runtime.ThreadServices(id), nil
	})
	add(srv, "stop_service", "Stop one of this thread's listening processes by pid.", func(ctx context.Context, in struct {
		PID int `json:"pid"`
	}) (any, error) {
		id, _ := ctx.Value(threadKey{}).(string)
		if id == "" {
			return nil, apperr.New(apperr.Invalid, "stop_service only works inside a Dory thread")
		}
		if err := s.Runtime.StopService(id, in.PID); err != nil {
			return nil, err
		}
		return map[string]any{"stopped": in.PID}, nil
	})
}
