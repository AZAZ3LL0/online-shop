package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qzq-kiim/shop/internal/domain/catalog"
	"github.com/qzq-kiim/shop/internal/domain/notify"
	"github.com/qzq-kiim/shop/internal/domain/order"
	"github.com/qzq-kiim/shop/internal/money"
	"github.com/qzq-kiim/shop/internal/storage/postgres/sqlcgen"
)

// uniqueViolation is the Postgres code for a broken unique constraint.
const uniqueViolation = "23505"

// OrderRepo stores orders, their price snapshot and the stock behind them.
type OrderRepo struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

// NewOrderRepo returns the order repository bound to the store.
func NewOrderRepo(s *Store) *OrderRepo { return &OrderRepo{pool: s.pool, q: s.q} }

var _ order.Repository = (*OrderRepo)(nil)

// Create writes the order, its items and the stock reservation in one
// transaction: an order never exists without the units behind it.
func (r *OrderRepo) Create(ctx context.Context, d order.Draft) (order.Order, error) {
	firstTouch, err := json.Marshal(d.FirstTouch)
	if err != nil {
		return order.Order{}, fmt.Errorf("marshal first touch: %w", err)
	}
	lastTouch, err := json.Marshal(d.LastTouch)
	if err != nil {
		return order.Order{}, fmt.Errorf("marshal last touch: %w", err)
	}

	placed := order.Order{
		Number:      d.Number,
		PublicToken: d.PublicToken,
		TGLinkCode:  d.TGLinkCode,
		Status:      order.StatusCreated,
		Subtotal:    d.Subtotal,
		Shipping:    d.Shipping,
		Total:       d.Total,
		Customer:    d.Customer,
		Items:       d.Items,
		ExpiresAt:   &d.ExpiresAt,
	}

	err = withTx(ctx, r.pool, func(q *sqlcgen.Queries) error {
		row, err := q.InsertCheckoutOrder(ctx, sqlcgen.InsertCheckoutOrderParams{
			Number:          d.Number,
			PublicToken:     d.PublicToken,
			TgLinkCode:      d.TGLinkCode,
			Status:          string(order.StatusCreated),
			SubtotalCents:   d.Subtotal.Cents,
			ShippingCents:   d.Shipping.Cents,
			TotalCents:      d.Total.Cents,
			Currency:        d.Total.Currency,
			CustomerName:    d.Customer.Name,
			CustomerContact: d.Customer.Contact,
			ShippingAddress: d.Customer.Address,
			Comment:         nullString(d.Customer.Comment),
			VisitorID:       nullUUID(d.VisitorID),
			FirstTouch:      firstTouch,
			LastTouch:       lastTouch,
			ExpiresAt:       ts(d.ExpiresAt),
		})
		if err != nil {
			if isUniqueViolation(err) {
				return fmt.Errorf("insert order: %w", order.ErrNumberTaken)
			}
			return fmt.Errorf("insert order: %w", err)
		}
		placed.ID = row.ID
		placed.CreatedAt = row.CreatedAt.Time

		// Reserving in a fixed order keeps two concurrent checkouts over the
		// same sizes from deadlocking each other.
		for _, item := range reservationOrder(d.Items) {
			n, err := q.ReserveVariant(ctx, sqlcgen.ReserveVariantParams{
				VariantID: item.VariantID,
				Qty:       int32of(item.Qty),
			})
			if err != nil {
				return fmt.Errorf("reserve variant: %w", err)
			}
			if n == 0 {
				return fmt.Errorf("variant %s: %w", item.VariantID, catalog.ErrOutOfStock)
			}
			err = q.InsertOrderItem(ctx, sqlcgen.InsertOrderItemParams{
				OrderID:        row.ID,
				VariantID:      item.VariantID,
				ProductTitle:   item.ProductTitle,
				Size:           item.Size,
				UnitPriceCents: item.UnitPrice.Cents,
				Qty:            int32of(item.Qty),
			})
			if err != nil {
				return fmt.Errorf("insert order item: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return order.Order{}, err
	}
	return placed, nil
}

// AttachPayment records the invoice and moves the order forward in the same
// transaction, so an order in awaiting_payment always has a payment row.
func (r *OrderRepo) AttachPayment(ctx context.Context, orderID uuid.UUID, from, to order.Status, p order.PaymentRef) error {
	return withTx(ctx, r.pool, func(q *sqlcgen.Queries) error {
		locked, err := q.LockOrderByID(ctx, orderID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("order %s: %w", orderID, order.ErrNotFound)
			}
			return fmt.Errorf("lock order: %w", err)
		}
		if order.Status(locked.Status) != from {
			return fmt.Errorf("order %s: %w", orderID, order.ErrConflict)
		}
		_, err = q.UpsertPayment(ctx, sqlcgen.UpsertPaymentParams{
			OrderID:           orderID,
			Provider:          p.Provider,
			ProviderPaymentID: p.ProviderPaymentID,
			InvoiceUrl:        nullString(p.InvoiceURL),
			Status:            p.Status,
		})
		if err != nil {
			return fmt.Errorf("upsert payment: %w", err)
		}
		n, err := q.SetOrderStatus(ctx, sqlcgen.SetOrderStatusParams{
			ID:       orderID,
			Status:   string(to),
			Status_2: string(from),
		})
		if err != nil {
			return fmt.Errorf("set order status: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("order %s: %w", orderID, order.ErrConflict)
		}
		// Nobody is normally following the order this early, so this usually
		// queues nothing; it is here so no transition is a special case.
		return enqueueOrderNotification(ctx, q, orderID, notify.KindStatusChanged, to,
			notify.StatusText(locked.Number, to))
	})
}

// ReleaseReservation gives the reserved units of an order back.
func (r *OrderRepo) ReleaseReservation(ctx context.Context, orderID uuid.UUID) error {
	return withTx(ctx, r.pool, func(q *sqlcgen.Queries) error {
		return releaseItems(ctx, q, orderID)
	})
}

// ByPublicToken loads the order behind a status-page token.
func (r *OrderRepo) ByPublicToken(ctx context.Context, token string) (order.Order, error) {
	row, err := r.q.GetOrderByPublicToken(ctx, token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return order.Order{}, fmt.Errorf("order token: %w", order.ErrNotFound)
		}
		return order.Order{}, fmt.Errorf("get order by token: %w", err)
	}
	return r.hydrate(ctx, order.Order{
		ID:          row.ID,
		Number:      row.Number,
		PublicToken: row.PublicToken,
		TGLinkCode:  row.TgLinkCode,
		Status:      order.Status(row.Status),
		Subtotal:    money.New(row.SubtotalCents, row.Currency),
		Shipping:    money.New(row.ShippingCents, row.Currency),
		Total:       money.New(row.TotalCents, row.Currency),
		Customer: order.Customer{
			Name:    row.CustomerName,
			Contact: row.CustomerContact,
			Address: row.ShippingAddress,
			Comment: derefString(row.Comment),
		},
		ExpiresAt: timePtr(row.ExpiresAt),
		PaidAt:    timePtr(row.PaidAt),
		CreatedAt: row.CreatedAt.Time,
	})
}

// ByNumber loads the order behind a human-readable number.
func (r *OrderRepo) ByNumber(ctx context.Context, number string) (order.Order, error) {
	row, err := r.q.GetOrderByNumber(ctx, number)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return order.Order{}, fmt.Errorf("order %s: %w", number, order.ErrNotFound)
		}
		return order.Order{}, fmt.Errorf("get order by number: %w", err)
	}
	return r.hydrate(ctx, order.Order{
		ID:          row.ID,
		Number:      row.Number,
		PublicToken: row.PublicToken,
		TGLinkCode:  row.TgLinkCode,
		Status:      order.Status(row.Status),
		Subtotal:    money.New(row.SubtotalCents, row.Currency),
		Shipping:    money.New(row.ShippingCents, row.Currency),
		Total:       money.New(row.TotalCents, row.Currency),
		Customer: order.Customer{
			Name:    row.CustomerName,
			Contact: row.CustomerContact,
			Address: row.ShippingAddress,
			Comment: derefString(row.Comment),
		},
		ExpiresAt: timePtr(row.ExpiresAt),
		PaidAt:    timePtr(row.PaidAt),
		CreatedAt: row.CreatedAt.Time,
	})
}

// ExpireDue expires the reservations whose deadline has passed. Only orders
// still waiting for payment are touched, which makes a second pass over the
// same rows a no-op.
func (r *OrderRepo) ExpireDue(ctx context.Context, now time.Time, limit int) (int, error) {
	var expired int
	err := withTx(ctx, r.pool, func(q *sqlcgen.Queries) error {
		due, err := q.ListDueOrders(ctx, sqlcgen.ListDueOrdersParams{
			ExpiresAt: ts(now),
			Limit:     int32of(limit),
		})
		if err != nil {
			return fmt.Errorf("list due orders: %w", err)
		}
		for _, row := range due {
			if err := releaseItems(ctx, q, row.ID); err != nil {
				return err
			}
			n, err := q.SetOrderStatus(ctx, sqlcgen.SetOrderStatusParams{
				ID:       row.ID,
				Status:   string(order.StatusExpired),
				Status_2: string(order.StatusAwaitingPayment),
			})
			if err != nil {
				return fmt.Errorf("expire order: %w", err)
			}
			if n == 0 {
				// Someone else moved it on between the select and the update.
				continue
			}
			err = enqueueOrderNotification(ctx, q, row.ID, notify.KindStatusChanged, order.StatusExpired,
				notify.StatusText(row.Number, order.StatusExpired))
			if err != nil {
				return err
			}
			expired += int(n)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return expired, nil
}

// hydrate fills the lines and the last invoice URL of an order.
func (r *OrderRepo) hydrate(ctx context.Context, o order.Order) (order.Order, error) {
	items, err := r.q.ListOrderItemsByOrder(ctx, o.ID)
	if err != nil {
		return order.Order{}, fmt.Errorf("list order items: %w", err)
	}
	for _, row := range items {
		o.Items = append(o.Items, order.Item{
			VariantID:    row.VariantID,
			ProductTitle: row.ProductTitle,
			Size:         row.Size,
			UnitPrice:    money.New(row.UnitPriceCents, o.Total.Currency),
			Qty:          int(row.Qty),
		})
	}

	url, err := r.q.GetLatestInvoiceURL(ctx, o.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return o, nil
	case err != nil:
		return order.Order{}, fmt.Errorf("get invoice url: %w", err)
	}
	o.InvoiceURL = derefString(url)
	return o, nil
}

// releaseItems gives every reserved unit of one order back.
func releaseItems(ctx context.Context, q *sqlcgen.Queries, orderID uuid.UUID) error {
	items, err := q.ListOrderItemsByOrder(ctx, orderID)
	if err != nil {
		return fmt.Errorf("list order items: %w", err)
	}
	for _, item := range items {
		_, err := q.ReleaseVariant(ctx, sqlcgen.ReleaseVariantParams{
			VariantID: item.VariantID,
			Qty:       item.Qty,
		})
		if err != nil {
			return fmt.Errorf("release variant: %w", err)
		}
	}
	return nil
}

// reservationOrder sorts the lines by variant id so concurrent checkouts always
// take the row locks in the same sequence.
func reservationOrder(items []order.Item) []order.Item {
	sorted := make([]order.Item, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].VariantID.String() < sorted[j].VariantID.String()
	})
	return sorted
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func timePtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}
