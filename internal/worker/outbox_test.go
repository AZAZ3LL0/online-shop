package worker_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/domain/notify"
	"github.com/qzq-kiim/shop/internal/telegram"
	"github.com/qzq-kiim/shop/internal/worker"
)

// memRepo is an in-memory outbox with the same claim rule as Postgres: only
// pending rows whose next attempt is due are handed out.
type memRepo struct {
	mu   sync.Mutex
	rows map[uuid.UUID]*row
	keys map[string]bool
}

type row struct {
	n       notify.Notification
	status  string
	nextAt  time.Time
	sentAt  time.Time
	lastErr string
}

func newMemRepo() *memRepo {
	return &memRepo{rows: map[uuid.UUID]*row{}, keys: map[string]bool{}}
}

func (m *memRepo) Enqueue(_ context.Context, n notify.Notification, dedupKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.keys[dedupKey] {
		return nil
	}
	m.keys[dedupKey] = true
	n.ID = uuid.New()
	m.rows[n.ID] = &row{n: n, status: notify.StatusPending}
	return nil
}

func (m *memRepo) ClaimDue(_ context.Context, now time.Time, limit int) ([]notify.Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []notify.Notification
	for id, r := range m.rows {
		if len(out) >= limit {
			break
		}
		if r.status != notify.StatusPending || r.nextAt.After(now) {
			continue
		}
		n := r.n
		n.ID = id
		out = append(out, n)
	}
	return out, nil
}

func (m *memRepo) MarkSent(_ context.Context, id uuid.UUID, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.rows[id]
	r.status = notify.StatusSent
	r.sentAt = now
	r.n.Attempts++
	return nil
}

func (m *memRepo) MarkRetry(_ context.Context, id uuid.UUID, next time.Time, cause string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.rows[id]
	r.n.Attempts++
	r.nextAt = next
	r.lastErr = cause
	return nil
}

func (m *memRepo) MarkFailed(_ context.Context, id uuid.UUID, cause string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.rows[id]
	r.status = notify.StatusFailed
	r.n.Attempts++
	r.lastErr = cause
	return nil
}

func (m *memRepo) countByStatus(status string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int
	for _, r := range m.rows {
		if r.status == status {
			n++
		}
	}
	return n
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func enqueue(t *testing.T, repo *memRepo, orderID uuid.UUID, text string) {
	t.Helper()
	payload, err := notify.NewPayload(text)
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	err = repo.Enqueue(context.Background(), notify.Notification{
		OrderID: &orderID,
		ChatID:  42,
		Kind:    notify.KindStatusChanged,
		Payload: payload,
	}, notify.DedupKey(orderID, notify.KindStatusChanged, "paid"))
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
}

// One transition must produce exactly one message, however many times the
// enqueue and the worker pass are repeated.
func TestOutboxIsIdempotent(t *testing.T) {
	repo := newMemRepo()
	bot := telegram.NewFake(discardLogger())
	orderID := uuid.New()

	enqueue(t, repo, orderID, "Order paid.")
	enqueue(t, repo, orderID, "Order paid.")

	w := worker.NewOutbox(repo, bot, discardLogger(), time.Minute)
	for range 3 {
		if err := w.Tick(context.Background()); err != nil {
			t.Fatalf("tick: %v", err)
		}
	}

	if got := len(bot.Sent()); got != 1 {
		t.Fatalf("bot received %d messages, want 1", got)
	}
	if got := repo.countByStatus(notify.StatusSent); got != 1 {
		t.Fatalf("%d rows sent, want 1", got)
	}
}

// A failing bot must not lose the message: it is rescheduled, not dropped.
func TestOutboxReschedulesOnFailure(t *testing.T) {
	repo := newMemRepo()
	bot := telegram.NewFake(discardLogger())
	orderID := uuid.New()
	enqueue(t, repo, orderID, "Order paid.")

	bot.FailNext(telegram.ErrFakeFailure)
	w := worker.NewOutbox(repo, bot, discardLogger(), time.Minute)
	if err := w.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(bot.Sent()) != 0 {
		t.Fatal("nothing must be recorded as sent after a failure")
	}
	if repo.countByStatus(notify.StatusFailed) != 0 {
		t.Fatal("one failure must not park the row")
	}

	// The retry is scheduled 30s out, so nothing is due right now.
	if err := w.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(bot.Sent()) != 0 {
		t.Fatal("row must not be delivered before its next attempt time")
	}
}

func TestOutboxParksUnrenderablePayload(t *testing.T) {
	repo := newMemRepo()
	bot := telegram.NewFake(discardLogger())
	err := repo.Enqueue(context.Background(), notify.Notification{
		ChatID:  42,
		Kind:    notify.KindStatusChanged,
		Payload: []byte(`{}`),
	}, "broken")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	w := worker.NewOutbox(repo, bot, discardLogger(), time.Minute)
	if err := w.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if repo.countByStatus(notify.StatusFailed) != 1 {
		t.Fatal("payload without text must be parked as failed")
	}
}

func TestBackoffLadder(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	want := []time.Duration{
		30 * time.Second,
		2 * time.Minute,
		10 * time.Minute,
		time.Hour,
		6 * time.Hour,
	}
	for i, d := range want {
		next, ok := notify.NextAttemptAt(now, i+1)
		if i+1 >= notify.MaxAttempts {
			if ok {
				t.Fatalf("attempt %d must exhaust the budget", i+1)
			}
			continue
		}
		if !ok {
			t.Fatalf("attempt %d must be retried", i+1)
		}
		if got := next.Sub(now); got != d {
			t.Errorf("attempt %d backoff = %v, want %v", i+1, got, d)
		}
	}
}
