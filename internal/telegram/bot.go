// Package telegram wraps the Bot API. Only the Bot interface leaves this
// package, so the fake and the real client are interchangeable.
package telegram

import "context"

// Button is one inline keyboard button pointing at a URL.
type Button struct {
	Text string
	URL  string
}

// Keyboard is an inline keyboard laid out row by row.
type Keyboard struct {
	Rows [][]Button
}

// Bot is the outgoing Telegram seam, tech.md §5.5.
type Bot interface {
	SendMessage(ctx context.Context, chatID int64, text string, kb *Keyboard) error
	SetWebhook(ctx context.Context, url, secret string) error
}
