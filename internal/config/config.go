// Package config reads the environment once, validates it and fails fast when a
// required value is missing. It is the only place that touches os.Getenv.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// ListenAddr is fixed: the reverse proxy always talks to the app on this port,
// so the public URL lives in APP_BASE_URL and no extra env var is needed.
const ListenAddr = ":8080"

// Environments.
const (
	EnvDev  = "dev"
	EnvProd = "prod"
)

// Provider selectors for the two external integrations.
const (
	ProviderFake        = "fake"
	ProviderNOWPayments = "nowpayments"
	ProviderTelegram    = "telegram"
)

// minSecretLen is the floor for the cookie signing key, tech.md §10.2.
const minSecretLen = 32

// ErrInvalid marks a configuration that must not start the application.
var ErrInvalid = errors.New("config: invalid")

// NOWPayments holds the payment provider credentials.
type NOWPayments struct {
	APIKey    string
	IPNSecret string
}

// Telegram holds the bot credentials and webhook secrets.
type Telegram struct {
	BotToken          string
	BotUsername       string
	WebhookSecret     string
	WebhookPathSecret string
}

// Config is the validated application configuration.
type Config struct {
	Env              string
	BaseURL          string
	Secret           []byte
	DatabaseURL      string
	PaymentsProvider string
	NOWPayments      NOWPayments
	TelegramProvider string
	Telegram         Telegram
	AdminTelegramIDs []int64
	OrderTTL         time.Duration
	ShippingCents    int64
}

// IsDev reports whether the application runs in development mode.
func (c Config) IsDev() bool { return c.Env == EnvDev }

// Load reads and validates the environment described in tech.md §10.2.
func Load() (Config, error) {
	var problems []string
	fail := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	cfg := Config{
		Env:              envOr("APP_ENV", EnvDev),
		BaseURL:          strings.TrimRight(os.Getenv("APP_BASE_URL"), "/"),
		Secret:           []byte(os.Getenv("APP_SECRET")),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		PaymentsProvider: envOr("PAYMENTS_PROVIDER", ProviderFake),
		NOWPayments: NOWPayments{
			APIKey:    os.Getenv("NOWPAYMENTS_API_KEY"),
			IPNSecret: os.Getenv("NOWPAYMENTS_IPN_SECRET"),
		},
		TelegramProvider: envOr("TELEGRAM_PROVIDER", ProviderFake),
		Telegram: Telegram{
			BotToken:          os.Getenv("TELEGRAM_BOT_TOKEN"),
			BotUsername:       strings.TrimPrefix(os.Getenv("TELEGRAM_BOT_USERNAME"), "@"),
			WebhookSecret:     os.Getenv("TELEGRAM_WEBHOOK_SECRET"),
			WebhookPathSecret: os.Getenv("TELEGRAM_WEBHOOK_PATH_SECRET"),
		},
	}

	if cfg.Env != EnvDev && cfg.Env != EnvProd {
		fail("APP_ENV must be %q or %q, got %q", EnvDev, EnvProd, cfg.Env)
	}
	if cfg.BaseURL == "" {
		fail("APP_BASE_URL is required")
	} else if _, err := url.ParseRequestURI(cfg.BaseURL); err != nil {
		fail("APP_BASE_URL is not a valid URL")
	}
	if len(cfg.Secret) < minSecretLen {
		fail("APP_SECRET is required and must be at least %d bytes", minSecretLen)
	}
	if cfg.DatabaseURL == "" {
		fail("DATABASE_URL is required")
	}

	switch cfg.PaymentsProvider {
	case ProviderFake:
	case ProviderNOWPayments:
		if cfg.NOWPayments.APIKey == "" {
			fail("NOWPAYMENTS_API_KEY is required when PAYMENTS_PROVIDER=nowpayments")
		}
		if cfg.NOWPayments.IPNSecret == "" {
			fail("NOWPAYMENTS_IPN_SECRET is required when PAYMENTS_PROVIDER=nowpayments")
		}
	default:
		fail("PAYMENTS_PROVIDER must be %q or %q", ProviderFake, ProviderNOWPayments)
	}

	switch cfg.TelegramProvider {
	case ProviderFake:
	case ProviderTelegram:
		if cfg.Telegram.BotToken == "" {
			fail("TELEGRAM_BOT_TOKEN is required when TELEGRAM_PROVIDER=telegram")
		}
		if cfg.Telegram.BotUsername == "" {
			fail("TELEGRAM_BOT_USERNAME is required when TELEGRAM_PROVIDER=telegram")
		}
		if cfg.Telegram.WebhookSecret == "" {
			fail("TELEGRAM_WEBHOOK_SECRET is required when TELEGRAM_PROVIDER=telegram")
		}
		if cfg.Telegram.WebhookPathSecret == "" {
			fail("TELEGRAM_WEBHOOK_PATH_SECRET is required when TELEGRAM_PROVIDER=telegram")
		}
	default:
		fail("TELEGRAM_PROVIDER must be %q or %q", ProviderFake, ProviderTelegram)
	}

	ids, err := parseIDs(os.Getenv("ADMIN_TELEGRAM_IDS"))
	if err != nil {
		fail("ADMIN_TELEGRAM_IDS: %v", err)
	}
	cfg.AdminTelegramIDs = ids

	ttl, err := parseInt("ORDER_TTL_MINUTES", 30)
	switch {
	case err != nil:
		fail("%v", err)
	case ttl <= 0:
		fail("ORDER_TTL_MINUTES must be positive")
	}
	cfg.OrderTTL = time.Duration(ttl) * time.Minute

	shipping, err := parseInt("SHIPPING_CENTS", 0)
	switch {
	case err != nil:
		fail("%v", err)
	case shipping < 0:
		fail("SHIPPING_CENTS must not be negative")
	}
	cfg.ShippingCents = shipping

	if len(problems) > 0 {
		return Config{}, fmt.Errorf("%w: %s", ErrInvalid, strings.Join(problems, "; "))
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func parseInt(key string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	return v, nil
}

func parseIDs(raw string) ([]int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	ids := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%q is not a telegram id", p)
		}
		ids = append(ids, id)
	}
	return ids, nil
}
