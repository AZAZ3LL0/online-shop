// Package payment maps provider payment states onto the internal order status
// machine. It knows nothing about HTTP or about NOWPayments transport.
package payment

import (
	"errors"

	"github.com/google/uuid"
	"github.com/qzq-kiim/shop/internal/domain/order"
)

// ErrInvalidSignature is returned when a webhook body fails its HMAC check.
var ErrInvalidSignature = errors.New("payment: invalid webhook signature")

// Payment is one provider invoice attached to an order.
type Payment struct {
	ID                uuid.UUID
	OrderID           uuid.UUID
	Provider          string
	ProviderPaymentID string
	InvoiceURL        string
	PayCurrency       string
	Status            string // raw provider status
}

// Provider raw statuses handled by the IPN endpoint, tech.md §5.4.
const (
	ProviderWaiting       = "waiting"
	ProviderConfirming    = "confirming"
	ProviderSending       = "sending"
	ProviderConfirmed     = "confirmed"
	ProviderFinished      = "finished"
	ProviderPartiallyPaid = "partially_paid"
	ProviderFailed        = "failed"
	ProviderExpired       = "expired"
	ProviderRefunded      = "refunded"
)

// MapStatus turns a raw provider status into the internal order status.
// fullyPaid says whether actually_paid covers pay_amount; it only matters for
// confirmed/finished, where a short payment must not count as paid.
func MapStatus(providerStatus string, fullyPaid bool) (order.Status, bool) {
	switch providerStatus {
	case ProviderWaiting, ProviderConfirming, ProviderSending, ProviderPartiallyPaid:
		return order.StatusAwaitingPayment, true
	case ProviderConfirmed, ProviderFinished:
		if !fullyPaid {
			return order.StatusAwaitingPayment, true
		}
		return order.StatusPaid, true
	case ProviderFailed, ProviderExpired:
		return order.StatusExpired, true
	case ProviderRefunded:
		return order.StatusRefunded, true
	default:
		return "", false
	}
}
