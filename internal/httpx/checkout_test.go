package httpx_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qzq-kiim/shop/internal/payments/nowpayments"
)

var (
	reInvoice    = regexp.MustCompile(`/dev/pay/(ORD-[0-9A-Z-]+)`)
	reOrderToken = regexp.MustCompile(`/order/([0-9a-f]{32})`)
)

// placed is one order as the tests address it afterwards.
type placed struct {
	number    string
	token     string
	variantID string
}

// checkout walks the buyer path up to the invoice redirect and returns the
// order behind it. Everything the later assertions need is read back out of
// the pages, never out of the handlers.
func checkout(t *testing.T, env *shopEnv, qty string) placed {
	t.Helper()

	_, home := get(t, env.client, env.server.URL+"/")
	token := capture(t, reCSRF, home, "csrf token")
	variantID := capture(t, reVariant, home, "variant id")

	status, body := send(t, env.client, http.MethodPost, env.server.URL+"/cart/items", env.server.URL, url.Values{
		"csrf_token": {token},
		"variant_id": {variantID},
		"qty":        {qty},
	})
	if status != http.StatusOK {
		t.Fatalf("POST /cart/items = %d: %s", status, body)
	}

	status, location, body := sendNoRedirect(t, env, http.MethodPost, "/checkout", checkoutForm(token))
	if status != http.StatusSeeOther {
		t.Fatalf("POST /checkout = %d, want 303: %s", status, body)
	}
	number := capture(t, reInvoice, location, "order number in the invoice url")

	_, payPage := get(t, env.client, env.server.URL+location)
	return placed{
		number:    number,
		token:     capture(t, reOrderToken, payPage, "public token"),
		variantID: variantID,
	}
}

func checkoutForm(csrfToken string) url.Values {
	return url.Values{
		"csrf_token": {csrfToken},
		"name":       {"Samat Sadriev"},
		"contact":    {"buyer@example.com"},
		"address":    {"Almaty, Abay 10, apt 4"},
		"comment":    {"leave at the door"},
		"consent":    {"on"},
	}
}

// sendNoRedirect posts a form once and hands back the redirect instead of
// following it.
func sendNoRedirect(t *testing.T, env *shopEnv, method, path string, form url.Values) (int, string, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, env.server.URL+path, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", env.server.URL)
	req.Header.Set("User-Agent", "e2e-test-browser")

	client := &http.Client{
		Jar:           env.client.Jar,
		Timeout:       20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, resp.Header.Get("Location"), string(body)
}

// variantStock reads the stock columns straight from the database: the shop
// never publishes them, and the reservation invariant is about the row.
func variantStock(t *testing.T, env *shopEnv, variantID string) (int, int) {
	t.Helper()
	var stock, reserved int
	err := env.store.Pool().QueryRow(context.Background(),
		`SELECT stock, reserved FROM product_variants WHERE id = $1`, variantID).Scan(&stock, &reserved)
	if err != nil {
		t.Fatalf("read variant stock: %v", err)
	}
	return stock, reserved
}

func orderStatusOf(t *testing.T, env *shopEnv, number string) string {
	t.Helper()
	var status string
	err := env.store.Pool().QueryRow(context.Background(),
		`SELECT status FROM orders WHERE number = $1`, number).Scan(&status)
	if err != nil {
		t.Fatalf("read order status: %v", err)
	}
	return status
}

// TestCheckoutPlacesTheOrderAndReservesTheStock is the acceptance criteria of
// S3.1 and S3.2: a price snapshot, a reservation, a deadline and a redirect to
// the invoice.
func TestCheckoutPlacesTheOrderAndReservesTheStock(t *testing.T) {
	env := startShopEnv(t)

	before, reservedBefore := variantStock(t, env, firstVariant(t, env))
	order := checkout(t, env, "2")

	stock, reserved := variantStock(t, env, order.variantID)
	if stock != before {
		t.Errorf("stock changed on checkout: %d, want %d", stock, before)
	}
	if reserved != reservedBefore+2 {
		t.Errorf("reserved = %d, want %d", reserved, reservedBefore+2)
	}
	if got := orderStatusOf(t, env, order.number); got != "awaiting_payment" {
		t.Errorf("order status = %q, want awaiting_payment", got)
	}

	var (
		unitPrice int64
		qty       int
	)
	err := env.store.Pool().QueryRow(context.Background(),
		`SELECT oi.unit_price_cents, oi.qty FROM order_items oi
		 JOIN orders o ON o.id = oi.order_id WHERE o.number = $1`, order.number).Scan(&unitPrice, &qty)
	if err != nil {
		t.Fatalf("read order item: %v", err)
	}
	if unitPrice != 3500 || qty != 2 {
		t.Errorf("price snapshot = %d cents x %d, want 3500 x 2", unitPrice, qty)
	}

	var expiresAt, createdAt time.Time
	err = env.store.Pool().QueryRow(context.Background(),
		`SELECT expires_at, created_at FROM orders WHERE number = $1`, order.number).Scan(&expiresAt, &createdAt)
	if err != nil {
		t.Fatalf("read order deadline: %v", err)
	}
	if held := expiresAt.Sub(createdAt); held < 29*time.Minute || held > 31*time.Minute {
		t.Errorf("reservation held for %s, want the configured 30 minutes", held)
	}
}

// The catalogue price may move afterwards; the order keeps what it was placed
// at, tech.md §8.3.
func TestOrderKeepsItsPriceWhenTheCatalogueMoves(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "1")

	_, err := env.store.Pool().Exec(context.Background(),
		`UPDATE products SET price_cents = 9900`)
	if err != nil {
		t.Fatalf("change price: %v", err)
	}

	var unitPrice int64
	err = env.store.Pool().QueryRow(context.Background(),
		`SELECT oi.unit_price_cents FROM order_items oi
		 JOIN orders o ON o.id = oi.order_id WHERE o.number = $1`, order.number).Scan(&unitPrice)
	if err != nil {
		t.Fatalf("read order item: %v", err)
	}
	if unitPrice != 3500 {
		t.Errorf("snapshot price = %d, want 3500", unitPrice)
	}
}

