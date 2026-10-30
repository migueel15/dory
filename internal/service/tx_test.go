package service

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/migueel15/dory/migrations"
	"github.com/pressly/goose/v3"
)

// TestInTxRollsBack needs DORY_TEST_DATABASE_URL pointing at an empty database.
func TestInTxRollsBack(t *testing.T) {
	url := os.Getenv("DORY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("DORY_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	goose.SetBaseFS(migrations.FS)
	if err = goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpContext(ctx, stdlib.OpenDBFromPool(pool), "."); err != nil {
		t.Fatal(err)
	}
	svc := New(pool)
	boom := errors.New("boom")
	err = svc.InTx(ctx, func(tx Service) error {
		if _, err := tx.Projects().Put(ctx, "test", Project{Slug: "rollback", Name: "R"}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("InTx error = %v", err)
	}
	if rows, _ := svc.Projects().List(ctx, Filter{Fields: map[string]any{"slug": "rollback"}}); len(rows) != 0 {
		t.Fatal("failed batch left a project behind")
	}
	if err = svc.InTx(ctx, func(tx Service) error {
		_, err := tx.Projects().Put(ctx, "test", Project{Slug: "commit", Name: "C"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if rows, _ := svc.Projects().List(ctx, Filter{Fields: map[string]any{"slug": "commit"}}); len(rows) != 1 {
		t.Fatal("committed batch is missing")
	}
}
