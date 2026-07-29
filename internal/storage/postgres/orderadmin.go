package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/qzq-kiim/shop/internal/domain/analytics"
	"github.com/qzq-kiim/shop/internal/domain/notify"
	"github.com/qzq-kiim/shop/internal/domain/order"
	"github.com/qzq-kiim/shop/internal/domain/payment"
	"github.com/qzq-kiim/shop/internal/money"
	"github.com/qzq-kiim/shop/internal/storage/postgres/sqlcgen"
	"github.com/qzq-kiim/shop/internal/telegram"
)

var _ order.AdminRepository = (*OrderRepo)(nil)

// ListForAdmin returns one filtered page of orders. The filter is applied in
// SQL, never by scanning rows in Go (tech.md §8.2).
func (r *OrderRepo) ListForAdmin(ctx context.Context, f order.Filter) (order.List, error) {
	rows, err := r.q.ListAdminOrders(ctx, sqlcgen.ListAdminOrdersParams{
		Status:      nullString(string(f.Status)),
		CreatedFrom: nullTime(f.From),
		CreatedTo:   nullTime(f.To),
		Number:      nullString(f.Number),
		ProductID:   nullUUID(f.ProductID),
		PageSize:    int32of(f.PageSize),
		PageOffset:  int32of(f.Offset()),
	})
	if err != nil {
		return order.List{}, fmt.Errorf("list admin orders: %w", err)
	}

	list := order.List{
		Orders:   make([]order.Summary, 0, len(rows)),
		Page:     f.Page,
		PageSize: f.PageSize,
	}
	for _, row := range rows {
		list.Total = int(row.TotalRows)
		list.Orders = append(list.Orders, order.Summary{
			ID:              row.ID,
			Number:          row.Number,
			Status:          order.Status(row.Status),
			Total:           money.New(row.TotalCents, row.Currency),
			CustomerName:    row.CustomerName,
			CustomerContact: row.CustomerContact,
			Units:           int(row.Units),
			CreatedAt:       row.CreatedAt.Time,
			PaidAt:          timePtr(row.PaidAt),
		})
	}
	return list, nil
}

// DetailByID loads one order with the lines, the attribution and the timestamps
// the card shows.
func (r *OrderRepo) DetailByID(ctx context.Context, id uuid.UUID) (order.Detail, error) {
	row, err := r.q.GetAdminOrder(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return order.Detail{}, fmt.Errorf("order %s: %w", id, order.ErrNotFound)
		}
		return order.Detail{}, fmt.Errorf("get admin order: %w", err)
	}

	hydrated, err := r.hydrate(ctx, order.Order{
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
	if err != nil {
		return order.Detail{}, err
	}

	return order.Detail{
		Order:       hydrated,
		FirstTouch:  decodeTouch(row.FirstTouch),
		LastTouch:   decodeTouch(row.LastTouch),
		ShippedAt:   timePtr(row.ShippedAt),
		CancelledAt: timePtr(row.CancelledAt),
	}, nil
}

// Transition moves an order by hand. The status change and the message that
// announces it are written together: either both land or neither does.
func (r *OrderRepo) Transition(ctx context.Context, id uuid.UUID, from, to order.Status) error {
	return withTx(ctx, r.pool, func(q *sqlcgen.Queries) error {
		locked, err := q.LockOrderByID(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("order %s: %w", id, order.ErrNotFound)
			}
			return fmt.Errorf("lock order: %w", err)
		}
		if order.Status(locked.Status) != from {
			return fmt.Errorf("order %s: %w", id, order.ErrConflict)
		}
		n, err := q.SetOrderStatus(ctx, sqlcgen.SetOrderStatusParams{
			ID:       id,
			Status:   string(to),
			Status_2: string(from),
		})
		if err != nil {
			return fmt.Errorf("set order status: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("order %s: %w", id, order.ErrConflict)
		}
		return enqueueOrderNotification(ctx, q, id, notify.KindStatusChanged, to,
			notify.StatusText(locked.Number, to))
	})
}

// PaymentEventsByOrder returns the raw provider log of one order, newest first.
func (r *PaymentRepo) PaymentEventsByOrder(ctx context.Context, orderID uuid.UUID) ([]payment.LogEntry, error) {
	rows, err := r.q.ListPaymentEventsByOrder(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("list payment events: %w", err)
	}
	out := make([]payment.LogEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, payment.LogEntry{
			ProviderStatus:    row.ProviderStatus,
			ProviderPaymentID: row.ProviderPaymentID,
			SignatureOK:       row.SignatureOk,
			PayCurrency:       derefString(row.PayCurrency),
			PayAmount:         numericString(row.PayAmount),
			ActuallyPaid:      numericString(row.ActuallyPaid),
			ReceivedAt:        row.ReceivedAt.Time,
		})
	}
	return out, nil
}

// LinksByOrder lists the chats following one order.
func (r *TelegramRepo) LinksByOrder(ctx context.Context, orderID uuid.UUID) ([]telegram.ChatLink, error) {
	rows, err := r.q.ListTelegramLinksByOrder(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("list telegram links: %w", err)
	}
	out := make([]telegram.ChatLink, 0, len(rows))
	for _, row := range rows {
		out = append(out, telegram.ChatLink{
			ChatID:   row.ChatID,
			Username: derefString(row.Username),
			LinkedAt: row.LinkedAt.Time,
		})
	}
	return out, nil
}

// decodeTouch reads an attribution snapshot back out of jsonb. A snapshot that
// cannot be read costs the card nothing, so it comes back empty rather than
// failing the whole page.
func decodeTouch(raw []byte) analytics.Touch {
	var t analytics.Touch
	if len(raw) == 0 {
		return t
	}
	if err := json.Unmarshal(raw, &t); err != nil {
		return analytics.Touch{}
	}
	return t
}

// numericString renders a provider decimal for display. Money never becomes a
// float on the way (tech.md §6.1), so the digits travel as text.
func numericString(n pgtype.Numeric) string {
	if !n.Valid {
		return ""
	}
	v, err := n.Value()
	if err != nil || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
