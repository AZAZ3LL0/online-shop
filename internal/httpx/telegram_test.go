package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/qzq-kiim/shop/internal/httpx/handler/webhook"
)

// The chat every bot test talks from.
const testChatID = int64(4242)

// update posts one bot update the way Telegram would: the secret in the path,
// the secret token in the header.
func update(t *testing.T, env *shopEnv, body map[string]any, pathSecret, headerSecret string) (int, string) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal update: %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		env.server.URL+"/webhooks/telegram/"+pathSecret, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("build update request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(webhook.HeaderSecretToken, headerSecret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("deliver update: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out bytes.Buffer
	_, _ = out.ReadFrom(resp.Body)
	return resp.StatusCode, out.String()
}

// message builds a /start or /status update from this chat.
func message(updateID int64, text string) map[string]any {
	return map[string]any{
		"update_id": updateID,
		"message": map[string]any{
			"chat": map[string]any{"id": testChatID},
			"from": map[string]any{"username": "buyer"},
			"text": text,
		},
	}
}

func countRows(t *testing.T, env *shopEnv, query string, args ...any) int {
	t.Helper()
	var n int
	if err := env.store.Pool().QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func linkCount(t *testing.T, env *shopEnv) int {
	return countRows(t, env, `SELECT count(*) FROM telegram_links`)
}

func notificationCount(t *testing.T, env *shopEnv, kind string) int {
	return countRows(t, env, `SELECT count(*) FROM notifications WHERE kind = $1`, kind)
}

// A body without the right secrets never reaches the bot at all, and it must
// leave no trace: not even the update id is recorded (tech.md §5.5).
func TestTelegramWebhookRejectsAWrongSecret(t *testing.T) {
	env := startShopEnv(t)

	cases := []struct {
		name         string
		pathSecret   string
		headerSecret string
	}{
		{"wrong header token", webhookPathSecret, "not-the-secret"},
		{"missing header token", webhookPathSecret, ""},
		{"wrong path secret", "not-the-path", webhookSecret},
	}
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, _ := update(t, env, message(int64(100+i), "/status"), c.pathSecret, c.headerSecret)
			if status != http.StatusUnauthorized {
				t.Fatalf("%s = %d, want 401", c.name, status)
			}
		})
	}
	if n := countRows(t, env, `SELECT count(*) FROM telegram_updates`); n != 0 {
		t.Errorf("%d rejected updates were recorded, want none", n)
	}
	if len(env.bot.Sent()) != 0 {
		t.Error("a rejected update must not produce a reply")
	}
}

// S4.2 acceptance: /start <code> links the chat to the order and queues exactly
// one confirmation, and redelivering the same update_id changes nothing
// (tech.md §11.2, idempotency).
func TestStartWithACodeLinksTheOrderExactlyOnce(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "1")
	code := linkCodeOf(t, env, order.number)

	for range 3 {
		status, body := update(t, env, message(500, "/start "+code), webhookPathSecret, webhookSecret)
		if status != http.StatusOK {
			t.Fatalf("redelivered /start = %d, want 200: %s", status, body)
		}
	}

	if n := linkCount(t, env); n != 1 {
		t.Errorf("telegram_links rows = %d, want 1", n)
	}
	if n := notificationCount(t, env, "order_linked"); n != 1 {
		t.Errorf("order_linked notifications = %d, want 1", n)
	}

	var chatID int64
	err := env.store.Pool().QueryRow(context.Background(),
		`SELECT l.chat_id FROM telegram_links l
		 JOIN orders o ON o.id = l.order_id WHERE o.number = $1`, order.number).Scan(&chatID)
	if err != nil {
		t.Fatalf("read link: %v", err)
	}
	if chatID != testChatID {
		t.Errorf("linked chat = %d, want %d", chatID, testChatID)
	}
}

// A distinct update_id carrying the same command is not a redelivery, but the
// link and its confirmation are still written only once.
func TestRepeatedStartFromANewUpdateStillLinksOnce(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "1")
	code := linkCodeOf(t, env, order.number)

	for i := range 3 {
		if status, _ := update(t, env, message(int64(600+i), "/start "+code), webhookPathSecret, webhookSecret); status != http.StatusOK {
			t.Fatalf("/start = %d, want 200", status)
		}
	}
	if n := linkCount(t, env); n != 1 {
		t.Errorf("telegram_links rows = %d, want 1", n)
	}
	if n := notificationCount(t, env, "order_linked"); n != 1 {
		t.Errorf("order_linked notifications = %d, want 1", n)
	}
}

