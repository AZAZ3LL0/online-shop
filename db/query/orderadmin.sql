-- Queries behind the admin order list and card (tech.md §8.4).

-- Every filter is optional and applied only when its argument is not null, so
-- one query serves the whole filter bar. The total travels in a window count:
-- the page and the number of pages come out of a single round trip, and the
-- WHERE clause exists exactly once.
-- name: ListAdminOrders :many
SELECT o.id, o.number, o.status, o.total_cents, o.currency,
       o.customer_name, o.customer_contact, o.created_at, o.paid_at,
       (SELECT coalesce(sum(i.qty), 0) FROM order_items i WHERE i.order_id = o.id)::int AS units,
       count(*) OVER ()::bigint AS total_rows
FROM orders o
WHERE (sqlc.narg(status)::text IS NULL OR o.status = sqlc.narg(status)::text)
  AND (sqlc.narg(created_from)::timestamptz IS NULL OR o.created_at >= sqlc.narg(created_from)::timestamptz)
  AND (sqlc.narg(created_to)::timestamptz IS NULL OR o.created_at < sqlc.narg(created_to)::timestamptz)
  AND (sqlc.narg(number)::text IS NULL OR o.number ILIKE '%' || sqlc.narg(number)::text || '%')
  AND (sqlc.narg(product_id)::uuid IS NULL OR EXISTS (
        SELECT 1
        FROM order_items i
        JOIN product_variants v ON v.id = i.variant_id
        WHERE i.order_id = o.id AND v.product_id = sqlc.narg(product_id)::uuid))
ORDER BY o.created_at DESC
LIMIT sqlc.arg(page_size)::int OFFSET sqlc.arg(page_offset)::int;

-- name: GetAdminOrder :one
SELECT id, number, public_token, tg_link_code, status,
       subtotal_cents, shipping_cents, total_cents, currency,
       customer_name, customer_contact, shipping_address, comment,
       first_touch, last_touch,
       expires_at, paid_at, shipped_at, cancelled_at, created_at
FROM orders
WHERE id = $1;

-- The raw provider log of one order, newest first (tech.md §8.4). Events whose
-- signature failed never reach a payment row and so never reach a card.
-- name: ListPaymentEventsByOrder :many
SELECT e.provider_status, e.signature_ok, e.received_at,
       p.provider_payment_id, p.pay_currency, p.pay_amount, p.actually_paid
FROM payment_events e
JOIN payments p ON p.id = e.payment_id
WHERE p.order_id = $1
ORDER BY e.received_at DESC;

-- name: ListTelegramLinksByOrder :many
SELECT chat_id, username, linked_at
FROM telegram_links
WHERE order_id = $1
ORDER BY linked_at;
