package nodestate_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/nodestate"
)

func openStore(t *testing.T) (*nodestate.Store, *sql.DB) {
	t.Helper()
	db, err := localdb.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return nodestate.New(db), db
}

func TestConcurrentUpdate(t *testing.T) {
	ctx := context.Background()
	store, _ := openStore(t)
	var got int
	if found, err := store.Get(ctx, "counter", &got); found || err != nil {
		t.Fatalf("missing key: found=%v, err=%v", found, err)
	}
	if err := store.Put(ctx, "counter", 5); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			if err := store.Put(ctx, "latest", i); err != nil {
				t.Error(err)
			}
			var latest int
			if found, err := store.Get(ctx, "latest", &latest); !found || err != nil {
				t.Errorf("latest: found=%v, err=%v", found, err)
			}
			var count int
			if err := store.Update(ctx, "counter", &count, func() error {
				count++
				return nil
			}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()

	aborted := errors.New("abort")
	if err := store.Update(ctx, "counter", &got, func() error {
		got = -1
		return aborted
	}); !errors.Is(err, aborted) {
		t.Fatalf("update error = %v, want %v", err, aborted)
	}
	if found, err := store.Get(ctx, "counter", &got); !found || got != 25 || err != nil {
		t.Fatalf("counter = %d, found=%v, err=%v", got, found, err)
	}
}

func TestImportFile(t *testing.T) {
	ctx := context.Background()
	store, db := openStore(t)
	path := filepath.Join(t.TempDir(), "legacy.json")
	write := func(raw string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("{")
	if err := store.ImportFile(ctx, nodestate.KeyNodeIcon, path); err == nil {
		t.Fatal("imported invalid JSON")
	}
	const raw = " { \"icon\": \"moon\" }\n"
	write(raw)
	if err := store.ImportFile(ctx, nodestate.KeyNodeIcon, path); err != nil {
		t.Fatal(err)
	}
	archived, err := os.ReadFile(path + ".migrated")
	if err != nil || string(archived) != raw {
		t.Fatalf("archive = %q, err=%v", archived, err)
	}
	write("{")
	if err := store.ImportFile(ctx, nodestate.KeyNodeIcon, path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := db.QueryRowContext(ctx, "SELECT value FROM node_kv WHERE key = ?", nodestate.KeyNodeIcon).Scan(&got); err != nil || got != raw {
		t.Fatalf("value = %q, err=%v", got, err)
	}
}
