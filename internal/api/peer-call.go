package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
)

// ErrUnreachable is returned when the mesh cannot reach a node.
var ErrUnreachable = errors.New("the environment is not reachable")

// PeerCall sends a JSON request to the API of another node through the mesh
// and decodes its JSON answer into out, when out is not nil.
func (s *Server) PeerCall(ctx context.Context, node, method, path string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req := httptest.NewRequestWithContext(ctx, method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	if s.Remote == nil || !s.Remote.Forward(rec, req, node) {
		return ErrUnreachable
	}
	if rec.Code >= 300 {
		return fmt.Errorf("the environment answered %d: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if out != nil && rec.Body.Len() > 0 {
		return json.Unmarshal(rec.Body.Bytes(), out)
	}
	return nil
}

// SendFromPeer delivers a message an agent wrote to a thread of another node.
func (s *Server) SendFromPeer(ctx context.Context, node, thread, sender, text string) error {
	return s.PeerCall(ctx, node, http.MethodPost, "/api/threads/"+thread+"/messages", map[string]string{"text": text, "sender_thread_id": sender}, nil)
}
