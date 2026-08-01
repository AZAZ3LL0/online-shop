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

// ErrTransitionNotAllowed is returned when the panel asks for a status the
// machine of tech.md §5.1 does not allow from the current one.
var ErrTransitionNotAllowed = errors.New("order: transition not allowed")

// Paging bounds of the admin order list.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// manualTargets are the moves an administrator may make by hand. The rest of the
// machine belongs to a provider callback (paid, refunded) or to the reservation
// worker (expired), tech.md §5.1.
var manualTargets = []Status{StatusShipped, StatusDelivered, StatusCancelled}

// ManualTargets lists the statuses the panel may offer for an order in from.
// The judge is still CanTransition; this only narrows it to what a human owns.
func ManualTargets(from Status) []Status {
	allowed := make([]Status, 0, len(manualTargets))
	for _, to := range manualTargets {
		if CanTransition(from, to) {
			allowed = append(allowed, to)
		}
	}
	return allowed
}

// Filter narrows the admin order list. A zero value asks for everything.
type Filter struct {
	Status    Status // empty matches any status
	From      *time.Time
	To        *time.Time
	ProductID *uuid.UUID
	Number    string // substring of the human-readable number
	Page      int    // 1-based
	PageSize  int
}

// Offset is where the requested page starts.
func (f Filter) Offset() int { return (f.Page - 1) * f.PageSize }

// Summary is one row of the admin order list.
type Summary struct {
	ID              uuid.UUID
	Number          string
	Status          Status
	Total           money.Amount
	CustomerName    string
	CustomerContact string
	Units           int
	// PartiallyPaid marks an order the provider was short-paid for.
	PartiallyPaid bool
	CreatedAt     time.Time
	PaidAt        *time.Time
}

// List is one page of the admin order list.
type List struct {
	Orders   []Summary
	Total    int
	Page     int
	PageSize int
}

// TotalPages is how many pages the current filter yields, at least one.
func (l List) TotalPages() int {
	if l.PageSize <= 0 || l.Total <= 0 {
		return 1
	}
	return (l.Total + l.PageSize - 1) / l.PageSize
}

// Detail is one order as the admin card shows it: the order itself plus the
// fields only the panel reads.
type Detail struct {
	Order       Order
	FirstTouch  analytics.Touch
	LastTouch   analytics.Touch
	ShippedAt   *time.Time
	CancelledAt *time.Time
}

// ManualTargets lists the moves the panel may offer for this order.
func (d Detail) ManualTargets() []Status { return ManualTargets(d.Order.Status) }

// AdminRepository is the storage the admin panel needs on top of Repository.
type AdminRepository interface {
	// ListForAdmin returns one filtered page of orders and the total behind it.
	ListForAdmin(ctx context.Context, f Filter) (List, error)
	// DetailByID loads one order with the fields only the panel shows.
	DetailByID(ctx context.Context, id uuid.UUID) (Detail, error)
	// Transition moves an order under its own lock and queues the message that
	// announces the new status, both in one transaction.
	Transition(ctx context.Context, id uuid.UUID, from, to Status) error
}

// AdminService is the order side of the admin panel. Handlers ask it for pages
// and for transitions; they never judge a status change themselves.
type AdminService struct {
	repo AdminRepository
}

// NewAdminService wires the admin order service.
func NewAdminService(repo AdminRepository) *AdminService { return &AdminService{repo: repo} }

// List returns one page of orders, with the paging bounds applied.
func (s *AdminService) List(ctx context.Context, f Filter) (List, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	switch {
	case f.PageSize <= 0:
		f.PageSize = DefaultPageSize
	case f.PageSize > MaxPageSize:
		f.PageSize = MaxPageSize
	}
	list, err := s.repo.ListForAdmin(ctx, f)
	if err != nil {
		return List{}, fmt.Errorf("list orders: %w", err)
	}
	return list, nil
}

// Detail loads one order for the card.
func (s *AdminService) Detail(ctx context.Context, id uuid.UUID) (Detail, error) {
	return s.repo.DetailByID(ctx, id)
}

// ChangeStatus applies a manual transition. The move has to be one a human owns
// and one CanTransition allows; everything else leaves the order untouched.
func (s *AdminService) ChangeStatus(ctx context.Context, id uuid.UUID, to Status) (Detail, error) {
	current, err := s.repo.DetailByID(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	from := current.Order.Status
	if !allowedManually(from, to) {
		return current, fmt.Errorf("order %s: %s to %s: %w", current.Order.Number, from, to, ErrTransitionNotAllowed)
	}
	if err := s.repo.Transition(ctx, id, from, to); err != nil {
		return current, err
	}
	return s.repo.DetailByID(ctx, id)
}

func allowedManually(from, to Status) bool {
	for _, candidate := range ManualTargets(from) {
		if candidate == to {
			return true
		}
	}
	return false
}
