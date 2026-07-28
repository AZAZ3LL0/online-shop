-- The primary key is the whole deduplication rule: a redelivered update touches
-- no row and reports zero, so the handler can leave without any side effect.
-- name: InsertTelegramUpdate :execrows
INSERT INTO telegram_updates (update_id)
VALUES ($1)
ON CONFLICT (update_id) DO NOTHING;

-- name: GetOrderByLinkCode :one
SELECT id, number, status
FROM orders
WHERE tg_link_code = $1;

-- name: InsertTelegramLink :execrows
INSERT INTO telegram_links (order_id, chat_id, username)
VALUES ($1, $2, $3)
ON CONFLICT (order_id, chat_id) DO NOTHING;

-- name: ListOrdersByChat :many
SELECT o.id, o.number, o.status
FROM orders o
JOIN telegram_links l ON l.order_id = o.id
WHERE l.chat_id = $1
ORDER BY o.created_at DESC;
