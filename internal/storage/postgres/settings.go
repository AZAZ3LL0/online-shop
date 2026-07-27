package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/qzq-kiim/shop/internal/storage/postgres/sqlcgen"
)

// ErrNoSetting is returned when a settings key has never been written.
var ErrNoSetting = errors.New("postgres: setting not found")

// Known settings keys, tech.md §4 (settings).
const (
	SettingShippingCents  = "shipping_cents"
	SettingShopPaused     = "shop_paused"
	SettingOrderTTLMinute = "order_ttl_minutes"
)

// SettingsRepo is the single key-value access point for runtime settings.
type SettingsRepo struct {
	q *sqlcgen.Queries
}

// NewSettingsRepo returns the settings repository bound to the store.
func NewSettingsRepo(s *Store) *SettingsRepo { return &SettingsRepo{q: s.q} }

// Get returns the raw jsonb value stored under key.
func (r *SettingsRepo) Get(ctx context.Context, key string) ([]byte, error) {
	v, err := r.q.GetSetting(ctx, key)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%q: %w", key, ErrNoSetting)
		}
		return nil, fmt.Errorf("get setting: %w", err)
	}
	return v, nil
}

// Set writes the raw jsonb value under key.
func (r *SettingsRepo) Set(ctx context.Context, key string, value []byte) error {
	err := r.q.UpsertSetting(ctx, sqlcgen.UpsertSettingParams{Key: key, Value: value})
	if err != nil {
		return fmt.Errorf("upsert setting: %w", err)
	}
	return nil
}
