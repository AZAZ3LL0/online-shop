// Package catalog holds the product side of the domain: products, size variants
// and the read interface the shop handlers depend on.
package catalog

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/qzq-kiim/shop/internal/money"
)

// ErrOutOfStock is returned when the requested quantity exceeds availability.
var ErrOutOfStock = errors.New("catalog: out of stock")

// ErrNotFound is returned when a product or variant does not exist.
var ErrNotFound = errors.New("catalog: not found")

// Product is one t-shirt model with its size variants.
type Product struct {
	ID          uuid.UUID
	Slug        string
	Title       string
	Description string
	Price       money.Amount
	ImageFront  string
	ImageBack   string
	IsActive    bool
	Variants    []Variant
}

// Variant is one size of a product.
type Variant struct {
	ID       uuid.UUID
	Size     string // S|M|L|XL|XXL
	SKU      string
	Stock    int
	Reserved int
}

// Available reports how many units may still be sold.
func (v Variant) Available() int { return v.Stock - v.Reserved }

// Repository is the read access the shop needs from storage.
type Repository interface {
	ListActive(ctx context.Context) ([]Product, error)
	BySlug(ctx context.Context, slug string) (Product, error)
	VariantByID(ctx context.Context, id uuid.UUID) (Variant, Product, error)
}