func TestCheckoutRejectsAnIncompleteForm(t *testing.T) {
	env := startShopEnv(t)
	_, home := get(t, env.client, env.server.URL+"/")
	csrfToken := capture(t, reCSRF, home, "csrf token")
	variantID := capture(t, reVariant, home, "variant id")

	send(t, env.client, http.MethodPost, env.server.URL+"/cart/items", env.server.URL, url.Values{
		"csrf_token": {csrfToken},
		"variant_id": {variantID},
		"qty":        {"1"},
	})

	form := checkoutForm(csrfToken)
	form.Set("contact", "not-a-contact")
	status, body := send(t, env.client, http.MethodPost, env.server.URL+"/checkout", env.server.URL, form)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("invalid contact = %d, want 422", status)
	}
	if !strings.Contains(body, "Enter an e-mail or a Telegram @username.") {
		t.Errorf("the contact error is not shown: %s", body)
	}

	form = checkoutForm(csrfToken)
	form.Del("consent")
	status, body = send(t, env.client, http.MethodPost, env.server.URL+"/checkout", env.server.URL, form)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("missing consent = %d, want 422", status)
	}
	if !strings.Contains(body, "Tick the box to place the order.") {
		t.Errorf("the consent error is not shown: %s", body)
	}

	if _, reserved := variantStock(t, env, variantID); reserved != 0 {
		t.Errorf("a rejected form must not reserve stock, reserved = %d", reserved)
	}
}

// The stock can run out between the cart and the form submission.
func TestCheckoutRefusesWhenTheStockRanOut(t *testing.T) {
	env := startShopEnv(t)
	_, home := get(t, env.client, env.server.URL+"/")
	csrfToken := capture(t, reCSRF, home, "csrf token")
	variantID := capture(t, reVariant, home, "variant id")

	send(t, env.client, http.MethodPost, env.server.URL+"/cart/items", env.server.URL, url.Values{
		"csrf_token": {csrfToken},
		"variant_id": {variantID},
		"qty":        {"2"},
	})
	_, err := env.store.Pool().Exec(context.Background(),
		`UPDATE product_variants SET stock = 1 WHERE id = $1`, variantID)
	if err != nil {
		t.Fatalf("shrink stock: %v", err)
	}

	status, body := send(t, env.client, http.MethodPost, env.server.URL+"/checkout", env.server.URL, checkoutForm(csrfToken))
	if status != http.StatusConflict {
		t.Fatalf("checkout over the stock = %d, want 409: %s", status, body)
	}
	stock, reserved := variantStock(t, env, variantID)
	if stock-reserved < 0 {
		t.Errorf("stock invariant broken: stock %d, reserved %d", stock, reserved)
	}
	if reserved != 0 {
		t.Errorf("a refused checkout must hold nothing, reserved = %d", reserved)
	}
}

// When the provider does not answer, the buyer sees no invoice, so the units
// must go back on the shelf.
func TestCheckoutReleasesTheStockWhenTheProviderFails(t *testing.T) {
	env := startShopEnv(t)
	_, home := get(t, env.client, env.server.URL+"/")
	csrfToken := capture(t, reCSRF, home, "csrf token")
	variantID := capture(t, reVariant, home, "variant id")

	send(t, env.client, http.MethodPost, env.server.URL+"/cart/items", env.server.URL, url.Values{
		"csrf_token": {csrfToken},
		"variant_id": {variantID},
		"qty":        {"2"},
	})
	env.fake.FailNext(nowpayments.ErrFakeFailure)

	status, body := send(t, env.client, http.MethodPost, env.server.URL+"/checkout", env.server.URL, checkoutForm(csrfToken))
	if status != http.StatusServiceUnavailable {
		t.Fatalf("provider failure = %d, want 503: %s", status, body)
	}
	if _, reserved := variantStock(t, env, variantID); reserved != 0 {
		t.Errorf("reserved = %d, want the units released", reserved)
	}
}

