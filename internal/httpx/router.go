// Package httpx mounts every route group and the cross-cutting middleware
// chain. Route patterns live here and nowhere else.
package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/qzq-kiim/shop/internal/config"
	"github.com/qzq-kiim/shop/internal/domain/analytics"
	"github.com/qzq-kiim/shop/internal/domain/cart"
	"github.com/qzq-kiim/shop/internal/domain/catalog"
	"github.com/qzq-kiim/shop/internal/domain/order"
	"github.com/qzq-kiim/shop/internal/domain/payment"
	"github.com/qzq-kiim/shop/internal/domain/settings"
	"github.com/qzq-kiim/shop/internal/httpx/cookies"
	"github.com/qzq-kiim/shop/internal/httpx/handler/admin"
	"github.com/qzq-kiim/shop/internal/httpx/handler/dev"
	"github.com/qzq-kiim/shop/internal/httpx/handler/shop"
	"github.com/qzq-kiim/shop/internal/httpx/handler/webhook"
	"github.com/qzq-kiim/shop/internal/httpx/middleware"
	"github.com/qzq-kiim/shop/internal/payments/nowpayments"
	"github.com/qzq-kiim/shop/internal/telegram"
)

// StaticDir is where the built CSS, the vendored JS and the images live.
const StaticDir = "web/static"

// Health is the liveness dependency: the database must answer.
type Health interface {
	Ping(ctx context.Context) error
}

// Deps is everything the router needs, injected by main.
type Deps struct {
	Config       config.Config
	Log          *slog.Logger
	Signer       *cookies.Signer
	Catalog      catalog.Repository
	Carts        *cart.Service
	Orders       *order.Service
	Payments     *payment.Service
	Provider     nowpayments.Provider
	Analytics    analytics.Repository
	Metrics      analytics.MetricsRepository
	Events       webhook.Analytics
	Admins       admin.Repository
	AdminOrders  *order.AdminService
	AdminCatalog *catalog.AdminService
	Settings     *settings.Service
	PaymentLog   admin.PaymentLog
	Sessions     middleware.SessionReader
	Links        telegram.Repository
	ChatLinks    admin.ChatLinks
	Bot          telegram.Bot
	Health       Health
	Limiter      *middleware.Limiter
	Currency     string
}

