// Command api is the HTTP front end for the health-check engine.
//
//	go run ./cmd/api
//	curl -s localhost:8080/healthz
//	curl -s -X POST localhost:8080/runs -H 'X-Tenant-ID: acme' --data-binary @testdata/urls.txt
//	curl -s localhost:8080/runs/<id> -H 'X-Tenant-ID: acme'
//
// This file is the COMPOSITION ROOT: the one place concrete types are chosen
// and wired together with plain constructors. Nothing below cmd/ knows which
// store, which router, or which logger it is talking to.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/hemantksingh/go-starter/internal/config"
	"github.com/hemantksingh/go-starter/internal/engine"
	"github.com/hemantksingh/go-starter/internal/healthrun"
	"github.com/hemantksingh/go-starter/internal/httpapi"
	"github.com/hemantksingh/go-starter/internal/platform/logger"
	"github.com/hemantksingh/go-starter/internal/store/memory"
	"github.com/hemantksingh/go-starter/internal/tenant"
	"go.uber.org/zap"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log, err := logger.New(cfg.Env)
	if err != nil {
		return err
	}
	defer log.Sync() //nolint:errcheck

	// --- wire the graph, inner layers first ----------------------------------
	tenants, err := tenant.LoadFile(cfg.HTTP.TenantsFile)
	if err != nil {
		return err
	}
	defer tenants.Close()

	eng := engine.New(cfg.Engine, log)
	store := memory.NewRunStore() // Phase 3: postgres.NewRunStore(pool)
	runs := healthrun.NewService(store, eng, cfg.Engine.MaxURLs, log)

	srv := &http.Server{
		Addr:         cfg.HTTP.Addr,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		Handler: httpapi.New(httpapi.Deps{
			Log:          log,
			Tenants:      tenants,
			Runs:         runs,
			MaxBodyBytes: cfg.HTTP.MaxBodyBytes,
		}),
	}

	// --- run until a signal, then drain ---------------------------------------
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() {
		log.Info("api listening",
			zap.String("addr", cfg.HTTP.Addr),
			zap.Int("tenants", tenants.Len()),
			zap.Int("engine_workers", cfg.Engine.Workers))
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return fmt.Errorf("listen: %w", err) // bind failure etc.; ErrServerClosed can't happen here
	case <-ctx.Done():
	}

	// Shutdown order: stop accepting and drain HTTP first, THEN cancel the
	// background runs. Each step gets the configured grace period.
	log.Info("shutting down", zap.Duration("grace", cfg.HTTP.ShutdownTimeout))

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	// A deadline error here means the grace period ran out with requests
	// still in flight and they were cut off — exactly the case worth a warning.
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Warn("http shutdown did not drain cleanly", zap.Error(err))
	}
	if err := runs.Shutdown(shutdownCtx); err != nil {
		log.Warn("runs shutdown", zap.Error(err))
	}
	log.Info("bye")
	return nil
}
