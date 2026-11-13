package t3

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/coder/websocket"
)

// echoServer answers every request on the same socket with its own id and
// counts tickets and connections.
func echoServer(t *testing.T, tickets, conns *atomic.Int32) Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/websocket-ticket/rpc", func(w http.ResponseWriter, r *http.Request) {
		tickets.Add(1)
		w.Write([]byte(`{"ticket":"tk"}`))
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conns.Add(1)
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		var mu sync.Mutex
		for {
			_, data, err := conn.Read(context.Background())
			if err != nil {
				return
			}
			var req struct {
				ID string `json:"id"`
			}
			json.Unmarshal(data, &req)
			go func() {
				mu.Lock()
				defer mu.Unlock()
				conn.Write(context.Background(), websocket.MessageText, []byte(`{"_tag":"Exit","requestId":"`+req.ID+`","exit":{"_tag":"Success","value":"`+req.ID+`"}}`))
			}()
		}
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return Client{BaseURL: s.URL, Token: "good", HTTP: s.Client()}
}

func TestCallsShareOneSocket(t *testing.T) {
	var tickets, conns atomic.Int32
	c := echoServer(t, &tickets, &conns)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out string
			if err := c.call(context.Background(), "x", nil, &out); err != nil || out == "" {
				t.Errorf("call: %q %v", out, err)
			}
		}()
	}
	wg.Wait()
	if tickets.Load() != 1 || conns.Load() != 1 {
		t.Fatalf("tickets=%d conns=%d", tickets.Load(), conns.Load())
	}
}

func TestDeadSocketIsReplaced(t *testing.T) {
	var tickets, conns atomic.Int32
	c := echoServer(t, &tickets, &conns)
	var out string
	if err := c.call(context.Background(), "x", nil, &out); err != nil {
		t.Fatal(err)
	}
	s, _ := c.session(context.Background())
	s.conn.CloseNow()
	if err := c.call(context.Background(), "x", nil, &out); err != nil {
		t.Fatalf("after close: %v", err)
	}
	if conns.Load() != 2 {
		t.Fatalf("conns=%d", conns.Load())
	}
}
