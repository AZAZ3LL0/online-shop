package cart_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/domain/cart"
	"github.com/qzq-kiim/shop/internal/domain/catalog"
	"github.com/qzq-kiim/shop/internal/money"
)

// fixedRates is a shipping fee that never changes during a test.
type fixedRates int64

func (f fixedRates) ShippingCents(context.Context) (int64, error) { return int64(f), nil }

// fakeCarts is an in-memory cart.Repository. It stores exactly what the service
// asks it to store, so a test failure means the rule is wrong, not the storage.
type fakeCarts struct {
	items  map[uuid.UUID][]cart.Item
	writes int // every call that changes stored state
}

func newFakeCarts() *fakeCarts {
	return &fakeCarts{items: make(map[uuid.UUID][]cart.Item)}
}

func (f *fakeCarts) Create(context.Context, *uuid.UUID) (uuid.UUID, error) {
	id := uuid.New()
	f.items[id] = nil
	return id, nil
}

func (f *fakeCarts) Exists(_ context.Context, cartID uuid.UUID) (bool, error) {
	_, ok := f.items[cartID]
	return ok, nil
}

func (f *fakeCarts) Get(_ context.Context, cartID uuid.UUID) (cart.Cart, error) {
	return cart.Cart{ID: cartID, Items: f.items[cartID]}, nil
}

func (f *fakeCarts) QtyOf(_ context.Context, cartID, variantID uuid.UUID) (int, error) {
	for _, item := range f.items[cartID] {
		if item.VariantID == variantID {
			return item.Qty, nil
		}
	}
	return 0, nil
}

func (f *fakeCarts) AddItem(_ context.Context, cartID, variantID uuid.UUID, qty int) error {
	f.writes++
	for i, item := range f.items[cartID] {
		if item.VariantID == variantID {
			f.items[cartID][i].Qty = qty
			return nil
		}
	}
	f.items[cartID] = append(f.items[cartID], cart.Item{
		ID:        uuid.New(),
		VariantID: variantID,
		UnitPrice: money.New(3500, "USD"),
		Qty:       qty,
		Available: availableOf(variantID),
	})
	return nil
}

func (f *fakeCarts) SetQty(_ context.Context, cartID, itemID uuid.UUID, qty int) error {
	for i, item := range f.items[cartID] {
		if item.ID == itemID {
			f.writes++
			f.items[cartID][i].Qty = qty
			return nil
		}
	}
	return cart.ErrItemNotFound
}

func (f *fakeCarts) RemoveItem(_ context.Context, cartID, itemID uuid.UUID) error {
	kept := make([]cart.Item, 0, len(f.items[cartID]))
	for _, item := range f.items[cartID] {
		if item.ID != itemID {
			kept = append(kept, item)
		}
	}
	if len(kept) == len(f.items[cartID]) {
		return cart.ErrItemNotFound
	}
	f.writes++
	f.items[cartID] = kept
	return nil
}

// The size run the fake catalog serves: one roomy variant and one nearly gone.
var (
	plentyVariant = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	scarceVariant = uuid.MustParse("22222222-2222-2222-2222-222222222222")
)

func availableOf(variantID uuid.UUID) int {
	if variantID == scarceVariant {
		return 2
	}
	return 20
}

// fakeCatalog serves the two variants above and nothing else.
type fakeCatalog struct{}

func (fakeCatalog) ListActive(context.Context) ([]catalog.Product, error) { return nil, nil }

func (fakeCatalog) BySlug(context.Context, string) (catalog.Product, error) {
	return catalog.Product{}, catalog.ErrNotFound
}

func (fakeCatalog) VariantByID(_ context.Context, id uuid.UUID) (catalog.Variant, catalog.Product, error) {
	if id != plentyVariant && id != scarceVariant {
		return catalog.Variant{}, catalog.Product{}, catalog.ErrNotFound
	}
	return catalog.Variant{ID: id, Size: "M", Stock: availableOf(id)}, catalog.Product{}, nil
}

func newService(carts cart.Repository) *cart.Service {
	return cart.NewService(carts, fakeCatalog{}, "USD", fixedRates(0))
}

func openCart(t *testing.T, s *cart.Service, carts *fakeCarts) uuid.UUID {
	t.Helper()
	id, err := s.EnsureCart(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("ensure cart: %v", err)
	}
	carts.writes = 0
	return id
}

// TestAddRejectsQuantitiesOutsideTheLimit is the S2.1 acceptance criteria on the
// domain side: only 1..10 units of a size may ever reach storage.
func TestAddRejectsQuantitiesOutsideTheLimit(t *testing.T) {
	tests := []struct {
		name string
		qty  int
	}{
		{"zero", 0},
		{"negative", -1},
		{"one over the limit", cart.MaxQty + 1},
		{"absurd", 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			carts := newFakeCarts()
			s := newService(carts)
			cartID := openCart(t, s, carts)

			err := s.Add(context.Background(), cartID, plentyVariant, tt.qty)
			if !errors.Is(err, cart.ErrCartItemLimit) {
				t.Fatalf("Add(qty=%d) = %v, want ErrCartItemLimit", tt.qty, err)
			}
			if carts.writes != 0 {
				t.Fatalf("a rejected quantity wrote to storage %d times", carts.writes)
			}
		})
	}
}

