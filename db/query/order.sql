-- name: InsertCheckoutOrder :one
INSERT INTO orders (number, public_token, tg_link_code, status,
                    subtotal_cents, shipping_cents, total_cents, currency,
                    customer_name, customer_contact, shipping_address, comment,
                    visitor_id, first_touch, last_touch, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING id, created_at;

-- name: GetOrderByPublicToken :one
SELECT id, number, public_token, tg_link_code, status,
       subtotal_cents, shipping_cents, total_cents, currency,
       customer_name, customer_contact, shipping_address, comment,
       expires_at, paid_at, created_at
FROM orders
WHERE public_token = $1;

-- name: GetOrderByNumber :one
SELECT id, number, public_token, tg_link_code, status,
       subtotal_cents, shipping_cents, total_cents, currency,
       customer_name, customer_contact, shipping_address, comment,
       expires_at, paid_at, created_at
FROM orders
WHERE number = $1;

-- name: ListOrderItemsByOrder :many
SELECT variant_id, product_title, size, unit_price_cents, qty
FROM order_items
WHERE order_id = $1
ORDER BY id;

-- name: LockOrderByNumber :one
SELECT id, status
FROM orders
WHERE number = $1
FOR UPDATE;

-- name: LockOrderByID :one
SELECT id, number, status
FROM orders
WHERE id = $1
FOR UPDATE;

-- name: SetOrderStatus :execrows
UPDATE orders
SET status = $2,
    updated_at = now(),
    paid_at = CASE WHEN $2 = 'paid' THEN now() ELSE paid_at END,
    cancelled_at = CASE WHEN $2 = 'cancelled' THEN now() ELSE cancelled_at END
WHERE id = $1 AND status = $3;

-- name: ReserveVariant :execrows
UPDATE product_variants
SET reserved = reserved + sqlc.arg(qty)::int
WHERE id = sqlc.arg(variant_id) AND stock - reserved >= sqlc.arg(qty)::int;

-- The releases clamp at zero so that replaying a release can never push the
-- columns below their check constraints: stock - reserved >= 0 always holds.
-- name: ReleaseVariant :execrows
UPDATE product_variants
SET reserved = reserved - LEAST(reserved, sqlc.arg(qty)::int)
WHERE id = sqlc.arg(variant_id);

-- name: CommitVariantStock :execrows
UPDATE product_variants
SET stock = stock - LEAST(stock, sqlc.arg(qty)::int),
    reserved = reserved - LEAST(reserved, sqlc.arg(qty)::int)
WHERE id = sqlc.arg(variant_id);

-- name: ListDueOrders :many
SELECT id, number
FROM orders
WHERE status = 'awaiting_payment'
  AND expires_at IS NOT NULL
  AND expires_at < $1
ORDER BY expires_at
LIMIT $2
FOR UPDATE SKIP LOCKED;

-- A provider callback opens its own payment row without an invoice url, so the
-- link the buyer can still use is the latest row that actually carries one.
-- name: GetLatestInvoiceURL :one
SELECT invoice_url
FROM payments
WHERE order_id = $1 AND invoice_url IS NOT NULL
ORDER BY created_at DESC
LIMIT 1;
