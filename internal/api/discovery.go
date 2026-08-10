package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/phuslu/log"

	"github.com/sanbei101/blue-book/internal/db"
	"github.com/sanbei101/blue-book/internal/pkg/jwt"
	"github.com/sanbei101/blue-book/internal/pkg/media"
	"github.com/sanbei101/blue-book/internal/pkg/render"
)

type DiscoveryHandler struct {
	store *db.Store
}

func NewDiscoveryHandler(store *db.Store) *DiscoveryHandler {
	return &DiscoveryHandler{store: store}
}

type tagResponse struct {
	// 标签 ID
	ID uuid.UUID `json:"id"`
	// 标签名称
	Name string `json:"name"`
	// 标签描述
	Description string `json:"description,omitempty"`
	// 帖子数量
	PostCount int64 `json:"post_count,omitempty"`
	// 创建时间
	CreatedAt time.Time `json:"created_at"`
}

func toTagResponse(tag *db.Tag) tagResponse {
	return tagResponse{
		ID:          tag.ID,
		Name:        tag.Name,
		Description: tag.Description,
		CreatedAt:   tag.CreatedAt,
	}
}

func tagResponseWithCount(tag *db.Tag, postCount int64) tagResponse {
	response := toTagResponse(tag)
	response.PostCount = postCount
	return response
}

