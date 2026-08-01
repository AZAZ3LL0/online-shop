package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/qzq-kiim/shop/internal/config"
	"github.com/qzq-kiim/shop/internal/telegram"
)

// setWebhook registers the bot callback URL of this deployment, tech.md §5.5.
// It is a deploy step rather than something serve does on boot: pointing the
// bot at a host is a one-off decision, and a restarted staging container must
// never steal the webhook from production.
//
// TODO(contract-gap): tech.md §2 does not list this subcommand yet.
func setWebhook(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg.Env)

	callback, err := webhookCallback(cfg)
	if err != nil {
		return err
	}
	if err := telegram.NewClient(cfg.Telegram.BotToken).SetWebhook(ctx, callback, cfg.Telegram.WebhookSecret); err != nil {
		return fmt.Errorf("set-webhook: %w", err)
	}

	// The path secret is part of the callback URL, so only the host is logged
	// (tech.md §9.11).
	host, _ := hostOf(cfg.BaseURL)
	log.Info("telegram webhook registered", slog.String("host", host))
	return nil
}

// webhookCallback builds the callback URL and refuses every address Telegram
// cannot reach or that would unhook the live bot.
func webhookCallback(cfg config.Config) (string, error) {
	if cfg.TelegramProvider != config.ProviderTelegram {
		return "", fmt.Errorf("set-webhook: TELEGRAM_PROVIDER must be %q, got %q", config.ProviderTelegram, cfg.TelegramProvider)
	}
	base, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return "", fmt.Errorf("set-webhook: APP_BASE_URL: %w", err)
	}
	if base.Scheme != "https" {
		return "", fmt.Errorf("set-webhook: APP_BASE_URL must be https, got %q", cfg.BaseURL)
	}
	switch strings.ToLower(base.Hostname()) {
	case "", "localhost", "127.0.0.1", "::1":
		return "", fmt.Errorf("set-webhook: APP_BASE_URL must be the public domain, got %q", cfg.BaseURL)
	}
	return cfg.BaseURL + telegram.WebhookPath(cfg.Telegram.WebhookPathSecret), nil
}

func hostOf(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	return u.Host, nil
}
