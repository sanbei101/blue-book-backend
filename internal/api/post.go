package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/phuslu/log"

	"github.com/sanbei101/blue-book/internal/db"
	"github.com/sanbei101/blue-book/internal/pkg/jwt"
	"github.com/sanbei101/blue-book/internal/pkg/media"
	"github.com/sanbei101/blue-book/internal/pkg/render"
)

func Pagination(r *http.Request, defaultPage, defaultPageSize, maxPageSize int) (
	offset, limit int,
) {
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = defaultPage
	}
	pageSize, err := strconv.Atoi(r.URL.Query().Get("page_size"))
	if err != nil || pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return (page - 1) * pageSize, pageSize
}

type PostHandler struct {
	store     *db.Store
	presigner *media.Presigner
}

func NewPostHandler(store *db.Store, presigner *media.Presigner) *PostHandler {
	return &PostHandler{store: store, presigner: presigner}
}

type createPostRequest struct {
	// 帖子标题
	Title string `json:"title" validate:"required,max=100"`
	// 帖子内容
	Content string `json:"content" validate:"required"`
	// 媒体列表
	Media []createMediaItem `json:"media"`
	// 标签名称
	Tags []string `json:"tags"`
}
type createMediaItem struct {
	// 媒体对象 key
	MediaURL string `json:"media_url" validate:"required"`
	// 媒体类型 (image/video)
	MediaType string `json:"media_type" validate:"required,oneof=image video"`
	// 排序序号
	SortOrder int16 `json:"sort_order"`
}

type createPostResponse struct {
	// 帖子 ID
	ID uuid.UUID `json:"id"`
}

type getPostsResponse struct {
	// 帖子 ID
	ID uuid.UUID `json:"id"`
	// 标题
	Title string `json:"title"`
	// 内容
	Content string `json:"content"`
	// 浏览量
	ViewCount int64 `json:"view_count"`
	// 点赞数
	LikeCount int64 `json:"like_count"`
	// 收藏数
	CollectCount int64 `json:"collect_count"`
	// 评论数
	CommentCount int64 `json:"comment_count"`
	// 当前用户是否已点赞
	Liked bool `json:"liked"`
	// 当前用户是否已收藏
	Collected bool `json:"collected"`
	// 标签列表
	Tags []tagResponse `json:"tags,omitempty"`
	// 作者信息
	Author authorResponse `json:"author"`
	// 媒体列表
	Media []mediaResponse `json:"media,omitempty"`
	// 创建时间
	CreatedAt time.Time `json:"created_at"`
}

type listPostsItemResponse struct {
	// 帖子 ID
	ID uuid.UUID `json:"id"`
	// 标题
	Title string `json:"title"`
	// 内容
	Content string `json:"content"`
	// 浏览量
	ViewCount int64 `json:"view_count"`
	// 点赞数
	LikeCount int64 `json:"like_count"`
	// 收藏数
	CollectCount int64 `json:"collect_count"`
	// 评论数
	CommentCount int64 `json:"comment_count"`
	// 作者信息
	Author authorResponse `json:"author"`
	// 封面 URL
	CoverURL string `json:"cover_url,omitempty"`
	// 创建时间
	CreatedAt time.Time `json:"created_at"`
}

type authorResponse struct {
	// 用户 ID
	ID uuid.UUID `json:"id"`
	// 用户名
	Username string `json:"username"`
	// 头像地址
	AvatarURL string `json:"avatar_url,omitempty"`
}

type mediaResponse struct {
	// 媒体 ID
	ID uuid.UUID `json:"id"`
	// 媒体 URL
	MediaURL string `json:"media_url"`
	// 媒体类型
	MediaType string `json:"media_type"`
	// 图片宽度
	Width int32 `json:"width"`
	// 图片高度
	Height int32 `json:"height"`
	// 排序序号
	SortOrder int16 `json:"sort_order"`
}

