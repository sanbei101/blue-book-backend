-- name: ListViewerPostStates :many
SELECT
    p.id AS post_id,
    EXISTS(
        SELECT 1 FROM likes l
        WHERE l.user_id = @viewer_id AND l.target_id = p.id AND l.target_type = 1
    ) AS viewer_liked,
    EXISTS(
        SELECT 1 FROM collections c
        WHERE c.user_id = @viewer_id AND c.post_id = p.id
    ) AS viewer_collected,
    EXISTS(
        SELECT 1 FROM follows f
        WHERE f.follower_id = @viewer_id AND f.following_id = p.user_id
    ) AS viewer_following
FROM posts p
WHERE p.id = ANY(@post_ids::uuid[]);

-- name: ListViewerCommentLikedStates :many
SELECT
    c.id AS comment_id,
    EXISTS(
        SELECT 1 FROM likes l
        WHERE l.user_id = @viewer_id AND l.target_id = c.id AND l.target_type = 2
    ) AS viewer_liked
FROM comments c
WHERE c.id = ANY(@comment_ids::uuid[]);

-- name: ListViewerFollowingStates :many
SELECT
    u.id AS user_id,
    EXISTS(
        SELECT 1 FROM follows f
        WHERE f.follower_id = @viewer_id AND f.following_id = u.id
    ) AS viewer_following
FROM users u
WHERE u.id = ANY(@user_ids::uuid[]);
