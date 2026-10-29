package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/migueel15/dory/internal/service"
)

const maxBody = 1 << 20

// mountResource registers list/create/get/put/archive routes for one resource.
// rec exposes the embedded Record so ids can be checked and timestamps ignored.
func mountResource[T any](mux *http.ServeMux, path string, res service.Resource[T], rec func(*T) *service.Record) {
	base := "/api/" + path

	mux.HandleFunc("GET "+base, func(w http.ResponseWriter, r *http.Request) {
		f, err := parseFilter(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
			return
		}
		items, err := res.List(r.Context(), f)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, items)
	})

	mux.HandleFunc("POST "+base, func(w http.ResponseWriter, r *http.Request) {
		put(w, r, res, rec, "")
	})

	mux.HandleFunc("GET "+base+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, err := res.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	})

	mux.HandleFunc("PUT "+base+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		put(w, r, res, rec, r.PathValue("id"))
	})

	mux.HandleFunc("DELETE "+base+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := res.Archive(r.Context(), actor(r), r.PathValue("id")); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// put handles POST (pathID empty, body id optional) and PUT (path id wins).
func put[T any](w http.ResponseWriter, r *http.Request, res service.Resource[T], rec func(*T) *service.Record, pathID string) {
	var v T
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_argument", "invalid JSON body: "+err.Error())
		return
	}
	record := rec(&v)
	if pathID != "" {
		if record.ID != "" && record.ID != pathID {
			writeError(w, http.StatusBadRequest, "invalid_argument", "body id does not match path id")
			return
		}
		record.ID = pathID
	}
	// Server-owned fields are never taken from the client.
	*record = service.Record{ID: record.ID}
	out, err := res.Put(r.Context(), actor(r), v)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func parseFilter(r *http.Request) (service.Filter, error) {
	q := r.URL.Query()
	var f service.Filter
	for _, p := range []struct {
		name string
		dst  *int32
	}{{"limit", &f.Limit}, {"offset", &f.Offset}} {
		if s := q.Get(p.name); s != "" {
			n, err := strconv.ParseInt(s, 10, 32)
			if err != nil || n < 0 || (p.name == "limit" && n < 1) {
				return f, &paramError{p.name + " is out of range"}
			}
			*p.dst = int32(n)
		}
	}
	if s := q.Get("include_archived"); s != "" {
		b, err := strconv.ParseBool(s)
		if err != nil {
			return f, &paramError{"include_archived must be a boolean"}
		}
		f.IncludeArchived = b
	}
	if s := q.Get("filter"); s != "" {
		if err := json.Unmarshal([]byte(s), &f.Fields); err != nil {
			return f, &paramError{"filter must be a JSON object"}
		}
	}
	return f, nil
}

type paramError struct{ msg string }

func (e *paramError) Error() string { return e.msg }
