package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/qzq-kiim/shop/internal/domain/cart"
	"github.com/qzq-kiim/shop/internal/money"
	"github.com/qzq-kiim/shop/internal/storage/postgres/sqlcgen"
)

// CartRepo stores carts and their lines.
type CartRepo struct {
	q *sqlcgen.Queries
}

// NewCartRepo returns the cart repository bound to the store.
func NewCartRepo(s *Store) *CartRepo { return &CartRepo{q: s.q} }

var _ cart.Repository = (*CartRepo)(nil)

// Create opens a new cart, optionally tied to a known visitor.
func (r *CartRepo) Create(ctx context.Context, visitorID *uuid.UUID) (uuid.UUID, error) {
	id, err := r.q.CreateCart(ctx, nullUUID(visitorID))
	if err != nil {
		return uuid.Nil, fmt.Errorf("create cart: %w", err)
	}
	return id, nil
}

// Exists reports whether a cart id still refers to a live cart.
func (r *CartRepo) Exists(ctx context.Context, cartID uuid.UUID) (bool, error) {
	ok, err := r.q.CartExists(ctx, cartID)
	if err != nil {
		return false, fmt.Errorf("cart exists: %w", err)
	}
	return ok, nil
}

// Get loads a cart with its priced lines.
func (r *CartRepo) Get(ctx context.Context, cartID uuid.UUID) (cart.Cart, error) {
	rows, err := r.q.ListCartItems(ctx, cartID)
	if err != nil {
		return cart.Cart{}, fmt.Errorf("list cart items: %w", err)
	}
	c := cart.Cart{ID: cartID, Items: make([]cart.Item, 0, len(rows))}
	for _, row := range rows {
		c.Items = append(c.Items, cart.Item{
			ID:           row.ID,
			VariantID:    row.VariantID,
			ProductSlug:  row.Slug,
			ProductTitle: row.Title,
			ImageFront:   row.ImageFront,
			Size:         row.Size,
			UnitPrice:    money.New(row.PriceCents, row.Currency),
			Qty:          int(row.Qty),
			Available:    int(row.Stock - row.Reserved),
		})
	}
	return c, nil
}

// AddItem sets the line quantity for a variant, creating the line if needed.
func (r *CartRepo) AddItem(ctx context.Context, cartID, variantID uuid.UUID, qty int) error {
	err := r.q.InsertCartItem(ctx, sqlcgen.InsertCartItemParams{
		CartID:    cartID,
		VariantID: variantID,
		Qty:       int32of(qty),
	})
	if err != nil {
		return fmt.Errorf("insert cart item: %w", err)
	}
	if err := r.q.TouchCart(ctx, cartID); err != nil {
		return fmt.Errorf("touch cart: %w", err)
	}
	return nil
}

// QtyOf returns the current quantity of a variant in the cart, 0 when absent.
func (r *CartRepo) QtyOf(ctx context.Context, cartID, variantID uuid.UUID) (int, error) {
	row, err := r.q.GetCartItemByVariant(ctx, sqlcgen.GetCartItemByVariantParams{
		CartID:    cartID,
		VariantID: variantID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("get cart item: %w", err)
	}
	return int(row.Qty), nil
}

// SetQty changes the quantity of a line owned by the cart.
func (r *CartRepo) SetQty(ctx context.Context, cartID, itemID uuid.UUID, qty int) error {
	n, err := r.q.UpdateCartItemQty(ctx, sqlcgen.UpdateCartItemQtyParams{
		CartID: cartID,
		ID:     itemID,
		Qty:    int32of(qty),
	})
	if err != nil {
		return fmt.Errorf("update cart item qty: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("item %s: %w", itemID, cart.ErrItemNotFound)
	}
	if err := r.q.TouchCart(ctx, cartID); err != nil {
		return fmt.Errorf("touch cart: %w", err)
	}
	return nil
}

// RemoveItem drops a line owned by the cart.
func (r *CartRepo) RemoveItem(ctx context.Context, cartID, itemID uuid.UUID) error {
	n, err := r.q.DeleteCartItem(ctx, sqlcgen.DeleteCartItemParams{CartID: cartID, ID: itemID})
	if err != nil {
		return fmt.Errorf("delete cart item: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("item %s: %w", itemID, cart.ErrItemNotFound)
	}
	if err := r.q.TouchCart(ctx, cartID); err != nil {
		return fmt.Errorf("touch cart: %w", err)
	}
	return nil
}
