-- name: EnqueueNotification :execrows
INSERT INTO notifications (order_id, chat_id, kind, payload, dedup_key, status, next_attempt_at)
VALUES ($1, $2, $3, $4, $5, 'pending', now())
ON CONFLICT (dedup_key) DO NOTHING;

-- Queued for whoever follows the order in Telegram. An order nobody tracks
-- selects no rows, which is exactly right: there is no one to tell. The dedup
-- key is order_id|kind|status (tech.md §4), so one transition yields one message.
-- name: EnqueueOrderNotification :execrows
INSERT INTO notifications (order_id, chat_id, kind, payload, dedup_key, status, next_attempt_at)
SELECT $1, l.chat_id, $2, $3, $4, 'pending', now()
FROM telegram_links l
WHERE l.order_id = $1
ON CONFLICT (dedup_key) DO NOTHING;

-- name: ClaimDueNotifications :many
SELECT id, order_id, chat_id, kind, payload, attempts
FROM notifications
WHERE status = 'pending' AND next_attempt_at <= $1
ORDER BY next_attempt_at
LIMIT $2
FOR UPDATE SKIP LOCKED;

-- name: MarkNotificationSent :exec
UPDATE notifications
SET status = 'sent', sent_at = $2, attempts = attempts + 1, last_error = NULL
WHERE id = $1;

-- name: MarkNotificationRetry :exec
UPDATE notifications
SET attempts = attempts + 1, next_attempt_at = $2, last_error = $3
WHERE id = $1;

-- name: MarkNotificationFailed :exec
UPDATE notifications
SET status = 'failed', attempts = attempts + 1, last_error = $2
WHERE id = $1;

-- name: CountNotificationsByStatus :one
SELECT count(*) FROM notifications WHERE status = $1;
