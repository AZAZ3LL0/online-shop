package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/qzq-kiim/shop/internal/domain/catalog"
	"github.com/qzq-kiim/shop/internal/money"
	"github.com/qzq-kiim/shop/internal/storage/postgres/sqlcgen"
)

// CatalogRepo reads products and variants.
type CatalogRepo struct {
	q *sqlcgen.Queries
}

// NewCatalogRepo returns the catalog repository bound to the store.
func NewCatalogRepo(s *Store) *CatalogRepo { return &CatalogRepo{q: s.q} }

var _ catalog.Repository = (*CatalogRepo)(nil)

// ListActive returns active products with their variants, in sort order.
func (r *CatalogRepo) ListActive(ctx context.Context) ([]catalog.Product, error) {
	rows, err := r.q.ListActiveProducts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active products: %w", err)
	}

	products := make([]catalog.Product, 0, len(rows))
	ids := make([]uuid.UUID, 0, len(rows))
	index := make(map[uuid.UUID]int, len(rows))
	for i, row := range rows {
		products = append(products, catalog.Product{
			ID:          row.ID,
			Slug:        row.Slug,
			Title:       row.Title,
			Description: row.Description,
			Price:       money.New(row.PriceCents, row.Currency),
			ImageFront:  row.ImageFront,
			ImageBack:   row.ImageBack,
			IsActive:    row.IsActive,
		})
		ids = append(ids, row.ID)
		index[row.ID] = i
	}
	if len(ids) == 0 {
		return products, nil
	}

	variants, err := r.q.ListVariantsByProductIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list variants: %w", err)
	}
	for _, v := range variants {
		i, ok := index[v.ProductID]
		if !ok {
			continue
		}
		products[i].Variants = append(products[i].Variants, catalog.Variant{
			ID:       v.ID,
			Size:     v.Size,
			SKU:      v.Sku,
			Stock:    int(v.Stock),
			Reserved: int(v.Reserved),
		})
	}
	return products, nil
}

// BySlug returns one active product with its variants.
func (r *CatalogRepo) BySlug(ctx context.Context, slug string) (catalog.Product, error) {
	row, err := r.q.GetProductBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return catalog.Product{}, fmt.Errorf("product %q: %w", slug, catalog.ErrNotFound)
		}
		return catalog.Product{}, fmt.Errorf("get product by slug: %w", err)
	}

	product := catalog.Product{
		ID:          row.ID,
		Slug:        row.Slug,
		Title:       row.Title,
		Description: row.Description,
		Price:       money.New(row.PriceCents, row.Currency),
		ImageFront:  row.ImageFront,
		ImageBack:   row.ImageBack,
		IsActive:    row.IsActive,
	}

	variants, err := r.q.ListVariantsByProductIDs(ctx, []uuid.UUID{row.ID})
	if err != nil {
		return catalog.Product{}, fmt.Errorf("list variants: %w", err)
	}
	for _, v := range variants {
		product.Variants = append(product.Variants, catalog.Variant{
			ID:       v.ID,
			Size:     v.Size,
			SKU:      v.Sku,
			Stock:    int(v.Stock),
			Reserved: int(v.Reserved),
		})
	}
	return product, nil
}

// VariantByID returns a variant together with the product it belongs to.
func (r *CatalogRepo) VariantByID(ctx context.Context, id uuid.UUID) (catalog.Variant, catalog.Product, error) {
	row, err := r.q.GetVariantWithProduct(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return catalog.Variant{}, catalog.Product{}, fmt.Errorf("variant %s: %w", id, catalog.ErrNotFound)
		}
		return catalog.Variant{}, catalog.Product{}, fmt.Errorf("get variant: %w", err)
	}
	variant := catalog.Variant{
		ID:       row.ID,
		Size:     row.Size,
		SKU:      row.Sku,
		Stock:    int(row.Stock),
		Reserved: int(row.Reserved),
	}
	product := catalog.Product{
		ID:          row.ProductID,
		Slug:        row.Slug,
		Title:       row.Title,
		Description: row.Description,
		Price:       money.New(row.PriceCents, row.Currency),
		ImageFront:  row.ImageFront,
		ImageBack:   row.ImageBack,
		IsActive:    row.IsActive,
	}
	return variant, product, nil
}
