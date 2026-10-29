// Package web owns embedded SPA assets. Replace Handler in the web slice.
package web

import "net/http"

func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "web implementation pending", http.StatusNotImplemented)
	})
}
