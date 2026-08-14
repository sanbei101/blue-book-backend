package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/phuslu/log"

	"github.com/sanbei101/blue-book/internal/db"
	"github.com/sanbei101/blue-book/internal/pkg/jwt"
	"github.com/sanbei101/blue-book/internal/pkg/media"
	"github.com/sanbei101/blue-book/internal/pkg/render"
)

type CollectionHandler struct {
	store *db.Store
}

func NewCollectionHandler(store *db.Store) *CollectionHandler {
	return &CollectionHandler{store: store}
}

type collectionStatusResponse struct {
	// 是否已收藏
	ViewerCollected bool `json:"viewer_collected" validate:"required"`
	// 收藏数量
	CollectCount int64 `json:"collect_count" validate:"required,min=0"`
}

// 收藏帖子
//
//	@Summary	收藏帖子
//	@Tags		collections
//	@Security	BearerAuth
//	@Param		post_id	path		string				true	"帖子 ID"
//	@Param		body	body		collectPostRequest	false	"收藏夹"
//	@Success	200		{object}	render.Response[collectionStatusResponse]
//	@Failure	400		{object}	render.errorResponse
//	@Failure	404		{object}	render.errorResponse
//	@Failure	500		{object}	render.errorResponse
//	@Router		/posts/{post_id}/collection [put]
func (h *CollectionHandler) Collect(w http.ResponseWriter, r *http.Request) {
	postID, ok := parseUUIDParam(r, "post_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的帖子 ID")
		return
	}
	userID := jwt.GetUserIDFromContext(r)

	body, err := render.ReadOptionalBody[collectPostRequest](w, r)
	if err != nil {
		return
	}
	folderID := body.FolderID

	var resp collectionStatusResponse
	err = h.store.ExecTx(r.Context(), func(q *db.Queries) error {
		if _, err := q.GetPostByID(r.Context(), postID); err != nil {
			return err
		}
		if folderID != nil {
			if _, err := q.GetCollectionFolderByID(r.Context(), db.GetCollectionFolderByIDParams{
				ID:     *folderID,
				UserID: userID,
			}); err != nil {
				return err
			}
		}
		rows, err := q.AddCollection(r.Context(), db.AddCollectionParams{
			UserID:   userID,
			PostID:   postID,
			FolderID: folderID,
		})
		if err != nil {
			return err
		}
		if rows > 0 {
			if err := q.IncrementPostCollectCount(r.Context(), postID); err != nil {
				return err
			}
		}
		collected, err := q.IsCollected(r.Context(), db.IsCollectedParams{
			UserID: userID,
			PostID: postID,
		})
		if err != nil {
			return err
		}
		post, err := q.GetPostByID(r.Context(), postID)
		if err != nil {
			return err
		}
		resp = collectionStatusResponse{ViewerCollected: collected, CollectCount: post.CollectCount}
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			render.Error(w, http.StatusNotFound, "帖子或收藏夹不存在")
			return
		}
		log.Error().Err(err).Msg("收藏失败")
		render.Error(w, http.StatusInternalServerError, "收藏失败")
		return
	}
	render.Success(w, "收藏成功", resp)
}

type collectPostRequest struct {
	// 收藏夹 ID,可选
	FolderID *uuid.UUID `json:"folder_id"`
}

// 取消收藏帖子
//
//	@Summary	取消收藏帖子
//	@Tags		collections
//	@Security	BearerAuth
//	@Param		post_id	path		string	true	"帖子 ID"
//	@Success	200		{object}	render.Response[collectionStatusResponse]
//	@Failure	400		{object}	render.errorResponse
//	@Failure	404		{object}	render.errorResponse
//	@Failure	500		{object}	render.errorResponse
//	@Router		/posts/{post_id}/collection [delete]
func (h *CollectionHandler) Uncollect(w http.ResponseWriter, r *http.Request) {
	postID, ok := parseUUIDParam(r, "post_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的帖子 ID")
		return
	}
	userID := jwt.GetUserIDFromContext(r)

	var resp collectionStatusResponse
	err := h.store.ExecTx(r.Context(), func(q *db.Queries) error {
		rows, err := q.RemoveCollection(r.Context(), db.RemoveCollectionParams{
			UserID: userID,
			PostID: postID,
		})
		if err != nil {
			return err
		}
		if rows > 0 {
			if err := q.DecrementPostCollectCount(r.Context(), postID); err != nil {
				return err
			}
		}
		collected, err := q.IsCollected(r.Context(), db.IsCollectedParams{
			UserID: userID,
			PostID: postID,
		})
		if err != nil {
			return err
		}
		post, err := q.GetPostByID(r.Context(), postID)
		if err != nil {
			return err
		}
		resp = collectionStatusResponse{ViewerCollected: collected, CollectCount: post.CollectCount}
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			render.Error(w, http.StatusNotFound, "帖子不存在")
			return
		}
		log.Error().Err(err).Msg("取消收藏失败")
		render.Error(w, http.StatusInternalServerError, "取消收藏失败")
		return
	}
	render.Success(w, "取消收藏成功", resp)
}

