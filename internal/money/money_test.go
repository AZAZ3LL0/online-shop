package money_test

import (
	"errors"
	"math/rand"
	"testing"

	"github.com/qzq-kiim/shop/internal/money"
)

func TestAddRejectsCurrencyMismatch(t *testing.T) {
	_, err := money.New(100, "USD").Add(money.New(100, "EUR"))
	if !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Fatalf("want ErrCurrencyMismatch, got %v", err)
	}
}

func TestString(t *testing.T) {
	cases := []struct {
		in   money.Amount
		want string
	}{
		{money.New(1200, "USD"), "$12.00"},
		{money.New(5, "USD"), "$0.05"},
		{money.New(0, "USD"), "$0.00"},
		{money.New(-1250, "USD"), "$-12.50"},
		{money.New(1234, "EUR"), "12.34 EUR"},
	}
	for _, c := range cases {
		if got := c.in.String(); got != c.want {
			t.Errorf("String(%d %s) = %q, want %q", c.in.Cents, c.in.Currency, got, c.want)
		}
	}
}

// Sum of line totals must equal the sum of parts for any random basket.
func TestSumInvariant(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for range 200 {
		total := money.Zero("USD")
		var want int64
		for range rng.Intn(12) + 1 {
			unit := int64(rng.Intn(20000))
			qty := int64(rng.Intn(10) + 1)
			line := money.New(unit, "USD").Mul(qty)
			want += unit * qty
			var err error
			total, err = total.Add(line)
			if err != nil {
				t.Fatalf("add: %v", err)
			}
		}
		if total.Cents != want {
			t.Fatalf("total = %d, want %d", total.Cents, want)
		}
	}
}
