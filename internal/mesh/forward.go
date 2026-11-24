package mesh

import (
	"bufio"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
)

const proxyPrefix = peerPrefix + "/proxy"

// Forward sends an API call to the node that owns it and copies the answer,
// streams included. False leaves the response available to the caller.
func (m *Mesh) Forward(w http.ResponseWriter, r *http.Request, node string) bool {
	peer, err := m.peer(r.Context(), node)
	if err != nil || peer.address == "" {
		return false
	}
	target := &url.URL{Scheme: "https", Host: peer.address}
	tracked := &forwardWriter{ResponseWriter: w}
	failed := false
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.URL.Path = proxyPrefix + r.URL.Path
			if r.URL.RawPath != "" {
				pr.Out.URL.RawPath = proxyPrefix + r.URL.RawPath
			}
			pr.Out.Header.Del("Cookie")
			pr.Out.Header.Del("Authorization")
		},
		Transport:     peer.transport,
		FlushInterval: -1,
		ErrorHandler: func(_ http.ResponseWriter, _ *http.Request, err error) {
			failed = true
			log.Printf("mesh: forward to %s: %v", node, err)
		},
	}
	proxy.ServeHTTP(tracked, r)
	return !failed || tracked.written
}

type forwardWriter struct {
	http.ResponseWriter
	written bool
}

func (w *forwardWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *forwardWriter) WriteHeader(code int) {
	w.written = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *forwardWriter) Write(p []byte) (int, error) {
	w.written = true
	return w.ResponseWriter.Write(p)
}

func (w *forwardWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err == nil {
		w.written = true
	}
	return conn, rw, err
}
