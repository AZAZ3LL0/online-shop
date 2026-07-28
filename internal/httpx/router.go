// Package httpx mounts every route group and the cross-cutting middleware
// chain. Route patterns live here and nowhere else.
package httpx

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/qzq-kiim/shop/internal/config"
	"github.com/qzq-kiim/shop/internal/domain/analytics"
	"github.com/qzq-kiim/shop/internal/domain/cart"
	"github.com/qzq-kiim/shop/internal/domain/catalog"
	"github.com/qzq-kiim/shop/internal/domain/order"
	"github.com/qzq-kiim/shop/internal/domain/payment"
	"github.com/qzq-kiim/shop/internal/httpx/cookies"
	"github.com/qzq-kiim/shop/internal/httpx/handler/admin"
	"github.com/qzq-kiim/shop/internal/httpx/handler/dev"
	"github.com/qzq-kiim/shop/internal/httpx/handler/shop"
	"github.com/qzq-kiim/shop/internal/httpx/handler/webhook"
	"github.com/qzq-kiim/shop/internal/httpx/middleware"
	"github.com/qzq-kiim/shop/internal/payments/nowpayments"
)

// StaticDir is where the built CSS, the vendored JS and the images live.
const StaticDir = "web/static"

// Health is the liveness dependency: the database must answer.
type Health interface {
	Ping(ctx context.Context) error
}

// Deps is everything the router needs, injected by main.
type Deps struct {
	Config    config.Config
	Log       *slog.Logger
	Signer    *cookies.Signer
	Catalog   catalog.Repository
	Carts     *cart.Service
	Orders    *order.Service
	Payments  *payment.Service
	Provider  nowpayments.Provider
	Analytics analytics.Repository
	Admins    admin.Repository
	Sessions  middleware.SessionReader
	Health    Health
	Limiter   *middleware.Limiter
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
		OrderTTL:    d.Config.OrderTTL,
		IsDev:       d.Config.IsDev(),
	})
	adminHandler := admin.New(d.Admins, d.Signer, d.Log)
	adminAuth := middleware.AdminAuth(d.Sessions, d.Signer, "/admin/login")
	ipn := webhook.NewNOWPayments(d.Provider, d.Payments, d.Log)

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
	mux.HandleFunc("GET /healthz", healthz(d.Health))

	mux.HandleFunc("GET /admin/login", adminHandler.LoginForm)
	mux.HandleFunc("POST /admin/login", adminHandler.Login)
	mux.Handle("POST /admin/logout", adminAuth(http.HandlerFunc(adminHandler.Logout)))
	mux.Handle("GET /admin", adminAuth(http.HandlerFunc(adminHandler.Dashboard)))

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
		middleware.CSRF(d.Signer, d.Config.BaseURL, "/webhooks/"),
		middleware.Attribution(d.Analytics, d.Signer, d.Log, "/healthz", "/dev/", "/admin"),
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
