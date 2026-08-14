-- name: AddPostLike :execrows
INSERT INTO likes (
    id, user_id, target_id, target_type
) VALUES (
    @id, @user_id, @post_id, 1
) ON CONFLICT (user_id, target_id, target_type) DO NOTHING;

-- name: RemovePostLike :execrows
DELETE FROM likes
WHERE user_id = @user_id AND target_id = @post_id AND target_type = 1;

-- name: AddCommentLike :execrows
INSERT INTO likes (
    id, user_id, target_id, target_type
) VALUES (
    @id, @user_id, @comment_id, 2
) ON CONFLICT (user_id, target_id, target_type) DO NOTHING;

-- name: RemoveCommentLike :execrows
DELETE FROM likes
WHERE user_id = @user_id AND target_id = @comment_id AND target_type = 2;

-- name: IsPostLiked :one
SELECT EXISTS(
    SELECT 1 FROM likes
    WHERE user_id = @user_id AND target_id = @post_id AND target_type = 1
) AS liked;

-- name: IsCommentLiked :one
SELECT EXISTS(
    SELECT 1 FROM likes
    WHERE user_id = @user_id AND target_id = @comment_id AND target_type = 2
) AS liked;

-- name: ListLikedPosts :many
SELECT
    p.id, p.title, p.content, p.view_count, p.like_count, p.collect_count,
    p.comment_count, p.created_at,
    u.id AS author_id, u.username AS author_username, u.avatar_url AS author_avatar,
    COALESCE(pm.media_key, '') AS cover_key,
    COALESCE(pm.width, 0) AS width,
    COALESCE(pm.height, 0) AS height,
    l.created_at AS liked_at
FROM likes l
JOIN posts p ON p.id = l.target_id
JOIN users u ON p.user_id = u.id
LEFT JOIN LATERAL (
    SELECT media_key, width, height
    FROM post_media
    WHERE post_id = p.id
    ORDER BY sort_order ASC
    LIMIT 1
) pm ON true
WHERE l.user_id = @user_id AND l.target_type = 1
ORDER BY l.created_at DESC, l.target_id DESC
LIMIT @limit_count OFFSET @offset_count;

-- name: CountLikedPosts :one
SELECT COUNT(*)::bigint
FROM likes
WHERE user_id = @user_id AND target_type = 1;