// 获取我的收藏列表
//
//	@Summary	获取我的收藏列表
//	@Tags		collections
//	@Security	BearerAuth
//	@Param		page		query		int	false	"页码"	default(1)
//	@Param		page_size	query		int	false	"每页数量"	default(20)
//	@Success	200			{object}	render.Response[pageResponse[listPostsItemResponse]]
//	@Failure	500			{object}	render.errorResponse
//	@Router		/me/collections [get]
func (h *CollectionHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := jwt.GetUserIDFromContext(r)
	offset, pageSize := Pagination(r, 1, 20, 50)

	rows, err := h.store.ListCollections(r.Context(), db.ListCollectionsParams{
		UserID:      userID,
		OffsetCount: int32(offset),
		LimitCount:  int32(pageSize),
	})
	if err != nil {
		log.Error().Err(err).Msg("获取收藏列表失败")
		render.Error(w, http.StatusInternalServerError, "获取收藏列表失败")
		return
	}
	total, err := h.store.CountCollections(r.Context(), userID)
	if err != nil {
		log.Error().Err(err).Msg("统计收藏列表失败")
		render.Error(w, http.StatusInternalServerError, "获取收藏列表失败")
		return
	}

	items := make([]listPostsItemResponse, 0, len(rows))
	for i := range rows {
		item := listPostsItemResponse{
			ID:           rows[i].ID,
			Title:        rows[i].Title,
			Content:      rows[i].Content,
			ViewCount:    rows[i].ViewCount,
			LikeCount:    rows[i].LikeCount,
			CollectCount: rows[i].CollectCount,
			CommentCount: rows[i].CommentCount,
			CoverURL:     media.CDNURL(rows[i].CoverKey),
			Width:        rows[i].Width,
			Height:       rows[i].Height,
			Author:       toAuthorFromFeed(rows[i].AuthorID, rows[i].AuthorUsername, rows[i].AuthorAvatar),
		}
		item.ViewerLiked, item.ViewerCollected, item.Author.ViewerFollowing, err = viewerPostStates(
			r.Context(), h.store, userID, item.ID, item.Author.ID,
		)
		if err != nil {
			log.Error().Err(err).Msg("获取收藏帖子查看者状态失败")
			render.Error(w, http.StatusInternalServerError, "获取收藏列表失败")
			return
		}
		items = append(items, item)
	}
	render.Success(w, "查询成功", newPageResponse(items, offset, pageSize, total))
}

