// Package worker holds the background loops: outbox delivery today, expired
// reservation release from stage S3.
package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/qzq-kiim/shop/internal/domain/notify"
	"github.com/qzq-kiim/shop/internal/telegram"
)

// batchSize bounds one delivery pass.
const batchSize = 20

// Outbox drains the notifications table into Telegram, with the retry ladder
// from tech.md §5.5.
type Outbox struct {
	repo     notify.Repository
	bot      telegram.Bot
	log      *slog.Logger
	interval time.Duration
	now      func() time.Time
}

// NewOutbox wires the outbox worker.
func NewOutbox(repo notify.Repository, bot telegram.Bot, log *slog.Logger, interval time.Duration) *Outbox {
	return &Outbox{repo: repo, bot: bot, log: log, interval: interval, now: time.Now}
}

// Run drains the outbox until the context is cancelled.
func (o *Outbox) Run(ctx context.Context) {
	ticker := time.NewTicker(o.interval)
	defer ticker.Stop()

	for {
		if err := o.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
			o.log.Error("outbox pass failed", slog.String("error", err.Error()))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Tick delivers one batch. It is safe to run twice over the same rows: a row
// that was already sent is no longer pending, so it is never claimed again.
func (o *Outbox) Tick(ctx context.Context) error {
	due, err := o.repo.ClaimDue(ctx, o.now(), batchSize)
	if err != nil {
		return err
	}
	for _, n := range due {
		o.deliver(ctx, n)
	}
	return nil
}

func (o *Outbox) deliver(ctx context.Context, n notify.Notification) {
	text, err := notify.TextOf(n.Payload)
	if err != nil {
		// A row we cannot render will never become deliverable: park it.
		if markErr := o.repo.MarkFailed(ctx, n.ID, err.Error()); markErr != nil {
			o.log.Error("mark notification failed", slog.String("error", markErr.Error()))
		}
		return
	}

	sendErr := o.bot.SendMessage(ctx, n.ChatID, text, nil)
	if sendErr == nil {
		if err := o.repo.MarkSent(ctx, n.ID, o.now()); err != nil {
			o.log.Error("mark notification sent", slog.String("error", err.Error()))
		}
		return
	}

	attempts := n.Attempts + 1
	next, retry := notify.NextAttemptAt(o.now(), attempts)
	if !retry {
		o.log.Error("notification exhausted retries",
			slog.String("id", n.ID.String()),
			slog.String("error", sendErr.Error()))
		if err := o.repo.MarkFailed(ctx, n.ID, sendErr.Error()); err != nil {
			o.log.Error("mark notification failed", slog.String("error", err.Error()))
		}
		return
	}
	o.log.Warn("notification delivery failed, will retry",
		slog.String("id", n.ID.String()),
		slog.Int("attempts", attempts),
		slog.String("error", sendErr.Error()))
	if err := o.repo.MarkRetry(ctx, n.ID, next, sendErr.Error()); err != nil {
		o.log.Error("mark notification retry", slog.String("error", err.Error()))
	}
}
