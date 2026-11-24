package mesh

import (
	"database/sql"
	"fmt"
	"maps"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/oplog"
	"github.com/migueel15/dory/internal/schema"
)

type replica struct {
	db  *sql.DB
	log *oplog.Log
}

func newReplica(t *testing.T) replica {
	t.Helper()
	db, err := localdb.Open(t.Context(), ":memory:")
	checkReplication(t, err)
	t.Cleanup(func() { db.Close() })
	log, err := oplog.Open(t.Context(), db)
	checkReplication(t, err)
	return replica{db, log}
}

func checkReplication(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func pairReplicas(t *testing.T, a, b replica) {
	t.Helper()
	for _, pair := range [][2]replica{{a, b}, {b, a}} {
		peer := pair[1].log.Node()
		_, err := pair[0].db.Exec(`INSERT INTO peers(node_id, public_key, paired_at) VALUES (?, ?, '')`, peer, peer)
		checkReplication(t, err)
	}
}

func transfer(t *testing.T, a, b replica) {
	t.Helper()
	checkReplication(t, replicate(t.Context(), replicationSource{a.log.Since, a.log.SnapshotPage},
		replicationTarget{b.log.Vector, b.log.Apply, b.log.ApplySnapshot}))
}

func exchangeReplicas(t *testing.T, a, b replica) {
	t.Helper()
	transfer(t, a, b)
	transfer(t, b, a)
	for _, pair := range [][2]replica{{a, b}, {b, a}} {
		known, err := pair[1].log.Vector(t.Context())
		checkReplication(t, err)
		checkReplication(t, pair[0].log.RecordPeerVector(t.Context(), pair[1].log.Node(), known))
	}
}

func writeReplica(t *testing.T, r replica, entity, id string, fields map[string]any) {
	t.Helper()
	_, err := r.log.Write(t.Context(), entity, id, fields)
	checkReplication(t, err)
}

func materialized(t *testing.T, r replica) map[string][]string {
	t.Helper()
	result := map[string][]string{}
	for _, entity := range schema.All() {
		cols := append(append([]string{}, entity.Key...), entity.ColumnNames()...)
		rows, err := r.db.Query(fmt.Sprintf("SELECT json_array(%s) FROM %s ORDER BY %s",
			strings.Join(cols, ","), entity.Table, strings.Join(entity.Key, ",")))
		checkReplication(t, err)
		for rows.Next() {
			var raw string
			checkReplication(t, rows.Scan(&raw))
			result[entity.Name] = append(result[entity.Name], raw)
		}
		checkReplication(t, rows.Err())
		checkReplication(t, rows.Close())
	}
	return result
}

func TestCompactionConverges(t *testing.T) {
	for _, seed := range []int64{1, 7, 19, 42, 83, 235} {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			nodes := []replica{newReplica(t), newReplica(t), newReplica(t)}
			for i := range nodes {
				for j := range i {
					pairReplicas(t, nodes[i], nodes[j])
				}
				id := fmt.Sprintf("%08d-project", i)
				writeReplica(t, nodes[i], "project", id, map[string]any{"slug": "shared", "name": id, "created_at": "", "updated_at": ""})
				writeReplica(t, nodes[i], "task", id, map[string]any{"project_id": id, "title": id, "created_at": "", "updated_at": ""})
			}
			writeReplica(t, nodes[0], "memory", "m", map[string]any{"scope": "global", "content": "base", "created_at": "", "updated_at": ""})
			exchangeReplicas(t, nodes[0], nodes[1])
			exchangeReplicas(t, nodes[0], nodes[2])
			for i, r := range nodes {
				writeReplica(t, r, "memory", "m", map[string]any{"content": fmt.Sprintf("concurrente %d", i)})
			}
			for step := range 120 {
				i := rng.Intn(len(nodes))
				j := (i + 1 + rng.Intn(len(nodes)-1)) % len(nodes)
				id := fmt.Sprintf("%08d-project", rng.Intn(len(nodes)))
				value := fmt.Sprintf("edición %d", step)
				switch rng.Intn(6) {
				case 0:
					writeReplica(t, nodes[i], "memory", "m", map[string]any{"content": value})
				case 1:
					var present bool
					checkReplication(t, nodes[i].db.QueryRow("SELECT EXISTS(SELECT 1 FROM tasks WHERE id = ?)", id).Scan(&present))
					if present {
						writeReplica(t, nodes[i], "task", id, map[string]any{"title": value})
					}
				case 2:
					var present bool
					checkReplication(t, nodes[i].db.QueryRow("SELECT EXISTS(SELECT 1 FROM projects WHERE id = ?)", id).Scan(&present))
					if present {
						writeReplica(t, nodes[i], "project", id, map[string]any{"slug": fmt.Sprintf("slug-%d", rng.Intn(3)), "name": value})
					}
				case 3, 4:
					exchangeReplicas(t, nodes[i], nodes[j])
				case 5:
					before := materialized(t, nodes[i])
					checkReplication(t, nodes[i].log.Compact(t.Context()))
					if after := materialized(t, nodes[i]); !reflect.DeepEqual(before, after) {
						t.Fatalf("compaction at step %d changed rows: %v -> %v", step, before, after)
					}
				}
			}
			exchangeReplicas(t, nodes[0], nodes[1])
			exchangeReplicas(t, nodes[0], nodes[2])
			exchangeReplicas(t, nodes[0], nodes[1])
			checkReplication(t, nodes[0].log.Compact(t.Context()))
			var count int
			checkReplication(t, nodes[0].db.QueryRow("SELECT COUNT(*) FROM ops").Scan(&count))
			if count != 0 {
				t.Fatalf("fully acknowledged oplog contains %d operations", count)
			}
			newNode := newReplica(t)
			pairReplicas(t, nodes[0], newNode)
			exchangeReplicas(t, nodes[0], newNode)
			want := materialized(t, nodes[0])
			for i, r := range append(nodes[1:], newNode) {
				if got := materialized(t, r); !reflect.DeepEqual(got, want) {
					t.Fatalf("replica %d diverged:\n%v\nwant:\n%v", i+1, got, want)
				}
			}
		})
	}
}

