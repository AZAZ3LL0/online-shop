package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

const defaultSessionSecret = "change-me-in-production-32chars!!"

type Config struct {
	Port              string
	Env               string
	BaseURL           string
	DatabaseURL       string
	SessionSecret     string
	NowPaymentsKey    string
	NowPaymentsSecret string
	SuccessURL        string
	CancelURL         string
}

// IsProduction reports whether the app runs in production mode.
func (c *Config) IsProduction() bool { return c.Env == "production" }

// CookieSecure returns whether cookies should carry the Secure flag. Enabled in
// production (the app is expected to sit behind TLS/Nginx there).
func (c *Config) CookieSecure() bool { return c.IsProduction() }

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file, reading from environment")
	}

	cfg := &Config{
		Port:              getEnv("PORT", "8080"),
		Env:               getEnv("ENV", "development"),
		BaseURL:           getEnv("BASE_URL", "http://localhost:8080"),
		DatabaseURL:       mustEnv("DATABASE_URL"),
		SessionSecret:     getEnv("SESSION_SECRET", defaultSessionSecret),
		NowPaymentsKey:    os.Getenv("NOWPAYMENTS_API_KEY"),
		NowPaymentsSecret: os.Getenv("NOWPAYMENTS_IPN_SECRET"),
		SuccessURL:        getEnv("NOWPAYMENTS_SUCCESS_URL", "http://localhost:8080/order/"),
		CancelURL:         getEnv("NOWPAYMENTS_CANCEL_URL", "http://localhost:8080/cart"),
	}

	// Fail fast in production on insecure/placeholder configuration.
	if cfg.IsProduction() {
		if cfg.SessionSecret == defaultSessionSecret || len(cfg.SessionSecret) < 32 {
			log.Fatal("SESSION_SECRET must be set to a strong value (>=32 chars) in production")
		}
		if cfg.NowPaymentsSecret == "" {
			log.Println("WARNING: NOWPAYMENTS_IPN_SECRET is empty — payment webhooks will be rejected")
		}
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s is not set", key)
	}
	return v
}