// NewRouter mounts every route group behind the middleware chain of
// SKELETON.md §3.6: request_id -> recover -> logging -> securityheaders ->
// csrf -> attribution -> ratelimit.
func NewRouter(d Deps) http.Handler {
	shopHandler := shop.New(shop.Deps{
		Catalog:     d.Catalog,
		Carts:       d.Carts,
		Orders:      d.Orders,
		Payments:    d.Provider,
		Analytics:   d.Analytics,
		Cookies:     d.Signer,
		Log:         d.Log,
		BaseURL:     d.Config.BaseURL,
		BotUsername: d.Config.Telegram.BotUsername,

		Settings: d.Settings,
		IsDev:    d.Config.IsDev(),
	})
	adminHandler := admin.New(admin.Deps{
		Admins:   d.Admins,
		Metrics:  d.Metrics,
		Orders:   d.AdminOrders,
		Catalog:  d.Catalog,
		Products: d.AdminCatalog,
		Settings: d.Settings,
		Payments: d.PaymentLog,
		Links:    d.ChatLinks,
		Cookies:  d.Signer,
		Currency: d.Currency,
		Log:      d.Log,
	})
	adminAuth := middleware.AdminAuth(d.Sessions, d.Signer, "/admin/login")
	// The Mini App runs on the same handlers and the same session store; an
	// expired session bounces back to the launch page instead of a login form,
	// because inside Telegram there is no form to fill in (tech.md §8.5).
	miniAuth := middleware.AdminAuth(d.Sessions, d.Signer, admin.MiniPrefix)
	tgapp := admin.NewTelegramAuth(d.Admins, d.Signer, d.Config.Telegram.BotToken, d.Config.AdminTelegramIDs, d.Log)
	ipn := webhook.NewNOWPayments(d.Provider, d.Payments, d.Events, d.Log)
	bot := webhook.NewTelegram(
		telegram.NewRouter(d.Links, d.Bot), d.Links,
		d.Config.Telegram.WebhookSecret, d.Config.Telegram.WebhookPathSecret, d.Log)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", shopHandler.Home)
	mux.HandleFunc("GET /product/{slug}", shopHandler.Product)
	mux.HandleFunc("GET /cart", shopHandler.CartPage)
	mux.HandleFunc("POST /cart/items", shopHandler.AddItem)
	mux.HandleFunc("PATCH /cart/items/{id}", shopHandler.UpdateItem)
	mux.HandleFunc("DELETE /cart/items/{id}", shopHandler.RemoveItem)
	mux.HandleFunc("GET /checkout", shopHandler.CheckoutForm)
	mux.HandleFunc("POST /checkout", shopHandler.Checkout)
	mux.HandleFunc("GET /order/{token}", shopHandler.OrderPage)
	mux.HandleFunc("GET /order/{token}/status", shopHandler.OrderStatus)
	mux.HandleFunc("GET /payment/return/{token}", shopHandler.PaymentReturn)
	mux.HandleFunc("POST /webhooks/nowpayments", ipn.Handle)
	// The path secret is part of the address, tech.md §5.5. It is compared in
	// the handler rather than baked into the pattern so a wrong one is a 401
	// and not a 404 that would tell a prober the route exists at all.
	mux.HandleFunc("POST /webhooks/telegram/{secret}", bot.Handle)
	mux.HandleFunc("GET /healthz", healthz(d.Health))

	mux.HandleFunc("GET /admin/login", adminHandler.LoginForm)
	mux.HandleFunc("POST /admin/login", adminHandler.Login)
	mux.Handle("POST /admin/logout", adminAuth(http.HandlerFunc(adminHandler.Logout)))
	mux.Handle("GET /admin", adminAuth(http.HandlerFunc(adminHandler.Dashboard)))
	mux.Handle("GET /admin/orders", adminAuth(http.HandlerFunc(adminHandler.Orders)))
	mux.Handle("GET /admin/orders/{id}", adminAuth(http.HandlerFunc(adminHandler.Order)))
	mux.Handle("POST /admin/orders/{id}/status", adminAuth(http.HandlerFunc(adminHandler.OrderStatus)))
	mux.Handle("GET /admin/products", adminAuth(http.HandlerFunc(adminHandler.Products)))
	mux.Handle("POST /admin/products/{id}/price", adminAuth(http.HandlerFunc(adminHandler.ProductPrice)))
	mux.Handle("POST /admin/products/{id}/stock", adminAuth(http.HandlerFunc(adminHandler.ProductStock)))
	mux.Handle("GET /admin/settings", adminAuth(http.HandlerFunc(adminHandler.Settings)))
	mux.Handle("POST /admin/settings", adminAuth(http.HandlerFunc(adminHandler.SettingsSave)))
	mux.Handle("GET /admin/analytics", adminAuth(http.HandlerFunc(adminHandler.Analytics)))
	mux.Handle("GET /admin/api/metrics", adminAuth(http.HandlerFunc(adminHandler.Metrics)))

	// The Mini App, tech.md §5.3: the launch exchange plus the same admin pages
	// in the AdminMini layout.
	mux.HandleFunc("GET /tgapp", tgapp.Entry)
	mux.HandleFunc("POST /tgapp/auth", tgapp.Auth)
	mux.Handle("GET /tgapp/{$}", miniAuth(http.HandlerFunc(adminHandler.Dashboard)))
	mux.Handle("GET /tgapp/orders", miniAuth(http.HandlerFunc(adminHandler.Orders)))
	mux.Handle("GET /tgapp/orders/{id}", miniAuth(http.HandlerFunc(adminHandler.Order)))
	mux.Handle("GET /tgapp/products", miniAuth(http.HandlerFunc(adminHandler.Products)))
	mux.Handle("GET /tgapp/analytics", miniAuth(http.HandlerFunc(adminHandler.Analytics)))

	if d.Config.IsDev() {
		devHandler := dev.New(d.Log)
		mux.HandleFunc("GET /dev/kitchen-sink", devHandler.KitchenSink)
		// The fake provider sends the buyer to a local payment page, so the
		// whole checkout works without a provider key (tech.md §5.4).
		if fake, ok := d.Provider.(*nowpayments.Fake); ok {
			pay := dev.NewPayments(fake, d.Orders, http.HandlerFunc(ipn.Handle), d.Config.Telegram.BotUsername, d.Log)
			mux.HandleFunc("GET /dev/pay/{number}", pay.Page)
			mux.HandleFunc("POST /dev/pay/{number}", pay.Simulate)
		}
	}

	app := middleware.Chain(mux,
		middleware.RequestID(),
		middleware.Recover(d.Log),
		middleware.Logging(d.Log),
		middleware.SecurityHeaders(!d.Config.IsDev()),
		middleware.CSRF(d.Signer, d.Config.BaseURL, denyCSRF(shopHandler, adminHandler), "/webhooks/"),
		middleware.Attribution(d.Analytics, d.Signer, d.Log, "/healthz", "/dev/", "/admin", "/tgapp"),
		middleware.RateLimit(d.Limiter),
	)

	// Static assets skip the cookie, attribution and rate-limit layers: they
	// must not create visits and must not consume a visitor's request budget.
	static := middleware.Chain(
		http.StripPrefix("/static/", staticFileServer()),
		middleware.RequestID(),
		middleware.Recover(d.Log),
		middleware.SecurityHeaders(!d.Config.IsDev()),
	)

	root := http.NewServeMux()
	root.Handle("/static/", static)
	root.Handle("/", app)
	return root
}

// denyCSRF picks who explains a rejected request: the panel keeps its own
// chrome, everything else is a storefront page.
func denyCSRF(shopHandler *shop.Handler, adminHandler *admin.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/admin") || strings.HasPrefix(r.URL.Path, admin.MiniPrefix) {
			adminHandler.Forbidden(w, r)
			return
		}
		shopHandler.Forbidden(w, r)
	}
}

func staticFileServer() http.Handler {
	return http.FileServer(http.Dir(StaticDir))
}

func healthz(h Health) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h.Ping(r.Context()); err != nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	}
}
