-- name: PurgeDemoData :exec
TRUNCATE payment_events, payments, order_items, orders, cart_items, carts,
         notifications, telegram_links, telegram_updates, events, visits,
         price_history, product_variants, products RESTART IDENTITY CASCADE;

-- name: InsertProduct :one
INSERT INTO products (slug, title, description, price_cents, currency,
                      image_front, image_back, is_active, sort_order)
VALUES ($1, $2, $3, $4, $5, $6, $7, true, $8)
RETURNING id;

-- name: InsertVariant :exec
INSERT INTO product_variants (product_id, size, sku, stock, reserved)
VALUES ($1, $2, $3, $4, 0);

-- name: InsertPriceHistory :exec
INSERT INTO price_history (product_id, old_price_cents, new_price_cents, reason, created_at)
VALUES ($1, $2, $3, $4, $5);

-- name: InsertOrder :one
INSERT INTO orders (number, public_token, tg_link_code, status,
                    subtotal_cents, shipping_cents, total_cents, currency,
                    customer_name, customer_contact, shipping_address,
                    visitor_id, first_touch, last_touch, paid_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $16)
RETURNING id;

-- name: InsertOrderItem :exec
INSERT INTO order_items (order_id, variant_id, product_title, size, unit_price_cents, qty)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListVariantIDs :many
SELECT v.id, v.product_id, v.size, p.title, p.price_cents
FROM product_variants v
JOIN products p ON p.id = v.product_id
ORDER BY p.sort_order, v.size;
