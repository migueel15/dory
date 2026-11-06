package t3

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// streamServer answers Ping and Chunk frames and records what the client
// sends back: Pong after Ping, Ack after each Chunk.
func streamServer(t *testing.T, chunks int, replies *[]string) Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/websocket-ticket/rpc", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ticket":"tk"}`))
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		_, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		var req struct{ ID, Tag string }
		json.Unmarshal(data, &req)
		*replies = append(*replies, req.Tag)
		conn.Write(r.Context(), websocket.MessageText, []byte(`{"_tag":"Ping"}`))
		_, data, _ = conn.Read(r.Context())
		*replies = append(*replies, string(data))
		for range chunks {
			conn.Write(r.Context(), websocket.MessageText, []byte(`{"_tag":"Chunk","requestId":"`+req.ID+`","values":[{"kind":"thread.updated"}]}`))
			_, data, _ = conn.Read(r.Context())
			*replies = append(*replies, strings.Replace(string(data), req.ID, "ID", 1))
		}
		conn.Write(r.Context(), websocket.MessageText, []byte(`{"_tag":"Exit","requestId":"`+req.ID+`","exit":{"_tag":"Success"}}`))
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return Client{BaseURL: s.URL, Token: "good", HTTP: s.Client()}
}

func TestStreamAcksEachChunkAndAnswersPing(t *testing.T) {
	var replies []string
	c := streamServer(t, 2, &replies)
	got := 0
	err := c.Stream(context.Background(), methodSubscribeShell, shellPayload, func() { got++ })
	if !errors.Is(err, errStreamEnded) {
		t.Fatalf("err = %v, want stream ended", err)
	}
	want := []string{methodSubscribeShell, `{"_tag":"Pong"}`, `{"_tag":"Ack","requestId":"ID"}`, `{"_tag":"Ack","requestId":"ID"}`}
	if got != 2 || strings.Join(replies, "|") != strings.Join(want, "|") {
		t.Fatalf("chunks = %d, replies = %v", got, replies)
	}
}

func TestFollowThreadSkipsSnapshot(t *testing.T) {
	var replies []string
	c := streamServer(t, 3, &replies)
	signal := make(chan struct{}, 1)
	FollowThread(context.Background(), c, "t1", signal)
	if replies[0] != methodSubscribeThread || len(signal) != 1 {
		t.Fatalf("replies = %v, signals = %d", replies, len(signal))
	}
}

func TestStreamStopsWithContext(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/websocket-ticket/rpc", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"ticket":"tk"}`)) })
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		conn.Read(r.Context())
		<-r.Context().Done()
	})
	s := httptest.NewServer(mux)
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := (Client{BaseURL: s.URL, Token: "x", HTTP: s.Client()}).Stream(ctx, methodSubscribeShell, nil, func() {}); err == nil {
		t.Fatal("stream did not stop with its context")
	}
}
