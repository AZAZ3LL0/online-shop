-- name: InsertVisit :exec
INSERT INTO visits (visitor_id, session_id, landing_path, referrer,
                    utm_source, utm_medium, utm_campaign, utm_content, utm_term,
                    user_agent, is_bot, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- name: InsertEvent :exec
INSERT INTO events (visitor_id, session_id, type, payload, created_at)
VALUES ($1, $2, $3, $4, $5);

-- InsertOrderPaidEvent closes the funnel from the provider callback, which
-- carries no cookies of its own. The event is filed against the last session the
-- buyer was seen in, so it lines up with the earlier steps of the same visitor.
-- An order whose visitor never landed a visit produces no event and is simply
-- not part of the funnel.
-- name: InsertOrderPaidEvent :exec
INSERT INTO events (visitor_id, session_id, type, payload, created_at)
SELECT v.visitor_id, v.session_id, 'order_paid', @payload::jsonb, now()
FROM visits v
JOIN orders o ON o.visitor_id = v.visitor_id
WHERE o.id = @order_id
ORDER BY v.created_at DESC
LIMIT 1;
