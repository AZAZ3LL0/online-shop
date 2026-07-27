// Package cart holds the server-side cart: the source of truth for what a
// visitor intends to buy. The browser keeps no cart state.
package cart

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/qzq-kiim/shop/internal/money"
)

// Quantity bounds, mirrored by the cart_items check constraint.
const (
	MinQty = 1
	MaxQty = 10
)

// ErrCartItemLimit is returned when a quantity falls outside MinQty..MaxQty.
var ErrCartItemLimit = errors.New("cart: qty out of range")

// ErrItemNotFound is returned when a cart line does not belong to the cart.
var ErrItemNotFound = errors.New("cart: item not found")

// Item is one cart line, priced from the current catalog price.
type Item struct {
	ID           uuid.UUID
	VariantID    uuid.UUID
	ProductSlug  string
	ProductTitle string
	ImageFront   string
	Size         string
	UnitPrice    money.Amount
	Qty          int
	Available    int
}

// LineTotal is the price of the whole line.
func (i Item) LineTotal() money.Amount { return i.UnitPrice.Mul(int64(i.Qty)) }

// Cart is a visitor's basket.
type Cart struct {
	ID    uuid.UUID
	Items []Item
}

// Subtotal sums the lines. The empty cart is zero in the shop currency.
func (c Cart) Subtotal(currency string) (money.Amount, error) {
	total := money.Zero(currency)
	for _, item := range c.Items {
		var err error
		total, err = total.Add(item.LineTotal())
		if err != nil {
			return money.Amount{}, fmt.Errorf("cart subtotal: %w", err)
		}
	}
	return total, nil
}

// Count is the number of units in the cart, for the header badge.
func (c Cart) Count() int {
	var n int
	for _, item := range c.Items {
		n += item.Qty
	}
	return n
}

// IsEmpty reports whether the cart holds no lines.
func (c Cart) IsEmpty() bool { return len(c.Items) == 0 }

// ValidateQty enforces the quantity bounds before storage is touched.
func ValidateQty(qty int) error {
	if qty < MinQty || qty > MaxQty {
		return fmt.Errorf("%w: %d not in %d..%d", ErrCartItemLimit, qty, MinQty, MaxQty)
	}
	return nil
}

// Repository is the storage access the cart service depends on.
type Repository interface {
	Create(ctx context.Context, visitorID *uuid.UUID) (uuid.UUID, error)
	Exists(ctx context.Context, cartID uuid.UUID) (bool, error)
	Get(ctx context.Context, cartID uuid.UUID) (Cart, error)
	QtyOf(ctx context.Context, cartID, variantID uuid.UUID) (int, error)
	AddItem(ctx context.Context, cartID, variantID uuid.UUID, qty int) error
	SetQty(ctx context.Context, cartID, itemID uuid.UUID, qty int) error
	RemoveItem(ctx context.Context, cartID, itemID uuid.UUID) error
}
