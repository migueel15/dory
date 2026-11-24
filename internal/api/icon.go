package api

import (
	"context"
	"log"
	"net/http"
	"regexp"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/nodestate"
)

// The icon of a node is one id of a fixed set the interface knows. Each node
// keeps its own in SQLite and publishes it with its description, so
// every node of the mesh shows the same icon.
var iconID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

type nodeIcon struct {
	Icon string `json:"icon"`
}

// Info describes this node with its current icon.
func (s *Server) Info() Node {
	n := s.Node
	if s.NodeState == nil {
		return n
	}
	var saved nodeIcon
	if found, err := s.NodeState.Get(context.Background(), nodestate.KeyNodeIcon, &saved); err != nil {
		log.Printf("api: load node icon: %v", err)
	} else if found {
		n.Icon = saved.Icon
	}
	return n
}

func (s *Server) setIcon(w http.ResponseWriter, r *http.Request) {
	var body nodeIcon
	if !decode(w, r, &body) {
		return
	}
	if body.Icon != "" && !iconID.MatchString(body.Icon) {
		writeErr(w, r, apperr.New(apperr.InvalidIcon, "invalid icon"))
		return
	}
	if s.NodeState == nil {
		writeErr(w, r, apperr.New(apperr.Unavailable, "node state is not available"))
		return
	}
	if err := s.NodeState.Put(r.Context(), nodestate.KeyNodeIcon, body); err != nil {
		writeErr(w, r, err)
		return
	}
	if s.InfoChanged != nil {
		s.InfoChanged()
	}
	w.WriteHeader(http.StatusNoContent)
}
