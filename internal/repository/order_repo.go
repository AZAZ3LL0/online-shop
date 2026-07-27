package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qzq-kiim/shop/internal/model"
)

// ErrAlreadyProcessed is returned when a paid/cancelled order receives another
// payment webhook. Callers treat it as a successful no-op (idempotency).
var ErrAlreadyProcessed = errors.New("order already in a terminal state")

type OrderRepo struct {
	db *pgxpool.Pool
}

func NewOrderRepo(db *pgxpool.Pool) *OrderRepo {
	return &OrderRepo{db: db}
}

func (r *OrderRepo) Create(ctx context.Context, o *model.Order) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO orders (session_id, status, total_amount, currency,
		  customer_name, customer_email, customer_phone, delivery_address)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 RETURNING id, uuid, created_at, updated_at`,
		o.SessionID, o.Status, o.TotalAmount, o.Currency,
		o.CustomerName, o.CustomerEmail, o.CustomerPhone, o.DeliveryAddress).
		Scan(&o.ID, &o.UUID, &o.CreatedAt, &o.UpdatedAt)
}

func (r *OrderRepo) AddItems(ctx context.Context, orderID int, items []model.OrderItem) error {
	for _, item := range items {
		_, err := r.db.Exec(ctx,
			`INSERT INTO order_items (order_id, product_id, variant_id, size, quantity, price)
			 VALUES ($1,$2,$3,$4,$5,$6)`,
			orderID, item.ProductID, item.VariantID, item.Size, item.Quantity, item.Price)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *OrderRepo) SetPaymentID(ctx context.Context, orderID int, nowpaymentsID string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE orders SET nowpayments_id=$1, status='awaiting_payment', updated_at=NOW()
		 WHERE id=$2`, nowpaymentsID, orderID)
	return err
}

func (r *OrderRepo) UpdateStatus(ctx context.Context, orderUUID string, status model.OrderStatus) error {
	_, err := r.db.Exec(ctx,
		`UPDATE orders SET status=$1, updated_at=NOW() WHERE uuid=$2`, status, orderUUID)
	return err
}

// MarkPaidAndDecrementStock atomically flips an order to 'paid' and decrements
// the stock of every ordered variant, in a single transaction. It is
// idempotent: if the order is already paid or cancelled it returns
// ErrAlreadyProcessed without touching stock. The row is locked FOR UPDATE so
// concurrent duplicate webhooks cannot double-decrement.
func (r *OrderRepo) MarkPaidAndDecrementStock(ctx context.Context, orderUUID uuid.UUID) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var orderID int
	var status model.OrderStatus
	err = tx.QueryRow(ctx,
		`SELECT id, status FROM orders WHERE uuid=$1 FOR UPDATE`, orderUUID).
		Scan(&orderID, &status)
	if err != nil {
		return err
	}
	if status == model.StatusPaid || status == model.StatusCancelled {
		return ErrAlreadyProcessed
	}

	if _, err := tx.Exec(ctx,
		`UPDATE orders SET status=$1, updated_at=NOW() WHERE id=$2`,
		model.StatusPaid, orderID); err != nil {
		return err
	}

	// Decrement stock per line. GREATEST guards against negative stock in the
	// rare case an item oversold between order creation and payment.
	rows, err := tx.Query(ctx,
		`SELECT variant_id, quantity FROM order_items WHERE order_id=$1`, orderID)
	if err != nil {
		return err
	}
	type line struct {
		variantID int
		quantity  int
	}
	var lines []line
	for rows.Next() {
		var l line
		if err := rows.Scan(&l.variantID, &l.quantity); err != nil {
			rows.Close()
			return err
		}
		lines = append(lines, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, l := range lines {
		if _, err := tx.Exec(ctx,
			`UPDATE product_variants SET stock = GREATEST(stock - $1, 0) WHERE id=$2`,
			l.quantity, l.variantID); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *OrderRepo) GetByUUID(ctx context.Context, id uuid.UUID) (*model.Order, error) {
	o := &model.Order{}
	err := r.db.QueryRow(ctx,
		`SELECT id, uuid, session_id, status, total_amount, currency,
		  customer_name, customer_email, customer_phone, delivery_address,
		  COALESCE(nowpayments_id,''), created_at, updated_at
		 FROM orders WHERE uuid=$1`, id).
		Scan(&o.ID, &o.UUID, &o.SessionID, &o.Status, &o.TotalAmount, &o.Currency,
			&o.CustomerName, &o.CustomerEmail, &o.CustomerPhone, &o.DeliveryAddress,
			&o.NowpaymentsID, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if err := r.loadItems(ctx, o); err != nil {
		return nil, err
	}
	return o, nil
}

func (r *OrderRepo) loadItems(ctx context.Context, o *model.Order) error {
	rows, err := r.db.Query(ctx,
		`SELECT oi.id, oi.order_id, oi.product_id, oi.variant_id, oi.size, oi.quantity, oi.price,
		        p.name, p.slug
		 FROM order_items oi
		 JOIN products p ON p.id = oi.product_id
		 WHERE oi.order_id=$1`, o.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		item := model.OrderItem{}
		if err := rows.Scan(&item.ID, &item.OrderID, &item.ProductID, &item.VariantID,
			&item.Size, &item.Quantity, &item.Price, &item.ProductName, &item.ProductSlug); err != nil {
			return err
		}
		o.Items = append(o.Items, item)
	}
	return nil
}
