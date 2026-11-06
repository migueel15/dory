package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/migueel15/dory/internal/api"
	"github.com/migueel15/dory/internal/auth"
	"github.com/migueel15/dory/internal/config"
	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/mcp"
	"github.com/migueel15/dory/internal/notifications"
	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/internal/t3"
	"github.com/migueel15/dory/internal/titles"
	"github.com/migueel15/dory/migrations"
	"github.com/migueel15/dory/web"
	"github.com/pressly/goose/v3"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: dory serve | migrate [up|down|status] | user create | token create")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	switch os.Args[1] {
	case "migrate":
		action := "up"
		if len(os.Args) > 2 {
			action = os.Args[2]
		}
		if action != "up" && action != "down" && action != "status" {
			return fmt.Errorf("unknown migration command %q", action)
		}
		db := stdlib.OpenDBFromPool(pool)
		defer db.Close()
		goose.SetBaseFS(migrations.FS)
		if err = goose.SetDialect("postgres"); err != nil {
			return err
		}
		return goose.RunContext(ctx, action, db, ".")
	case "serve":
		if err = pool.Ping(ctx); err != nil {
			return err
		}
		svc := service.New(pool)
		mux := http.NewServeMux()
		monitor := t3.NewMonitor(svc)
		notices, err := notifications.New(ctx, pool)
		if err != nil {
			return err
		}
		monitor.Observe = notices.Observe
		notices.Mount(mux)
		go notices.Run(ctx)
		go monitor.Run(ctx)
		titleSettings := titles.Store{Pool: pool}
		titleSettings.Mount(mux)
		titleGen := titles.New(svc, monitor, titleSettings)
		titleGen.Recover(ctx)
		api.Register(mux, titleGen.Wrap(svc), monitor, github.New(svc, os.Getenv("DORY_GITHUB_TOKEN")))
		auth.Register(mux, svc, cfg)
		mux.Handle("/mcp", mcp.Handler(titleGen.Wrap(svc), monitor.Locate))
		mux.Handle("/", web.Handler())
		server := &http.Server{Addr: cfg.Listen, Handler: auth.Middleware(svc, cfg)(mux), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
		done := make(chan error, 1)
		go func() { done <- server.ListenAndServe() }()
		select {
		case err = <-done:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return server.Shutdown(shutdown)
		}
	default:
		handled, err := auth.Command(ctx, os.Args[1:], service.New(pool))
		if err != nil {
			return err
		}
		if handled {
			return nil
		}
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}
