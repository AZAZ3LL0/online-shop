package notify

import (
	"fmt"

	"github.com/qzq-kiim/shop/internal/domain/order"
)

// statusLines is what each status means to a buyer. The map is exhaustive over
// tech.md §5.1 so a status can never reach a chat as a bare identifier.
var statusLines = map[order.Status]string{
	order.StatusCreated:         "is being prepared for payment.",
	order.StatusAwaitingPayment: "is waiting for your payment.",
	order.StatusPaid:            "is paid. We are packing it now.",
	order.StatusShipped:         "has shipped.",
	order.StatusDelivered:       "has been delivered. Enjoy it.",
	order.StatusExpired:         "expired before it was paid, and the sizes went back on the shelf.",
	order.StatusCancelled:       "was cancelled.",
	order.StatusRefunded:        "was refunded.",
}

// StatusText is the single renderer of a status message. Every outbox row and
// every bot reply goes through it, so a buyer reads one wording per status.
func StatusText(number string, status order.Status) string {
	line, ok := statusLines[status]
	if !ok {
		return fmt.Sprintf("Order %s changed status.", number)
	}
	return fmt.Sprintf("Order %s %s", number, line)
}

// LinkedText confirms that a chat now follows an order.
func LinkedText(number string, status order.Status) string {
	return "You are now tracking this order. " + StatusText(number, status)
}
