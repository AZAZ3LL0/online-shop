-- name: ListActiveProducts :many
SELECT id, slug, title, description, price_cents, currency, image_front, image_back, is_active, sort_order
FROM products
WHERE is_active = true
ORDER BY sort_order, created_at;

-- name: GetProductBySlug :one
SELECT id, slug, title, description, price_cents, currency, image_front, image_back, is_active, sort_order
FROM products
WHERE slug = $1 AND is_active = true;

-- name: ListVariantsByProductIDs :many
SELECT id, product_id, size, sku, stock, reserved
FROM product_variants
WHERE product_id = ANY (@product_ids::uuid[])
ORDER BY product_id, array_position(ARRAY['S', 'M', 'L', 'XL', 'XXL']::text[], size);

-- name: GetVariantWithProduct :one
SELECT v.id, v.product_id, v.size, v.sku, v.stock, v.reserved,
       p.slug, p.title, p.description, p.price_cents, p.currency,
       p.image_front, p.image_back, p.is_active, p.sort_order
FROM product_variants v
JOIN products p ON p.id = v.product_id
WHERE v.id = $1;
