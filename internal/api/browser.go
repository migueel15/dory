package api

import (
	"context"
	"github.com/migueel15/dory/internal/apperr"
	"net"
	"net/http"
	"strconv"

	"github.com/coder/websocket"
	"github.com/migueel15/dory/internal/data"
)

// mountBrowser serves the browser of each thread on the node that runs it:
// its state, the channel of the side panel and the tunnel that lets the
// desktop app load the loopback ports of a remote node.
func (s *Server) mountBrowser(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/threads/{id}/browser", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		if s.Browser == nil {
			writeErr(w, r, apperr.New(apperr.Unavailable, "this node has no browser").With("reason", "no_browser"))
			return
		}
		st, err := s.Browser.State(r.Context(), t.ID)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	}))
	mux.HandleFunc("POST /api/threads/{id}/browser/tabs", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		var body struct {
			URL string `json:"url"`
		}
		if !decode(w, r, &body) {
			return
		}
		if s.Browser == nil {
			writeErr(w, r, apperr.New(apperr.Unavailable, "this node has no browser").With("reason", "no_browser"))
			return
		}
		tab, err := s.Browser.Add(r.Context(), t.ID, body.URL)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, tab)
	}))
	mux.HandleFunc("GET /api/threads/{id}/browser/ws", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		if s.Browser == nil {
			writeErr(w, r, apperr.New(apperr.Unavailable, "this node has no browser").With("reason", "no_browser"))
			return
		}
		// The auth middleware already refuses cross-origin sessions; behind
		// the mesh proxy the Host is the peer's.
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer ws.CloseNow()
		s.Browser.Serve(r.Context(), ws, t.ID, r.URL.Query().Get("desktop") == "1")
	}))
	mux.HandleFunc("GET /api/threads/{id}/browser/tunnel", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		port, err := strconv.Atoi(r.URL.Query().Get("port"))
		if err != nil || port < 1 || port > 65535 {
			writeErr(w, r, apperr.New(apperr.Invalid, "port is out of range"))
			return
		}
		if s.Settings.Get().BrowserTunnelDisabled {
			writeErr(w, r, apperr.New(apperr.Forbidden, "the browser tunnel is turned off on this node").With("reason", "tunnel_off"))
			return
		}
		if s.ownPort(port) {
			writeErr(w, r, apperr.New(apperr.Forbidden, "the tunnel does not reach Dory's ports").With("reason", "tunnel_dory_port"))
			return
		}
		target, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			writeErr(w, r, apperr.Wrap(apperr.Unreachable, err, "the port is not reachable on this node"))
			return
		}
		defer target.Close()
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		conn := websocket.NetConn(ctx, ws, websocket.MessageBinary)
		defer conn.Close()
		pipe(conn, target)
	}))
}

// pipe copies both ways until one side closes.
func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) {
		buf := make([]byte, 32<<10)
		for {
			n, err := src.Read(buf)
			if n > 0 {
				if _, werr := dst.Write(buf[:n]); werr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		done <- struct{}{}
	}
	go cp(a, b)
	go cp(b, a)
	<-done
}

// ownPort tells whether port is one Dory listens on.
func (s *Server) ownPort(port int) bool {
	if _, p, err := net.SplitHostPort(s.Node.Listen); err == nil && p == strconv.Itoa(port) {
		return true
	}
	return s.MeshPort > 0 && port == s.MeshPort
}
