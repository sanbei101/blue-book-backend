-- name: CreatePostNotification :exec
INSERT INTO notifications (
    id, recipient_id, actor_id, notification_type, post_id
) VALUES (
    @id, @recipient_id, @actor_id, @notification_type, @post_id
);

-- name: CreateCommentNotification :exec
INSERT INTO notifications (
    id, recipient_id, actor_id, notification_type, post_id, comment_id
) VALUES (
    @id, @recipient_id, @actor_id, @notification_type, @post_id, @comment_id
);

-- name: ListNotifications :many
SELECT
    n.id, n.notification_type, n.post_id, n.comment_id,
    n.created_at, n.read_at,
    u.id AS actor_id, u.username AS actor_username, u.avatar_url AS actor_avatar
FROM notifications n
JOIN users u ON u.id = n.actor_id
WHERE n.recipient_id = @recipient_id
ORDER BY n.created_at DESC, n.id DESC
LIMIT @limit_count OFFSET @offset_count;

-- name: CountNotifications :one
SELECT COUNT(*)::bigint
FROM notifications
WHERE recipient_id = @recipient_id;

-- name: CountUnreadNotifications :one
SELECT COUNT(*)::bigint
FROM notifications
WHERE recipient_id = @recipient_id AND read_at IS NULL;

-- name: MarkNotificationRead :execrows
UPDATE notifications
SET read_at = COALESCE(read_at, NOW())
WHERE id = @id AND recipient_id = @recipient_id;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications
SET read_at = NOW()
WHERE recipient_id = @recipient_id AND read_at IS NULL;
