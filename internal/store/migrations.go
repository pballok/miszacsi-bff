package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers the pgx5:// database driver
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate applies all pending up migrations to the database at databaseURL.
func Migrate(ctx context.Context, databaseURL string) error {
	return runMigration(ctx, databaseURL, (*migrate.Migrate).Up)
}

// migrateDown reverts every migration. It exists for the migration tests.
func migrateDown(ctx context.Context, databaseURL string) error {
	return runMigration(ctx, databaseURL, (*migrate.Migrate).Down)
}

func runMigration(ctx context.Context, databaseURL string, step func(*migrate.Migrate) error) (err error) {
	src, err := iofs.New(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("open embedded migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, pgx5URL(databaseURL))
	if err != nil {
		return errors.Join(fmt.Errorf("connect migrator: %w", err), src.Close())
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if closeErr := errors.Join(srcErr, dbErr); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close migrator: %w", closeErr))
		}
	}()

	done := make(chan error, 1)
	go func() { done <- step(m) }()

	select {
	case err = <-done:
	case <-ctx.Done():
		m.GracefulStop <- true
		err = <-done
		if errors.Is(err, migrate.ErrNoChange) {
			err = nil
		}
		return errors.Join(fmt.Errorf("migrate: %w", ctx.Err()), err)
	}
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// pgx5URL rewrites a postgres:// or postgresql:// URL to the scheme the golang-migrate pgx/v5
// driver registers.
func pgx5URL(databaseURL string) string {
	for _, scheme := range []string{"postgresql://", "postgres://"} {
		if rest, ok := strings.CutPrefix(databaseURL, scheme); ok {
			return "pgx5://" + rest
		}
	}
	return databaseURL
}
