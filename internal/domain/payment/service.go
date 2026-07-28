package payment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/domain/order"
)

// ProviderNOWPayments is the only payment provider of the shop, tech.md §2.
const ProviderNOWPayments = "nowpayments"

// statusInvalidSignature is what a rejected callback is filed under, so a
// forged body still leaves a trace without ever being parsed as a real status.
const statusInvalidSignature = "invalid_signature"

// Callback is one provider notification, already verified and parsed by the
// transport layer. Decimal amounts stay text: money never becomes a float.
type Callback struct {
	ProviderPaymentID string
	ProviderStatus    string
	OrderNumber       string
	PayAmount         string
	ActuallyPaid      string
	PayCurrency       string
	Raw               []byte
}

// Event is one row of the raw callback log, tech.md §4 (payment_events).
type Event struct {
	ProviderStatus string
	Payload        []byte
	SignatureOK    bool
	DedupKey       string
}

// Command is one callback asking for a state change. Event, payment and order
// are written in a single transaction, so a callback that fails halfway can be
// redelivered instead of being silently deduplicated away.
type Command struct {
	Event             Event
	OrderNumber       string
	ProviderPaymentID string
	ProviderStatus    string
	PayCurrency       string
	PayAmount         string
	ActuallyPaid      string
	// Decide is applied to the current status under the order lock. It is the
	// only judge of whether the callback changes anything.
	Decide func(current order.Status) (order.Status, bool)
}

// Transition is the status change a command produced.
type Transition struct {
	OrderID uuid.UUID
	From    order.Status
	To      order.Status
	Applied bool
}

// Outcome is what storage made of one command.
type Outcome struct {
	Duplicate    bool // the provider redelivered a callback already on file
	OrderMissing bool // the callback names an order this shop does not have
	Transition
}

// Repository is the storage the payment service depends on.
type Repository interface {
	// Apply records the callback, upserts the payment and moves the order with
	// its stock, all in one transaction.
	Apply(ctx context.Context, cmd Command) (Outcome, error)
	// RecordRejected files a callback that failed its signature check.
	RecordRejected(ctx context.Context, ev Event) error
}

// Result reports what one callback did, for the log and for the tests.
type Result struct {
	Unknown bool // the provider sent a status this shop does not map
	Outcome
}

// Service applies provider callbacks to orders. It is idempotent by
// construction: the dedup key stops redelivery, and the status machine stops
// anything that arrives out of order.
type Service struct {
	repo Repository
}

// NewService wires the payment service.
func NewService(repo Repository) *Service { return &Service{repo: repo} }

// Apply records the callback and moves the order when the callback asks for a
// status the machine allows.
func (s *Service) Apply(ctx context.Context, cb Callback) (Result, error) {
	target, known := MapStatus(cb.ProviderStatus, FullyPaid(cb.PayAmount, cb.ActuallyPaid))

	out, err := s.repo.Apply(ctx, Command{
		Event: Event{
			ProviderStatus: cb.ProviderStatus,
			Payload:        cb.Raw,
			SignatureOK:    true,
			DedupKey:       DedupKey(cb),
		},
		OrderNumber:       cb.OrderNumber,
		ProviderPaymentID: cb.ProviderPaymentID,
		ProviderStatus:    cb.ProviderStatus,
		PayCurrency:       cb.PayCurrency,
		PayAmount:         cb.PayAmount,
		ActuallyPaid:      cb.ActuallyPaid,
		Decide:            decide(target, known),
	})
	if err != nil {
		return Result{}, fmt.Errorf("apply callback: %w", err)
	}
	return Result{Unknown: !known, Outcome: out}, nil
}

// RecordRejected files a callback whose signature did not verify. The body is
// never parsed and no state is touched, tech.md §5.4 rule 1.
func (s *Service) RecordRejected(ctx context.Context, rawBody []byte) error {
	err := s.repo.RecordRejected(ctx, Event{
		ProviderStatus: statusInvalidSignature,
		Payload:        rawBody,
		SignatureOK:    false,
		DedupKey:       statusInvalidSignature + "|" + digest(rawBody),
	})
	if err != nil {
		return fmt.Errorf("record rejected callback: %w", err)
	}
	return nil
}

// decide turns a target status into the rule storage applies under the order
// lock: only a transition the machine allows is carried out, everything else
// leaves the order untouched (tech.md §5.1).
func decide(target order.Status, known bool) func(order.Status) (order.Status, bool) {
	return func(current order.Status) (order.Status, bool) {
		if !known || !order.CanTransition(current, target) {
			return current, false
		}
		return target, true
	}
}

// DedupKey is sha256(provider_payment_id|status|payload), tech.md §4.
func DedupKey(cb Callback) string {
	return digest([]byte(cb.ProviderPaymentID + "|" + cb.ProviderStatus + "|" + string(cb.Raw)))
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// FullyPaid reports whether actually_paid covers pay_amount. Both are decimal
// strings and are compared as exact rationals, never as floats. A missing or
// unreadable amount counts as unpaid: a short payment must never be mistaken
// for a full one (tech.md §15, partial payments are settled by hand).
func FullyPaid(payAmount, actuallyPaid string) bool {
	owed, ok := new(big.Rat).SetString(payAmount)
	if !ok {
		return false
	}
	if owed.Sign() <= 0 {
		return true
	}
	paid, ok := new(big.Rat).SetString(actuallyPaid)
	if !ok {
		return false
	}
	return paid.Cmp(owed) >= 0
}
