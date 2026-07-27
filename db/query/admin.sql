-- name: UpsertAdminUser :one
INSERT INTO admin_users (login, password_hash)
VALUES ($1, $2)
ON CONFLICT (login) DO UPDATE SET password_hash = EXCLUDED.password_hash
RETURNING id;

-- name: GetAdminByLogin :one
SELECT id, login, password_hash, telegram_id
FROM admin_users
WHERE login = $1;

-- name: SetAdminLastLogin :exec
UPDATE admin_users SET last_login_at = now() WHERE id = $1;

-- name: CreateAdminSession :one
INSERT INTO admin_sessions (admin_id, ip, user_agent, expires_at)
VALUES ($1, NULLIF(@ip::text, '')::inet, $2, $3)
RETURNING id;

-- name: GetAdminSession :one
SELECT s.id, s.admin_id, s.expires_at, u.login
FROM admin_sessions s
JOIN admin_users u ON u.id = s.admin_id
WHERE s.id = $1 AND s.expires_at > now();

-- name: DeleteAdminSession :exec
DELETE FROM admin_sessions WHERE id = $1;

-- name: DeleteExpiredAdminSessions :exec
DELETE FROM admin_sessions WHERE expires_at <= now();
