package mesh

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"
)

// A WebSocket to a thread of another node reaches it through the mesh, as
// the browser panel and its tunnel do.
func TestForwardWebSocket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := start(t, ctx, "a"), start(t, ctx, "b")
	a.m.API = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Log("accept", err)
			return
		}
		defer ws.CloseNow()
		_, m, _ := ws.Read(r.Context())
		ws.Write(r.Context(), websocket.MessageText, m)
	})
	code, _ := a.m.PairCode()
	if _, err := b.m.Pair(ctx, fmt.Sprintf("127.0.0.1:%d", a.m.port), code); err != nil {
		t.Fatal(err)
	}
	eventually(t, "presence", func() bool { pb, _ := b.m.Peers(ctx); return len(pb) == 1 && pb[0].Connected })
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !b.m.Forward(w, r, a.m.log.Node()) {
			t.Log("forward failed")
		}
	}))
	defer front.Close()
	ws, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(front.URL, "http")+"/api/x", nil)
	if err != nil {
		t.Fatal(err, resp)
	}
	ws.Write(ctx, websocket.MessageText, []byte("hi"))
	_, m, err := ws.Read(ctx)
	if err != nil || string(m) != "hi" {
		t.Fatalf("echo %q, %v", m, err)
	}
}
