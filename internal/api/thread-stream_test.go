package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/runtime"
)

// readEvents collects the SSE events of a response until n have arrived.
func readEvents(t *testing.T, resp *http.Response, n int) []string {
	t.Helper()
	var out, cur []string
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if len(cur) > 0 {
				out = append(out, strings.Join(cur, "|"))
				cur = nil
			}
			if len(out) == n {
				return out
			}
			continue
		}
		if !strings.HasPrefix(line, ":") {
			cur = append(cur, line)
		}
	}
	t.Fatalf("stream ended with %v", out)
	return nil
}

func TestThreadStreamResumesAfterLastEventID(t *testing.T) {
	ctx := context.Background()
	s, _ := newFinishServer(t, Settings{})
	th, err := s.Store.Threads().Create(ctx, data.Thread{NodeID: s.Node.ID, Provider: "claude", Title: "x"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b", "c"} {
		s.Runtime.Save(ctx, runtime.Message{ID: id, ThreadID: th.ID, Role: "user", Text: id})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("id", th.ID)
		s.streamThread(w, r, th)
	}))
	defer srv.Close()
	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(rctx, "GET", srv.URL, nil)
	req.Header.Set("Last-Event-ID", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	// A live event follows the replay with the next number.
	go func() {
		time.Sleep(300 * time.Millisecond)
		s.Runtime.Save(ctx, runtime.Message{ID: "d", ThreadID: th.ID, Role: "user", Text: "d"})
	}()
	got := readEvents(t, resp, 3)
	if !strings.HasPrefix(got[0], "id: 2|") || !strings.HasPrefix(got[1], "id: 3|") {
		t.Fatalf("replay = %v", got)
	}
	if !strings.HasPrefix(got[2], "id: 4|") {
		t.Fatalf("live = %v", got)
	}
}