func toAuthorFromFeed(authorID uuid.UUID, authorUsername string, authorAvatar pgtype.Text) authorResponse {
	a := authorResponse{ID: authorID, Username: authorUsername}
	if authorAvatar.Valid {
		a.AvatarURL = authorAvatar.String
	}
	return a
}

func toMediaResponse(m *db.PostMedium) mediaResponse {
	return mediaResponse{
		ID:        m.ID,
		MediaURL:  m.MediaURL,
		MediaType: string(m.MediaType),
		Width:     m.Width,
		Height:    m.Height,
		SortOrder: m.SortOrder,
	}
}

func (h *PostHandler) toCreatePostMediaParams(
	ctx context.Context,
	postID uuid.UUID,
	media []createMediaItem,
) ([]db.CreatePostMediaParams, error) {
	params := make([]db.CreatePostMediaParams, len(media))
	for i, item := range media {
		mediaType := db.MediaTypeEnumImage
		if item.MediaType == "video" {
			mediaType = db.MediaTypeEnumVideo
		}

		var width, height int32
		if mediaType == db.MediaTypeEnumImage {
			if h.presigner == nil {
				return nil, media.ErrNotConfigured
			}
			var err error
			width, height, err = h.presigner.ImageDimensions(ctx, item.MediaURL)
			if err != nil {
				return nil, err
			}
		}

		params[i] = db.CreatePostMediaParams{
			ID:        uuid.Must(uuid.NewV7()),
			PostID:    postID,
			MediaURL:  item.MediaURL,
			MediaType: mediaType,
			Width:     width,
			Height:    height,
			SortOrder: item.SortOrder,
		}
	}
	return params, nil
}

func normalizePostTags(tags []string) ([]string, error) {
	if len(tags) > 10 {
		return nil, errors.New("最多添加 10 个标签")
	}

	seen := make(map[string]struct{}, len(tags))
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.TrimPrefix(tag, "#"))
		if tag == "" {
			continue
		}
		if len([]rune(tag)) > 50 {
			return nil, errors.New("标签长度不能超过 50 个字符")
		}
		tag = strings.ToLower(tag)
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}
	return result, nil
}

func replacePostTags(ctx context.Context, q *db.Queries, postID uuid.UUID, tags []string) error {
	if err := q.DeletePostTags(ctx, postID); err != nil {
		return err
	}
	for _, name := range tags {
		tag, err := q.CreateTag(ctx, db.CreateTagParams{
			ID:          uuid.Must(uuid.NewV7()),
			Name:        name,
			Description: "",
		})
		if err != nil {
			return err
		}
		if _, err := q.AddPostTag(ctx, db.AddPostTagParams{
			PostID: postID,
			TagID:  tag.ID,
		}); err != nil {
			return err
		}
	}
	return nil
}

