-- name: CreateUser :one
INSERT INTO users (
    id, username, password_hash, avatar_url, bio
) VALUES (
    @id, @username, @password_hash, @avatar_url, @bio
) RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = @id LIMIT 1;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = @username LIMIT 1;

-- name: UpdateUser :one
UPDATE users SET username = @username, avatar_url = @avatar_url, bio = @bio, updated_at = NOW() WHERE id = @id RETURNING *;

-- name: UpdateUserPassword :execrows
UPDATE users SET password_hash = @password_hash, updated_at = NOW() WHERE id = @id;

-- name: CreateSession :one
INSERT INTO user_sessions (
    id, user_id, refresh_token_hash, expires_at
) VALUES (
    @id, @user_id, @refresh_token_hash, @expires_at
) RETURNING *;

-- name: GetActiveSessionByTokenHash :one
SELECT * FROM user_sessions
WHERE refresh_token_hash = @refresh_token_hash
  AND revoked_at IS NULL
  AND expires_at > NOW()
LIMIT 1;

-- name: RevokeSession :exec
UPDATE user_sessions
SET revoked_at = NOW()
WHERE id = @id AND revoked_at IS NULL;

-- name: RevokeAllUserSessions :exec
UPDATE user_sessions
SET revoked_at = NOW()
WHERE user_id = @user_id AND revoked_at IS NULL;
