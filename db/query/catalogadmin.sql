-- Queries behind the admin product page (tech.md §8.3).

-- The panel sees deactivated models too: they still carry stock and history.
-- name: ListProductsForAdmin :many
SELECT id, slug, title, description, price_cents, currency,
       image_front, image_back, is_active, sort_order
FROM products
ORDER BY sort_order, created_at;

-- name: LockProductForPrice :one
SELECT price_cents, currency
FROM products
WHERE id = $1
FOR UPDATE;

-- name: SetProductPrice :execrows
UPDATE products
SET price_cents = $2, updated_at = now()
WHERE id = $1;

-- name: InsertPriceChange :exec
INSERT INTO price_history (product_id, old_price_cents, new_price_cents, changed_by, reason)
VALUES ($1, $2, $3, $4, $5);

-- name: ListPriceHistory :many
SELECT h.old_price_cents, h.new_price_cents, h.reason, h.created_at, u.login, p.currency
FROM price_history h
JOIN products p ON p.id = h.product_id
LEFT JOIN admin_users u ON u.id = h.changed_by
WHERE h.product_id = $1
ORDER BY h.created_at DESC
LIMIT $2;

-- The stock never lands below what is already reserved, so a size with units in
-- a live checkout cannot be sold twice: no matching row means the edit is
-- refused rather than clamped (tech.md §4).
-- name: SetVariantStock :execrows
UPDATE product_variants
SET stock = sqlc.arg(stock)::int
WHERE id = sqlc.arg(variant_id) AND sqlc.arg(stock)::int >= reserved;

-- name: GetVariantProductID :one
SELECT product_id
FROM product_variants
WHERE id = $1;
