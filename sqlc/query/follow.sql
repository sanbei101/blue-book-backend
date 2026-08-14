-- name: AddFollow :execrows
INSERT INTO follows (
    follower_id, following_id
) VALUES (
    @follower_id, @following_id
) ON CONFLICT (follower_id, following_id) DO NOTHING;

-- name: RemoveFollow :execrows
DELETE FROM follows
WHERE follower_id = @follower_id AND following_id = @following_id;

-- name: IsFollowing :one
SELECT EXISTS(
    SELECT 1 FROM follows
    WHERE follower_id = @follower_id AND following_id = @following_id
) AS following;

-- name: GetFollowerCount :one
SELECT COUNT(*)::bigint FROM follows WHERE following_id = @user_id;

-- name: GetFollowingCount :one
SELECT COUNT(*)::bigint FROM follows WHERE follower_id = @user_id;

-- name: CountFollowers :one
SELECT COUNT(*)::bigint FROM follows WHERE following_id = @following_id;

-- name: CountFollowing :one
SELECT COUNT(*)::bigint FROM follows WHERE follower_id = @follower_id;

-- name: ListFollowers :many
SELECT
    u.id, u.username, u.avatar_url, u.bio
FROM follows f
JOIN users u ON f.follower_id = u.id
WHERE f.following_id = @following_id
ORDER BY f.created_at DESC, f.follower_id DESC
LIMIT @limit_count OFFSET @offset_count;

-- name: ListFollowing :many
SELECT
    u.id, u.username, u.avatar_url, u.bio
FROM follows f
JOIN users u ON f.following_id = u.id
WHERE f.follower_id = @follower_id
ORDER BY f.created_at DESC, f.following_id DESC
LIMIT @limit_count OFFSET @offset_count;

-- name: CreateFollowNotification :exec
INSERT INTO notifications (id, recipient_id, actor_id, notification_type)
VALUES (@id, @recipient_id, @actor_id, 'user_followed');