// TestAddAccumulatesUpToTheLimit pins the rule that the limit applies to the
// line, not to the request: ten singles are fine, the eleventh is not.
func TestAddAccumulatesUpToTheLimit(t *testing.T) {
	carts := newFakeCarts()
	s := newService(carts)
	cartID := openCart(t, s, carts)
	ctx := context.Background()

	for i := 1; i <= cart.MaxQty; i++ {
		if err := s.Add(ctx, cartID, plentyVariant, 1); err != nil {
			t.Fatalf("add %d of %d: %v", i, cart.MaxQty, err)
		}
	}

	c, _ := carts.Get(ctx, cartID)
	if len(c.Items) != 1 {
		t.Fatalf("adding the same size again made %d lines, want 1", len(c.Items))
	}
	if c.Items[0].Qty != cart.MaxQty {
		t.Fatalf("line quantity = %d, want %d", c.Items[0].Qty, cart.MaxQty)
	}

	if err := s.Add(ctx, cartID, plentyVariant, 1); !errors.Is(err, cart.ErrCartItemLimit) {
		t.Fatalf("add past the limit = %v, want ErrCartItemLimit", err)
	}
	if c, _ = carts.Get(ctx, cartID); c.Items[0].Qty != cart.MaxQty {
		t.Fatalf("rejected add changed the line to %d", c.Items[0].Qty)
	}
}

// TestAddStopsAtAvailability keeps the cart inside what the catalog can ship.
func TestAddStopsAtAvailability(t *testing.T) {
	carts := newFakeCarts()
	s := newService(carts)
	cartID := openCart(t, s, carts)
	ctx := context.Background()

	if err := s.Add(ctx, cartID, scarceVariant, 2); err != nil {
		t.Fatalf("add the last two: %v", err)
	}
	if err := s.Add(ctx, cartID, scarceVariant, 1); !errors.Is(err, catalog.ErrOutOfStock) {
		t.Fatalf("add beyond availability = %v, want ErrOutOfStock", err)
	}
	if err := s.Add(ctx, cartID, uuid.New(), 1); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("add of an unknown variant = %v, want ErrNotFound", err)
	}
}

// TestSetQtyIsIdempotent is the other half of S2.1: repeating the same update
// leaves the cart exactly as it was.
func TestSetQtyIsIdempotent(t *testing.T) {
	carts := newFakeCarts()
	s := newService(carts)
	cartID := openCart(t, s, carts)
	ctx := context.Background()

	if err := s.Add(ctx, cartID, plentyVariant, 3); err != nil {
		t.Fatalf("add: %v", err)
	}
	c, _ := carts.Get(ctx, cartID)
	itemID := c.Items[0].ID

	for i := range 3 {
		if err := s.SetQty(ctx, cartID, itemID, 4); err != nil {
			t.Fatalf("update %d: %v", i+1, err)
		}
		c, _ = carts.Get(ctx, cartID)
		if len(c.Items) != 1 || c.Items[0].Qty != 4 {
			t.Fatalf("after update %d the cart is %+v, want one line of 4", i+1, c.Items)
		}
	}
}

// TestSetQtyRejectsBadInput covers the error paths of the update route.
func TestSetQtyRejectsBadInput(t *testing.T) {
	carts := newFakeCarts()
	s := newService(carts)
	cartID := openCart(t, s, carts)
	ctx := context.Background()

	if err := s.Add(ctx, cartID, scarceVariant, 1); err != nil {
		t.Fatalf("add: %v", err)
	}
	c, _ := carts.Get(ctx, cartID)
	itemID := c.Items[0].ID

	for _, qty := range []int{0, cart.MaxQty + 1} {
		if err := s.SetQty(ctx, cartID, itemID, qty); !errors.Is(err, cart.ErrCartItemLimit) {
			t.Fatalf("SetQty(%d) = %v, want ErrCartItemLimit", qty, err)
		}
	}
	if err := s.SetQty(ctx, cartID, itemID, 3); !errors.Is(err, catalog.ErrOutOfStock) {
		t.Fatalf("SetQty above availability = %v, want ErrOutOfStock", err)
	}
	if err := s.SetQty(ctx, cartID, uuid.New(), 2); !errors.Is(err, cart.ErrItemNotFound) {
		t.Fatalf("SetQty on a foreign line = %v, want ErrItemNotFound", err)
	}

	if c, _ = carts.Get(ctx, cartID); c.Items[0].Qty != 1 {
		t.Fatalf("rejected updates changed the line to %d", c.Items[0].Qty)
	}
}

// TestViewWithoutACartTouchesNothing pins that browsing never opens a cart: the
// cookie-less visitor gets an empty cart and no row is created for them.
func TestViewWithoutACartTouchesNothing(t *testing.T) {
	carts := newFakeCarts()
	s := newService(carts)

	c, totals, err := s.View(context.Background(), nil)
	if err != nil {
		t.Fatalf("view without a cart: %v", err)
	}
	if !c.IsEmpty() || c.Count() != 0 {
		t.Fatalf("view without a cart = %+v, want an empty cart", c)
	}
	if !totals.Total.IsZero() || !totals.Shipping.IsZero() {
		t.Fatalf("empty cart totals = %+v, want zero", totals)
	}
	if len(carts.items) != 0 {
		t.Fatalf("%d carts were created by a read", len(carts.items))
	}
}
