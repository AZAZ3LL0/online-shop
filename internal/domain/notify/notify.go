// Package notify owns the outbox of messages the shop sends to Telegram and the
// retry policy the worker applies.
package notify

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Kind enumerates the outgoing message kinds, tech.md §4 (notifications.kind).
type Kind string

// Notification kinds.
const (
	KindOrderLinked   Kind = "order_linked"
	KindStatusChanged Kind = "status_changed"
	KindPaymentFailed Kind = "payment_failed"
)

// Delivery states of an outbox row.
const (
	StatusPending = "pending"
	StatusSent    = "sent"
	StatusFailed  = "failed"
)

// MaxAttempts is the retry budget before a row is parked as failed.
const MaxAttempts = 5

// backoff is the fixed retry ladder from tech.md §5.5.
var backoff = []time.Duration{
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
	time.Hour,
	6 * time.Hour,
}

// Notification is one queued outbound message.
type Notification struct {
	ID       uuid.UUID
	OrderID  *uuid.UUID
	ChatID   int64
	Kind     Kind
	Payload  []byte
	Attempts int
	Text     string
}

// NextAttemptAt returns when a row that has already failed attempts times should
// be retried, and whether any retry is left at all.
func NextAttemptAt(now time.Time, attempts int) (time.Time, bool) {
	if attempts < 1 || attempts >= MaxAttempts {
		return time.Time{}, false
	}
	return now.Add(backoff[attempts-1]), true
}

// DedupKey is the uniqueness rule that keeps one message per status transition.
func DedupKey(orderID uuid.UUID, kind Kind, status string) string {
	return orderID.String() + "|" + string(kind) + "|" + status
}

// Repository is the outbox access the worker depends on.
type Repository interface {
	Enqueue(ctx context.Context, n Notification, dedupKey string, text string) error
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]Notification, error)
	MarkSent(ctx context.Context, id uuid.UUID, now time.Time) error
	MarkRetry(ctx context.Context, id uuid.UUID, next time.Time, cause string) error
	MarkFailed(ctx context.Context, id uuid.UUID, cause string) error
}
