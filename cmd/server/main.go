// Command server runs the MiSzaCsi BFF: it applies migrations, then serves the GraphQL API
// until SIGTERM or SIGINT.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pballok/miszacsi-bff/internal/config"
	"github.com/pballok/miszacsi-bff/internal/graph"
	"github.com/pballok/miszacsi-bff/internal/httpserver"
	"github.com/pballok/miszacsi-bff/internal/logging"
	"github.com/pballok/miszacsi-bff/internal/service"
	"github.com/pballok/miszacsi-bff/internal/store"
)

func main() {
	if err := run(context.Background(), os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, getenv func(string) string, stdout io.Writer) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, os.Interrupt)
	defer stop()

	cfg, err := config.Load(getenv)
	if err != nil {
		return err
	}
	logger := logging.New(stdout, cfg.LogLevel)

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("create database pool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	if err := store.Migrate(ctx, cfg.DatabaseURL); err != nil {
		return err
	}

	st := store.New(pool)
	svc := service.New(st, time.Now)
	gql := graph.NewHandler(svc, logger, graph.Options{
		MaxDepth:      cfg.GraphQLMaxDepth,
		MaxComplexity: cfg.GraphQLMaxComplexity,
		Development:   cfg.Development(),
	})
	var ready atomic.Bool
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpserver.New(gql, st, &ready, logger, cfg),
		ReadHeaderTimeout: 5 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServe() }()
	ready.Store(true)
	logger.InfoContext(ctx, "starting", "addr", cfg.HTTPAddr, "app_env", cfg.AppEnv)

	select {
	case err := <-serveErr:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	ready.Store(false)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shut down http server: %w", err)
	}
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server: %w", err)
	}
	pool.Close()
	logger.Info("stopped")
	return nil
}
