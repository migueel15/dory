package api

import (
	"encoding/json"
	"github.com/migueel15/dory/internal/apperr"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

// The icon of a node is one id of a fixed set the interface knows. Each node
// keeps its own in its data folder and publishes it with its description, so
// every node of the mesh shows the same icon.
var iconID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

var iconMu sync.Mutex

func (s *Server) iconFile() string { return filepath.Join(s.Node.DataDir, "node.json") }

// Info describes this node with its current icon.
func (s *Server) Info() Node {
	iconMu.Lock()
	defer iconMu.Unlock()
	n := s.Node
	var saved struct {
		Icon string `json:"icon"`
	}
	if raw, err := os.ReadFile(s.iconFile()); err == nil && json.Unmarshal(raw, &saved) == nil {
		n.Icon = saved.Icon
	}
	return n
}

func (s *Server) setIcon(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Icon string `json:"icon"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Icon != "" && !iconID.MatchString(body.Icon) {
		writeErr(w, r, apperr.New(apperr.InvalidIcon, "invalid icon"))
		return
	}
	raw, _ := json.Marshal(map[string]string{"icon": body.Icon})
	iconMu.Lock()
	err := os.WriteFile(s.iconFile(), raw, 0o600)
	iconMu.Unlock()
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if s.InfoChanged != nil {
		s.InfoChanged()
	}
	w.WriteHeader(http.StatusNoContent)
}
