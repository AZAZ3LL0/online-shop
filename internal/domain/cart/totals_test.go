package cart_test

import (
	"context"
	"errors"
	"math/rand/v2"
	"testing"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/domain/cart"
	"github.com/qzq-kiim/shop/internal/money"
)

// randomCart builds a cart of random lines within the limits the routes
// enforce: 1..10 units of a size, prices anywhere from a cent to $10,000.
func randomCart(r *rand.Rand, lines int) cart.Cart {
	c := cart.Cart{ID: uuid.New()}
	for range lines {
		c.Items = append(c.Items, cart.Item{
			ID:        uuid.New(),
			VariantID: uuid.New(),
			UnitPrice: money.New(r.Int64N(1_000_000)+1, "USD"),
			Qty:       r.IntN(cart.MaxQty) + 1,
		})
	}
	return c
}

// TestSubtotalEqualsTheSumOfTheLines is the S2.2 invariant: for any set of
// lines, what the cart says equals what the lines say, computed independently
// of the production sum.
func TestSubtotalEqualsTheSumOfTheLines(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))

	for run := range 500 {
		c := randomCart(r, r.IntN(12))

		var want int64
		for _, item := range c.Items {
			want += item.UnitPrice.Cents * int64(item.Qty)
		}

		got, err := c.Subtotal("USD")
		if err != nil {
			t.Fatalf("run %d: subtotal: %v", run, err)
		}
		if got.Cents != want {
			t.Fatalf("run %d: subtotal = %d, want %d (lines: %+v)", run, got.Cents, want, c.Items)
		}
		if got.Currency != "USD" {
			t.Fatalf("run %d: subtotal currency = %q", run, got.Currency)
		}
	}
}

// TestCountEqualsTheUnitsInTheCart pins the header badge against the same set
// of random carts.
func TestCountEqualsTheUnitsInTheCart(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))

	for run := range 200 {
		c := randomCart(r, r.IntN(12))

		want := 0
		for _, item := range c.Items {
			want += item.Qty
		}
		if got := c.Count(); got != want {
			t.Fatalf("run %d: count = %d, want %d", run, got, want)
		}
		if c.IsEmpty() != (len(c.Items) == 0) {
			t.Fatalf("run %d: IsEmpty disagrees with the lines", run)
		}
	}
}

// TestTotalIsSubtotalPlusShipping is the other half of the S2.2 invariant: the
// flat fee from settings is added once, whatever the cart holds.
func TestTotalIsSubtotalPlusShipping(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	ctx := context.Background()

	for run := range 300 {
		fee := int64(r.IntN(2000))
		s := cart.NewService(newFakeCarts(), fakeCatalog{}, "USD", fixedRates(fee))
		c := randomCart(r, r.IntN(12))

		totals, err := s.Totals(ctx, c)
		if err != nil {
			t.Fatalf("run %d: totals: %v", run, err)
		}

		subtotal, err := c.Subtotal("USD")
		if err != nil {
			t.Fatalf("run %d: subtotal: %v", run, err)
		}
		if totals.Subtotal != subtotal {
			t.Fatalf("run %d: totals carry %v, the lines sum to %v", run, totals.Subtotal, subtotal)
		}

		wantShipping := fee
		if c.IsEmpty() {
			wantShipping = 0
		}
		if totals.Shipping.Cents != wantShipping {
			t.Fatalf("run %d: shipping = %d, want %d for %d lines",
				run, totals.Shipping.Cents, wantShipping, len(c.Items))
		}
		if totals.Total.Cents != totals.Subtotal.Cents+totals.Shipping.Cents {
			t.Fatalf("run %d: total %d != subtotal %d + shipping %d",
				run, totals.Total.Cents, totals.Subtotal.Cents, totals.Shipping.Cents)
		}
		for _, amount := range []money.Amount{totals.Subtotal, totals.Shipping, totals.Total} {
			if amount.Currency != "USD" {
				t.Fatalf("run %d: %v left the shop currency", run, amount)
			}
		}
	}
}

// TestShippingComesFromSettings covers the edge values the settings key can
// hold, including one nobody should ever write.
func TestShippingComesFromSettings(t *testing.T) {
	tests := []struct {
		name string
		fee  int64
		want int64
	}{
		{"free", 0, 0},
		{"flat fee", 599, 599},
		{"negative is never a discount", -500, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := cart.NewService(newFakeCarts(), fakeCatalog{}, "USD", fixedRates(tt.fee))
			c := cart.Cart{Items: []cart.Item{{UnitPrice: money.New(3500, "USD"), Qty: 2}}}

			totals, err := s.Totals(context.Background(), c)
			if err != nil {
				t.Fatalf("totals: %v", err)
			}
			if totals.Shipping.Cents != tt.want {
				t.Fatalf("shipping = %d, want %d", totals.Shipping.Cents, tt.want)
			}
			if totals.Total.Cents != 7000+tt.want {
				t.Fatalf("total = %d, want %d", totals.Total.Cents, 7000+tt.want)
			}
		})
	}
}

// TestSubtotalRefusesMixedCurrencies keeps a broken catalog from producing a
// plausible-looking wrong number.
func TestSubtotalRefusesMixedCurrencies(t *testing.T) {
	c := cart.Cart{Items: []cart.Item{
		{UnitPrice: money.New(3500, "USD"), Qty: 1},
		{UnitPrice: money.New(3500, "EUR"), Qty: 1},
	}}

	if _, err := c.Subtotal("USD"); !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Fatalf("mixed currencies = %v, want ErrCurrencyMismatch", err)
	}
}

// TestEmptyCartIsFree pins the one case the storefront shows most often.
func TestEmptyCartIsFree(t *testing.T) {
	s := cart.NewService(newFakeCarts(), fakeCatalog{}, "USD", fixedRates(1500))

	totals, err := s.Totals(context.Background(), cart.Cart{})
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	if !totals.Subtotal.IsZero() || !totals.Shipping.IsZero() || !totals.Total.IsZero() {
		t.Fatalf("empty cart = %+v, want zero throughout", totals)
	}
}
