package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qzq-kiim/shop/internal/domain/catalog"
	"github.com/qzq-kiim/shop/internal/money"
	"github.com/qzq-kiim/shop/internal/storage/postgres/sqlcgen"
)

// CatalogAdminRepo writes the catalogue edits the admin panel makes. The
// storefront repository stays read-only.
type CatalogAdminRepo struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

// NewCatalogAdminRepo returns the admin catalogue repository bound to the store.
func NewCatalogAdminRepo(s *Store) *CatalogAdminRepo {
	return &CatalogAdminRepo{pool: s.pool, q: s.q}
}

var _ catalog.AdminRepository = (*CatalogAdminRepo)(nil)

// ListAll returns every product with its sizes, deactivated ones included.
func (r *CatalogAdminRepo) ListAll(ctx context.Context) ([]catalog.Product, error) {
	rows, err := r.q.ListProductsForAdmin(ctx)
	if err != nil {
		return nil, fmt.Errorf("list products for admin: %w", err)
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

// ChangePrice moves the price and records why, in one transaction: the trail in
// price_history is written with the new number or not at all (tech.md §8.3).
func (r *CatalogAdminRepo) ChangePrice(ctx context.Context, change catalog.PriceChange) error {
	return withTx(ctx, r.pool, func(q *sqlcgen.Queries) error {
		locked, err := q.LockProductForPrice(ctx, change.ProductID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("product %s: %w", change.ProductID, catalog.ErrNotFound)
			}
			return fmt.Errorf("lock product: %w", err)
		}

		n, err := q.SetProductPrice(ctx, sqlcgen.SetProductPriceParams{
			ID:         change.ProductID,
			PriceCents: change.NewPrice.Cents,
		})
		if err != nil {
			return fmt.Errorf("set product price: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("product %s: %w", change.ProductID, catalog.ErrNotFound)
		}

		err = q.InsertPriceChange(ctx, sqlcgen.InsertPriceChangeParams{
			ProductID:     change.ProductID,
			OldPriceCents: locked.PriceCents,
			NewPriceCents: change.NewPrice.Cents,
			ChangedBy:     nullUUID(&change.ChangedBy),
			Reason:        nullString(change.Reason),
		})
		if err != nil {
			return fmt.Errorf("insert price change: %w", err)
		}
		return nil
	})
}

// PriceHistory returns the most recent price changes of one product.
func (r *CatalogAdminRepo) PriceHistory(ctx context.Context, productID uuid.UUID, limit int) ([]catalog.PriceEntry, error) {
	rows, err := r.q.ListPriceHistory(ctx, sqlcgen.ListPriceHistoryParams{
		ProductID: productID,
		Limit:     int32of(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list price history: %w", err)
	}
	out := make([]catalog.PriceEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, catalog.PriceEntry{
			OldPrice:  money.New(row.OldPriceCents, row.Currency),
			NewPrice:  money.New(row.NewPriceCents, row.Currency),
			Reason:    derefString(row.Reason),
			ChangedBy: derefString(row.Login),
			CreatedAt: row.CreatedAt.Time,
		})
	}
	return out, nil
}

// SetStock replaces the stock of one size, refusing to leave reserved units
// uncovered.
func (r *CatalogAdminRepo) SetStock(ctx context.Context, variantID uuid.UUID, stock int) error {
	n, err := r.q.SetVariantStock(ctx, sqlcgen.SetVariantStockParams{
		VariantID: variantID,
		Stock:     int32of(stock),
	})
	if err != nil {
		return fmt.Errorf("set variant stock: %w", err)
	}
	if n == 0 {
		// Either the size is gone or the number would cut into a live
		// reservation. Both are refusals, and the second is the interesting one.
		if _, err := r.ProductOfVariant(ctx, variantID); err != nil {
			return err
		}
		return fmt.Errorf("variant %s: %w", variantID, catalog.ErrStockBelowReserved)
	}
	return nil
}

// ProductOfVariant reports which product a size belongs to.
func (r *CatalogAdminRepo) ProductOfVariant(ctx context.Context, variantID uuid.UUID) (uuid.UUID, error) {
	id, err := r.q.GetVariantProductID(ctx, variantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, fmt.Errorf("variant %s: %w", variantID, catalog.ErrNotFound)
		}
		return uuid.Nil, fmt.Errorf("get variant product: %w", err)
	}
	return id, nil
}
