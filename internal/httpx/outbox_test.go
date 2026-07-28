package httpx_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/qzq-kiim/shop/internal/telegram"
	"github.com/qzq-kiim/shop/internal/worker"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// track links the chat to the order through the real bot webhook, so the tests
// below start from the state a buyer would actually be in.
func track(t *testing.T, env *shopEnv, updateID int64, number string) {
	t.Helper()
	code := linkCodeOf(t, env, number)
	if status, body := update(t, env, message(updateID, "/start "+code), webhookPathSecret, webhookSecret); status != http.StatusOK {
		t.Fatalf("/start = %d: %s", status, body)
	}
}

// outboxTexts returns the queued messages for one order, newest last.
func outboxTexts(t *testing.T, env *shopEnv, number string) []string {
	t.Helper()
	rows, err := env.store.Pool().Query(context.Background(),
		`SELECT n.payload->>'text' FROM notifications n
		 JOIN orders o ON o.id = n.order_id
		 WHERE o.number = $1 AND n.kind = 'status_changed'
		 ORDER BY n.created_at`, number)
	if err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			t.Fatalf("scan outbox row: %v", err)
		}
		out = append(out, text)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate outbox: %v", err)
	}
	return out
}

// S4.3 acceptance: one transition queues exactly one message, however often the
// provider redelivers the callback behind it (tech.md §11.2).
func TestStatusChangeQueuesExactlyOneMessage(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "1")
	track(t, env, 2000, order.number)

	for range 3 {
		if status, body := callback(t, env, order.number, "finished", true); status != http.StatusOK {
			t.Fatalf("callback = %d: %s", status, body)
		}
	}

	texts := outboxTexts(t, env, order.number)
	if len(texts) != 1 {
		t.Fatalf("queued %d messages for one transition, want 1: %v", len(texts), texts)
	}
	if !strings.Contains(texts[0], order.number) || !strings.Contains(texts[0], "paid") {
		t.Errorf("the queued message does not describe the transition: %q", texts[0])
	}
}

// Two different transitions are two different messages: the dedup key includes
// the status, so it must not swallow the second one.
func TestEachTransitionGetsItsOwnMessage(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "1")
	track(t, env, 2100, order.number)

	for _, status := range []string{"finished", "refunded"} {
		if code, body := callback(t, env, order.number, status, true); code != http.StatusOK {
			t.Fatalf("callback %s = %d: %s", status, code, body)
		}
	}

	texts := outboxTexts(t, env, order.number)
	if len(texts) != 2 {
		t.Fatalf("queued %d messages for two transitions, want 2: %v", len(texts), texts)
	}
	if !strings.Contains(texts[1], "refunded") {
		t.Errorf("the second message does not describe the refund: %q", texts[1])
	}
}

// An order nobody follows queues nothing: there is no one to tell, and an
// outbox row without a chat would never be deliverable.
func TestAnUntrackedOrderQueuesNothing(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "1")

	if status, body := callback(t, env, order.number, "finished", true); status != http.StatusOK {
		t.Fatalf("callback = %d: %s", status, body)
	}
	if got := orderStatusOf(t, env, order.number); got != "paid" {
		t.Fatalf("order status = %q, want paid", got)
	}
	if texts := outboxTexts(t, env, order.number); len(texts) != 0 {
		t.Errorf("an untracked order queued %v", texts)
	}
}

// The whole S4 path, end to end: the buyer follows the order in the bot, pays,
// and the worker delivers the status message.
func TestTrackedOrderIsToldItWasPaid(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "1")
	track(t, env, 2200, order.number)

	if status, body := callback(t, env, order.number, "finished", true); status != http.StatusOK {
		t.Fatalf("callback = %d: %s", status, body)
	}

	out := worker.NewOutbox(env.notify, env.bot, testLogger(), time.Minute)
	if err := out.Tick(context.Background()); err != nil {
		t.Fatalf("outbox tick: %v", err)
	}

	// The chat also gets the linking confirmation, and the worker claims the two
	// rows in no fixed order, so the payment message is looked for among them.
	var told bool
	for _, m := range env.bot.Sent() {
		if m.ChatID == testChatID && strings.Contains(m.Text, order.number) && strings.Contains(m.Text, "is paid") {
			told = true
		}
	}
	if !told {
		t.Fatalf("the buyer was never told about the payment, got %v", env.bot.Sent())
	}

	// A second pass over the same rows must not send anything again.
	before := len(env.bot.Sent())
	if err := out.Tick(context.Background()); err != nil {
		t.Fatalf("second outbox tick: %v", err)
	}
	if after := len(env.bot.Sent()); after != before {
		t.Errorf("a second pass sent %d more messages", after-before)
	}
}

// Telegram answers 429 when it is being pushed too hard. The message must be
// rescheduled, never dropped (TASKS.md S4.3, error path).
func TestOutboxReschedulesWhenTelegramRefuses(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "1")
	track(t, env, 2300, order.number)

	// The linking confirmation is drained first, so the refusal below lands on
	// the status message and on nothing else.
	out := worker.NewOutbox(env.notify, env.bot, testLogger(), time.Minute)
	if err := out.Tick(context.Background()); err != nil {
		t.Fatalf("drain outbox: %v", err)
	}

	if status, body := callback(t, env, order.number, "finished", true); status != http.StatusOK {
		t.Fatalf("callback = %d: %s", status, body)
	}
	env.bot.FailNext(telegram.ErrFakeFailure)
	if err := out.Tick(context.Background()); err != nil {
		t.Fatalf("outbox tick: %v", err)
	}

	var status string
	var attempts int
	var nextAttempt time.Time
	err := env.store.Pool().QueryRow(context.Background(),
		`SELECT n.status, n.attempts, n.next_attempt_at FROM notifications n
		 JOIN orders o ON o.id = n.order_id
		 WHERE o.number = $1 AND n.kind = 'status_changed'`, order.number).
		Scan(&status, &attempts, &nextAttempt)
	if err != nil {
		t.Fatalf("read notification: %v", err)
	}
	if status != "pending" {
		t.Errorf("status = %q, want the row still pending after one refusal", status)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
	// The first rung of the ladder in tech.md §5.5 is 30 seconds out.
	if delay := time.Until(nextAttempt); delay < 20*time.Second {
		t.Errorf("next attempt is %s away, want the 30s first backoff", delay)
	}
}
