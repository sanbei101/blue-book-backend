-- name: CreateCollectionFolder :one
INSERT INTO collection_folders (id, user_id, name)
VALUES (@id, @user_id, @name)
RETURNING *;

-- name: ListCollectionFolders :many
SELECT * FROM collection_folders
WHERE user_id = @user_id
ORDER BY created_at DESC, id DESC
LIMIT @limit_count OFFSET @offset_count;

-- name: CountCollectionFolders :one
SELECT COUNT(*)::bigint FROM collection_folders WHERE user_id = @user_id;

-- name: GetCollectionFolderByID :one
SELECT * FROM collection_folders
WHERE id = @id AND user_id = @user_id
LIMIT 1;

-- name: UpdateCollectionFolder :one
UPDATE collection_folders
SET name = @name, updated_at = NOW()
WHERE id = @id AND user_id = @user_id
RETURNING *;

-- name: DeleteCollectionFolder :execrows
DELETE FROM collection_folders WHERE id = @id AND user_id = @user_id;

-- name: AddCollection :execrows
INSERT INTO collections (user_id, post_id, folder_id)
VALUES (@user_id, @post_id, @folder_id)
ON CONFLICT (user_id, post_id) DO NOTHING;

-- name: RemoveCollection :execrows
DELETE FROM collections
WHERE user_id = @user_id AND post_id = @post_id;

-- name: IsCollected :one
SELECT EXISTS(
    SELECT 1 FROM collections
    WHERE user_id = @user_id AND post_id = @post_id
) AS collected;

-- name: ListCollections :many
SELECT
    p.id, p.title, p.content, p.view_count, p.like_count, p.collect_count,
    p.comment_count, p.created_at,
    u.id AS author_id, u.username AS author_username, u.avatar_url AS author_avatar,
    COALESCE(pm.media_key, '') AS cover_key,
    COALESCE(pm.width, 0) AS width,
    COALESCE(pm.height, 0) AS height,
    c.created_at AS collected_at
FROM collections c
JOIN posts p ON p.id = c.post_id
JOIN users u ON p.user_id = u.id
LEFT JOIN LATERAL (
    SELECT media_key, width, height FROM post_media
    WHERE post_id = p.id ORDER BY sort_order ASC LIMIT 1
) pm ON true
WHERE c.user_id = @user_id
ORDER BY c.created_at DESC, c.post_id DESC
LIMIT @limit_count OFFSET @offset_count;

-- name: CountCollections :one
SELECT COUNT(*)::bigint FROM collections WHERE user_id = @user_id;
