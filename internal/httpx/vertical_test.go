package httpx_test

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/qzq-kiim/shop/internal/config"
	"github.com/qzq-kiim/shop/internal/domain/cart"
	"github.com/qzq-kiim/shop/internal/domain/catalog"
	"github.com/qzq-kiim/shop/internal/domain/order"
	"github.com/qzq-kiim/shop/internal/domain/payment"
	"github.com/qzq-kiim/shop/internal/httpx"
	"github.com/qzq-kiim/shop/internal/httpx/cookies"
	"github.com/qzq-kiim/shop/internal/httpx/middleware"
	"github.com/qzq-kiim/shop/internal/payments/nowpayments"
	"github.com/qzq-kiim/shop/internal/storage/postgres"
	"github.com/qzq-kiim/shop/internal/telegram"
	"github.com/qzq-kiim/shop/migrations"
)

// orderTTL is the reservation lifetime the tests run with.
const orderTTL = 30 * time.Minute

// The bot the tests run against. These are not credentials: the fake bot never
// talks to Telegram, and the two secrets only have to match themselves so the
// webhook's constant-time checks are exercised (tech.md §9.4).
const (
	botUsername       = "qzq_test_bot"
	webhookSecret     = "test-webhook-secret"
	webhookPathSecret = "test-path-secret"
)

var (
	reCSRF    = regexp.MustCompile(`name="csrf-token" content="([0-9a-f]+)"`)
	reVariant = regexp.MustCompile(`name="variant_id"[\s\S]*?<option value="([0-9a-f-]{36})"`)
	reItem    = regexp.MustCompile(`action="/cart/items/([0-9a-f-]{36})"`)
)

// shopEnv is one running shop: a real Postgres, the real router and the fake
// payment provider behind it. Slices that need more than HTTP - stock counts,
// the expiry worker - reach for the store through it.
type shopEnv struct {
	server *httptest.Server
	client *http.Client
	store  *postgres.Store
	orders *postgres.OrderRepo
	notify *postgres.NotifyRepo
	links  *postgres.TelegramRepo
	fake   *nowpayments.Fake
	bot    *telegram.Fake
}

// startShop keeps the signature the catalogue slices use.
func startShop(t *testing.T) (*httptest.Server, *http.Client) {
	t.Helper()
	env := startShopEnv(t)
	return env.server, env.client
}

