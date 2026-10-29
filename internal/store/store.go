// Package store persists reservations in PostgreSQL through sqlc-generated queries.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pballok/miszacsi-bff/internal/service"
	"github.com/pballok/miszacsi-bff/internal/store/db"
)

// sqlStateExclusionViolation is raised when reservations_no_overlap rejects a row.
const sqlStateExclusionViolation = "23P01"

// Store implements service.Store on a pgx connection pool.
type Store struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// New returns a Store using pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, q: db.New(pool)}
}

// Ping checks that the database is reachable.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	return nil
}

// withTx runs fn in a transaction, committing when fn succeeds and rolling back otherwise.
func (s *Store) withTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	if err := fn(s.q.WithTx(tx)); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
		}
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// mapErr translates database errors into service errors.
func mapErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %w", service.ErrNotFound, err)
	}
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == sqlStateExclusionViolation {
		return fmt.Errorf("%w: %w", service.ErrSlotConflict, err)
	}
	return fmt.Errorf("database: %w", err)
}

func parseUUID(s string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		return pgtype.UUID{}, fmt.Errorf("parse uuid %q: %w", s, err)
	}
	return id, nil
}

func uuidString(id pgtype.UUID) string {
	return id.String()
}