// 创建帖子
//
//	@Summary	创建帖子
//	@Tags		posts
//	@Security	BearerAuth
//	@Param		body	body		createPostRequest	true	"帖子内容"
//	@Success	200		{object}	render.Response[createPostResponse]
//	@Failure	400		{object}	render.errorResponse
//	@Failure	500		{object}	render.errorResponse
//	@Router		/posts [post]
func (h *PostHandler) Create(w http.ResponseWriter, r *http.Request) {
	body, err := render.ReadBody[createPostRequest](w, r)
	if err != nil {
		return
	}
	tags, err := normalizePostTags(body.Tags)
	if err != nil {
		render.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	currentUserID := jwt.GetUserIDFromContext(r)
	postID := uuid.Must(uuid.NewV7())
	mediaParams, err := h.toCreatePostMediaParams(r.Context(), postID, body.Media)
	if err != nil {
		log.Error().Err(err).Msg("解析帖子媒体尺寸失败")
		render.Error(w, http.StatusBadRequest, "无法读取图片尺寸")
		return
	}

	var created db.Post
	err = h.store.ExecTx(r.Context(), func(q *db.Queries) error {
		post, err := q.CreatePost(r.Context(), db.CreatePostParams{
			ID:      postID,
			UserID:  currentUserID,
			Title:   body.Title,
			Content: body.Content,
		})
		if err != nil {
			log.Error().Err(err).Msg("创建帖子失败")
			return err
		}
		created = post
		if len(mediaParams) > 0 {
			_, err := q.CreatePostMedia(r.Context(), mediaParams)
			if err != nil {
				log.Error().Err(err).Msg("创建帖子媒体失败")
				return err
			}
		}
		if err := replacePostTags(r.Context(), q, post.ID, tags); err != nil {
			log.Error().Err(err).Msg("创建帖子标签失败")
			return err
		}
		return nil
	})
	if err != nil {
		render.Error(w, http.StatusInternalServerError, "创建帖子失败")
		return
	}
	render.Success(w, "创建成功", createPostResponse{ID: created.ID})
}

// 获取帖子列表
//
//	@Summary	获取帖子列表
//	@Tags		posts
//	@Param		page		query		int	false	"页码"	default(1)
//	@Param		page_size	query		int	false	"每页数量"	default(20)
//	@Success	200			{object}	render.Response[[]listPostsItemResponse]
//	@Failure	500			{object}	render.errorResponse
//	@Router		/posts [get]
func (h *PostHandler) ListFeed(w http.ResponseWriter, r *http.Request) {
	offset, pageSize := Pagination(r, 1, 20, 50)
	rows, err := h.store.ListPostsFeed(r.Context(), db.ListPostsFeedParams{
		OffsetCount: int32(offset),
		LimitCount:  int32(pageSize),
	})
	if err != nil {
		render.Error(w, http.StatusInternalServerError, "获取信息流失败")
		return
	}

	posts := make([]listPostsItemResponse, 0, len(rows))
	for i := range rows {
		posts = append(posts, listPostsItemResponse{
			ID:           rows[i].ID,
			Title:        rows[i].Title,
			Content:      rows[i].Content,
			ViewCount:    rows[i].ViewCount,
			LikeCount:    rows[i].LikeCount,
			CollectCount: rows[i].CollectCount,
			CommentCount: rows[i].CommentCount,
			CreatedAt:    rows[i].CreatedAt,
			CoverURL:     rows[i].CoverURL,
			Author:       toAuthorFromFeed(rows[i].AuthorID, rows[i].AuthorUsername, rows[i].AuthorAvatar),
		})
	}

	render.Success(w, "查询成功", posts)
}

// 获取帖子详情
//
//	@Summary	获取帖子详情
//	@Tags		posts
//	@Param		post_id	path		string	true	"帖子 ID"
//	@Success	200		{object}	render.Response[getPostsResponse]
//	@Failure	400		{object}	render.errorResponse
//	@Failure	404		{object}	render.errorResponse
//	@Router		/posts/{post_id} [get]
func (h *PostHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	postID, ok := parseUUIDParam(r, "post_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的帖子 ID")
		return
	}

	row, err := h.store.GetPostByID(r.Context(), postID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			render.Error(w, http.StatusNotFound, "帖子不存在")
			return
		}
		log.Error().Err(err).Msg("获取帖子失败")
		render.Error(w, http.StatusInternalServerError, "获取帖子失败")
		return
	}

	err = h.store.IncrementViewCount(r.Context(), postID)
	if err != nil {
		log.Error().Err(err).Msg("增加帖子浏览量失败")
	}

	media, err := h.store.GetPostMediaByPostID(r.Context(), row.ID)
	if err != nil {
		log.Error().Err(err).Msg("获取帖子媒体失败")
		render.Error(w, http.StatusInternalServerError, "获取帖子媒体失败")
		return
	}
	mediaList := make([]mediaResponse, 0, len(media))
	for i := range media {
		mediaList = append(mediaList, toMediaResponse(&media[i]))
	}
	tags, err := h.store.ListTagsByPostID(r.Context(), row.ID)
	if err != nil {
		log.Error().Err(err).Msg("获取帖子标签失败")
		render.Error(w, http.StatusInternalServerError, "获取帖子标签失败")
		return
	}
	tagList := make([]tagResponse, 0, len(tags))
	for i := range tags {
		tagList = append(tagList, toTagResponse(&tags[i]))
	}

	resp := getPostsResponse{
		ID:           row.ID,
		Title:        row.Title,
		Content:      row.Content,
		ViewCount:    row.ViewCount,
		LikeCount:    row.LikeCount,
		CollectCount: row.CollectCount,
		CommentCount: row.CommentCount,
		CreatedAt:    row.CreatedAt,
		Author:       toAuthorFromFeed(row.AuthorID, row.AuthorUsername, row.AuthorAvatar),
		Media:        mediaList,
		Tags:         tagList,
	}

	if currentUserID := jwt.GetUserIDFromContext(r); currentUserID != uuid.Nil {
		liked, err := h.store.IsPostLiked(r.Context(), db.IsPostLikedParams{
			UserID: currentUserID,
			PostID: row.ID,
		})
		if err != nil {
			log.Error().Err(err).Msg("获取点赞状态失败")
		}
		collected, err := h.store.IsCollected(r.Context(), db.IsCollectedParams{
			UserID: currentUserID,
			PostID: row.ID,
		})
		if err != nil {
			log.Error().Err(err).Msg("获取收藏状态失败")
		}
		resp.Liked = liked
		resp.Collected = collected
	}

	render.Success(w, "查询成功", resp)
}

