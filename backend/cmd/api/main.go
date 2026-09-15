// Command api serves the notes API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mare-analitica/go-vite-swarm-starter/backend/internal/config"
	"github.com/mare-analitica/go-vite-swarm-starter/backend/internal/events"
	"github.com/mare-analitica/go-vite-swarm-starter/backend/internal/httpapi"
	"github.com/mare-analitica/go-vite-swarm-starter/backend/internal/storage"
	"github.com/mare-analitica/go-vite-swarm-starter/backend/internal/store"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("version", version)
	if err := run(log); err != nil {
		log.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.FromEnvironment()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := connectWithRetry(ctx, cfg.DB.DSN(), log)
	if err != nil {
		return err
	}
	defer db.Close()

	applied, err := db.Migrate(ctx)
	if err != nil {
		return err
	}
	log.Info("migrations", "applied", applied)

	objects, err := storage.New(cfg.S3)
	if err != nil {
		return err
	}

	publisher := events.NewWebhook(cfg.N8NWebhookURL, log)

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.Handler(httpapi.Deps{
			Notes:      db,
			Database:   db,
			Storage:    objects,
			Events:     publisher,
			Log:        log,
			CORSOrigin: cfg.CORSAllowedOrigin,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	err = srv.Shutdown(shutdownCtx)
	publisher.Wait(shutdownCtx)
	return err
}

// connectWithRetry waits for PostgreSQL during stack start-up instead of
// crash-looping while the database container is still initializing.
func connectWithRetry(ctx context.Context, dsn string, log *slog.Logger) (*store.Postgres, error) {
	deadline := time.Now().Add(60 * time.Second)
	for {
		attemptCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		db, err := store.Open(attemptCtx, dsn)
		cancel()
		if err == nil {
			return db, nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return nil, err
		}
		log.Warn("database not ready, retrying", "error", err)
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