// S3.4: the status page and its poll.
func TestOrderStatusPageAndPolling(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "1")

	status, page := get(t, env.client, env.server.URL+"/order/"+order.token)
	if status != http.StatusOK {
		t.Fatalf("GET /order/{token} = %d", status)
	}
	if !strings.Contains(page, order.number) {
		t.Error("the status page does not show the order number")
	}
	if !strings.Contains(page, "awaiting_payment") {
		t.Errorf("the status page does not show the status: %s", page)
	}
	if !strings.Contains(page, "/order/"+order.token+"/status") {
		t.Error("the status page does not poll its own status endpoint")
	}

	status, raw := get(t, env.client, env.server.URL+"/order/"+order.token+"/status")
	if status != http.StatusOK {
		t.Fatalf("GET status = %d", status)
	}
	var body struct {
		Status    string `json:"status"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if body.Status != "awaiting_payment" {
		t.Errorf("polled status = %q", body.Status)
	}
	if _, err := time.Parse(time.RFC3339, body.ExpiresAt); err != nil {
		t.Errorf("expires_at = %q, want an RFC3339 deadline", body.ExpiresAt)
	}
}

// An unknown token gets the ordinary not-found page: the shop does not confirm
// to a guesser whether an order exists.
func TestOrderPageHidesUnknownTokens(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "1")

	status, body := get(t, env.client, env.server.URL+"/order/"+strings.Repeat("0", 32))
	if status != http.StatusNotFound {
		t.Fatalf("unknown token = %d, want 404", status)
	}
	if strings.Contains(body, order.number) || strings.Contains(body, "expired") {
		t.Errorf("the not-found page leaks order details: %s", body)
	}
}

// The provider return url only leads back to the status page: what the order is
// worth is decided by the callback, never by the browser coming back.
func TestPaymentReturnRedirectsToTheOrder(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "1")

	client := &http.Client{
		Jar:           env.client.Jar,
		Timeout:       20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	status, _ := get(t, client, env.server.URL+"/payment/return/"+order.token)
	if status != http.StatusSeeOther {
		t.Fatalf("payment return = %d, want 303", status)
	}
	if got := orderStatusOf(t, env, order.number); got != "awaiting_payment" {
		t.Errorf("the return must not change the status, got %q", got)
	}
}

func firstVariant(t *testing.T, env *shopEnv) string {
	t.Helper()
	_, home := get(t, env.client, env.server.URL+"/")
	return capture(t, reVariant, home, "variant id")
}

// Several buyers can reach the last units at the same time. Whatever the
// interleaving, the shop must not sell more than it has: the reservation is
// taken under a row lock, so the losers get a refusal and not a phantom order
// (TASKS.md S3.1, stock invariant under concurrent reservations).
func TestConcurrentCheckoutsNeverOversell(t *testing.T) {
	env := startShopEnv(t)
	variantID := firstVariant(t, env)

	const available = 3
	_, err := env.store.Pool().Exec(context.Background(),
		`UPDATE product_variants SET stock = $2, reserved = 0 WHERE id = $1`, variantID, available)
	if err != nil {
		t.Fatalf("set stock: %v", err)
	}

	const buyers = 8
	type attempt struct {
		env    *shopEnv
		client *http.Client
		token  string
	}
	attempts := make([]attempt, 0, buyers)
	for range buyers {
		// Every buyer gets their own cookie jar, so they are separate carts.
		jar, err := cookiejar.New(nil)
		if err != nil {
			t.Fatalf("cookie jar: %v", err)
		}
		buyer := &shopEnv{
			server: env.server,
			client: &http.Client{Jar: jar, Timeout: 20 * time.Second},
			store:  env.store,
			orders: env.orders,
			fake:   env.fake,
		}
		_, home := get(t, buyer.client, buyer.server.URL+"/")
		csrfToken := capture(t, reCSRF, home, "csrf token")
		if status, body := send(t, buyer.client, http.MethodPost, buyer.server.URL+"/cart/items",
			buyer.server.URL, url.Values{
				"csrf_token": {csrfToken},
				"variant_id": {variantID},
				"qty":        {"1"},
			}); status != http.StatusOK {
			t.Fatalf("POST /cart/items = %d: %s", status, body)
		}
		attempts = append(attempts, attempt{env: buyer, client: buyer.client, token: csrfToken})
	}

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		succeeded int
		refused   int
	)
	for _, a := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, _, _ := sendNoRedirect(t, a.env, http.MethodPost, "/checkout", checkoutForm(a.token))
			mu.Lock()
			defer mu.Unlock()
			switch status {
			case http.StatusSeeOther:
				succeeded++
			case http.StatusConflict:
				refused++
			default:
				t.Errorf("concurrent checkout = %d, want 303 or 409", status)
			}
		}()
	}
	wg.Wait()

	if succeeded != available {
		t.Errorf("%d checkouts went through, want exactly the %d available units", succeeded, available)
	}
	if succeeded+refused != buyers {
		t.Errorf("%d of %d checkouts got a definite answer", succeeded+refused, buyers)
	}
	stock, reserved := variantStock(t, env, variantID)
	if reserved != available || stock-reserved != 0 {
		t.Errorf("stock/reserved = %d/%d, want %d/%d", stock, reserved, available, available)
	}
	assertStockInvariant(t, env)
}