type searchPostResponse struct {
	ID           uuid.UUID      `json:"id"`
	Title        string         `json:"title"`
	Content      string         `json:"content"`
	ViewCount    int64          `json:"view_count"`
	LikeCount    int64          `json:"like_count"`
	CollectCount int64          `json:"collect_count"`
	CommentCount int64          `json:"comment_count"`
	Author       authorResponse `json:"author"`
	CoverURL     string         `json:"cover_url,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	Relevance    float32        `json:"relevance,omitempty"`
}

func toSearchPostResponse(row *db.SearchPostsRow) searchPostResponse {
	return searchPostResponse{
		ID:           row.ID,
		Title:        row.Title,
		Content:      row.Content,
		ViewCount:    row.ViewCount,
		LikeCount:    row.LikeCount,
		CollectCount: row.CollectCount,
		CommentCount: row.CommentCount,
		Author:       toAuthorFromFeed(row.AuthorID, row.AuthorUsername, row.AuthorAvatar),
		CoverURL:     media.CDNURL(row.CoverKey),
		CreatedAt:    row.CreatedAt,
		Relevance:    row.Relevance,
	}
}

func toSearchPostFromTag(row *db.ListTagPostsRow) listPostsItemResponse {
	return listPostsItemResponse{
		ID:           row.ID,
		Title:        row.Title,
		Content:      row.Content,
		ViewCount:    row.ViewCount,
		LikeCount:    row.LikeCount,
		CollectCount: row.CollectCount,
		CommentCount: row.CommentCount,
		Author:       toAuthorFromFeed(row.AuthorID, row.AuthorUsername, row.AuthorAvatar),
		CoverURL:     media.CDNURL(row.CoverKey),
		CreatedAt:    row.CreatedAt,
	}
}

type searchResponse struct {
	Posts []searchPostResponse `json:"posts,omitempty"`
	Users []followUserResponse `json:"users,omitempty"`
	Tags  []tagResponse        `json:"tags,omitempty"`
}

type searchHistoryResponse struct {
	Keyword    string    `json:"keyword"`
	SearchedAt time.Time `json:"searched_at"`
}

func searchType(value string) (string, error) {
	if value == "" {
		return "all", nil
	}
	if value != "all" && value != "posts" && value != "users" && value != "tags" {
		return "", errors.New("type 必须是 all、posts、users 或 tags")
	}
	return value, nil
}

func (h *DiscoveryHandler) searchPosts(
	r *http.Request,
	keyword string,
	offset, limit int,
) ([]searchPostResponse, error) {
	rows, err := h.store.SearchPosts(r.Context(), db.SearchPostsParams{
		SearchQuery: keyword,
		OffsetCount: int32(offset),
		LimitCount:  int32(limit),
	})
	if err != nil {
		return nil, err
	}
	items := make([]searchPostResponse, 0, len(rows))
	for i := range rows {
		items = append(items, toSearchPostResponse(&rows[i]))
	}
	return items, nil
}

func (h *DiscoveryHandler) searchUsers(
	r *http.Request,
	keyword string,
	offset, limit int,
) ([]followUserResponse, error) {
	rows, err := h.store.SearchUsers(r.Context(), db.SearchUsersParams{
		SearchQuery: pgtype.Text{String: keyword, Valid: true},
		OffsetCount: int32(offset),
		LimitCount:  int32(limit),
	})
	if err != nil {
		return nil, err
	}
	items := make([]followUserResponse, 0, len(rows))
	for i := range rows {
		items = append(items, newFollowUserResponse(
			rows[i].ID,
			rows[i].Username,
			rows[i].AvatarURL,
			rows[i].Bio,
		))
	}
	return items, nil
}

func (h *DiscoveryHandler) searchTags(r *http.Request, keyword string, offset, limit int) ([]tagResponse, error) {
	rows, err := h.store.SearchTags(r.Context(), db.SearchTagsParams{
		SearchQuery: pgtype.Text{String: keyword, Valid: true},
		OffsetCount: int32(offset),
		LimitCount:  int32(limit),
	})
	if err != nil {
		return nil, err
	}
	items := make([]tagResponse, 0, len(rows))
	for i := range rows {
		items = append(items, toTagResponse(&rows[i]))
	}
	return items, nil
}

// 综合搜索
//
//	@Summary	综合搜索
//	@Tags		discovery
//	@Param		q			query		string	true	"搜索关键词"
//	@Param		type		query		string	false	"搜索类型: all/posts/users/tags"
//	@Param		page		query		int		false	"页码"	default(1)
//	@Param		page_size	query		int		false	"每页数量"	default(20)
//	@Success	200			{object}	render.Response[searchResponse]
//	@Failure	400			{object}	render.errorResponse
//	@Failure	500			{object}	render.errorResponse
//	@Router		/search [get]
func (h *DiscoveryHandler) Search(w http.ResponseWriter, r *http.Request) {
	keyword := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if keyword == "" || len([]rune(keyword)) > 100 {
		render.Error(w, http.StatusBadRequest, "搜索关键词不能为空且不能超过 100 个字符")
		return
	}
	typeValue, err := searchType(r.URL.Query().Get("type"))
	if err != nil {
		render.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	offset, pageSize := Pagination(r, 1, 20, 50)

	if err := h.store.RecordSearchTerm(r.Context(), keyword); err != nil {
		log.Error().Err(err).Msg("记录热搜词失败")
	}
	if userID := jwt.GetUserIDFromContext(r); userID != uuid.Nil {
		if err := h.store.AddSearchHistory(r.Context(), db.AddSearchHistoryParams{
			UserID:  userID,
			Keyword: keyword,
		}); err != nil {
			log.Error().Err(err).Msg("记录搜索历史失败")
		}
	}

	response := searchResponse{}
	switch typeValue {
	case "all", "posts":
		response.Posts, err = h.searchPosts(r, keyword, offset, pageSize)
		if err != nil {
			log.Error().Err(err).Msg("搜索帖子失败")
			render.Error(w, http.StatusInternalServerError, "搜索失败")
			return
		}
	}
	if typeValue == "all" || typeValue == "users" {
		response.Users, err = h.searchUsers(r, keyword, offset, pageSize)
		if err != nil {
			log.Error().Err(err).Msg("搜索用户失败")
			render.Error(w, http.StatusInternalServerError, "搜索失败")
			return
		}
	}
	if typeValue == "all" || typeValue == "tags" {
		response.Tags, err = h.searchTags(r, keyword, offset, pageSize)
		if err != nil {
			log.Error().Err(err).Msg("搜索标签失败")
			render.Error(w, http.StatusInternalServerError, "搜索失败")
			return
		}
	}

	render.Success(w, "搜索成功", response)
}

// 搜索联想
//
//	@Summary	搜索联想
//	@Tags		discovery
//	@Param		q	query		string	true	"关键词"
//	@Success	200	{object}	render.Response[searchResponse]
//	@Failure	400	{object}	render.errorResponse
//	@Failure	500	{object}	render.errorResponse
//	@Router		/search/suggestions [get]
func (h *DiscoveryHandler) Suggestions(w http.ResponseWriter, r *http.Request) {
	keyword := strings.TrimSpace(r.URL.Query().Get("q"))
	if keyword == "" || len([]rune(keyword)) > 100 {
		render.Error(w, http.StatusBadRequest, "关键词不能为空且不能超过 100 个字符")
		return
	}
	users, err := h.searchUsers(r, keyword, 0, 5)
	if err != nil {
		log.Error().Err(err).Msg("获取用户联想失败")
		render.Error(w, http.StatusInternalServerError, "获取联想失败")
		return
	}
	tags, err := h.searchTags(r, keyword, 0, 5)
	if err != nil {
		log.Error().Err(err).Msg("获取标签联想失败")
		render.Error(w, http.StatusInternalServerError, "获取联想失败")
		return
	}
	render.Success(w, "查询成功", searchResponse{Users: users, Tags: tags})
}

type trendingSearchResponse struct {
	Keyword     string `json:"keyword"`
	SearchCount int64  `json:"search_count"`
}

// 获取热搜词
//
//	@Summary	获取热搜词
//	@Tags		discovery
//	@Param		limit	query		int	false	"数量"	default(10)
//	@Success	200		{object}	render.Response[[]trendingSearchResponse]
//	@Failure	500		{object}	render.errorResponse
//	@Router		/search/trending [get]
func (h *DiscoveryHandler) Trending(w http.ResponseWriter, r *http.Request) {
	limit := 10
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 50 {
		limit = 50
	}
	rows, err := h.store.ListTrendingSearches(r.Context(), int32(limit))
	if err != nil {
		log.Error().Err(err).Msg("获取热搜失败")
		render.Error(w, http.StatusInternalServerError, "获取热搜失败")
		return
	}
	items := make([]trendingSearchResponse, 0, len(rows))
	for i := range rows {
		items = append(items, trendingSearchResponse{
			Keyword:     rows[i].Keyword,
			SearchCount: rows[i].SearchCount,
		})
	}
	render.Success(w, "查询成功", items)
}

// 获取搜索历史
//
//	@Summary	获取搜索历史
//	@Tags		discovery
//	@Security	BearerAuth
//	@Success	200	{object}	render.Response[[]searchHistoryResponse]
//	@Failure	500	{object}	render.errorResponse
//	@Router		/me/search-history [get]
func (h *DiscoveryHandler) ListHistory(w http.ResponseWriter, r *http.Request) {
	rows, err := h.store.ListSearchHistory(r.Context(), db.ListSearchHistoryParams{
		UserID:     jwt.GetUserIDFromContext(r),
		LimitCount: 50,
	})
	if err != nil {
		log.Error().Err(err).Msg("获取搜索历史失败")
		render.Error(w, http.StatusInternalServerError, "获取搜索历史失败")
		return
	}
	items := make([]searchHistoryResponse, 0, len(rows))
	for i := range rows {
		items = append(items, searchHistoryResponse{
			Keyword:    rows[i].Keyword,
			SearchedAt: rows[i].SearchedAt,
		})
	}
	render.Success(w, "查询成功", items)
}

// 清空搜索历史
//
//	@Summary	清空搜索历史
//	@Tags		discovery
//	@Security	BearerAuth
//	@Success	204	{object}	render.ResponseWithoutData
//	@Failure	500	{object}	render.errorResponse
//	@Router		/me/search-history [delete]
func (h *DiscoveryHandler) DeleteHistory(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteSearchHistory(r.Context(), jwt.GetUserIDFromContext(r)); err != nil {
		log.Error().Err(err).Msg("清空搜索历史失败")
		render.Error(w, http.StatusInternalServerError, "清空搜索历史失败")
		return
	}
	render.SuccessNoData(w, "清空成功")
}

type topicResponse struct {
	// 专题 ID
	ID uuid.UUID `json:"id"`
	// 专题名称
	Name string `json:"name"`
	// 专题描述
	Description string `json:"description,omitempty"`
	// 封面地址
	CoverURL string `json:"cover_url,omitempty"`
	// 专题帖子数量
	PostCount int64 `json:"post_count"`
	// 创建时间
	CreatedAt time.Time `json:"created_at"`
}

func toTopicResponse(topic *db.Topic, postCount int64) topicResponse {
	return topicResponse{
		ID:          topic.ID,
		Name:        topic.Name,
		Description: topic.Description,
		CoverURL:    topic.CoverURL,
		PostCount:   postCount,
		CreatedAt:   topic.CreatedAt,
	}
}

// 获取专题列表
//
//	@Summary	获取专题列表
//	@Tags		discovery
//	@Param		page		query		int	false	"页码"	default(1)
//	@Param		page_size	query		int	false	"每页数量"	default(20)
//	@Success	200			{object}	render.Response[[]topicResponse]
//	@Failure	500			{object}	render.errorResponse
//	@Router		/topics [get]
func (h *DiscoveryHandler) ListTopics(w http.ResponseWriter, r *http.Request) {
	offset, pageSize := Pagination(r, 1, 20, 50)
	topics, err := h.store.ListTopics(r.Context(), db.ListTopicsParams{
		OffsetCount: int32(offset),
		LimitCount:  int32(pageSize),
	})
	if err != nil {
		log.Error().Err(err).Msg("获取专题列表失败")
		render.Error(w, http.StatusInternalServerError, "获取专题列表失败")
		return
	}
	items := make([]topicResponse, 0, len(topics))
	for i := range topics {
		count, err := h.store.CountTopicPosts(r.Context(), topics[i].ID)
		if err != nil {
			log.Error().Err(err).Msg("统计专题帖子失败")
			render.Error(w, http.StatusInternalServerError, "获取专题列表失败")
			return
		}
		items = append(items, toTopicResponse(&topics[i], count))
	}
	render.Success(w, "查询成功", items)
}

// 获取专题详情
//
//	@Summary	获取专题详情
//	@Tags		discovery
//	@Param		topic_id	path		string	true	"专题 ID"
//	@Success	200			{object}	render.Response[topicResponse]
//	@Failure	400			{object}	render.errorResponse
//	@Failure	404			{object}	render.errorResponse
//	@Failure	500			{object}	render.errorResponse
//	@Router		/topics/{topic_id} [get]
func (h *DiscoveryHandler) GetTopic(w http.ResponseWriter, r *http.Request) {
	topicID, ok := parseUUIDParam(r, "topic_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的专题 ID")
		return
	}
	topic, err := h.store.GetTopicByID(r.Context(), topicID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			render.Error(w, http.StatusNotFound, "专题不存在")
			return
		}
		log.Error().Err(err).Msg("获取专题失败")
		render.Error(w, http.StatusInternalServerError, "获取专题失败")
		return
	}
	count, err := h.store.CountTopicPosts(r.Context(), topicID)
	if err != nil {
		log.Error().Err(err).Msg("统计专题帖子失败")
		render.Error(w, http.StatusInternalServerError, "获取专题失败")
		return
	}
	render.Success(w, "查询成功", toTopicResponse(&topic, count))
}

// 获取专题帖子
//
//	@Summary	获取专题帖子
//	@Tags		discovery
//	@Param		topic_id	path		string	true	"专题 ID"
//	@Param		page		query		int		false	"页码"	default(1)
//	@Param		page_size	query		int		false	"每页数量"	default(20)
//	@Success	200			{object}	render.Response[[]listPostsItemResponse]
//	@Failure	400			{object}	render.errorResponse
//	@Failure	404			{object}	render.errorResponse
//	@Failure	500			{object}	render.errorResponse
//	@Router		/topics/{topic_id}/posts [get]
func (h *DiscoveryHandler) ListTopicPosts(w http.ResponseWriter, r *http.Request) {
	topicID, ok := parseUUIDParam(r, "topic_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的专题 ID")
		return
	}
	if _, err := h.store.GetTopicByID(r.Context(), topicID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			render.Error(w, http.StatusNotFound, "专题不存在")
			return
		}
		log.Error().Err(err).Msg("获取专题失败")
		render.Error(w, http.StatusInternalServerError, "获取专题帖子失败")
		return
	}
	offset, pageSize := Pagination(r, 1, 20, 50)
	rows, err := h.store.ListTopicPosts(r.Context(), db.ListTopicPostsParams{
		TopicID:     topicID,
		OffsetCount: int32(offset),
		LimitCount:  int32(pageSize),
	})
	if err != nil {
		log.Error().Err(err).Msg("获取专题帖子失败")
		render.Error(w, http.StatusInternalServerError, "获取专题帖子失败")
		return
	}
	items := make([]listPostsItemResponse, 0, len(rows))
	for i := range rows {
		items = append(items, listPostsItemResponse{
			ID:           rows[i].ID,
			Title:        rows[i].Title,
			Content:      rows[i].Content,
			ViewCount:    rows[i].ViewCount,
			LikeCount:    rows[i].LikeCount,
			CollectCount: rows[i].CollectCount,
			CommentCount: rows[i].CommentCount,
			Author:       toAuthorFromFeed(rows[i].AuthorID, rows[i].AuthorUsername, rows[i].AuthorAvatar),
			CoverURL:     media.CDNURL(rows[i].CoverKey),
			CreatedAt:    rows[i].CreatedAt,
		})
	}
	render.Success(w, "查询成功", items)
}

type createTagRequest struct {
	// 标签名称
	Name string `json:"name" validate:"required,max=50"`
	// 标签描述
	Description string `json:"description" validate:"max=200"`
}

// 创建标签
//
//	@Summary	创建标签
//	@Tags		discovery
//	@Security	BearerAuth
//	@Param		body	body		createTagRequest	true	"标签信息"
//	@Success	200		{object}	render.Response[tagResponse]
//	@Failure	400		{object}	render.errorResponse
//	@Failure	500		{object}	render.errorResponse
//	@Router		/tags [post]
func (h *DiscoveryHandler) CreateTag(w http.ResponseWriter, r *http.Request) {
	body, err := render.ReadBody[createTagRequest](w, r)
	if err != nil {
		return
	}
	name := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(body.Name, "#")))
	if name == "" {
		render.Error(w, http.StatusBadRequest, "标签名称不能为空")
		return
	}
	tag, err := h.store.CreateTag(r.Context(), db.CreateTagParams{
		ID:          uuid.Must(uuid.NewV7()),
		Name:        name,
		Description: strings.TrimSpace(body.Description),
	})
	if err != nil {
		log.Error().Err(err).Msg("创建标签失败")
		render.Error(w, http.StatusInternalServerError, "创建标签失败")
		return
	}
	render.Success(w, "创建成功", toTagResponse(&tag))
}

// 获取标签详情
//
//	@Summary	获取标签详情
//	@Tags		discovery
//	@Param		tag_id	path		string	true	"标签 ID"
//	@Success	200		{object}	render.Response[tagResponse]
//	@Failure	400		{object}	render.errorResponse
//	@Failure	404		{object}	render.errorResponse
//	@Failure	500		{object}	render.errorResponse
//	@Router		/tags/{tag_id} [get]
func (h *DiscoveryHandler) GetTag(w http.ResponseWriter, r *http.Request) {
	tagID, ok := parseUUIDParam(r, "tag_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的标签 ID")
		return
	}
	tag, err := h.store.GetTagByID(r.Context(), tagID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			render.Error(w, http.StatusNotFound, "标签不存在")
			return
		}
		log.Error().Err(err).Msg("获取标签失败")
		render.Error(w, http.StatusInternalServerError, "获取标签失败")
		return
	}
	count, err := h.store.CountTagPosts(r.Context(), tagID)
	if err != nil {
		log.Error().Err(err).Msg("统计标签帖子失败")
		render.Error(w, http.StatusInternalServerError, "获取标签失败")
		return
	}
	render.Success(w, "查询成功", tagResponseWithCount(&tag, count))
}

// 获取标签帖子
//
//	@Summary	获取标签帖子
//	@Tags		discovery
//	@Param		tag_id		path		string	true	"标签 ID"
//	@Param		page		query		int		false	"页码"	default(1)
//	@Param		page_size	query		int		false	"每页数量"	default(20)
//	@Success	200			{object}	render.Response[[]listPostsItemResponse]
//	@Failure	400			{object}	render.errorResponse
//	@Failure	404			{object}	render.errorResponse
//	@Failure	500			{object}	render.errorResponse
//	@Router		/tags/{tag_id}/posts [get]
func (h *DiscoveryHandler) ListTagPosts(w http.ResponseWriter, r *http.Request) {
	tagID, ok := parseUUIDParam(r, "tag_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的标签 ID")
		return
	}
	if _, err := h.store.GetTagByID(r.Context(), tagID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			render.Error(w, http.StatusNotFound, "标签不存在")
			return
		}
		log.Error().Err(err).Msg("获取标签失败")
		render.Error(w, http.StatusInternalServerError, "获取标签帖子失败")
		return
	}
	offset, pageSize := Pagination(r, 1, 20, 50)
	rows, err := h.store.ListTagPosts(r.Context(), db.ListTagPostsParams{
		TagID:       tagID,
		OffsetCount: int32(offset),
		LimitCount:  int32(pageSize),
	})
	if err != nil {
		log.Error().Err(err).Msg("获取标签帖子失败")
		render.Error(w, http.StatusInternalServerError, "获取标签帖子失败")
		return
	}
	items := make([]listPostsItemResponse, 0, len(rows))
	for i := range rows {
		items = append(items, toSearchPostFromTag(&rows[i]))
	}
	render.Success(w, "查询成功", items)
}

// 获取发现流
//
//	@Summary	获取发现流
//	@Tags		discovery
//	@Param		page		query		int	false	"页码"	default(1)
//	@Param		page_size	query		int	false	"每页数量"	default(20)
//	@Success	200			{object}	render.Response[[]listPostsItemResponse]
//	@Failure	500			{object}	render.errorResponse
//	@Router		/feed/recommended [get]
func (h *DiscoveryHandler) Recommended(w http.ResponseWriter, r *http.Request) {
	offset, pageSize := Pagination(r, 1, 20, 50)
	rows, err := h.store.ListRecommendedPosts(r.Context(), db.ListRecommendedPostsParams{
		OffsetCount: int32(offset),
		LimitCount:  int32(pageSize),
	})
	if err != nil {
		log.Error().Err(err).Msg("获取发现流失败")
		render.Error(w, http.StatusInternalServerError, "获取发现流失败")
		return
	}
	items := make([]listPostsItemResponse, 0, len(rows))
	for i := range rows {
		items = append(items, listPostsItemResponse{
			ID:           rows[i].ID,
			Title:        rows[i].Title,
			Content:      rows[i].Content,
			ViewCount:    rows[i].ViewCount,
			LikeCount:    rows[i].LikeCount,
			CollectCount: rows[i].CollectCount,
			CommentCount: rows[i].CommentCount,
			Author:       toAuthorFromFeed(rows[i].AuthorID, rows[i].AuthorUsername, rows[i].AuthorAvatar),
			CoverURL:     media.CDNURL(rows[i].CoverKey),
			CreatedAt:    rows[i].CreatedAt,
		})
	}
	render.Success(w, "查询成功", items)
}
