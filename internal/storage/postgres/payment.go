package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qzq-kiim/shop/internal/domain/order"
	"github.com/qzq-kiim/shop/internal/domain/payment"
	"github.com/qzq-kiim/shop/internal/storage/postgres/sqlcgen"
)

// PaymentRepo stores provider payments and the raw callback log.
type PaymentRepo struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

// NewPaymentRepo returns the payment repository bound to the store.
func NewPaymentRepo(s *Store) *PaymentRepo { return &PaymentRepo{pool: s.pool, q: s.q} }

var _ payment.Repository = (*PaymentRepo)(nil)

// Apply is the whole effect of one callback: the raw event, the payment row,
// the order status and the stock behind it, in a single transaction. The dedup
// key is inserted inside that transaction, so a callback that fails halfway
// leaves nothing behind and can be redelivered.
func (r *PaymentRepo) Apply(ctx context.Context, cmd payment.Command) (payment.Outcome, error) {
	var out payment.Outcome
	err := withTx(ctx, r.pool, func(q *sqlcgen.Queries) error {
		locked, err := q.LockOrderByNumber(ctx, cmd.OrderNumber)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				out = payment.Outcome{OrderMissing: true}
				return recordEvent(ctx, q, pgtype.UUID{}, cmd.Event, &out)
			}
			return fmt.Errorf("lock order: %w", err)
		}

		paymentID, err := q.UpsertPayment(ctx, sqlcgen.UpsertPaymentParams{
			OrderID:           locked.ID,
			Provider:          payment.ProviderNOWPayments,
			ProviderPaymentID: cmd.ProviderPaymentID,
			PayCurrency:       nullString(cmd.PayCurrency),
			PayAmount:         numeric(cmd.PayAmount),
			ActuallyPaid:      numeric(cmd.ActuallyPaid),
			Status:            cmd.ProviderStatus,
		})
		if err != nil {
			return fmt.Errorf("upsert payment: %w", err)
		}

		current := order.Status(locked.Status)
		out = payment.Outcome{Transition: payment.Transition{OrderID: locked.ID, From: current, To: current}}
		if err := recordEvent(ctx, q, pgtype.UUID{Bytes: paymentID, Valid: true}, cmd.Event, &out); err != nil {
			return err
		}
		if out.Duplicate {
			return nil
		}

		target, ok := cmd.Decide(current)
		if !ok {
			return nil
		}
		n, err := q.SetOrderStatus(ctx, sqlcgen.SetOrderStatusParams{
			ID:       locked.ID,
			Status:   string(target),
			Status_2: string(current),
		})
		if err != nil {
			return fmt.Errorf("set order status: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("order %s: %w", cmd.OrderNumber, order.ErrConflict)
		}
		if err := settleStock(ctx, q, locked.ID, target); err != nil {
			return err
		}
		out.To = target
		out.Applied = true
		return nil
	})
	if err != nil {
		return payment.Outcome{}, err
	}
	return out, nil
}

// RecordRejected files a callback that failed its signature check. Nothing else
// is touched: the body was never parsed.
func (r *PaymentRepo) RecordRejected(ctx context.Context, ev payment.Event) error {
	_, err := r.q.InsertPaymentEvent(ctx, sqlcgen.InsertPaymentEventParams{
		ProviderStatus: ev.ProviderStatus,
		Payload:        ev.Payload,
		SignatureOk:    ev.SignatureOK,
		DedupKey:       ev.DedupKey,
	})
	if err != nil {
		return fmt.Errorf("insert payment event: %w", err)
	}
	return nil
}

// recordEvent appends the raw callback and reports a redelivery through out.
func recordEvent(ctx context.Context, q *sqlcgen.Queries, paymentID pgtype.UUID, ev payment.Event, out *payment.Outcome) error {
	n, err := q.InsertPaymentEvent(ctx, sqlcgen.InsertPaymentEventParams{
		PaymentID:      paymentID,
		ProviderStatus: ev.ProviderStatus,
		Payload:        ev.Payload,
		SignatureOk:    ev.SignatureOK,
		DedupKey:       ev.DedupKey,
	})
	if err != nil {
		return fmt.Errorf("insert payment event: %w", err)
	}
	out.Duplicate = n == 0
	return nil
}

// settleStock turns a status change into the stock move behind it, tech.md §4:
// paid takes the units out of stock, expired gives the reservation back.
func settleStock(ctx context.Context, q *sqlcgen.Queries, orderID uuid.UUID, to order.Status) error {
	switch to {
	case order.StatusPaid:
		items, err := q.ListOrderItemsByOrder(ctx, orderID)
		if err != nil {
			return fmt.Errorf("list order items: %w", err)
		}
		for _, item := range items {
			_, err := q.CommitVariantStock(ctx, sqlcgen.CommitVariantStockParams{
				VariantID: item.VariantID,
				Qty:       item.Qty,
			})
			if err != nil {
				return fmt.Errorf("commit variant stock: %w", err)
			}
		}
		return nil
	case order.StatusExpired:
		return releaseItems(ctx, q, orderID)
	default:
		return nil
	}
}

// numeric turns a provider decimal string into a numeric column value. An empty
// or unreadable amount is stored as NULL rather than as a guess.
func numeric(v string) pgtype.Numeric {
	var n pgtype.Numeric
	if v == "" {
		return n
	}
	if err := n.Scan(v); err != nil {
		return pgtype.Numeric{}
	}
	return n
}
