package api

import (
	"github.com/migueel15/dory/internal/apperr"
	"net/http"
	"os"
	"time"

	"github.com/migueel15/dory/internal/runtime"
)

// usageRanges maps the range names of the usage page to their length.
var usageRanges = map[string]time.Duration{"24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour, "90d": 90 * 24 * time.Hour}

// mountUsageHistory serves the token usage and API-equivalent cost read from
// the harness logs of a node.
func (s *Server) mountUsageHistory(mux *http.ServeMux) {
	home, _ := os.UserHomeDir()
	serve := func(w http.ResponseWriter, r *http.Request, breakdown bool) {
		name := r.URL.Query().Get("range")
		if name == "" {
			name = "30d"
		}
		span, ok := usageRanges[name]
		if !ok {
			writeErr(w, r, apperr.New(apperr.InvalidRange, "range must be 24h, 7d, 30d or 90d"))
			return
		}
		loc := time.Local
		if tz := r.URL.Query().Get("tz"); tz != "" {
			if l, err := time.LoadLocation(tz); err == nil {
				loc = l
			}
		}
		now := time.Now().In(loc)
		from := now.Add(-span)
		if name != "24h" {
			// Whole days, today included.
			from = time.Date(now.Year(), now.Month(), now.Day()-int(span/(24*time.Hour))+1, 0, 0, 0, 0, loc)
		}
		if thread := r.PathValue("thread"); thread != "" {
			if s.Runtime == nil {
				writeJSON(w, http.StatusOK, []runtime.UsageTurn{})
				return
			}
			turns, err := s.Runtime.ThreadUsageTurns(r.Context(), thread, from)
			if err != nil {
				writeErr(w, r, apperr.New(apperr.Internal, "could not read the thread usage").With("reason", "usage_turns"))
				return
			}
			writeJSON(w, http.StatusOK, turns)
			return
		}
		if breakdown {
			if s.Runtime == nil {
				writeJSON(w, http.StatusOK, []runtime.UsageBreakdownRow{})
				return
			}
			rows, err := s.Runtime.UsageBreakdown(r.Context(), from)
			if err != nil {
				writeErr(w, r, apperr.New(apperr.Internal, "could not read the usage breakdown").With("reason", "usage_breakdown"))
				return
			}
			writeJSON(w, http.StatusOK, rows)
			return
		}
		writeJSON(w, http.StatusOK, runtime.UsageHistory(s.Runtime.Paths(home), from, name == "24h", loc))
	}
	mux.HandleFunc("GET /api/nodes/{node}/usage/history", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		serve(w, r, false)
	}))
	mux.HandleFunc("GET /api/nodes/{node}/usage/breakdown", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		serve(w, r, true)
	}))
	mux.HandleFunc("GET /api/nodes/{node}/usage/threads/{thread}/turns", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		serve(w, r, true)
	}))
}
