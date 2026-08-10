-- name: CreateComment :one
INSERT INTO comments (
    id, post_id, user_id, parent_id, content
) VALUES (
    @id, @post_id, @user_id, @parent_id, @content
) RETURNING *;

-- name: GetCommentByID :one
SELECT * FROM comments WHERE id = @id LIMIT 1;

-- name: DeleteComment :execrows
DELETE FROM comments WHERE id = @id AND user_id = @user_id;

-- name: IncrementCommentLikeCount :exec
UPDATE comments SET like_count = like_count + 1 WHERE id = @id;

-- name: DecrementCommentLikeCount :exec
UPDATE comments SET like_count = GREATEST(like_count - 1, 0) WHERE id = @id;

-- name: ListCommentsByPostID :many
SELECT
    c.id, c.post_id, c.user_id, c.parent_id, c.content, c.like_count, c.created_at,
    u.username AS author_username, u.avatar_url AS author_avatar
FROM comments c
JOIN users u ON c.user_id = u.id
WHERE c.post_id = @post_id
ORDER BY c.created_at ASC
LIMIT @limit_count OFFSET @offset_count;
