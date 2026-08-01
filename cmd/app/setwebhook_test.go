package main

import (
	"strings"
	"testing"

	"github.com/qzq-kiim/shop/internal/config"
)

func TestWebhookCallback(t *testing.T) {
	t.Parallel()

	prod := func(baseURL string) config.Config {
		return config.Config{
			BaseURL:          baseURL,
			TelegramProvider: config.ProviderTelegram,
			Telegram:         config.Telegram{WebhookPathSecret: "pathsecret"},
		}
	}

	tests := []struct {
		name string
		cfg  config.Config
		want string
	}{
		{
			name: "public https domain",
			cfg:  prod("https://shop.example"),
			want: "https://shop.example/webhooks/telegram/pathsecret",
		},
		{
			name: "fake provider is refused",
			cfg: config.Config{
				BaseURL:          "https://shop.example",
				TelegramProvider: config.ProviderFake,
			},
		},
		{name: "plain http is refused", cfg: prod("http://shop.example")},
		{name: "localhost is refused", cfg: prod("https://localhost:8080")},
		{name: "loopback is refused", cfg: prod("https://127.0.0.1")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := webhookCallback(tc.cfg)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("webhookCallback(%q) = %q, want an error", tc.cfg.BaseURL, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("webhookCallback(%q): %v", tc.cfg.BaseURL, err)
			}
			if got != tc.want {
				t.Fatalf("webhookCallback(%q) = %q, want %q", tc.cfg.BaseURL, got, tc.want)
			}
		})
	}
}

// The path secret is a credential; a failure must not echo it back.
func TestWebhookCallbackErrorsHideThePathSecret(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		BaseURL:          "http://shop.example",
		TelegramProvider: config.ProviderTelegram,
		Telegram:         config.Telegram{WebhookPathSecret: "pathsecret"},
	}
	_, err := webhookCallback(cfg)
	if err == nil {
		t.Fatal("want an error for a plain http base url")
	}
	if strings.Contains(err.Error(), "pathsecret") {
		t.Fatalf("error leaks the path secret: %v", err)
	}
}