// 获取收藏夹中的帖子
//
//	@Summary	获取收藏夹中的帖子
//	@Tags		collections
//	@Security	BearerAuth
//	@Param		folder_id	path		string	true	"收藏夹 ID"
//	@Param		page		query		int		false	"页码"	default(1)
//	@Param		page_size	query		int		false	"每页数量"	default(20)
//	@Success	200			{object}	render.Response[pageResponse[listPostsItemResponse]]
//	@Failure	400			{object}	render.errorResponse
//	@Failure	404			{object}	render.errorResponse
//	@Failure	500			{object}	render.errorResponse
//	@Router		/me/collections/folders/{folder_id}/posts [get]
func (h *CollectionHandler) ListFolderPosts(w http.ResponseWriter, r *http.Request) {
	userID := jwt.GetUserIDFromContext(r)
	folderID, ok := parseUUIDParam(r, "folder_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的收藏夹 ID")
		return
	}
	if _, err := h.store.GetCollectionFolderByID(r.Context(), db.GetCollectionFolderByIDParams{
		ID: folderID, UserID: userID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			render.Error(w, http.StatusNotFound, "收藏夹不存在")
			return
		}
		log.Error().Err(err).Msg("获取收藏夹失败")
		render.Error(w, http.StatusInternalServerError, "获取收藏夹帖子失败")
		return
	}
	offset, pageSize := Pagination(r, 1, 20, 50)
	rows, err := h.store.ListCollectionFolderPosts(r.Context(), db.ListCollectionFolderPostsParams{
		UserID: userID, FolderID: &folderID, OffsetCount: int32(offset), LimitCount: int32(pageSize),
	})
	if err != nil {
		log.Error().Err(err).Msg("获取收藏夹帖子失败")
		render.Error(w, http.StatusInternalServerError, "获取收藏夹帖子失败")
		return
	}
	total, err := h.store.CountCollectionFolderPosts(r.Context(), db.CountCollectionFolderPostsParams{
		UserID: userID, FolderID: &folderID,
	})
	if err != nil {
		log.Error().Err(err).Msg("统计收藏夹帖子失败")
		render.Error(w, http.StatusInternalServerError, "获取收藏夹帖子失败")
		return
	}
	items := make([]listPostsItemResponse, 0, len(rows))
	for i := range rows {
		item := listPostsItemResponse{
			ID: rows[i].ID, Title: rows[i].Title, Content: rows[i].Content,
			ViewCount: rows[i].ViewCount, LikeCount: rows[i].LikeCount,
			CollectCount: rows[i].CollectCount, CommentCount: rows[i].CommentCount,
			CoverURL: media.CDNURL(rows[i].CoverKey), Width: rows[i].Width,
			Height: rows[i].Height, CreatedAt: rows[i].CreatedAt,
			Author: toAuthorFromFeed(rows[i].AuthorID, rows[i].AuthorUsername, rows[i].AuthorAvatar),
		}
		item.ViewerLiked, item.ViewerCollected, item.Author.ViewerFollowing, err = viewerPostStates(
			r.Context(), h.store, userID, item.ID, item.Author.ID,
		)
		if err != nil {
			log.Error().Err(err).Msg("获取收藏夹帖子查看者状态失败")
			render.Error(w, http.StatusInternalServerError, "获取收藏夹帖子失败")
			return
		}
		items = append(items, item)
	}
	render.Success(w, "查询成功", newPageResponse(items, offset, pageSize, total))
}

type folderResponse struct {
	// 收藏夹 ID
	ID uuid.UUID `json:"id" validate:"required"`
	// 收藏夹名称
	Name string `json:"name" validate:"required"`
	// 创建时间
	CreatedAt time.Time `json:"created_at" validate:"required"`
}

func toFolderResponse(f *db.CollectionFolder) folderResponse {
	return folderResponse{
		ID:        f.ID,
		Name:      f.Name,
		CreatedAt: f.CreatedAt,
	}
}

// 获取收藏夹列表
//
//	@Summary	获取收藏夹列表
//	@Tags		collections
//	@Security	BearerAuth
//	@Param		page		query		int	false	"页码"	default(1)
//	@Param		page_size	query		int	false	"每页数量"	default(20)
//	@Success	200			{object}	render.Response[pageResponse[folderResponse]]
//	@Failure	500			{object}	render.errorResponse
//	@Router		/me/collections/folders [get]
func (h *CollectionHandler) ListFolders(w http.ResponseWriter, r *http.Request) {
	userID := jwt.GetUserIDFromContext(r)
	offset, pageSize := Pagination(r, 1, 20, 50)
	folders, err := h.store.ListCollectionFolders(r.Context(), db.ListCollectionFoldersParams{
		UserID:      userID,
		OffsetCount: int32(offset),
		LimitCount:  int32(pageSize),
	})
	if err != nil {
		log.Error().Err(err).Msg("获取收藏夹列表失败")
		render.Error(w, http.StatusInternalServerError, "获取收藏夹列表失败")
		return
	}
	total, err := h.store.CountCollectionFolders(r.Context(), userID)
	if err != nil {
		log.Error().Err(err).Msg("统计收藏夹列表失败")
		render.Error(w, http.StatusInternalServerError, "获取收藏夹列表失败")
		return
	}
	items := make([]folderResponse, 0, len(folders))
	for i := range folders {
		items = append(items, toFolderResponse(&folders[i]))
	}
	render.Success(w, "查询成功", newPageResponse(items, offset, pageSize, total))
}

