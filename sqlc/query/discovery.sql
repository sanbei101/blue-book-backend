-- name: CreateTag :one
INSERT INTO tags (id, name, description)
VALUES (@id, @name, @description)
ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
RETURNING *;

-- name: GetTagByID :one
SELECT * FROM tags WHERE id = @id LIMIT 1;

-- name: GetTagByName :one
SELECT * FROM tags WHERE name = @name LIMIT 1;

-- name: ListTagsByPostID :many
SELECT t.id, t.name, t.description, t.created_at
FROM post_tags pt
JOIN tags t ON t.id = pt.tag_id
WHERE pt.post_id = @post_id
ORDER BY t.name ASC;

-- name: AddPostTag :execrows
INSERT INTO post_tags (post_id, tag_id)
VALUES (@post_id, @tag_id)
ON CONFLICT (post_id, tag_id) DO NOTHING;

-- name: DeletePostTags :exec
DELETE FROM post_tags WHERE post_id = @post_id;

-- name: SearchPosts :many
SELECT
    p.id, p.title, p.content, p.view_count, p.like_count, p.collect_count,
    p.comment_count, p.created_at,
    u.id AS author_id, u.username AS author_username, u.avatar_url AS author_avatar,
    COALESCE(pm.media_key, '') AS cover_key,
    ts_rank(
        to_tsvector('simple', p.title || ' ' || p.content),
        plainto_tsquery('simple', @search_query)
    ) AS relevance
FROM posts p
JOIN users u ON p.user_id = u.id
LEFT JOIN LATERAL (
    SELECT media_key
    FROM post_media
    WHERE post_id = p.id
    ORDER BY sort_order ASC
    LIMIT 1
) pm ON true
WHERE to_tsvector('simple', p.title || ' ' || p.content)
      @@ plainto_tsquery('simple', @search_query)
ORDER BY relevance DESC, p.created_at DESC, p.id DESC
LIMIT @limit_count OFFSET @offset_count;

-- name: CountSearchPosts :one
SELECT COUNT(*)::bigint
FROM posts
WHERE to_tsvector('simple', title || ' ' || content)
      @@ plainto_tsquery('simple', @search_query);

-- name: SearchUsers :many
SELECT id, username, avatar_url, bio
FROM users
WHERE username ILIKE '%' || @search_query || '%'
ORDER BY username ASC
LIMIT @limit_count OFFSET @offset_count;

-- name: CountSearchUsers :one
SELECT COUNT(*)::bigint
FROM users
WHERE username ILIKE '%' || @search_query || '%';

-- name: SearchTags :many
SELECT id, name, description, created_at
FROM tags
WHERE name ILIKE '%' || @search_query || '%'
ORDER BY name ASC
LIMIT @limit_count OFFSET @offset_count;

-- name: CountSearchTags :one
SELECT COUNT(*)::bigint
FROM tags
WHERE name ILIKE '%' || @search_query || '%';

-- name: ListTagPosts :many
SELECT
    p.id, p.title, p.content, p.view_count, p.like_count, p.collect_count,
    p.comment_count, p.created_at,
    u.id AS author_id, u.username AS author_username, u.avatar_url AS author_avatar,
    COALESCE(pm.media_key, '') AS cover_key
FROM post_tags pt
JOIN posts p ON p.id = pt.post_id
JOIN users u ON p.user_id = u.id
LEFT JOIN LATERAL (
    SELECT media_key
    FROM post_media
    WHERE post_id = p.id
    ORDER BY sort_order ASC
    LIMIT 1
) pm ON true
WHERE pt.tag_id = @tag_id
ORDER BY p.created_at DESC, p.id DESC
LIMIT @limit_count OFFSET @offset_count;

-- name: CountTagPosts :one
SELECT COUNT(*)::bigint
FROM post_tags
WHERE tag_id = @tag_id;

-- name: GetTopicByID :one
SELECT * FROM topics WHERE id = @id LIMIT 1;

-- name: ListTopics :many
SELECT * FROM topics
ORDER BY created_at DESC, id DESC
LIMIT @limit_count OFFSET @offset_count;

-- name: CountTopics :one
SELECT COUNT(*)::bigint FROM topics;

-- name: CountTopicPosts :one
SELECT COUNT(*)::bigint
FROM topic_posts
WHERE topic_id = @topic_id;

-- name: ListTopicPosts :many
SELECT
    p.id, p.title, p.content, p.view_count, p.like_count, p.collect_count,
    p.comment_count, p.created_at,
    u.id AS author_id, u.username AS author_username, u.avatar_url AS author_avatar,
    COALESCE(pm.media_key, '') AS cover_key
FROM topic_posts tp
JOIN posts p ON p.id = tp.post_id
JOIN users u ON p.user_id = u.id
LEFT JOIN LATERAL (
    SELECT media_key
    FROM post_media
    WHERE post_id = p.id
    ORDER BY sort_order ASC
    LIMIT 1
) pm ON true
WHERE tp.topic_id = @topic_id
ORDER BY tp.created_at DESC, tp.post_id DESC
LIMIT @limit_count OFFSET @offset_count;

-- name: AddSearchHistory :exec
INSERT INTO search_history (user_id, keyword)
VALUES (@user_id, @keyword)
ON CONFLICT (user_id, keyword)
DO UPDATE SET searched_at = NOW();

-- name: ListSearchHistory :many
SELECT keyword, searched_at
FROM search_history
WHERE user_id = @user_id
ORDER BY searched_at DESC, keyword ASC
LIMIT @limit_count OFFSET @offset_count;

-- name: CountSearchHistory :one
SELECT COUNT(*)::bigint FROM search_history WHERE user_id = @user_id;

-- name: DeleteSearchHistory :exec
DELETE FROM search_history WHERE user_id = @user_id;

-- name: RecordSearchTerm :exec
INSERT INTO search_terms (keyword, search_count)
VALUES (@keyword, 1)
ON CONFLICT (keyword)
DO UPDATE SET
    search_count = search_terms.search_count + 1,
    last_searched_at = NOW();

-- name: ListTrendingSearches :many
SELECT keyword, search_count
FROM search_terms
ORDER BY search_count DESC, last_searched_at DESC, keyword ASC
LIMIT @limit_count OFFSET @offset_count;

-- name: CountTrendingSearches :one
SELECT COUNT(*)::bigint FROM search_terms;

-- name: ListRecommendedPosts :many
SELECT
    p.id, p.title, p.content, p.view_count, p.like_count, p.collect_count,
    p.comment_count, p.created_at,
    u.id AS author_id, u.username AS author_username, u.avatar_url AS author_avatar,
    COALESCE(pm.media_key, '') AS cover_key
FROM posts p
JOIN users u ON p.user_id = u.id
LEFT JOIN LATERAL (
    SELECT media_key
    FROM post_media
    WHERE post_id = p.id
    ORDER BY sort_order ASC
    LIMIT 1
) pm ON true
ORDER BY (
    p.like_count * 3 + p.collect_count * 4 + p.comment_count * 2 + p.view_count
) DESC, p.created_at DESC, p.id DESC
LIMIT @limit_count OFFSET @offset_count;

-- name: CountRecommendedPosts :one
SELECT COUNT(*)::bigint FROM posts;
