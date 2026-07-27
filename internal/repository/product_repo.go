package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qzq-kiim/shop/internal/model"
)

type ProductRepo struct {
	db *pgxpool.Pool
}

func NewProductRepo(db *pgxpool.Pool) *ProductRepo {
	return &ProductRepo{db: db}
}

func (r *ProductRepo) GetAll(ctx context.Context) ([]*model.Product, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, slug, name, description, price, currency, is_active, created_at
		 FROM products WHERE is_active = TRUE
		 ORDER BY CASE slug WHEN 'white' THEN 1 WHEN 'black' THEN 2 WHEN 'besh' THEN 3 ELSE 4 END`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []*model.Product
	for rows.Next() {
		p := &model.Product{}
		if err := rows.Scan(&p.ID, &p.Slug, &p.Name, &p.Description,
			&p.Price, &p.Currency, &p.IsActive, &p.CreatedAt); err != nil {
			return nil, err
		}
		products = append(products, p)
	}

	for _, p := range products {
		if err := r.loadImages(ctx, p); err != nil {
			return nil, err
		}
		if err := r.loadVariants(ctx, p); err != nil {
			return nil, err
		}
	}
	return products, nil
}

func (r *ProductRepo) GetBySlug(ctx context.Context, slug string) (*model.Product, error) {
	p := &model.Product{}
	err := r.db.QueryRow(ctx,
		`SELECT id, slug, name, description, price, currency, is_active, created_at
		 FROM products WHERE slug = $1 AND is_active = TRUE`, slug).
		Scan(&p.ID, &p.Slug, &p.Name, &p.Description,
			&p.Price, &p.Currency, &p.IsActive, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err := r.loadImages(ctx, p); err != nil {
		return nil, err
	}
	if err := r.loadVariants(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (r *ProductRepo) GetVariantByID(ctx context.Context, id int) (*model.ProductVariant, error) {
	v := &model.ProductVariant{}
	err := r.db.QueryRow(ctx,
		`SELECT id, product_id, size, stock FROM product_variants WHERE id = $1`, id).
		Scan(&v.ID, &v.ProductID, &v.Size, &v.Stock)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (r *ProductRepo) loadImages(ctx context.Context, p *model.Product) error {
	rows, err := r.db.Query(ctx,
		`SELECT id, product_id, side, filename, sort_order
		 FROM product_images WHERE product_id = $1 ORDER BY sort_order`, p.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		img := model.ProductImage{}
		if err := rows.Scan(&img.ID, &img.ProductID, &img.Side, &img.Filename, &img.SortOrder); err != nil {
			return err
		}
		p.Images = append(p.Images, img)
	}
	return nil
}

func (r *ProductRepo) loadVariants(ctx context.Context, p *model.Product) error {
	rows, err := r.db.Query(ctx,
		`SELECT id, product_id, size, stock
		 FROM product_variants WHERE product_id = $1 ORDER BY ARRAY_POSITION(ARRAY['S','M','L','XL'], size)`, p.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		v := model.ProductVariant{}
		if err := rows.Scan(&v.ID, &v.ProductID, &v.Size, &v.Stock); err != nil {
			return err
		}
		p.Variants = append(p.Variants, v)
	}
	return nil
}
