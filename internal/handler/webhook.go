package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/qzq-kiim/shop/internal/repository"
	"github.com/qzq-kiim/shop/internal/service"
)

// maxWebhookBody caps the IPN body we're willing to read (defensive).
const maxWebhookBody = 64 * 1024

type WebhookHandler struct {
	orderRepo *repository.OrderRepo
	payment   *service.PaymentService
}

func NewWebhookHandler(orderRepo *repository.OrderRepo, payment *service.PaymentService) *WebhookHandler {
	return &WebhookHandler{orderRepo: orderRepo, payment: payment}
}

func (h *WebhookHandler) Handle(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxWebhookBody))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	sig := c.GetHeader("x-nowpayments-sig")
	if !h.payment.VerifyWebhook(body, sig) {
		log.Printf("webhook: invalid signature")
		c.Status(http.StatusUnauthorized)
		return
	}

	// NowPayments IPN payload. price_amount / price_currency are the values we
	// originally requested (in KZT); actually_paid is the crypto amount settled.
	var payload struct {
		OrderID       string      `json:"order_id"`
		PaymentStatus string      `json:"payment_status"`
		PriceAmount   json.Number `json:"price_amount"`
		PriceCurrency string      `json:"price_currency"`
		ActuallyPaid  json.Number `json:"actually_paid"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	// Only fully-settled payments flip an order to paid. Any other status
	// (waiting, partially_paid, expired, refunded, failed) is acknowledged so
	// NowPayments stops retrying, but the order is left untouched.
	if payload.PaymentStatus != "finished" && payload.PaymentStatus != "confirmed" {
		log.Printf("webhook: order %s status=%s (ignored)", payload.OrderID, payload.PaymentStatus)
		c.Status(http.StatusOK)
		return
	}

	orderUUID, err := uuid.Parse(payload.OrderID)
	if err != nil {
		log.Printf("webhook: bad order_id %q", payload.OrderID)
		c.Status(http.StatusOK) // ack — retrying won't help a malformed id
		return
	}

	order, err := h.orderRepo.GetByUUID(c.Request.Context(), orderUUID)
	if err != nil {
		log.Printf("webhook: order %s not found: %v", payload.OrderID, err)
		c.Status(http.StatusOK)
		return
	}

	// Defense-in-depth amount check: the settled invoice must be for the price
	// and currency we recorded on the order. Guards against downgrade/replay of
	// a mismatched (but validly-signed) notification.
	if !amountMatches(payload.PriceCurrency, payload.PriceAmount, order.Currency, order.TotalAmount) {
		log.Printf("webhook: amount/currency mismatch for order %s (got %s %s, want %d %s) — NOT marking paid",
			payload.OrderID, payload.PriceAmount.String(), payload.PriceCurrency, order.TotalAmount, order.Currency)
		c.Status(http.StatusOK)
		return
	}

	if err := h.orderRepo.MarkPaidAndDecrementStock(c.Request.Context(), orderUUID); err != nil {
		if errors.Is(err, repository.ErrAlreadyProcessed) {
			log.Printf("webhook: order %s already processed (duplicate IPN)", payload.OrderID)
			c.Status(http.StatusOK)
			return
		}
		log.Printf("webhook: mark paid error for %s: %v", payload.OrderID, err)
		c.Status(http.StatusInternalServerError) // let NowPayments retry
		return
	}
	log.Printf("webhook: order %s marked as paid", payload.OrderID)

	c.Status(http.StatusOK)
}

// amountMatches verifies the notified price covers the order total in the same
// currency. Amounts are compared in whole currency units: order.TotalAmount is
// stored in tiyn (KZT×100).
func amountMatches(gotCurrency string, gotAmount json.Number, wantCurrency string, wantTotalMinor int) bool {
	if !strings.EqualFold(strings.TrimSpace(gotCurrency), strings.TrimSpace(wantCurrency)) {
		return false
	}
	paid, err := gotAmount.Float64()
	if err != nil {
		return false
	}
	want := float64(wantTotalMinor) / 100.0
	// Allow a tiny epsilon for float representation.
	return paid+0.01 >= want
}
