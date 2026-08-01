// Package postgres implements the domain repositories on top of sqlc-generated
// queries. Nothing outside this package builds SQL.
package postgres

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qzq-kiim/shop/internal/storage/postgres/sqlcgen"
)

// Store owns the connection pool and hands out repositories.
type Store struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

// Open connects to Postgres and verifies the connection before returning.
func Open(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConns = 10

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Store{pool: pool, q: sqlcgen.New(pool)}, nil
}

// Close releases the pool.
func (s *Store) Close() { s.pool.Close() }

// Ping reports whether the database still answers.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	return nil
}

// Pool exposes the pool for the migration runner and integration tests.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// withTx runs fn inside one transaction. A domain unit of work - place an
// order and reserve its stock, apply a callback and settle its stock - is one
// call to this helper, so it either happens completely or not at all.
func withTx(ctx context.Context, pool *pgxpool.Pool, fn func(*sqlcgen.Queries) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(sqlcgen.New(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func nullUUID(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// nullTime turns an absent bound into a NULL argument, which the filtered
// queries read as "this filter is not set".
func nullTime(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return ts(*t)
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// int32of narrows an int for a Postgres integer column, clamping instead of
// wrapping around. Every caller passes an already validated small number; the
// clamp only exists so no conversion can silently overflow.
func int32of(n int) int32 {
	if n > math.MaxInt32 {
		return math.MaxInt32
	}
	if n < math.MinInt32 {
		return math.MinInt32
	}
	return int32(n)
}

func toUUID(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	id := uuid.UUID(v.Bytes)
	return &id
}
