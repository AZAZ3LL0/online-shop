package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/qzq-kiim/shop/internal/storage/postgres/sqlcgen"
)

// ErrNoAdmin is returned when a login or session does not resolve.
var ErrNoAdmin = errors.New("postgres: admin not found")

// AdminUser is the stored administrator record.
type AdminUser struct {
	ID           uuid.UUID
	Login        string
	PasswordHash string
	TelegramID   *int64
}

// AdminSession is a live admin session.
type AdminSession struct {
	ID        uuid.UUID
	AdminID   uuid.UUID
	Login     string
	ExpiresAt time.Time
}

// AdminRepo stores administrators and their sessions.
type AdminRepo struct {
	q *sqlcgen.Queries
}

// NewAdminRepo returns the admin repository bound to the store.
func NewAdminRepo(s *Store) *AdminRepo { return &AdminRepo{q: s.q} }

// Upsert creates the administrator or replaces the password hash of an existing one.
func (r *AdminRepo) Upsert(ctx context.Context, login, passwordHash string) (uuid.UUID, error) {
	id, err := r.q.UpsertAdminUser(ctx, sqlcgen.UpsertAdminUserParams{
		Login:        login,
		PasswordHash: passwordHash,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("upsert admin user: %w", err)
	}
	return id, nil
}

// ByLogin loads an administrator by login.
func (r *AdminRepo) ByLogin(ctx context.Context, login string) (AdminUser, error) {
	row, err := r.q.GetAdminByLogin(ctx, login)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AdminUser{}, ErrNoAdmin
		}
		return AdminUser{}, fmt.Errorf("get admin by login: %w", err)
	}
	return AdminUser{
		ID:           row.ID,
		Login:        row.Login,
		PasswordHash: row.PasswordHash,
		TelegramID:   row.TelegramID,
	}, nil
}

// CreateSession opens a session for an administrator.
func (r *AdminRepo) CreateSession(ctx context.Context, adminID uuid.UUID, ip, userAgent string, expiresAt time.Time) (uuid.UUID, error) {
	id, err := r.q.CreateAdminSession(ctx, sqlcgen.CreateAdminSessionParams{
		AdminID:   adminID,
		Ip:        ip,
		UserAgent: nullString(userAgent),
		ExpiresAt: ts(expiresAt),
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("create admin session: %w", err)
	}
	if err := r.q.SetAdminLastLogin(ctx, adminID); err != nil {
		return uuid.Nil, fmt.Errorf("set last login: %w", err)
	}
	return id, nil
}

// Session loads a session that has not expired yet.
func (r *AdminRepo) Session(ctx context.Context, id uuid.UUID) (AdminSession, error) {
	row, err := r.q.GetAdminSession(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AdminSession{}, ErrNoAdmin
		}
		return AdminSession{}, fmt.Errorf("get admin session: %w", err)
	}
	return AdminSession{
		ID:        row.ID,
		AdminID:   row.AdminID,
		Login:     row.Login,
		ExpiresAt: row.ExpiresAt.Time,
	}, nil
}

// DeleteSession ends a session.
func (r *AdminRepo) DeleteSession(ctx context.Context, id uuid.UUID) error {
	if err := r.q.DeleteAdminSession(ctx, id); err != nil {
		return fmt.Errorf("delete admin session: %w", err)
	}
	return nil
}

// DeleteExpiredSessions clears sessions past their expiry.
func (r *AdminRepo) DeleteExpiredSessions(ctx context.Context) error {
	if err := r.q.DeleteExpiredAdminSessions(ctx); err != nil {
		return fmt.Errorf("delete expired admin sessions: %w", err)
	}
	return nil
}
