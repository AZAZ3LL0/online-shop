package config_test

import (
	"errors"
	"testing"

	"github.com/qzq-kiim/shop/internal/config"
)

func setValid(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_BASE_URL", "http://localhost:8080")
	t.Setenv("APP_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "postgres://app:app@localhost:5432/shop?sslmode=disable")
	t.Setenv("PAYMENTS_PROVIDER", "fake")
	t.Setenv("TELEGRAM_PROVIDER", "fake")
	t.Setenv("ADMIN_TELEGRAM_IDS", "")
	t.Setenv("ORDER_TTL_MINUTES", "30")
	t.Setenv("SHIPPING_CENTS", "0")
}

func TestLoadWithoutExternalKeys(t *testing.T) {
	setValid(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("fake providers must load without external keys: %v", err)
	}
	if cfg.OrderTTL.Minutes() != 30 {
		t.Errorf("OrderTTL = %v, want 30m", cfg.OrderTTL)
	}
	if !cfg.IsDev() {
		t.Error("dev env not detected")
	}
}

func TestLoadRejectsShortSecret(t *testing.T) {
	setValid(t)
	t.Setenv("APP_SECRET", "too-short")
	if _, err := config.Load(); !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
}

func TestLoadRejectsMissingDatabaseURL(t *testing.T) {
	setValid(t)
	t.Setenv("DATABASE_URL", "")
	if _, err := config.Load(); !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
}

func TestLoadRequiresKeysForRealProviders(t *testing.T) {
	setValid(t)
	t.Setenv("PAYMENTS_PROVIDER", "nowpayments")
	if _, err := config.Load(); !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("want ErrInvalid for nowpayments without keys, got %v", err)
	}
}

func TestAdminTelegramIDs(t *testing.T) {
	setValid(t)
	t.Setenv("ADMIN_TELEGRAM_IDS", "1, 22 ,333")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := []int64{1, 22, 333}
	if len(cfg.AdminTelegramIDs) != len(want) {
		t.Fatalf("got %v, want %v", cfg.AdminTelegramIDs, want)
	}
	for i := range want {
		if cfg.AdminTelegramIDs[i] != want[i] {
			t.Fatalf("got %v, want %v", cfg.AdminTelegramIDs, want)
		}
	}
}
