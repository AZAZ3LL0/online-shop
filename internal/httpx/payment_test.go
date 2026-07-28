package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/qzq-kiim/shop/internal/httpx/handler/webhook"
	"github.com/qzq-kiim/shop/internal/money"
	"github.com/qzq-kiim/shop/internal/payments/nowpayments"
)

// callback delivers one provider notification the way NOWPayments would: a raw
// body plus the HMAC header. The signature is produced by the same code the
// endpoint verifies with, so a body that is tampered with cannot pass.
func callback(t *testing.T, env *shopEnv, number, status string, sign bool) (int, string) {
	t.Helper()
	body, err := json.Marshal(nowpayments.DevCallback(number, status, money.New(3500, "USD")))
	if err != nil {
		t.Fatalf("marshal callback: %v", err)
	}
	return deliver(t, env, body, sign)
}

func deliver(t *testing.T, env *shopEnv, body []byte, sign bool) (int, string) {
	t.Helper()
	signature := "00"
	if sign {
		var err error
		signature, err = env.fake.SignBody(body)
		if err != nil {
			t.Fatalf("sign callback: %v", err)
		}
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		env.server.URL+"/webhooks/nowpayments", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build callback request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(webhook.HeaderSignature, signature)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("deliver callback: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out bytes.Buffer
	_, _ = out.ReadFrom(resp.Body)
	return resp.StatusCode, out.String()
}

func paymentEventCount(t *testing.T, env *shopEnv) int {
	t.Helper()
	var n int
	err := env.store.Pool().QueryRow(context.Background(), `SELECT count(*) FROM payment_events`).Scan(&n)
	if err != nil {
		t.Fatalf("count payment events: %v", err)
	}
	return n
}

// A paid order takes its units out of stock exactly once, however often the
// provider redelivers the same notification (tech.md §11.2).
func TestCallbackIsIdempotent(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "2")
	stockBefore, reservedBefore := variantStock(t, env, order.variantID)

	for range 3 {
		if status, body := callback(t, env, order.number, "finished", true); status != http.StatusOK {
			t.Fatalf("callback = %d: %s", status, body)
		}
	}

	if got := orderStatusOf(t, env, order.number); got != "paid" {
		t.Fatalf("order status = %q, want paid", got)
	}
	stock, reserved := variantStock(t, env, order.variantID)
	if stock != stockBefore-2 {
		t.Errorf("stock = %d, want %d taken out exactly once", stock, stockBefore-2)
	}
	if reserved != reservedBefore-2 {
		t.Errorf("reserved = %d, want the reservation cleared once", reserved)
	}
	if stock-reserved < 0 {
		t.Errorf("stock invariant broken: stock %d, reserved %d", stock, reserved)
	}
	if n := paymentEventCount(t, env); n != 1 {
		t.Errorf("payment events = %d, want one row per distinct callback", n)
	}
}

// Notifications may arrive in any order; the status machine never runs
// backwards (tech.md §5.1).
func TestCallbacksOutOfOrderDoNotRollBack(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "1")
	stockBefore, _ := variantStock(t, env, order.variantID)

	for _, status := range []string{"finished", "waiting", "confirming", "expired"} {
		if code, body := callback(t, env, order.number, status, true); code != http.StatusOK {
			t.Fatalf("callback %s = %d: %s", status, code, body)
		}
	}

	if got := orderStatusOf(t, env, order.number); got != "paid" {
		t.Errorf("order status = %q, want paid to stand", got)
	}
	stock, reserved := variantStock(t, env, order.variantID)
	if stock != stockBefore-1 {
		t.Errorf("stock = %d, want %d", stock, stockBefore-1)
	}
	if reserved != 0 {
		t.Errorf("reserved = %d, want 0", reserved)
	}
}

// A forged body is rejected before it is parsed, and it changes nothing.
func TestCallbackWithABadSignatureIsRejected(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "1")

	status, _ := callback(t, env, order.number, "finished", false)
	if status != http.StatusUnauthorized {
		t.Fatalf("unsigned callback = %d, want 401", status)
	}
	if got := orderStatusOf(t, env, order.number); got != "awaiting_payment" {
		t.Errorf("a rejected callback changed the status to %q", got)
	}

	// The rejected body is still on file, flagged as unverified.
	var okFlag bool
	err := env.store.Pool().QueryRow(context.Background(),
		`SELECT signature_ok FROM payment_events ORDER BY received_at DESC LIMIT 1`).Scan(&okFlag)
	if err != nil {
		t.Fatalf("read payment event: %v", err)
	}
	if okFlag {
		t.Error("the rejected callback must be filed with signature_ok = false")
	}
}

// A short payment is not a payment: the order waits and a human settles it,
// tech.md §15.
func TestPartialPaymentDoesNotPayTheOrder(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "1")
	stockBefore, reservedBefore := variantStock(t, env, order.variantID)

	if status, body := callback(t, env, order.number, "partially_paid", true); status != http.StatusOK {
		t.Fatalf("partial callback = %d: %s", status, body)
	}
	if got := orderStatusOf(t, env, order.number); got != "awaiting_payment" {
		t.Errorf("order status = %q, want awaiting_payment", got)
	}

	stock, reserved := variantStock(t, env, order.variantID)
	if stock != stockBefore || reserved != reservedBefore {
		t.Errorf("a partial payment moved the stock: %d/%d, want %d/%d",
			stock, reserved, stockBefore, reservedBefore)
	}

	var raw string
	err := env.store.Pool().QueryRow(context.Background(),
		`SELECT status FROM payments WHERE provider_payment_id = $1`, nowpayments.DevPaymentID(order.number)).Scan(&raw)
	if err != nil {
		t.Fatalf("read payment: %v", err)
	}
	if raw != "partially_paid" {
		t.Errorf("the raw provider status = %q, the admin panel needs it as the flag", raw)
	}
}

// A failed payment gives the reservation back.
func TestFailedPaymentReleasesTheReservation(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "2")
	stockBefore, _ := variantStock(t, env, order.variantID)

	if status, body := callback(t, env, order.number, "failed", true); status != http.StatusOK {
		t.Fatalf("failed callback = %d: %s", status, body)
	}
	if got := orderStatusOf(t, env, order.number); got != "expired" {
		t.Errorf("order status = %q, want expired", got)
	}
	stock, reserved := variantStock(t, env, order.variantID)
	if stock != stockBefore || reserved != 0 {
		t.Errorf("stock/reserved = %d/%d, want %d/0", stock, reserved, stockBefore)
	}
}

// A callback about an order this shop does not have is filed and acknowledged:
// answering anything else only makes the provider retry forever.
func TestCallbackForAnUnknownOrderIsAccepted(t *testing.T) {
	env := startShopEnv(t)

	status, _ := callback(t, env, "ORD-000000-ZZZZ", "finished", true)
	if status != http.StatusOK {
		t.Fatalf("unknown order callback = %d, want 200", status)
	}
	if n := paymentEventCount(t, env); n != 1 {
		t.Errorf("payment events = %d, want the callback on file", n)
	}
}

// A body that is signed but not a callback is refused before anything is
// applied.
func TestCallbackWithAGarbageBodyIsRefused(t *testing.T) {
	env := startShopEnv(t)

	status, _ := deliver(t, env, []byte(`{"hello":"world"}`), true)
	if status != http.StatusBadRequest {
		t.Fatalf("garbage callback = %d, want 400", status)
	}
}
