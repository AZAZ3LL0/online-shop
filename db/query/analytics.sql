-- name: InsertVisit :exec
INSERT INTO visits (visitor_id, session_id, landing_path, referrer,
                    utm_source, utm_medium, utm_campaign, utm_content, utm_term,
                    user_agent, is_bot, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- name: InsertEvent :exec
INSERT INTO events (visitor_id, session_id, type, payload, created_at)
VALUES ($1, $2, $3, $4, $5);
