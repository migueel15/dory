package mesh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"strconv"
	"time"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/oplog"
)

func (m *Mesh) mountReplication(mux *http.ServeMux) {
	mux.HandleFunc("GET "+peerPrefix+"/vector", m.members(func(w http.ResponseWriter, r *http.Request, _ string) {
		v, err := m.log.Vector(r.Context())
		if err != nil {
			apperr.WriteHTTP(w, r, err)
			return
		}
		reply(w, v)
	}))
	mux.HandleFunc("POST "+peerPrefix+"/ops", m.members(func(w http.ResponseWriter, r *http.Request, _ string) {
		var since oplog.Vector
		if json.NewDecoder(r.Body).Decode(&since) != nil {
			apperr.WriteHTTP(w, r, apperr.New(apperr.ForStatus(400), "bad vector"))
			return
		}
		ops, err := m.log.Since(r.Context(), since, pageSize)
		if errors.Is(err, oplog.ErrSnapshotRequired) {
			apperr.WriteHTTP(w, r, apperr.New(apperr.ForStatus(http.StatusConflict), err.Error()))
			return
		}
		if err != nil {
			apperr.WriteHTTP(w, r, err)
			return
		}
		reply(w, ops)
	}))
	mux.HandleFunc("POST "+peerPrefix+"/apply", m.members(func(w http.ResponseWriter, r *http.Request, _ string) {
		var ops []oplog.Op
		if json.NewDecoder(io.LimitReader(r.Body, 64<<20)).Decode(&ops) != nil {
			apperr.WriteHTTP(w, r, apperr.New(apperr.ForStatus(400), "bad ops"))
			return
		}
		if err := m.log.Apply(r.Context(), ops); err != nil {
			apperr.WriteHTTP(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("GET "+peerPrefix+"/snapshot", m.members(func(w http.ResponseWriter, r *http.Request, _ string) {
		page := -1
		if raw := r.URL.Query().Get("page"); raw != "" {
			n, err := strconv.ParseInt(raw, 10, 32)
			if err != nil || n < 0 {
				apperr.WriteHTTP(w, r, apperr.New(apperr.Invalid, "bad snapshot page"))
				return
			}
			page = int(n)
		}
		snapshot, err := m.log.SnapshotPage(r.Context(), page)
		if err != nil {
			apperr.WriteHTTP(w, r, err)
			return
		}
		reply(w, snapshot)
	}))
	mux.HandleFunc("POST "+peerPrefix+"/snapshot", m.members(func(w http.ResponseWriter, r *http.Request, _ string) {
		var batch oplog.SnapshotBatch
		if json.NewDecoder(r.Body).Decode(&batch) != nil || batch.Horizon == nil {
			apperr.WriteHTTP(w, r, apperr.New(apperr.Invalid, "bad snapshot batch"))
			return
		}
		if err := m.log.ApplySnapshot(r.Context(), batch); err != nil {
			apperr.WriteHTTP(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("POST "+peerPrefix+"/ack", m.members(func(w http.ResponseWriter, r *http.Request, node string) {
		var vector oplog.Vector
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&vector) != nil || vector == nil {
			apperr.WriteHTTP(w, r, apperr.New(apperr.Invalid, "bad vector"))
			return
		}
		if err := m.log.RecordPeerVector(r.Context(), node, vector); err != nil {
			apperr.WriteHTTP(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

type replicationSource struct {
	since    func(context.Context, oplog.Vector, int) ([]oplog.Op, error)
	snapshot func(context.Context, int) (oplog.SnapshotPage, error)
}

type replicationTarget struct {
	vector        func(context.Context) (oplog.Vector, error)
	apply         func(context.Context, []oplog.Op) error
	applySnapshot func(context.Context, oplog.SnapshotBatch) error
}

// replicate transfers one direction. Snapshots and the tail needed to
// materialize them are committed together on the target.
func replicate(ctx context.Context, from replicationSource, to replicationTarget) error {
	vector, err := to.vector(ctx)
	if err != nil {
		return err
	}
	cursor := make(oplog.Vector, len(vector))
	cursor.Include(vector)
	var batch oplog.SnapshotBatch
	if from.snapshot != nil {
		meta, err := from.snapshot(ctx, -1)
		if err != nil {
			return err
		}
		if !cursor.Covers(meta.Horizon) {
			if to.applySnapshot == nil {
				return oplog.ErrSnapshotRequired
			}
			batch.Horizon = meta.Horizon
			cursor.Include(meta.Horizon)
			for page := 0; ; page++ {
				part, err := from.snapshot(ctx, page)
				if err != nil {
					return err
				}
				if !maps.Equal(part.Horizon, batch.Horizon) {
					return oplog.ErrSnapshotRequired
				}
				batch.Rows = append(batch.Rows, part.Rows...)
				if len(part.Rows) < pageSize {
					break
				}
			}
		}
	}
	for page := 0; ; page++ {
		if page == 1000 && batch.Horizon == nil {
			return fmt.Errorf("mesh: replication did not reach the end of the operation log")
		}
		ops, err := from.since(ctx, cursor, pageSize)
		if err != nil {
			return err
		}
		if batch.Horizon != nil {
			batch.Ops = append(batch.Ops, ops...)
		} else if len(ops) != 0 {
			if err := to.apply(ctx, ops); err != nil {
				return err
			}
		}
		for _, op := range ops {
			cursor[op.Node] = max(cursor[op.Node], op.Seq)
		}
		if len(ops) < pageSize {
			if batch.Horizon != nil {
				return to.applySnapshot(ctx, batch)
			}
			return nil
		}
	}
}

func (m *Mesh) exchange(ctx context.Context, c *http.Client, base, node string) error {
	var supportsSnapshots bool
	if err := m.db.QueryRowContext(ctx, `SELECT COALESCE(json_extract(info, '$.snapshots'), 0) FROM peers WHERE node_id = ?`, node).Scan(&supportsSnapshots); err != nil {
		return err
	}
	remote := replicationSource{
		since: func(ctx context.Context, v oplog.Vector, _ int) ([]oplog.Op, error) {
			var ops []oplog.Op
			err := postJSON(ctx, c, base+"/ops", v, &ops)
			return ops, err
		},
	}
	target := replicationTarget{
		vector: func(ctx context.Context) (oplog.Vector, error) {
			var v oplog.Vector
			err := getJSON(ctx, c, base+"/vector", &v)
			return v, err
		},
		apply: func(ctx context.Context, ops []oplog.Op) error {
			return postJSON(ctx, c, base+"/apply", ops, nil)
		},
	}
	if supportsSnapshots {
		remote.snapshot = func(ctx context.Context, page int) (oplog.SnapshotPage, error) {
			var out oplog.SnapshotPage
			url := base + "/snapshot"
			if page >= 0 {
				url += "?page=" + strconv.Itoa(page)
			}
			err := getJSON(ctx, c, url, &out)
			return out, err
		}
		target.applySnapshot = func(ctx context.Context, batch oplog.SnapshotBatch) error {
			return postJSON(ctx, c, base+"/snapshot", batch, nil)
		}
	}
	local := replicationTarget{m.log.Vector, m.log.Apply, m.log.ApplySnapshot}
	if err := replicate(ctx, remote, local); err != nil {
		return err
	}
	if err := replicate(ctx, replicationSource{m.log.Since, m.log.SnapshotPage}, target); err != nil {
		return err
	}
	theirs, err := target.vector(ctx)
	if err != nil {
		return err
	}
	if supportsSnapshots {
		mine, err := local.vector(ctx)
		if err != nil {
			return err
		}
		if err := postJSON(ctx, c, base+"/ack", mine, nil); err != nil {
			return err
		}
	}
	return m.log.RecordPeerVector(ctx, node, theirs)
}

func (m *Mesh) compact(ctx context.Context) {
	if err := m.log.Compact(ctx); err != nil && ctx.Err() == nil {
		log.Printf("mesh: compact oplog: %v", err)
	}
}

func (m *Mesh) compactHourly(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.compact(ctx)
		}
	}
}
