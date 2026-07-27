-- name: CreateCart :one
INSERT INTO carts (visitor_id)
VALUES ($1)
RETURNING id;

-- name: CartExists :one
SELECT EXISTS (SELECT 1 FROM carts WHERE id = $1);

-- name: TouchCart :exec
UPDATE carts SET updated_at = now() WHERE id = $1;

-- name: GetCartItemByVariant :one
SELECT id, qty FROM cart_items WHERE cart_id = $1 AND variant_id = $2;

-- name: InsertCartItem :exec
INSERT INTO cart_items (cart_id, variant_id, qty)
VALUES ($1, $2, $3)
ON CONFLICT (cart_id, variant_id) DO UPDATE SET qty = EXCLUDED.qty;

-- name: UpdateCartItemQty :execrows
UPDATE cart_items SET qty = $3 WHERE id = $2 AND cart_id = $1;

-- name: DeleteCartItem :execrows
DELETE FROM cart_items WHERE id = $2 AND cart_id = $1;

-- name: ListCartItems :many
SELECT ci.id, ci.variant_id, ci.qty,
       v.size, v.stock, v.reserved,
       p.slug, p.title, p.price_cents, p.currency, p.image_front
FROM cart_items ci
JOIN product_variants v ON v.id = ci.variant_id
JOIN products p ON p.id = v.product_id
WHERE ci.cart_id = $1
ORDER BY p.sort_order, array_position(ARRAY['S', 'M', 'L', 'XL', 'XXL']::text[], v.size);
