-- name: CreatePost :one
INSERT INTO posts (
    id, user_id, title, content
) VALUES (
    @id, @user_id, @title, @content
) RETURNING *;

-- name: CreatePostMedia :copyfrom
INSERT INTO post_media (
    id, post_id, media_key, media_type, width, height, sort_order
) VALUES (
    @id, @post_id, @media_key, @media_type, @width, @height, @sort_order
);

-- name: ListPostsFeed :many
SELECT
    p.id, p.title, p.content, p.view_count, p.like_count, p.collect_count,
    p.comment_count, p.created_at,
    u.id AS author_id, u.username AS author_username, u.avatar_url AS author_avatar,
    COALESCE(pm.media_key, '') AS cover_key,
    COALESCE(pm.width, 0) AS width,
    COALESCE(pm.height, 0) AS height
FROM (
    SELECT id, user_id, title, content, view_count, like_count, collect_count,
           comment_count, created_at
    FROM posts
    ORDER BY created_at DESC, id DESC
    LIMIT @limit_count OFFSET @offset_count
) p
JOIN users u ON p.user_id = u.id
LEFT JOIN LATERAL (
    SELECT media_key, width, height
    FROM post_media
    WHERE post_id = p.id
    ORDER BY sort_order ASC
    LIMIT 1
) pm ON true
ORDER BY p.created_at DESC, p.id DESC;

-- name: CountPostsFeed :one
SELECT COUNT(*)::bigint FROM posts;

-- name: GetPostByID :one
SELECT
    p.id, p.user_id, p.title, p.content, p.view_count, p.like_count,
    p.collect_count, p.comment_count, p.created_at, p.updated_at,
    u.id AS author_id, u.username AS author_username, u.avatar_url AS author_avatar
FROM posts p
JOIN users u ON p.user_id = u.id
WHERE p.id = @id LIMIT 1;

-- name: ListPostsByUser :many
SELECT
    p.id, p.title, p.content, p.view_count, p.like_count, p.collect_count,
    p.comment_count, p.created_at,
    u.id AS author_id, u.username AS author_username, u.avatar_url AS author_avatar,
    COALESCE(pm.media_key, '') AS cover_key,
    COALESCE(pm.width, 0) AS width,
    COALESCE(pm.height, 0) AS height
FROM posts p
JOIN users u ON p.user_id = u.id
LEFT JOIN LATERAL (
    SELECT media_key, width, height
    FROM post_media
    WHERE post_id = p.id
    ORDER BY sort_order ASC
    LIMIT 1
) pm ON true
WHERE p.user_id = @user_id
ORDER BY p.created_at DESC, p.id DESC
LIMIT @limit_count OFFSET @offset_count;

-- name: CountPostsByUserID :one
SELECT COUNT(*)::bigint FROM posts WHERE user_id = @user_id;

-- name: GetPostMediaByPostID :many
SELECT id, post_id, media_key, media_type, width, height, sort_order, created_at
FROM post_media
WHERE post_id = @post_id
ORDER BY sort_order;

-- name: DeletePost :exec
DELETE FROM posts WHERE id = @id AND user_id = @user_id;

-- name: UpdatePost :one
UPDATE posts
SET title = @title, content = @content, updated_at = NOW()
WHERE id = @id AND user_id = @user_id
RETURNING *;

-- name: IncrementViewCount :exec
UPDATE posts SET view_count = view_count + 1 WHERE id = @id;

-- name: IncrementPostLikeCount :exec
UPDATE posts SET like_count = like_count + 1 WHERE id = @id;

-- name: DecrementPostLikeCount :exec
UPDATE posts SET like_count = GREATEST(like_count - 1, 0) WHERE id = @id;

-- name: IncrementPostCommentCount :exec
UPDATE posts SET comment_count = comment_count + 1 WHERE id = @id;

-- name: DecrementPostCommentCount :exec
UPDATE posts SET comment_count = GREATEST(comment_count - 1, 0) WHERE id = @id;

-- name: RecalculatePostCommentCount :exec
UPDATE posts
SET comment_count = (SELECT COUNT(*)::bigint FROM comments WHERE post_id = @post_id)
WHERE id = @post_id;

-- name: IncrementPostCollectCount :exec
UPDATE posts SET collect_count = collect_count + 1 WHERE id = @id;

-- name: DecrementPostCollectCount :exec
UPDATE posts SET collect_count = GREATEST(collect_count - 1, 0) WHERE id = @id;

-- name: DeletePostMediaByPostID :exec
DELETE FROM post_media WHERE post_id = @post_id;
