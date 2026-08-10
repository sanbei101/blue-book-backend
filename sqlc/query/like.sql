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
