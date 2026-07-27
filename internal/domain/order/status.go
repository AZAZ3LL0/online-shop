// Package order owns the order aggregate and the single definition of the order
// status machine shared by payments, notifications and the admin panel.
package order

import "errors"

// ErrOrderExpired is returned when a stock reservation is no longer valid.
var ErrOrderExpired = errors.New("order: reservation expired")

// Status is the internal order status (orders.status).
type Status string

// Order statuses, tech.md §5.1.
const (
	StatusCreated         Status = "created"
	StatusAwaitingPayment Status = "awaiting_payment"
	StatusPaid            Status = "paid"
	StatusShipped         Status = "shipped"
	StatusDelivered       Status = "delivered"
	StatusExpired         Status = "expired"
	StatusCancelled       Status = "cancelled"
	StatusRefunded        Status = "refunded"
)

// transitions is the literal transcription of the diagram in tech.md §5.1.
// Nothing is added here without a CONTRACT GAP and a core version bump.
var transitions = map[Status][]Status{
	StatusCreated:         {StatusAwaitingPayment},
	StatusAwaitingPayment: {StatusPaid, StatusExpired},
	StatusPaid:            {StatusShipped, StatusRefunded},
	StatusShipped:         {StatusDelivered},
	StatusDelivered:       {},
	StatusExpired:         {StatusCancelled},
	StatusCancelled:       {},
	StatusRefunded:        {},
}

// CanTransition is the only place where a status change is judged admissible.
func CanTransition(from, to Status) bool {
	for _, allowed := range transitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// Valid reports whether s is a known status.
func Valid(s Status) bool {
	_, ok := transitions[s]
	return ok
}

// Terminal reports whether no further transition is possible from s.
func Terminal(s Status) bool {
	return len(transitions[s]) == 0
}
