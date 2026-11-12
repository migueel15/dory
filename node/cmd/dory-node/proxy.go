package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/node/localstore"
	"github.com/migueel15/dory/node/peer"
)

const t3Prefix = "/api/t3/"

// t3Owner returns the device that owns the T3 for this node: "" when this
// device has its own T3 instance (or none is known), else the owner_device of
// the replicated t3_instances (single owner in the MVP).
func t3Owner(ctx context.Context, store *localstore.Store, self string) (string, error) {
	ins, err := store.T3Instances().List(ctx, service.Filter{Limit: 100})
	if err != nil {
		return "", err
	}
	other := ""
	for _, in := range ins {
		owner, err := store.OwnerDevice(ctx, "t3instance", in.ID)
		if err != nil {
			continue
		}
		if owner == self {
			return "", nil
		}
		if other == "" {
			other = owner
		}
	}
	return other, nil
}

// t3Forward sends /api/t3/* to the owner node over mTLS when this node has no
// T3 of its own. Everything else goes to next.
func (n *Node) t3Forward(next http.Handler) http.Handler {
	self := n.Identity.DeviceID()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, t3Prefix) {
			next.ServeHTTP(w, r)
			return
		}
		owner, err := t3Owner(r.Context(), n.Store, self)
		if err != nil || owner == "" {
			next.ServeHTTP(w, r)
			return
		}
		name := owner
		if rec, ok, err := n.Store.AddressOf(r.Context(), owner); err == nil && ok && rec.Name != "" {
			name = rec.Name
		}
		if n.Presence.Status(owner).State != peer.Connected {
			peer.WriteProxyError(w, http.StatusServiceUnavailable, "owner_disconnected", fmt.Sprintf("%s desconectado", name))
			return
		}
		if n.conn == nil {
			next.ServeHTTP(w, r)
			return
		}
		cl, _, err := n.conn.Dial(r.Context(), owner)
		if err != nil {
			peer.WriteProxyError(w, http.StatusServiceUnavailable, "owner_disconnected", fmt.Sprintf("%s desconectado", name))
			return
		}
		cl.Proxy().ServeHTTP(w, r)
	})
}

// ownerProxy serves calls forwarded by members: it runs them on the local
// handler with the local session. The peer link (members only) authenticates
// the caller, so browser credentials and CSRF headers are replaced.
func ownerProxy(local func() http.Handler, session string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, t3Prefix) {
			http.NotFound(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		for _, h := range []string{"Authorization", "Cookie", "Origin", "Referer", "Sec-Fetch-Site"} {
			r2.Header.Del(h)
		}
		r2.AddCookie(&http.Cookie{Name: sessionCookie, Value: session})
		r2.RequestURI = r2.URL.RequestURI()
		local().ServeHTTP(w, r2)
	})
}
