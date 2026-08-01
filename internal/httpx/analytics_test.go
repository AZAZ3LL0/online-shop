package httpx_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/qzq-kiim/shop/internal/httpx/middleware"
)

// TestVisitRecordsAttributionOfANewSession is the S6.1 acceptance criteria: a
// request without a sid opens a session and files one visit carrying every utm
// field of the landing query.
func TestVisitRecordsAttributionOfANewSession(t *testing.T) {
	env := startShopEnv(t)

	landing := "/?utm_source=instagram&utm_medium=social&utm_campaign=drop-2" +
		"&utm_content=story&utm_term=oversized"
	status, _ := get(t, env.client, env.server.URL+landing)
	if status != http.StatusOK {
		t.Fatalf("GET %s = %d", landing, status)
	}

	var (
		source, medium, campaign, content, term, landingPath string
		isBot                                                bool
	)
	err := env.store.Pool().QueryRow(context.Background(), `
		SELECT coalesce(utm_source, ''), coalesce(utm_medium, ''), coalesce(utm_campaign, ''),
		       coalesce(utm_content, ''), coalesce(utm_term, ''), landing_path, is_bot
		FROM visits
		WHERE utm_campaign = 'drop-2'`).
		Scan(&source, &medium, &campaign, &content, &term, &landingPath, &isBot)
	if err != nil {
		t.Fatalf("the new session did not record a visit: %v", err)
	}

	for _, field := range []struct{ got, want, name string }{
		{source, "instagram", "utm_source"},
		{medium, "social", "utm_medium"},
		{campaign, "drop-2", "utm_campaign"},
		{content, "story", "utm_content"},
		{term, "oversized", "utm_term"},
		{landingPath, "/", "landing_path"},
	} {
		if field.got != field.want {
			t.Errorf("%s = %q, want %q", field.name, field.got, field.want)
		}
	}
	if isBot {
		t.Error("a normal browser must not be filed as a bot")
	}

	// The session cookie is a sliding window: a second page must not open a
	// second session.
	if status, _ := get(t, env.client, env.server.URL+"/cart"); status != http.StatusOK {
		t.Fatalf("GET /cart = %d", status)
	}
	var visits int
	if err := env.store.Pool().QueryRow(context.Background(),
		`SELECT count(*) FROM visits WHERE utm_campaign = 'drop-2'`).Scan(&visits); err != nil {
		t.Fatalf("count visits: %v", err)
	}
	if visits != 1 {
		t.Fatalf("visits recorded for one session = %d, want 1", visits)
	}
}

// TestBotTrafficIsFlaggedAndExcluded is the other half of S6.1: a crawler is
// marked and never reaches the reported numbers.
func TestBotTrafficIsFlaggedAndExcluded(t *testing.T) {
	env := startShopEnv(t)

	client := newClient(t)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		env.server.URL+"/?utm_campaign=crawler", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AhrefsBot/7.0)")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	_ = resp.Body.Close()

	var isBot bool
	err = env.store.Pool().QueryRow(context.Background(),
		`SELECT is_bot FROM visits WHERE utm_campaign = 'crawler'`).Scan(&isBot)
	if err != nil {
		t.Fatalf("the crawler visit was not recorded: %v", err)
	}
	if !isBot {
		t.Fatal("a known crawler must be flagged is_bot")
	}

	// A bot never writes events either, so the funnel cannot see it at all.
	var events int
	err = env.store.Pool().QueryRow(context.Background(), `
		SELECT count(*) FROM events e
		JOIN visits v ON v.session_id = e.session_id
		WHERE v.is_bot`).Scan(&events)
	if err != nil {
		t.Fatalf("count bot events: %v", err)
	}
	if events != 0 {
		t.Fatalf("bot sessions wrote %d events, want none", events)
	}
}

// TestFunnelEventsAreWrittenWhereTheyHappen covers the event side of S6.1: the
// server writes each funnel step itself, no client reports anything.
func TestFunnelEventsAreWrittenWhereTheyHappen(t *testing.T) {
	env := startShopEnv(t)

	_, home := get(t, env.client, env.server.URL+"/")
	token := capture(t, reCSRF, home, "csrf token")

	_, product := get(t, env.client, env.server.URL+"/product/qzq-black")
	variantID := capture(t, reVariant, product, "variant id")

	status, _ := send(t, env.client, http.MethodPost, env.server.URL+"/cart/items", env.server.URL, url.Values{
		"csrf_token": {token},
		"variant_id": {variantID},
		"qty":        {"1"},
	})
	if status != http.StatusOK {
		t.Fatalf("POST /cart/items = %d", status)
	}
	if status, _ := get(t, env.client, env.server.URL+"/checkout"); status != http.StatusOK {
		t.Fatalf("GET /checkout = %d", status)
	}

	// The seeded demo traffic shares the tables, so the walk is identified by
	// the session the test client was given.
	sessionID := sessionCookie(t, env)
	for _, want := range []string{"page_view", "product_view", "add_to_cart", "checkout_started"} {
		var n int
		err := env.store.Pool().QueryRow(context.Background(),
			`SELECT count(*) FROM events WHERE session_id = $1 AND type = $2`, sessionID, want).Scan(&n)
		if err != nil {
			t.Fatalf("count %s: %v", want, err)
		}
		if n == 0 {
			t.Errorf("no %s event was recorded for the walk", want)
		}
	}

	var slug string
	err := env.store.Pool().QueryRow(context.Background(),
		`SELECT payload->>'slug' FROM events WHERE session_id = $1 AND type = 'product_view'`,
		sessionID).Scan(&slug)
	if err != nil {
		t.Fatalf("read product_view payload: %v", err)
	}
	if slug != "qzq-black" {
		t.Fatalf("product_view payload slug = %q, want qzq-black", slug)
	}
}

// TestPaidOrderClosesTheFunnelOnce covers the last funnel step, which is
// written from the provider callback: it lands exactly once however often the
// same notification is redelivered (tech.md §11.2).
func TestPaidOrderClosesTheFunnelOnce(t *testing.T) {
	env := startShopEnv(t)
	placed := checkout(t, env, "1")

	for range 3 {
		if status, body := callback(t, env, placed.number, "finished", true); status != http.StatusOK {
			t.Fatalf("callback = %d: %s", status, body)
		}
	}

	var events int
	err := env.store.Pool().QueryRow(context.Background(), `
		SELECT count(*)
		FROM events e
		JOIN orders o ON o.visitor_id = e.visitor_id
		WHERE o.number = $1 AND e.type = 'order_paid'`, placed.number).Scan(&events)
	if err != nil {
		t.Fatalf("count order_paid events: %v", err)
	}
	if events != 1 {
		t.Fatalf("order_paid events for one purchase = %d, want 1", events)
	}
}

// sessionCookie returns the analytics session id the client was issued. The
// cookie is base64(value).base64(mac), see cookies.Signer.
func sessionCookie(t *testing.T, env *shopEnv) string {
	t.Helper()
	target, err := url.Parse(env.server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	for _, c := range env.client.Jar.Cookies(target) {
		if c.Name != middleware.CookieSession {
			continue
		}
		encoded, _, ok := strings.Cut(c.Value, ".")
		if !ok {
			t.Fatalf("sid cookie is not signed: %q", c.Value)
		}
		raw, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatalf("decode sid: %v", err)
		}
		return string(raw)
	}
	t.Fatal("the client was never given a sid cookie")
	return ""
}