func TestStalePeerBootstrapsAcrossSnapshotPagesAndTail(t *testing.T) {
	ctx := t.Context()
	source, offline, parent := newReplica(t), newReplica(t), newReplica(t)
	pairReplicas(t, source, offline)
	pairReplicas(t, source, parent)
	exchangeReplicas(t, source, offline)
	// The returning peer has a local edit the compacting node never saw.
	writeReplica(t, offline, "memory", "offline", map[string]any{"scope": "global", "content": "sin conexión", "created_at": "", "updated_at": ""})
	project := "parent"
	writeReplica(t, parent, "project", project, map[string]any{"slug": "parent", "name": "Parent", "created_at": "", "updated_at": ""})
	transfer(t, parent, source)
	checkReplication(t, source.log.Tx(ctx, func(tx *oplog.Tx) error {
		for i := 0; i <= pageSize; i++ {
			_, err := tx.Write("task", fmt.Sprintf("%04d", i), map[string]any{"project_id": project, "title": "hija", "created_at": "", "updated_at": ""})
			if err != nil {
				return err
			}
		}
		return nil
	}))
	_, err := source.db.Exec("UPDATE peer_vectors SET synced_at = ? WHERE node_id = ?",
		time.Now().Add(-31*24*time.Hour).UTC().Format(time.RFC3339), offline.log.Node())
	checkReplication(t, err)
	// Only the children can be compacted. Their parent must arrive in the tail.
	checkReplication(t, source.log.RecordPeerVector(ctx, parent.log.Node(), oplog.Vector{source.log.Node(): pageSize + 1}))
	checkReplication(t, source.log.Compact(ctx))
	page, err := source.log.SnapshotPage(ctx, 1)
	checkReplication(t, err)
	if !maps.Equal(page.Horizon, oplog.Vector{source.log.Node(): pageSize + 1}) || len(page.Rows) != 1 {
		t.Fatalf("second page = %+v", page)
	}
	exchangeReplicas(t, source, offline)
	if got, want := materialized(t, offline), materialized(t, source); !reflect.DeepEqual(got, want) {
		t.Fatal("returning peer did not converge")
	}
}
