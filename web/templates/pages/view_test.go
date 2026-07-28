package pages

import (
	"testing"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/domain/catalog"
	"github.com/qzq-kiim/shop/web/templates/components"
)

// TestSizeAvailability pins the S1.2 rule: a size is sold out when nothing is
// left to sell, and reserved units count as gone.
func TestSizeAvailability(t *testing.T) {
	tests := []struct {
		name      string
		stock     int
		reserved  int
		wantLabel string
		wantTone  components.Tone
	}{
		{"empty", 0, 0, "sold out", components.ToneDanger},
		{"fully reserved", 5, 5, "sold out", components.ToneDanger},
		{"over reserved", 2, 3, "sold out", components.ToneDanger},
		{"last one", 1, 0, "low stock", components.ToneWarning},
		{"at the threshold", 4, 1, "low stock", components.ToneWarning},
		{"plenty", 12, 2, "in stock", components.ToneSuccess},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sizeAvailability(catalog.Variant{Stock: tt.stock, Reserved: tt.reserved})
			if got.Label != tt.wantLabel || got.Tone != tt.wantTone {
				t.Fatalf("stock=%d reserved=%d -> %q/%s, want %q/%s",
					tt.stock, tt.reserved, got.Label, got.Tone, tt.wantLabel, tt.wantTone)
			}
		})
	}
}

// TestSizeOptionsDisableSoldOutSizes pins the other half of S1.2: a size with
// nothing available cannot be picked and says why.
func TestSizeOptionsDisableSoldOutSizes(t *testing.T) {
	product := catalog.Product{Variants: []catalog.Variant{
		{ID: uuid.New(), Size: "S", Stock: 4},
		{ID: uuid.New(), Size: "M", Stock: 3, Reserved: 3},
		{ID: uuid.New(), Size: "L", Stock: 0},
	}}

	options := sizeOptions(product)
	if len(options) != len(product.Variants) {
		t.Fatalf("size options = %d, want %d", len(options), len(product.Variants))
	}

	want := map[string]bool{"S": false, "M": true, "L": true}
	for _, opt := range options {
		disabled, known := want[opt.Label]
		if !known {
			t.Fatalf("unexpected size %q", opt.Label)
		}
		if opt.Disabled != disabled {
			t.Errorf("size %q disabled = %v, want %v", opt.Label, opt.Disabled, disabled)
		}
		if disabled && opt.Note != "sold out" {
			t.Errorf("size %q note = %q, want %q", opt.Label, opt.Note, "sold out")
		}
	}
}

// TestInStockFollowsAvailability guards the whole-product flag the page uses to
// choose between the add form and the sold out notice.
func TestInStockFollowsAvailability(t *testing.T) {
	soldOut := catalog.Product{Variants: []catalog.Variant{
		{Size: "S", Stock: 2, Reserved: 2},
		{Size: "M", Stock: 0},
	}}
	if inStock(soldOut) {
		t.Error("a product whose every size is taken must not be in stock")
	}

	partial := soldOut
	partial.Variants = append(append([]catalog.Variant{}, soldOut.Variants...), catalog.Variant{Size: "L", Stock: 1})
	if !inStock(partial) {
		t.Error("one available size is enough for the product to be in stock")
	}
}
