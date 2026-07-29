// Package settings owns the runtime parameters an operator can change without a
// release. Every value lives in the single key-value table of tech.md §4; the
// environment only supplies what the shop runs on until a key is written.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Keys, tech.md §4 (settings.key). Nothing else is stored in this table.
const (
	KeyShippingCents   = "shipping_cents"
	KeyShopPaused      = "shop_paused"
	KeyOrderTTLMinutes = "order_ttl_minutes"
)

// Keys is every setting the panel knows, in the order it shows them.
var Keys = []string{KeyShippingCents, KeyOrderTTLMinutes, KeyShopPaused}

// ErrMissing is returned by the repository when a key has never been written.
var ErrMissing = errors.New("settings: no value stored")

// ErrOutOfRange marks a value the panel refuses to store.
var ErrOutOfRange = errors.New("settings: value out of range")

// Bounds of the editable values. The reservation window has a floor because a
// buyer needs time to pay a crypto invoice, and a ceiling because held stock is
// stock nobody else can buy.
const (
	MaxShippingCents = 100_000
	MinTTLMinutes    = 5
	MaxTTLMinutes    = 1440
)

// Values is the whole runtime configuration of the shop.
type Values struct {
	ShippingCents int64
	OrderTTL      time.Duration
	ShopPaused    bool
}

// TTLMinutes is the reservation window as the form edits it.
func (v Values) TTLMinutes() int { return int(v.OrderTTL / time.Minute) }

// Validate reports what the panel refuses to store, in one pass so the form can
// show every problem at once.
func (v Values) Validate() error {
	var problems []string
	if v.ShippingCents < 0 || v.ShippingCents > MaxShippingCents {
		problems = append(problems, fmt.Sprintf("shipping must be between 0 and %d cents", MaxShippingCents))
	}
	if m := v.TTLMinutes(); m < MinTTLMinutes || m > MaxTTLMinutes {
		problems = append(problems, fmt.Sprintf("the reservation window must be between %d and %d minutes", MinTTLMinutes, MaxTTLMinutes))
	}
	if len(problems) > 0 {
		return fmt.Errorf("%w: %s", ErrOutOfRange, problems[0])
	}
	return nil
}

// Repository is the single key-value access point behind every setting. No
// setting gets a column of its own (tech.md §4).
type Repository interface {
	// Get returns the raw jsonb value, wrapping ErrMissing when the key has
	// never been written.
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte) error
}

// Service reads and writes the settings. Reads go to the database on every call
// rather than through a cache: a paused shop has to stay paused from the moment
// the operator says so, and one small query per page costs less than a stale
// answer.
type Service struct {
	repo     Repository
	defaults Values
}

// NewService wires the settings service over the values the environment starts
// the shop with (tech.md §10.2).
func NewService(repo Repository, defaults Values) *Service {
	return &Service{repo: repo, defaults: defaults}
}

// Defaults are the values the environment supplies, used for every key that has
// never been written.
func (s *Service) Defaults() Values { return s.defaults }

// Values reads the whole configuration.
func (s *Service) Values(ctx context.Context) (Values, error) {
	shipping, err := s.ShippingCents(ctx)
	if err != nil {
		return Values{}, err
	}
	ttl, err := s.OrderTTL(ctx)
	if err != nil {
		return Values{}, err
	}
	paused, err := s.Paused(ctx)
	if err != nil {
		return Values{}, err
	}
	return Values{ShippingCents: shipping, OrderTTL: ttl, ShopPaused: paused}, nil
}

// ShippingCents is what delivery costs right now.
func (s *Service) ShippingCents(ctx context.Context) (int64, error) {
	return readValue(ctx, s.repo, KeyShippingCents, s.defaults.ShippingCents)
}

// OrderTTL is how long a stock reservation is held.
func (s *Service) OrderTTL(ctx context.Context) (time.Duration, error) {
	minutes, err := readValue(ctx, s.repo, KeyOrderTTLMinutes, int64(s.defaults.TTLMinutes()))
	if err != nil {
		return 0, err
	}
	return time.Duration(minutes) * time.Minute, nil
}

// Paused reports whether the shop currently takes orders.
func (s *Service) Paused(ctx context.Context) (bool, error) {
	return readValue(ctx, s.repo, KeyShopPaused, s.defaults.ShopPaused)
}

// Save validates the whole set and writes every key.
func (s *Service) Save(ctx context.Context, in Values) error {
	if err := in.Validate(); err != nil {
		return err
	}
	for key, value := range map[string]any{
		KeyShippingCents:   in.ShippingCents,
		KeyOrderTTLMinutes: in.TTLMinutes(),
		KeyShopPaused:      in.ShopPaused,
	} {
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("settings: encode %s: %w", key, err)
		}
		if err := s.repo.Set(ctx, key, encoded); err != nil {
			return fmt.Errorf("settings: store %s: %w", key, err)
		}
	}
	return nil
}

// readValue decodes one stored setting, falling back to the environment default
// when the key was never written. A value that is there but unreadable is an
// error: silently running on a default would hide a broken configuration.
func readValue[T any](ctx context.Context, repo Repository, key string, fallback T) (T, error) {
	raw, err := repo.Get(ctx, key)
	if errors.Is(err, ErrMissing) {
		return fallback, nil
	}
	if err != nil {
		return fallback, fmt.Errorf("settings: read %s: %w", key, err)
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return fallback, fmt.Errorf("settings: decode %s: %w", key, err)
	}
	return value, nil
}