// 获取指定用户的帖子列表
//
//	@Summary	获取指定用户的帖子列表
//	@Tags		posts
//	@Param		user_id		path		string	true	"用户 ID"
//	@Param		page		query		int		false	"页码"	default(1)
//	@Param		page_size	query		int		false	"每页数量"	default(20)
//	@Success	200			{object}	render.Response[[]listPostsItemResponse]
//	@Failure	400			{object}	render.errorResponse
//	@Failure	500			{object}	render.errorResponse
//	@Router		/posts/user/{user_id} [get]
func (h *PostHandler) ListByUser(w http.ResponseWriter, r *http.Request) {
	userIDStr := chi.URLParam(r, "user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		render.Error(w, http.StatusBadRequest, "无效的用户 ID")
		return
	}
	offset, pageSize := Pagination(r, 1, 20, 50)
	rows, err := h.store.ListPostsByUser(r.Context(), db.ListPostsByUserParams{
		UserID:      userID,
		OffsetCount: int32(offset),
		LimitCount:  int32(pageSize),
	})
	if err != nil {
		render.Error(w, http.StatusInternalServerError, "获取帖子列表失败")
		return
	}

	posts := make([]listPostsItemResponse, 0, len(rows))
	for i := range rows {
		posts = append(posts, listPostsItemResponse{
			ID:           rows[i].ID,
			Title:        rows[i].Title,
			Content:      rows[i].Content,
			ViewCount:    rows[i].ViewCount,
			LikeCount:    rows[i].LikeCount,
			CollectCount: rows[i].CollectCount,
			CommentCount: rows[i].CommentCount,
			CreatedAt:    rows[i].CreatedAt,
			CoverURL:     rows[i].CoverURL,
			Author:       toAuthorFromFeed(rows[i].AuthorID, rows[i].AuthorUsername, rows[i].AuthorAvatar),
		})
	}

	render.Success(w, "查询成功", posts)
}

