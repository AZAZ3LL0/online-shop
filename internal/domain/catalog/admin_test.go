package catalog_test

import (
	"errors"
	"testing"

	"github.com/qzq-kiim/shop/internal/domain/catalog"
)

// A price typed by an operator becomes minor units exactly, without ever
// passing through a float (tech.md §6.1).
func TestParsePrice(t *testing.T) {
	cases := []struct {
		in    string
		cents int64
	}{
		{"35", 3500},
		{"35.5", 3550},
		{"35.50", 3550},
		{"0.05", 5},
		{"$42.99", 4299},
		{"42,99", 4299},
		{"  35.00  ", 3500},
		{"999999.99", 99999999},
		{"1000000.00", catalog.MaxPriceCents}, // the cap itself still reads
	}
	for _, c := range cases {
		got, err := catalog.ParsePrice(c.in, "USD")
		if err != nil {
			t.Errorf("ParsePrice(%q) failed: %v", c.in, err)
			continue
		}
		if got.Cents != c.cents {
			t.Errorf("ParsePrice(%q) = %d cents, want %d", c.in, got.Cents, c.cents)
		}
		if got.Currency != "USD" {
			t.Errorf("ParsePrice(%q) currency = %q", c.in, got.Currency)
		}
	}
}

func TestParsePriceRefusesWhatItCannotRead(t *testing.T) {
	for _, in := range []string{"", "   ", "abc", "-5", "35.005", "1e3", "35.", "1000001.00", "3 5"} {
		if _, err := catalog.ParsePrice(in, "USD"); !errors.Is(err, catalog.ErrPriceOutOfRange) {
			t.Errorf("ParsePrice(%q) error = %v, want ErrPriceOutOfRange", in, err)
		}
	}
}
