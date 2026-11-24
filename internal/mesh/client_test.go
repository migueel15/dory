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
	"github.com/migueel15/dory/internal/outbox"
	"github.com/migueel15/dory/internal/runtime"
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

func TestClientDeliverSendsTheDeliveryIDAndClassifiesErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := start(t, ctx, "client").m, start(t, ctx, "api").m
	answer := apperr.Newf(apperr.NotFound, "thread not found")
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/threads/{id}/delegations/complete", func(w http.ResponseWriter, r *http.Request) {
		var o runtime.Outcome
		if err := json.NewDecoder(r.Body).Decode(&o); err != nil || o.DeliveryID != "dlv-1" || o.ParentThreadID != "parent" || r.PathValue("id") != "parent" {
			t.Errorf("outcome = %+v, %v", o, err)
		}
		if answer != nil {
			apperr.WriteHTTP(w, r, answer)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	b.API = mux
	if err := a.addPeer(ctx, b.log.Node(), "api", b.id.PublicKey(), fmt.Sprintf("127.0.0.1:%d", b.port)); err != nil {
		t.Fatal(err)
	}
	if err := b.addPeer(ctx, a.log.Node(), "client", a.id.PublicKey(), fmt.Sprintf("127.0.0.1:%d", a.port)); err != nil {
		t.Fatal(err)
	}
	c, node := a.Client(), b.log.Node()
	payload := json.RawMessage(`{"parent_thread_id":"parent","status":"completed"}`)
	// Not found yet (the parent may not have synced) and unreachable nodes are retried.
	if err := c.Deliver(ctx, node, runtime.DelegationComplete, "dlv-1", payload); err == nil || outbox.IsPermanent(err) {
		t.Fatalf("404 = %v", err)
	}
	if err := c.Deliver(ctx, "missing", runtime.DelegationComplete, "dlv-1", payload); err == nil || outbox.IsPermanent(err) {
		t.Fatalf("unreachable = %v", err)
	}
	answer = apperr.New(apperr.Invalid, "bad outcome")
	if err := c.Deliver(ctx, node, runtime.DelegationComplete, "dlv-1", payload); !outbox.IsPermanent(err) {
		t.Fatalf("400 = %v", err)
	}
	if err := c.Deliver(ctx, node, "nope", "dlv-1", payload); !outbox.IsPermanent(err) {
		t.Fatalf("unknown kind = %v", err)
	}
	if err := c.Deliver(ctx, node, runtime.DelegationComplete, "dlv-1", json.RawMessage(`[1]`)); !outbox.IsPermanent(err) {
		t.Fatalf("invalid payload = %v", err)
	}
	answer = nil
	if err := c.Deliver(ctx, node, runtime.DelegationComplete, "dlv-1", payload); err != nil {
		t.Fatalf("delivery = %v", err)
	}
}