type createFolderRequest struct {
	// 收藏夹名称
	Name string `json:"name" validate:"required,max=50"`
}

// 创建收藏夹
//
//	@Summary	创建收藏夹
//	@Tags		collections
//	@Security	BearerAuth
//	@Param		body	body		createFolderRequest	true	"收藏夹信息"
//	@Success	200		{object}	render.Response[folderResponse]
//	@Failure	400		{object}	render.errorResponse
//	@Failure	409		{object}	render.errorResponse
//	@Failure	500		{object}	render.errorResponse
//	@Router		/me/collections/folders [post]
func (h *CollectionHandler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	userID := jwt.GetUserIDFromContext(r)
	body, err := render.ReadBody[createFolderRequest](w, r)
	if err != nil {
		return
	}
	folder, err := h.store.CreateCollectionFolder(r.Context(), db.CreateCollectionFolderParams{
		ID:     uuid.Must(uuid.NewV7()),
		UserID: userID,
		Name:   body.Name,
	})
	if err != nil {
		if isUniqueViolation(err) {
			render.Error(w, http.StatusConflict, "收藏夹名称已存在")
			return
		}
		log.Error().Err(err).Msg("创建收藏夹失败")
		render.Error(w, http.StatusInternalServerError, "创建收藏夹失败")
		return
	}
	render.Success(w, "创建成功", toFolderResponse(&folder))
}

type updateFolderRequest struct {
	// 收藏夹名称
	Name string `json:"name" validate:"required,max=50"`
}

// 修改收藏夹
//
//	@Summary	修改收藏夹
//	@Tags		collections
//	@Security	BearerAuth
//	@Param		folder_id	path		string				true	"收藏夹 ID"
//	@Param		body		body		updateFolderRequest	true	"收藏夹信息"
//	@Success	200			{object}	render.Response[folderResponse]
//	@Failure	400			{object}	render.errorResponse
//	@Failure	404			{object}	render.errorResponse
//	@Failure	500			{object}	render.errorResponse
//	@Router		/me/collections/folders/{folder_id} [patch]
func (h *CollectionHandler) UpdateFolder(w http.ResponseWriter, r *http.Request) {
	userID := jwt.GetUserIDFromContext(r)
	folderID, ok := parseUUIDParam(r, "folder_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的收藏夹 ID")
		return
	}
	body, err := render.ReadBody[updateFolderRequest](w, r)
	if err != nil {
		return
	}
	folder, err := h.store.UpdateCollectionFolder(r.Context(), db.UpdateCollectionFolderParams{
		ID:     folderID,
		UserID: userID,
		Name:   body.Name,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			render.Error(w, http.StatusNotFound, "收藏夹不存在")
			return
		}
		if isUniqueViolation(err) {
			render.Error(w, http.StatusConflict, "收藏夹名称已存在")
			return
		}
		log.Error().Err(err).Msg("修改收藏夹失败")
		render.Error(w, http.StatusInternalServerError, "修改收藏夹失败")
		return
	}
	render.Success(w, "更新成功", toFolderResponse(&folder))
}

// 删除收藏夹
//
//	@Summary	删除收藏夹
//	@Tags		collections
//	@Security	BearerAuth
//	@Param		folder_id	path		string	true	"收藏夹 ID"
//	@Success	204			{object}	render.ResponseWithoutData
//	@Failure	400			{object}	render.errorResponse
//	@Failure	404			{object}	render.errorResponse
//	@Failure	500			{object}	render.errorResponse
//	@Router		/me/collections/folders/{folder_id} [delete]
func (h *CollectionHandler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	userID := jwt.GetUserIDFromContext(r)
	folderID, ok := parseUUIDParam(r, "folder_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的收藏夹 ID")
		return
	}
	rows, err := h.store.DeleteCollectionFolder(r.Context(), db.DeleteCollectionFolderParams{
		ID:     folderID,
		UserID: userID,
	})
	if err != nil {
		log.Error().Err(err).Msg("删除收藏夹失败")
		render.Error(w, http.StatusInternalServerError, "删除收藏夹失败")
		return
	}
	if rows == 0 {
		render.Error(w, http.StatusNotFound, "收藏夹不存在")
		return
	}
	render.SuccessNoData(w, "删除成功")
}
