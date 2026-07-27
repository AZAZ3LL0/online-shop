package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/domain/notify"
	"github.com/qzq-kiim/shop/internal/storage/postgres/sqlcgen"
)

// NotifyRepo stores the outbox of messages bound for Telegram.
type NotifyRepo struct {
	q *sqlcgen.Queries
}

// NewNotifyRepo returns the outbox repository bound to the store.
func NewNotifyRepo(s *Store) *NotifyRepo { return &NotifyRepo{q: s.q} }

var _ notify.Repository = (*NotifyRepo)(nil)

// Enqueue adds a message unless its dedup key is already present.
func (r *NotifyRepo) Enqueue(ctx context.Context, n notify.Notification, dedupKey string) error {
	_, err := r.q.EnqueueNotification(ctx, sqlcgen.EnqueueNotificationParams{
		OrderID:  nullUUID(n.OrderID),
		ChatID:   n.ChatID,
		Kind:     string(n.Kind),
		Payload:  n.Payload,
		DedupKey: dedupKey,
	})
	if err != nil {
		return fmt.Errorf("enqueue notification: %w", err)
	}
	return nil
}

// ClaimDue returns pending messages whose retry time has come.
func (r *NotifyRepo) ClaimDue(ctx context.Context, now time.Time, limit int) ([]notify.Notification, error) {
	rows, err := r.q.ClaimDueNotifications(ctx, sqlcgen.ClaimDueNotificationsParams{
		NextAttemptAt: ts(now),
		Limit:         int32of(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("claim due notifications: %w", err)
	}
	out := make([]notify.Notification, 0, len(rows))
	for _, row := range rows {
		out = append(out, notify.Notification{
			ID:       row.ID,
			OrderID:  toUUID(row.OrderID),
			ChatID:   row.ChatID,
			Kind:     notify.Kind(row.Kind),
			Payload:  row.Payload,
			Attempts: int(row.Attempts),
		})
	}
	return out, nil
}

// MarkSent records a successful delivery.
func (r *NotifyRepo) MarkSent(ctx context.Context, id uuid.UUID, now time.Time) error {
	err := r.q.MarkNotificationSent(ctx, sqlcgen.MarkNotificationSentParams{ID: id, SentAt: ts(now)})
	if err != nil {
		return fmt.Errorf("mark notification sent: %w", err)
	}
	return nil
}

// MarkRetry schedules the next attempt after a failure.
func (r *NotifyRepo) MarkRetry(ctx context.Context, id uuid.UUID, next time.Time, cause string) error {
	err := r.q.MarkNotificationRetry(ctx, sqlcgen.MarkNotificationRetryParams{
		ID:            id,
		NextAttemptAt: ts(next),
		LastError:     nullString(cause),
	})
	if err != nil {
		return fmt.Errorf("mark notification retry: %w", err)
	}
	return nil
}

// MarkFailed parks a message whose retry budget is exhausted.
func (r *NotifyRepo) MarkFailed(ctx context.Context, id uuid.UUID, cause string) error {
	err := r.q.MarkNotificationFailed(ctx, sqlcgen.MarkNotificationFailedParams{
		ID:        id,
		LastError: nullString(cause),
	})
	if err != nil {
		return fmt.Errorf("mark notification failed: %w", err)
	}
	return nil
}

// CountByStatus reports how many outbox rows sit in a delivery state.
func (r *NotifyRepo) CountByStatus(ctx context.Context, status string) (int64, error) {
	n, err := r.q.CountNotificationsByStatus(ctx, status)
	if err != nil {
		return 0, fmt.Errorf("count notifications: %w", err)
	}
	return n, nil
}
