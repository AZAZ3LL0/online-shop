package order

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/domain/analytics"
	"github.com/qzq-kiim/shop/internal/money"
)

// numberAttempts is how often a taken order number is redrawn before the
// checkout gives up. Four random characters over one day collide rarely.
const numberAttempts = 5

// Service places orders and moves them along the status machine. Handlers hold
// no order rules of their own.
type Service struct {
	repo Repository
	ttl  time.Duration
	now  func() time.Time
}

// NewService wires the order service with the reservation lifetime.
func NewService(repo Repository, ttl time.Duration) *Service {
	return &Service{repo: repo, ttl: ttl, now: time.Now}
}

// PlaceInput is one checkout submission, already priced by the cart service.
type PlaceInput struct {
	Customer   Customer
	Items      []Item
	Subtotal   money.Amount
	Shipping   money.Amount
	Total      money.Amount
	VisitorID  *uuid.UUID
	FirstTouch analytics.Touch
	LastTouch  analytics.Touch
}

// Place validates the buyer's data, snapshots the prices and reserves the
// stock. The reservation and the order row are written in one transaction, so
// an order never exists without the stock behind it.
func (s *Service) Place(ctx context.Context, in PlaceInput) (Order, FieldErrors, error) {
	customer := Normalize(in.Customer)
	if errs := Validate(customer); errs.Any() {
		return Order{}, errs, ErrValidation
	}
	if len(in.Items) == 0 {
		return Order{}, nil, fmt.Errorf("%w: the cart is empty", ErrValidation)
	}

	now := s.now().UTC()
	for attempt := range numberAttempts {
		draft, err := s.draft(customer, in, now)
		if err != nil {
			return Order{}, nil, err
		}
		placed, err := s.repo.Create(ctx, draft)
		switch {
		case err == nil:
			return placed, nil, nil
		case errors.Is(err, ErrNumberTaken) && attempt < numberAttempts-1:
			continue
		default:
			return Order{}, nil, err
		}
	}
	return Order{}, nil, fmt.Errorf("place order: %w", ErrNumberTaken)
}

func (s *Service) draft(customer Customer, in PlaceInput, now time.Time) (Draft, error) {
	number, err := NewNumber(now)
	if err != nil {
		return Draft{}, err
	}
	token, err := NewPublicToken()
	if err != nil {
		return Draft{}, err
	}
	code, err := NewLinkCode()
	if err != nil {
		return Draft{}, err
	}
	return Draft{
		Number:      number,
		PublicToken: token,
		TGLinkCode:  code,
		Customer:    customer,
		Items:       in.Items,
		Subtotal:    in.Subtotal,
		Shipping:    in.Shipping,
		Total:       in.Total,
		VisitorID:   in.VisitorID,
		FirstTouch:  in.FirstTouch,
		LastTouch:   in.LastTouch,
		ExpiresAt:   now.Add(s.ttl),
	}, nil
}

// AwaitPayment records the created invoice and moves the order forward. The
// transition is judged here, by CanTransition, and nowhere else.
func (s *Service) AwaitPayment(ctx context.Context, o Order, p PaymentRef) error {
	if !CanTransition(o.Status, StatusAwaitingPayment) {
		return fmt.Errorf("order %s: cannot move %s to %s", o.Number, o.Status, StatusAwaitingPayment)
	}
	if err := s.repo.AttachPayment(ctx, o.ID, o.Status, StatusAwaitingPayment, p); err != nil {
		return fmt.Errorf("attach payment: %w", err)
	}
	return nil
}

// Abandon gives the stock back when the invoice could not be created. The order
// row stays for the admin panel; only the reservation is released, which is a
// stock operation and not a status transition (tech.md §4).
func (s *Service) Abandon(ctx context.Context, orderID uuid.UUID) error {
	if err := s.repo.ReleaseReservation(ctx, orderID); err != nil {
		return fmt.Errorf("release reservation: %w", err)
	}
	return nil
}

// ByPublicToken loads the order behind a status-page token.
func (s *Service) ByPublicToken(ctx context.Context, token string) (Order, error) {
	return s.repo.ByPublicToken(ctx, token)
}

// ByNumber loads the order behind a human-readable number.
func (s *Service) ByNumber(ctx context.Context, number string) (Order, error) {
	return s.repo.ByNumber(ctx, number)
}
