package api

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/migueel15/dory/internal/apperr"
	"net/http"
	"net/http/httptest"

	"github.com/migueel15/dory/internal/runtime"
)

// ErrUnreachable is returned when the mesh cannot reach a node.
var ErrUnreachable = apperr.New(apperr.Unreachable, "the environment is not reachable")

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
		return remoteError(rec.Code, rec.Body.Bytes())
	}
	if out != nil && rec.Body.Len() > 0 {
		return json.Unmarshal(rec.Body.Bytes(), out)
	}
	return nil
}

// CompleteOnPeer hands the outcome of a subagent turn to its parent's node.
func (s *Server) CompleteOnPeer(ctx context.Context, node string, o runtime.Outcome) error {
	return s.PeerCall(ctx, node, http.MethodPost, "/api/threads/"+o.ParentThreadID+"/delegations/complete", o, nil)
}

// remoteError rebuilds the domain error another node answered with, so a
// failure keeps its code across the mesh.
func remoteError(status int, body []byte) error {
	var b apperr.Body
	if json.Unmarshal(body, &b) == nil && b.Error.Code != "" {
		return &apperr.Error{Code: b.Error.Code, Message: b.Error.Message, Params: b.Error.Params}
	}
	return apperr.Newf(apperr.Unreachable, "the environment answered %d", status)
}
