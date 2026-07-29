package cart

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/domain/catalog"
	"github.com/qzq-kiim/shop/internal/money"
)

// Totals is the priced summary of a cart.
type Totals struct {
	Subtotal money.Amount
	Shipping money.Amount
	Total    money.Amount
}

// ShippingSource tells the cart what delivery costs right now. It is declared
// here by its consumer: the cart only ever needs this one number (tech.md §16.4).
type ShippingSource interface {
	ShippingCents(ctx context.Context) (int64, error)
}

// Service applies the cart rules. Handlers hold no cart logic of their own.
type Service struct {
	carts    Repository
	catalog  catalog.Repository
	currency string
	shipping ShippingSource
}

// NewService wires the cart service. Delivery is priced from settings, never
// from a number baked in at startup (tech.md §15).
func NewService(carts Repository, products catalog.Repository, currency string, shipping ShippingSource) *Service {
	return &Service{
		carts:    carts,
		catalog:  products,
		currency: currency,
		shipping: shipping,
	}
}

// EnsureCart returns the given cart when it still exists, and opens a new one
// otherwise. A stale cookie must never 500 the storefront.
func (s *Service) EnsureCart(ctx context.Context, cartID *uuid.UUID, visitorID *uuid.UUID) (uuid.UUID, error) {
	if cartID != nil {
		ok, err := s.carts.Exists(ctx, *cartID)
		if err != nil {
			return uuid.Nil, err
		}
		if ok {
			return *cartID, nil
		}
	}
	return s.carts.Create(ctx, visitorID)
}

// Add puts qty more of a variant into the cart, capped by stock and by MaxQty.
func (s *Service) Add(ctx context.Context, cartID, variantID uuid.UUID, qty int) error {
	if err := ValidateQty(qty); err != nil {
		return err
	}
	variant, _, err := s.catalog.VariantByID(ctx, variantID)
	if err != nil {
		return err
	}

	current, err := s.carts.QtyOf(ctx, cartID, variantID)
	if err != nil {
		return err
	}
	wanted := current + qty
	if err := ValidateQty(wanted); err != nil {
		return err
	}
	if variant.Available() < wanted {
		return fmt.Errorf("variant %s: %w", variantID, catalog.ErrOutOfStock)
	}
	return s.carts.AddItem(ctx, cartID, variantID, wanted)
}

// SetQty replaces the quantity of an existing line.
func (s *Service) SetQty(ctx context.Context, cartID, itemID uuid.UUID, qty int) error {
	if err := ValidateQty(qty); err != nil {
		return err
	}
	c, err := s.carts.Get(ctx, cartID)
	if err != nil {
		return err
	}
	for _, item := range c.Items {
		if item.ID != itemID {
			continue
		}
		if item.Available < qty {
			return fmt.Errorf("variant %s: %w", item.VariantID, catalog.ErrOutOfStock)
		}
		return s.carts.SetQty(ctx, cartID, itemID, qty)
	}
	return fmt.Errorf("item %s: %w", itemID, ErrItemNotFound)
}

// Remove drops a line from the cart.
func (s *Service) Remove(ctx context.Context, cartID, itemID uuid.UUID) error {
	return s.carts.RemoveItem(ctx, cartID, itemID)
}

// View loads the cart together with its totals.
func (s *Service) View(ctx context.Context, cartID uuid.UUID) (Cart, Totals, error) {
	c, err := s.carts.Get(ctx, cartID)
	if err != nil {
		return Cart{}, Totals{}, err
	}
	totals, err := s.Totals(ctx, c)
	if err != nil {
		return Cart{}, Totals{}, err
	}
	return c, totals, nil
}

// Totals prices a cart: subtotal from the lines, shipping from settings.
func (s *Service) Totals(ctx context.Context, c Cart) (Totals, error) {
	subtotal, err := c.Subtotal(s.currency)
	if err != nil {
		return Totals{}, err
	}
	shipping := money.Zero(s.currency)
	if !c.IsEmpty() {
		cents, err := s.shipping.ShippingCents(ctx)
		if err != nil {
			return Totals{}, fmt.Errorf("cart shipping: %w", err)
		}
		shipping = money.New(cents, s.currency)
	}
	total, err := subtotal.Add(shipping)
	if err != nil {
		return Totals{}, fmt.Errorf("cart total: %w", err)
	}
	return Totals{Subtotal: subtotal, Shipping: shipping, Total: total}, nil
}
