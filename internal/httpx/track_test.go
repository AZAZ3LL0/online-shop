package httpx_test

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

var reTrackLink = regexp.MustCompile(`https://t\.me/` + botUsername + `\?start=([0-9a-f]{16})`)

// linkCodeOf reads the code the order was given at checkout straight from the
// row: the page is then checked against it, not against itself.
func linkCodeOf(t *testing.T, env *shopEnv, number string) string {
	t.Helper()
	var code string
	err := env.store.Pool().QueryRow(context.Background(),
		`SELECT tg_link_code FROM orders WHERE number = $1`, number).Scan(&code)
	if err != nil {
		t.Fatalf("read link code: %v", err)
	}
	return code
}

// S4.1: the status page offers the bot, and the deep link carries the order's
// own code. The QR is the same link in a form a phone can scan off a desktop.
func TestOrderPageOffersTheTelegramEntryPoint(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "1")

	status, page := get(t, env.client, env.server.URL+"/order/"+order.token)
	if status != http.StatusOK {
		t.Fatalf("GET /order/{token} = %d", status)
	}
	if !strings.Contains(page, "Track order in Telegram") {
		t.Fatalf("the status page offers no Telegram entry point: %s", page)
	}

	got := capture(t, reTrackLink, page, "telegram deep link")
	if want := linkCodeOf(t, env, order.number); got != want {
		t.Errorf("deep link carries %q, want the order's own code %q", got, want)
	}
	if !strings.Contains(page, `src="data:image/png;base64,`) {
		t.Error("the status page carries no inline QR image")
	}
}

// The same entry point is offered before the buyer leaves for the invoice,
// which with the fake provider is the local payment page (tech.md §5.5).
func TestDevPaymentPageOffersTheTelegramEntryPoint(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "1")

	status, page := get(t, env.client, env.server.URL+"/dev/pay/"+order.number)
	if status != http.StatusOK {
		t.Fatalf("GET /dev/pay/{number} = %d", status)
	}
	got := capture(t, reTrackLink, page, "telegram deep link")
	if want := linkCodeOf(t, env, order.number); got != want {
		t.Errorf("deep link carries %q, want %q", got, want)
	}
}

// The link code is never published on a page that is not already behind the
// order's own token: knowing it is enough to subscribe to the order.
func TestLinkCodeIsNotLeakedToTheStorefront(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "1")
	code := linkCodeOf(t, env, order.number)

	for _, path := range []string{"/", "/cart", "/checkout"} {
		_, page := get(t, env.client, env.server.URL+path)
		if strings.Contains(page, code) {
			t.Errorf("%s leaks the telegram link code", path)
		}
	}
}