// An unknown code gets the same neutral answer as a bare /start: whether an
// order exists is never confirmed to a guesser (tech.md §5.5).
func TestStartWithAnUnknownCodeSaysNothing(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "1")

	if status, _ := update(t, env, message(700, "/start "+strings.Repeat("0", 16)), webhookPathSecret, webhookSecret); status != http.StatusOK {
		t.Fatalf("unknown code = %d, want 200", status)
	}
	if n := linkCount(t, env); n != 0 {
		t.Errorf("an unknown code created %d links", n)
	}

	sent := env.bot.Sent()
	if len(sent) != 1 {
		t.Fatalf("bot sent %d messages, want one neutral reply", len(sent))
	}
	if strings.Contains(sent[0].Text, order.number) {
		t.Errorf("the reply leaks an order number: %q", sent[0].Text)
	}

	// The bare /start answer must be the very same text.
	if status, _ := update(t, env, message(701, "/start"), webhookPathSecret, webhookSecret); status != http.StatusOK {
		t.Fatalf("bare /start = %d", status)
	}
	sent = env.bot.Sent()
	if len(sent) != 2 || sent[0].Text != sent[1].Text {
		t.Errorf("an unknown code is distinguishable from a bare /start:\n%q\n%q", sent[0].Text, sent[1].Text)
	}
}

// /status reports every order this chat follows.
func TestStatusListsTheLinkedOrders(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "1")
	code := linkCodeOf(t, env, order.number)

	if status, _ := update(t, env, message(800, "/start "+code), webhookPathSecret, webhookSecret); status != http.StatusOK {
		t.Fatalf("/start = %d", status)
	}
	if status, _ := update(t, env, message(801, "/status"), webhookPathSecret, webhookSecret); status != http.StatusOK {
		t.Fatalf("/status = %d", status)
	}

	sent := env.bot.Sent()
	if len(sent) == 0 {
		t.Fatal("/status produced no reply")
	}
	last := sent[len(sent)-1]
	if last.ChatID != testChatID {
		t.Errorf("reply went to chat %d, want %d", last.ChatID, testChatID)
	}
	if !strings.Contains(last.Text, order.number) {
		t.Errorf("/status does not name the order: %q", last.Text)
	}
	if !strings.Contains(last.Text, "waiting for your payment") {
		t.Errorf("/status does not describe the status in words: %q", last.Text)
	}
}

// A chat that follows nothing is told so, rather than getting an empty message.
func TestStatusWithoutAnyLink(t *testing.T) {
	env := startShopEnv(t)

	if status, _ := update(t, env, message(900, "/status"), webhookPathSecret, webhookSecret); status != http.StatusOK {
		t.Fatalf("/status = %d", status)
	}
	sent := env.bot.Sent()
	if len(sent) != 1 {
		t.Fatalf("bot sent %d messages, want 1", len(sent))
	}
	if !strings.Contains(sent[0].Text, "not following any order") {
		t.Errorf("unexpected reply: %q", sent[0].Text)
	}
}

// Anything that is not a command gets the help text, and a body Telegram would
// never send is acknowledged rather than retried forever.
func TestUnknownMessageAndGarbageBodyAreAcknowledged(t *testing.T) {
	env := startShopEnv(t)

	if status, _ := update(t, env, message(1000, "hello there"), webhookPathSecret, webhookSecret); status != http.StatusOK {
		t.Fatalf("plain message = %d, want 200", status)
	}
	sent := env.bot.Sent()
	if len(sent) != 1 || !strings.Contains(sent[0].Text, "/status") {
		t.Errorf("a plain message must get the command hint, got %v", sent)
	}

	status, _ := update(t, env, map[string]any{"nothing": "useful"}, webhookPathSecret, webhookSecret)
	if status != http.StatusOK {
		t.Fatalf("body without an update_id = %d, want 200 so Telegram stops retrying", status)
	}
}

// Two chats can follow the same order; the second link is stored even though
// tech.md's dedup key gives it no second copy of the announcement.
func TestASecondChatCanFollowTheSameOrder(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "1")
	code := linkCodeOf(t, env, order.number)

	first := message(1100, "/start "+code)
	if status, _ := update(t, env, first, webhookPathSecret, webhookSecret); status != http.StatusOK {
		t.Fatalf("first /start = %d", status)
	}

	second := message(1101, "/start "+code)
	second["message"].(map[string]any)["chat"] = map[string]any{"id": testChatID + 1}
	if status, _ := update(t, env, second, webhookPathSecret, webhookSecret); status != http.StatusOK {
		t.Fatalf("second /start = %d", status)
	}

	if n := linkCount(t, env); n != 2 {
		t.Errorf("telegram_links rows = %d, want 2", n)
	}
}
