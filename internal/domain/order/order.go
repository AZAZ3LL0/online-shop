package order

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/domain/analytics"
	"github.com/qzq-kiim/shop/internal/money"
)

// ErrNotFound is returned when no order matches the given key.
var ErrNotFound = errors.New("order: not found")

// ErrNumberTaken is returned when a generated order number already exists, so
// the service can draw another one.
var ErrNumberTaken = errors.New("order: number already taken")

// ErrConflict is returned when an order moved on between the read and the
// write, so the intended transition no longer applies.
var ErrConflict = errors.New("order: status changed concurrently")

// Customer is the buyer block of an order, tech.md §4 (orders).
type Customer struct {
	Name    string
	Contact string // email or @username, exactly one field by contract
	Address string
	Comment string
}

// Item is one line of an order: a price snapshot taken at checkout time, never
// a reference to the current catalogue price.
type Item struct {
	VariantID    uuid.UUID
	ProductTitle string
	Size         string
	UnitPrice    money.Amount
	Qty          int
}

// LineTotal is the price of the whole line.
func (i Item) LineTotal() money.Amount { return i.UnitPrice.Mul(int64(i.Qty)) }

// Order is the placed order as the storefront and the admin panel see it.
type Order struct {
	ID          uuid.UUID
	Number      string
	PublicToken string
	TGLinkCode  string
	Status      Status
	Subtotal    money.Amount
	Shipping    money.Amount
	Total       money.Amount
	Customer    Customer
	Items       []Item
	ExpiresAt   *time.Time
	PaidAt      *time.Time
	CreatedAt   time.Time
	InvoiceURL  string // last invoice of this order, empty until one is created
}

// IsExpired reports whether the stock reservation deadline has passed while the
// order was still waiting for its payment.
func (o Order) IsExpired(now time.Time) bool {
	if o.ExpiresAt == nil {
		return false
	}
	if o.Status != StatusCreated && o.Status != StatusAwaitingPayment {
		return false
	}
	return now.After(*o.ExpiresAt)
}

// Draft is everything storage needs to write a new order in one transaction:
// the row, its price snapshot and the stock reservation behind it.
type Draft struct {
	Number      string
	PublicToken string
	TGLinkCode  string
	Customer    Customer
	Items       []Item
	Subtotal    money.Amount
	Shipping    money.Amount
	Total       money.Amount
	VisitorID   *uuid.UUID
	FirstTouch  analytics.Touch
	LastTouch   analytics.Touch
	ExpiresAt   time.Time
}

// PaymentRef is the provider invoice attached to an order once it exists.
type PaymentRef struct {
	Provider          string
	ProviderPaymentID string
	InvoiceURL        string
	Status            string // raw provider status at creation time
}

// Repository is the storage the order service depends on. Every method is one
// unit of work: the transaction boundary is the operation, not the statement.
type Repository interface {
	// Create writes the order, its items and the stock reservation atomically.
	// It fails with catalog.ErrOutOfStock when a line no longer fits the stock.
	Create(ctx context.Context, d Draft) (Order, error)
	// AttachPayment records the invoice and moves the order to awaiting_payment.
	AttachPayment(ctx context.Context, orderID uuid.UUID, from, to Status, p PaymentRef) error
	// ReleaseReservation gives the reserved units back without touching status.
	ReleaseReservation(ctx context.Context, orderID uuid.UUID) error
	// ByPublicToken loads the order behind a status-page token.
	ByPublicToken(ctx context.Context, token string) (Order, error)
	// ByNumber loads the order behind a human-readable number.
	ByNumber(ctx context.Context, number string) (Order, error)
	// ExpireDue expires reservations whose deadline has passed and returns how
	// many orders it moved.
	ExpireDue(ctx context.Context, now time.Time, limit int) (int, error)
}
