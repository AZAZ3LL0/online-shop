-- name: EnqueueNotification :execrows
INSERT INTO notifications (order_id, chat_id, kind, payload, dedup_key, status, next_attempt_at)
VALUES ($1, $2, $3, $4, $5, 'pending', now())
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
