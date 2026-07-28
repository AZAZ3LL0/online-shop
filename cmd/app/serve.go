package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/qzq-kiim/shop/internal/config"
	"github.com/qzq-kiim/shop/internal/domain/cart"
	"github.com/qzq-kiim/shop/internal/domain/order"
	"github.com/qzq-kiim/shop/internal/domain/payment"
	"github.com/qzq-kiim/shop/internal/httpx"
	"github.com/qzq-kiim/shop/internal/httpx/cookies"
	"github.com/qzq-kiim/shop/internal/httpx/middleware"
	"github.com/qzq-kiim/shop/internal/payments/nowpayments"
	"github.com/qzq-kiim/shop/internal/storage/postgres"
	"github.com/qzq-kiim/shop/internal/telegram"
	"github.com/qzq-kiim/shop/internal/worker"
)

// shopCurrency is the storefront currency, tech.md §15.
const shopCurrency = "USD"

// Timeouts for the HTTP server and for a clean shutdown.
const (
	readHeaderTimeout = 5 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 2 * time.Minute
	shutdownTimeout   = 15 * time.Second
	outboxInterval    = 10 * time.Second
	expiryInterval    = time.Minute
	limiterInterval   = time.Minute
)

func serve(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg.Env)

	store, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	catalogRepo := postgres.NewCatalogRepo(store)
	cartRepo := postgres.NewCartRepo(store)
	adminRepo := postgres.NewAdminRepo(store)
	analyticsRepo := postgres.NewAnalyticsRepo(store)
	notifyRepo := postgres.NewNotifyRepo(store)
	orderRepo := postgres.NewOrderRepo(store)
	paymentRepo := postgres.NewPaymentRepo(store)
	telegramRepo := postgres.NewTelegramRepo(store)

	cartService := cart.NewService(cartRepo, catalogRepo, shopCurrency, cfg.ShippingCents)
	orderService := order.NewService(orderRepo, cfg.OrderTTL)
	paymentService := payment.NewService(paymentRepo)
	signer := cookies.NewSigner(cfg.Secret, !cfg.IsDev())
	limiter := middleware.NewLimiter()

	bot := newBot(cfg, log)
	// The payment provider is built at startup so a misconfiguration fails here
	// rather than on the first checkout.
	payments := newPaymentProvider(cfg)
	log.Info("providers selected",
		slog.String("payments", cfg.PaymentsProvider),
		slog.String("telegram", cfg.TelegramProvider),
		slog.String("payments_impl", fmt.Sprintf("%T", payments)),
	)

	router := httpx.NewRouter(httpx.Deps{
		Config:    cfg,
		Log:       log,
		Signer:    signer,
		Catalog:   catalogRepo,
		Carts:     cartService,
		Orders:    orderService,
		Payments:  paymentService,
		Provider:  payments,
		Analytics: analyticsRepo,
		Admins:    adminRepo,
		Sessions:  adminRepo,
		Links:     telegramRepo,
		Bot:       bot,
		Health:    store,
		Limiter:   limiter,
	})

	server := &http.Server{
		Addr:              config.ListenAddr,
		Handler:           router,
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		worker.NewOutbox(notifyRepo, bot, log, outboxInterval).Run(ctx)
	}()
	go func() {
		defer wg.Done()
		worker.NewExpirer(orderRepo, log, expiryInterval).Run(ctx)
	}()
	go func() {
		defer wg.Done()
		housekeeping(ctx, adminRepo, limiter, log)
	}()

	errc := make(chan error, 1)
	go func() {
		log.Info("listening", slog.String("addr", config.ListenAddr), slog.String("env", cfg.Env))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- fmt.Errorf("listen: %w", err)
		}
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	wg.Wait()
	return nil
}

// housekeeping drops expired admin sessions and stale rate-limit windows.
func housekeeping(ctx context.Context, admins *postgres.AdminRepo, limiter *middleware.Limiter, log *slog.Logger) {
	ticker := time.NewTicker(limiterInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			limiter.Cleanup()
			if err := admins.DeleteExpiredSessions(ctx); err != nil && ctx.Err() == nil {
				log.Error("cleanup admin sessions failed", slog.String("error", err.Error()))
			}
		}
	}
}

func newBot(cfg config.Config, log *slog.Logger) telegram.Bot {
	if cfg.TelegramProvider == config.ProviderTelegram {
		return telegram.NewClient(cfg.Telegram.BotToken)
	}
	return telegram.NewFake(log)
}

func newPaymentProvider(cfg config.Config) nowpayments.Provider {
	if cfg.PaymentsProvider == config.ProviderNOWPayments {
		return nowpayments.NewClient(cfg.NOWPayments.APIKey, cfg.NOWPayments.IPNSecret)
	}
	return nowpayments.NewFake(cfg.Secret)
}