// 删除帖子
//
//	@Summary	删除帖子
//	@Tags		posts
//	@Security	BearerAuth
//	@Param		post_id	path		string	true	"帖子 ID"
//	@Success	204		{object}	render.ResponseWithoutData
//	@Failure	400		{object}	render.errorResponse
//	@Failure	500		{object}	render.errorResponse
//	@Router		/posts/{post_id} [delete]
func (h *PostHandler) Delete(w http.ResponseWriter, r *http.Request) {
	postID, ok := parseUUIDParam(r, "post_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的帖子 ID")
		return
	}
	currentUserID := jwt.GetUserIDFromContext(r)

	post, err := h.store.GetPostByID(r.Context(), postID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			render.Error(w, http.StatusNotFound, "帖子不存在")
			return
		}
		log.Error().Err(err).Msg("获取帖子失败")
		render.Error(w, http.StatusInternalServerError, "删除失败")
		return
	}
	if post.UserID != currentUserID {
		render.Error(w, http.StatusForbidden, "只能删除自己的帖子")
		return
	}

	err = h.store.ExecTx(r.Context(), func(q *db.Queries) error {
		if err := q.DeletePostMediaByPostID(r.Context(), postID); err != nil {
			return err
		}
		return q.DeletePost(r.Context(), db.DeletePostParams{
			ID:     postID,
			UserID: currentUserID,
		})
	})
	if err != nil {
		log.Error().Err(err).Msg("删除帖子失败")
		render.Error(w, http.StatusInternalServerError, "删除失败")
		return
	}

	render.SuccessNoData(w, "删除成功")
}

type updatePostRequest struct {
	// 帖子标题
	Title string `json:"title" validate:"required,max=100"`
	// 帖子内容
	Content string `json:"content" validate:"required"`
	// 媒体列表
	Media []createMediaItem `json:"media"`
	// 标签名称
	Tags []string `json:"tags"`
}

// 编辑帖子
//
//	@Summary	编辑帖子
//	@Tags		posts
//	@Security	BearerAuth
//	@Param		post_id	path		string				true	"帖子 ID"
//	@Param		body	body		updatePostRequest	true	"更新内容"
//	@Success	200		{object}	render.Response[createPostResponse]
//	@Failure	400		{object}	render.errorResponse
//	@Failure	403		{object}	render.errorResponse
//	@Failure	404		{object}	render.errorResponse
//	@Failure	500		{object}	render.errorResponse
//	@Router		/posts/{post_id} [patch]
func (h *PostHandler) Update(w http.ResponseWriter, r *http.Request) {
	postID, ok := parseUUIDParam(r, "post_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的帖子 ID")
		return
	}
	currentUserID := jwt.GetUserIDFromContext(r)
	body, err := render.ReadBody[updatePostRequest](w, r)
	if err != nil {
		return
	}
	tags, err := normalizePostTags(body.Tags)
	if err != nil {
		render.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	mediaParams, err := h.toCreatePostMediaParams(r.Context(), postID, body.Media)
	if err != nil {
		log.Error().Err(err).Msg("解析帖子媒体尺寸失败")
		render.Error(w, http.StatusBadRequest, "无法读取图片尺寸")
		return
	}

	var updated db.Post
	err = h.store.ExecTx(r.Context(), func(q *db.Queries) error {
		post, err := q.UpdatePost(r.Context(), db.UpdatePostParams{
			ID:      postID,
			UserID:  currentUserID,
			Title:   body.Title,
			Content: body.Content,
		})
		if err != nil {
			return err
		}
		updated = post

		if err := q.DeletePostMediaByPostID(r.Context(), postID); err != nil {
			return err
		}
		if len(mediaParams) > 0 {
			if _, err := q.CreatePostMedia(r.Context(), mediaParams); err != nil {
				return err
			}
		}
		if err := replacePostTags(r.Context(), q, postID, tags); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			render.Error(w, http.StatusNotFound, "帖子不存在")
			return
		}
		log.Error().Err(err).Msg("编辑帖子失败")
		render.Error(w, http.StatusInternalServerError, "编辑失败")
		return
	}

	render.Success(w, "更新成功", createPostResponse{ID: updated.ID})
}
