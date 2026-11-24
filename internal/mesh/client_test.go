package mesh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/mesh/peerapi"
)

func TestClientCreateThreadAndErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := start(t, ctx, "client").m, start(t, ctx, "api").m
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/threads", func(w http.ResponseWriter, r *http.Request) {
		var in peerapi.CreateThreadRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Title != "New" {
			t.Errorf("create input = %+v, %v", in, err)
		}
		reply(w, peerapi.CreateThreadResponse{ThreadID: "thread-1"})
	})
	mux.HandleFunc("POST /api/threads/{id}/interrupt", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != "thread/1" {
			t.Errorf("thread ID = %q", r.PathValue("id"))
		}
		apperr.WriteHTTP(w, r, apperr.New(apperr.Busy, "thread busy").With("thread", r.PathValue("id")))
	})
	b.API = mux
	if err := a.addPeer(ctx, b.log.Node(), "api", b.id.PublicKey(), fmt.Sprintf("127.0.0.1:%d", b.port)); err != nil {
		t.Fatal(err)
	}
	if err := b.addPeer(ctx, a.log.Node(), "client", a.id.PublicKey(), fmt.Sprintf("127.0.0.1:%d", a.port)); err != nil {
		t.Fatal(err)
	}
	c := a.Client()
	out, err := c.CreateThread(ctx, b.log.Node(), peerapi.CreateThreadRequest{Title: "New"})
	if err != nil || out.ThreadID != "thread-1" {
		t.Fatalf("create result = %+v, %v", out, err)
	}
	var apiErr *apperr.Error
	if err := c.InterruptThread(ctx, b.log.Node(), "thread/1"); !errors.As(err, &apiErr) || apiErr.Code != apperr.Busy || apiErr.Message != "thread busy" || apiErr.Params["thread"] != "thread/1" {
		t.Fatalf("interrupt error = %v", err)
	}
	if err := c.InterruptThread(ctx, "missing", "t1"); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("unreachable error = %v", err)
	}
}
