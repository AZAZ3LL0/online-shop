package main

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	migrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qzq-kiim/shop/internal/config"
	"github.com/qzq-kiim/shop/internal/handler"
	"github.com/qzq-kiim/shop/internal/middleware"
	"github.com/qzq-kiim/shop/internal/repository"
	"github.com/qzq-kiim/shop/internal/service"
)

func main() {
	cfg := config.Load()

	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	m, err := migrate.New("file://migrations", cfg.DatabaseURL)
	if err != nil {
		log.Printf("migrate init: %v", err)
	} else {
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			log.Printf("migrate up: %v", err)
		}
	}

	productRepo := repository.NewProductRepo(pool)
	orderRepo := repository.NewOrderRepo(pool)
	paymentSvc := service.NewPaymentService(
		cfg.NowPaymentsKey, cfg.NowPaymentsSecret,
		cfg.SuccessURL, cfg.CancelURL, cfg.BaseURL,
	)
	orderSvc := service.NewOrderService(orderRepo, productRepo, paymentSvc)

	homeH := handler.NewHomeHandler(productRepo)
	productH := handler.NewProductHandler(productRepo)
	cartH := handler.NewCartHandler()
	orderH := handler.NewOrderHandler(orderRepo, orderSvc)
	webhookH := handler.NewWebhookHandler(orderRepo, paymentSvc)

	if err := handler.InitTemplates("templates"); err != nil {
		log.Fatalf("parse templates: %v", err)
	}

	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()
	// Trust only proxies on the local/compose network (Nginx). Prevents
	// spoofed X-Forwarded-* headers from arbitrary clients.
	if err := r.SetTrustedProxies([]string{"127.0.0.1", "::1", "172.16.0.0/12", "10.0.0.0/8", "192.168.0.0/16"}); err != nil {
		log.Fatalf("set trusted proxies: %v", err)
	}

	r.Use(middleware.SecurityHeaders(cfg.IsProduction()))
	r.Use(middleware.BodyLimit(64 * 1024))
	r.Static("/static", "./static")
	r.Use(middleware.Session(cfg.SessionSecret, cfg.CookieSecure()))
	// CSRF protects all unsafe requests except the HMAC-authenticated webhook.
	r.Use(middleware.CSRF(cfg.CookieSecure(), "/nowpayments/webhook"))

	r.GET("/", homeH.Index)
	r.GET("/product/:slug", productH.Show)
	r.GET("/product/:slug/image", productH.Image)
	r.POST("/cart/add", cartH.Add)
	r.GET("/cart", cartH.Show)
	r.POST("/order/create", orderH.Create)
	r.GET("/order/:uuid", orderH.Status)
	r.POST("/order/:uuid/pay", orderH.Pay)
	r.GET("/about", handler.AboutHandler)
	r.POST("/nowpayments/webhook", webhookH.Handle)
	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	log.Printf("starting on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