// startShopEnv brings up a real Postgres, applies the migrations, seeds the
// demo data and serves the router. Nothing here is mocked: this is the
// reference vertical the later slices copy.
func startShopEnv(t *testing.T) *shopEnv {
	t.Helper()

	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("shop"),
		tcpostgres.WithUsername("app"),
		tcpostgres.WithPassword("app"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		t.Skipf("postgres container unavailable: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(container) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	applyMigrations(t, dsn)

	store, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(store.Close)

	if err := postgres.NewSeeder(store).Run(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// The listener is opened first so the CSRF origin check can be configured
	// with the address the test client will actually use.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	baseURL := "http://" + listener.Addr().String()

	cfg := config.Config{
		Env:              config.EnvDev,
		BaseURL:          baseURL,
		Secret:           []byte("0123456789abcdef0123456789abcdef"),
		PaymentsProvider: config.ProviderFake,
		TelegramProvider: config.ProviderFake,
		Telegram: config.Telegram{
			BotUsername:       botUsername,
			WebhookSecret:     webhookSecret,
			WebhookPathSecret: webhookPathSecret,
		},
		OrderTTL: orderTTL,
	}
	catalogRepo := postgres.NewCatalogRepo(store)
	cartRepo := postgres.NewCartRepo(store)
	adminRepo := postgres.NewAdminRepo(store)
	orderRepo := postgres.NewOrderRepo(store)
	paymentRepo := postgres.NewPaymentRepo(store)
	telegramRepo := postgres.NewTelegramRepo(store)
	fake := nowpayments.NewFake(cfg.Secret)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bot := telegram.NewFake(log)

	router := httpx.NewRouter(httpx.Deps{
		Config:       cfg,
		Log:          log,
		Signer:       cookies.NewSigner(cfg.Secret, false),
		Catalog:      catalogRepo,
		Carts:        cart.NewService(cartRepo, catalogRepo, "USD", 0),
		Orders:       order.NewService(orderRepo, cfg.OrderTTL),
		Payments:     payment.NewService(paymentRepo),
		Provider:     fake,
		Analytics:    postgres.NewAnalyticsRepo(store),
		Admins:       adminRepo,
		AdminOrders:  order.NewAdminService(orderRepo),
		AdminCatalog: catalog.NewAdminService(postgres.NewCatalogAdminRepo(store), "USD"),
		Currency:     "USD",
		PaymentLog:   paymentRepo,
		Sessions:     adminRepo,
		Links:        telegramRepo,
		ChatLinks:    telegramRepo,
		Bot:          bot,
		Health:       store,
		Limiter:      middleware.NewLimiter(),
	})

	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: router, ReadHeaderTimeout: 5 * time.Second},
	}
	server.Start()
	t.Cleanup(server.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &shopEnv{
		server: server,
		links:  telegramRepo,
		client: &http.Client{Jar: jar, Timeout: 20 * time.Second},
		store:  store,
		orders: orderRepo,
		notify: postgres.NewNotifyRepo(store),
		fake:   fake,
		bot:    bot,
	}
}

func applyMigrations(t *testing.T, dsn string) {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("goose dialect: %v", err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("goose up: %v", err)
	}
}

func get(t *testing.T, client *http.Client, url string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("User-Agent", "e2e-test-browser")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, string(body)
}

func send(t *testing.T, client *http.Client, method, target, origin string, form url.Values) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, target, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", origin)
	req.Header.Set("User-Agent", "e2e-test-browser")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, string(body)
}

func capture(t *testing.T, re *regexp.Regexp, body, what string) string {
	t.Helper()
	m := re.FindStringSubmatch(body)
	if len(m) < 2 {
		t.Fatalf("could not find %s in the response", what)
	}
	return m[1]
}

// TestReferenceVertical walks the whole path the later slices copy:
// home -> add to cart -> cart fragment, through the real router, the real
// middleware chain and a real database.
func TestReferenceVertical(t *testing.T) {
	server, client := startShop(t)

	status, home := get(t, client, server.URL+"/")
	if status != http.StatusOK {
		t.Fatalf("GET / = %d", status)
	}
	for _, title := range []string{"QZQ Black", "QZQ White", "Besh"} {
		if !strings.Contains(home, title) {
			t.Fatalf("home page does not show %q", title)
		}
	}
	if !strings.Contains(home, "Your cart is empty") {
		t.Fatal("home page does not show the empty cart panel")
	}

	token := capture(t, reCSRF, home, "csrf token")
	variantID := capture(t, reVariant, home, "variant id")

	status, fragment := send(t, client, http.MethodPost, server.URL+"/cart/items", server.URL, url.Values{
		"csrf_token": {token},
		"variant_id": {variantID},
		"qty":        {"2"},
	})
	if status != http.StatusOK {
		t.Fatalf("POST /cart/items = %d, body: %s", status, fragment)
	}
	if strings.Contains(fragment, "<html") {
		t.Fatal("cart mutation must answer with a fragment, not a full page")
	}
	if !strings.Contains(fragment, "QZQ Black") {
		t.Fatalf("cart fragment does not list the added item: %s", fragment)
	}

	status, cartPage := get(t, client, server.URL+"/cart")
	if status != http.StatusOK {
		t.Fatalf("GET /cart = %d", status)
	}
	if !strings.Contains(cartPage, "QZQ Black") {
		t.Fatal("cart page does not list the added item")
	}
	// Two units at $35.00 each.
	if !strings.Contains(cartPage, "$70.00") {
		t.Fatalf("cart page does not show the line total: %s", cartPage)
	}

	itemID := capture(t, reItem, cartPage, "cart item id")

	status, _ = send(t, client, http.MethodPatch, server.URL+"/cart/items/"+itemID, server.URL, url.Values{
		"csrf_token": {token},
		"qty":        {"3"},
	})
	if status != http.StatusOK {
		t.Fatalf("PATCH qty=3 = %d", status)
	}

	status, fragment = send(t, client, http.MethodPatch, server.URL+"/cart/items/"+itemID, server.URL, url.Values{
		"csrf_token": {token},
		"qty":        {"11"},
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("PATCH qty=11 = %d, want 422", status)
	}
	if !strings.Contains(fragment, "between 1 and 10") {
		t.Fatalf("out of range quantity must be explained: %s", fragment)
	}

	// The rejected update must not have changed anything.
	_, cartPage = get(t, client, server.URL+"/cart")
	if !strings.Contains(cartPage, `value="3"`) {
		t.Fatal("rejected update must leave the quantity at 3")
	}

	status, fragment = send(t, client, http.MethodDelete, server.URL+"/cart/items/"+itemID, server.URL, url.Values{
		"csrf_token": {token},
	})
	if status != http.StatusOK {
		t.Fatalf("DELETE = %d", status)
	}
	if !strings.Contains(fragment, "Your cart is empty") {
		t.Fatal("cart must be empty after the line is removed")
	}
}

func TestCSRFAndOriginAreEnforced(t *testing.T) {
	server, client := startShop(t)
	_, home := get(t, client, server.URL+"/")
	token := capture(t, reCSRF, home, "csrf token")
	variantID := capture(t, reVariant, home, "variant id")

	status, _ := send(t, client, http.MethodPost, server.URL+"/cart/items", "http://evil.example", url.Values{
		"csrf_token": {token},
		"variant_id": {variantID},
		"qty":        {"1"},
	})
	if status != http.StatusForbidden {
		t.Fatalf("cross-origin post = %d, want 403", status)
	}

	status, _ = send(t, client, http.MethodPost, server.URL+"/cart/items", server.URL, url.Values{
		"csrf_token": {"deadbeef"},
		"variant_id": {variantID},
		"qty":        {"1"},
	})
	if status != http.StatusForbidden {
		t.Fatalf("wrong csrf token = %d, want 403", status)
	}
}

func TestAdminIsClosedWithoutSession(t *testing.T) {
	server, _ := startShop(t)
	client := &http.Client{
		Timeout:       20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	status, _ := get(t, client, server.URL+"/admin")
	if status != http.StatusSeeOther {
		t.Fatalf("GET /admin without a session = %d, want 303 to the login form", status)
	}
}

func TestHealthzAndSecurityHeaders(t *testing.T) {
	server, client := startShop(t)

	status, body := get(t, client, server.URL+"/healthz")
	if status != http.StatusOK || body != "ok" {
		t.Fatalf("GET /healthz = %d %q", status, body)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	} {
		if got := resp.Header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") {
		t.Errorf("CSP does not restrict scripts to self: %q", csp)
	}
	if resp.Header.Get(middleware.HeaderRequestID) == "" {
		t.Error("no request id on the response")
	}
}
