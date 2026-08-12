-- name: CreateAPIKey :one
INSERT INTO api_keys (id, user_id, name, key_prefix, key_hash)
VALUES (@id, @user_id, @name, @key_prefix, @key_hash)
RETURNING *;

-- name: GetActiveAPIKeyByHash :one
SELECT * FROM api_keys
WHERE key_hash = @key_hash AND revoked_at IS NULL
LIMIT 1;

-- name: ListAPIKeysByUser :many
SELECT * FROM api_keys
WHERE user_id = @user_id
ORDER BY created_at DESC, id DESC;

-- name: RevokeAPIKey :execrows
UPDATE api_keys
SET revoked_at = NOW()
WHERE id = @id AND user_id = @user_id AND revoked_at IS NULL;

-- name: TouchAPIKey :exec
UPDATE api_keys
SET last_used_at = NOW()
WHERE id = @id AND revoked_at IS NULL;
