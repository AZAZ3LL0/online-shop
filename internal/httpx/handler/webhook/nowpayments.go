// Package webhook holds the endpoints external systems call into. Every one of
// them verifies its signature before the body is parsed.
package webhook

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/domain/order"
	"github.com/qzq-kiim/shop/internal/domain/payment"
	"github.com/qzq-kiim/shop/internal/httpx/reqctx"
	"github.com/qzq-kiim/shop/internal/payments/nowpayments"
)

// HeaderSignature carries the HMAC-SHA512 of the callback, tech.md §5.4.
const HeaderSignature = "x-nowpayments-sig"

// maxBodyBytes caps how much of a callback is read into memory.
const maxBodyBytes = 1 << 20

// ipnTimeout bounds the work one callback may do, tech.md §16.2.
const ipnTimeout = 10 * time.Second

// Analytics is the funnel write this endpoint needs: the callback is where a
// purchase is actually completed, tech.md §5.6.
type Analytics interface {
	RecordOrderPaid(ctx context.Context, orderID uuid.UUID) error
}

// NOWPayments applies provider callbacks to orders.
type NOWPayments struct {
	provider  nowpayments.Provider
	payments  *payment.Service
	analytics Analytics
	log       *slog.Logger
}

// NewNOWPayments wires the callback endpoint.
func NewNOWPayments(provider nowpayments.Provider, payments *payment.Service, events Analytics, log *slog.Logger) *NOWPayments {
	return &NOWPayments{provider: provider, payments: payments, analytics: events, log: log}
}

// Handle verifies the signature, records the callback and moves the order. It
// answers 200 as soon as the callback is on file, so a retrying provider can
// never multiply the effect (tech.md §5.4).
func (h *NOWPayments) Handle(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := timeout(r)
	defer cancel()

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// The signature is checked before the body is parsed, tech.md §9.5.
	if err := h.provider.VerifySignature(body, r.Header.Get(HeaderSignature)); err != nil {
		if recErr := h.payments.RecordRejected(ctx, body); recErr != nil {
			h.log.Error("record rejected callback failed",
				slog.String("request_id", reqctx.RequestID(ctx)),
				slog.String("error", recErr.Error()))
		}
		h.log.Warn("callback signature rejected",
			slog.String("request_id", reqctx.RequestID(ctx)),
			slog.Bool("invalid_signature", errors.Is(err, payment.ErrInvalidSignature)))
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ipn, err := h.provider.ParseIPN(body)
	if err != nil {
		h.log.Warn("callback body rejected",
			slog.String("request_id", reqctx.RequestID(ctx)),
			slog.String("error", err.Error()))
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	result, err := h.payments.Apply(ctx, payment.Callback{
		ProviderPaymentID: ipn.PaymentID,
		ProviderStatus:    ipn.PaymentStatus,
		OrderNumber:       ipn.OrderID,
		PayAmount:         ipn.PayAmount,
		ActuallyPaid:      ipn.ActuallyPaid,
		PayCurrency:       ipn.PayCurrency,
		Raw:               body,
	})
	if err != nil {
		h.log.Error("apply callback failed",
			slog.String("request_id", reqctx.RequestID(ctx)),
			slog.String("provider_status", ipn.PaymentStatus),
			slog.String("error", err.Error()))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Only the transition that actually happened closes the funnel, so a
	// redelivered callback cannot count the same purchase twice.
	if result.Applied && result.To == order.StatusPaid {
		if err := h.analytics.RecordOrderPaid(ctx, result.OrderID); err != nil {
			h.log.Error("record order paid event failed",
				slog.String("request_id", reqctx.RequestID(ctx)),
				slog.String("error", err.Error()))
		}
	}

	// A short payment stays in awaiting_payment and is settled by hand from the
	// admin panel, tech.md §15. The raw status on the payment row is the flag.
	if ipn.PaymentStatus == payment.ProviderPartiallyPaid {
		h.log.Warn("partial payment received",
			slog.String("request_id", reqctx.RequestID(ctx)),
			slog.String("order", ipn.OrderID))
	}

	h.log.Info("callback applied",
		slog.String("request_id", reqctx.RequestID(ctx)),
		slog.String("provider_status", ipn.PaymentStatus),
		slog.Bool("duplicate", result.Duplicate),
		slog.Bool("unknown_status", result.Unknown),
		slog.Bool("order_missing", result.OrderMissing),
		slog.Bool("applied", result.Applied),
		slog.String("status", string(result.To)))

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

// timeout bounds one callback: the provider must not be able to hold a handler
// goroutine open, tech.md §16.2.
func timeout(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), ipnTimeout)
}
