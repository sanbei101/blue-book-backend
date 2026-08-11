package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/phuslu/log"

	"github.com/sanbei101/blue-book/internal/db"
	"github.com/sanbei101/blue-book/internal/pkg/jwt"
	"github.com/sanbei101/blue-book/internal/pkg/render"
)

type LikeHandler struct {
	store *db.Store
}

func NewLikeHandler(store *db.Store) *LikeHandler {
	return &LikeHandler{store: store}
}

type likeStatusResponse struct {
	// 是否已点赞
	ViewerLiked bool `json:"viewer_liked" validate:"required"`
	// 点赞数量
	LikeCount int64 `json:"like_count" validate:"required,min=0"`
}

func parseUUIDParam(r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

func (h *LikeHandler) likePostTx(r *http.Request, q *db.Queries, userID, postID uuid.UUID) (likeStatusResponse, error) {
	rows, err := q.AddPostLike(r.Context(), db.AddPostLikeParams{
		ID:     uuid.Must(uuid.NewV7()),
		UserID: userID,
		PostID: postID,
	})
	if err != nil {
		return likeStatusResponse{}, err
	}
	if rows > 0 {
		if err := q.IncrementPostLikeCount(r.Context(), postID); err != nil {
			return likeStatusResponse{}, err
		}
	}
	liked, err := q.IsPostLiked(r.Context(), db.IsPostLikedParams{UserID: userID, PostID: postID})
	if err != nil {
		return likeStatusResponse{}, err
	}
	post, err := q.GetPostByID(r.Context(), postID)
	if err != nil {
		return likeStatusResponse{}, err
	}
	return likeStatusResponse{ViewerLiked: liked, LikeCount: post.LikeCount}, nil
}

// 点赞帖子
//
//	@Summary	点赞帖子
//	@Tags		likes
//	@Security	BearerAuth
//	@Param		post_id	path		string	true	"帖子 ID"
//	@Success	200		{object}	render.Response[likeStatusResponse]
//	@Failure	400		{object}	render.errorResponse
//	@Failure	404		{object}	render.errorResponse
//	@Failure	500		{object}	render.errorResponse
//	@Router		/posts/{post_id}/like [put]
func (h *LikeHandler) LikePost(w http.ResponseWriter, r *http.Request) {
	postID, ok := parseUUIDParam(r, "post_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的帖子 ID")
		return
	}
	userID := jwt.GetUserIDFromContext(r)

	var resp likeStatusResponse
	err := h.store.ExecTx(r.Context(), func(q *db.Queries) error {
		var err error
		resp, err = h.likePostTx(r, q, userID, postID)
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			render.Error(w, http.StatusNotFound, "帖子不存在")
			return
		}
		log.Error().Err(err).Msg("点赞帖子失败")
		render.Error(w, http.StatusInternalServerError, "点赞失败")
		return
	}
	render.Success(w, "点赞成功", resp)
}

// 取消点赞帖子
//
//	@Summary	取消点赞帖子
//	@Tags		likes
//	@Security	BearerAuth
//	@Param		post_id	path		string	true	"帖子 ID"
//	@Success	200		{object}	render.Response[likeStatusResponse]
//	@Failure	400		{object}	render.errorResponse
//	@Failure	500		{object}	render.errorResponse
//	@Router		/posts/{post_id}/like [delete]
func (h *LikeHandler) UnlikePost(w http.ResponseWriter, r *http.Request) {
	postID, ok := parseUUIDParam(r, "post_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的帖子 ID")
		return
	}
	userID := jwt.GetUserIDFromContext(r)

	var resp likeStatusResponse
	err := h.store.ExecTx(r.Context(), func(q *db.Queries) error {
		rows, err := q.RemovePostLike(r.Context(), db.RemovePostLikeParams{UserID: userID, PostID: postID})
		if err != nil {
			return err
		}
		if rows > 0 {
			if err := q.DecrementPostLikeCount(r.Context(), postID); err != nil {
				return err
			}
		}
		liked, err := q.IsPostLiked(r.Context(), db.IsPostLikedParams{UserID: userID, PostID: postID})
		if err != nil {
			return err
		}
		post, err := q.GetPostByID(r.Context(), postID)
		if err != nil {
			return err
		}
		resp = likeStatusResponse{ViewerLiked: liked, LikeCount: post.LikeCount}
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			render.Error(w, http.StatusNotFound, "帖子不存在")
			return
		}
		log.Error().Err(err).Msg("取消点赞帖子失败")
		render.Error(w, http.StatusInternalServerError, "取消点赞失败")
		return
	}
	render.Success(w, "取消点赞成功", resp)
}

// 点赞评论
//
//	@Summary	点赞评论
//	@Tags		likes
//	@Security	BearerAuth
//	@Param		comment_id	path		string	true	"评论 ID"
//	@Success	200			{object}	render.Response[likeStatusResponse]
//	@Failure	400			{object}	render.errorResponse
//	@Failure	404			{object}	render.errorResponse
//	@Failure	500			{object}	render.errorResponse
//	@Router		/comments/{comment_id}/like [put]
func (h *LikeHandler) LikeComment(w http.ResponseWriter, r *http.Request) {
	commentID, ok := parseUUIDParam(r, "comment_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的评论 ID")
		return
	}
	userID := jwt.GetUserIDFromContext(r)

	var resp likeStatusResponse
	err := h.store.ExecTx(r.Context(), func(q *db.Queries) error {
		rows, err := q.AddCommentLike(r.Context(), db.AddCommentLikeParams{
			ID:        uuid.Must(uuid.NewV7()),
			UserID:    userID,
			CommentID: commentID,
		})
		if err != nil {
			return err
		}
		if rows > 0 {
			if err := q.IncrementCommentLikeCount(r.Context(), commentID); err != nil {
				return err
			}
		}
		comment, err := q.GetCommentByID(r.Context(), commentID)
		if err != nil {
			return err
		}
		liked, err := q.IsCommentLiked(r.Context(), db.IsCommentLikedParams{UserID: userID, CommentID: commentID})
		if err != nil {
			return err
		}
		resp = likeStatusResponse{ViewerLiked: liked, LikeCount: int64(comment.LikeCount)}
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			render.Error(w, http.StatusNotFound, "评论不存在")
			return
		}
		log.Error().Err(err).Msg("点赞评论失败")
		render.Error(w, http.StatusInternalServerError, "点赞失败")
		return
	}
	render.Success(w, "点赞成功", resp)
}

// 取消点赞评论
//
//	@Summary	取消点赞评论
//	@Tags		likes
//	@Security	BearerAuth
//	@Param		comment_id	path		string	true	"评论 ID"
//	@Success	200			{object}	render.Response[likeStatusResponse]
//	@Failure	400			{object}	render.errorResponse
//	@Failure	500			{object}	render.errorResponse
//	@Router		/comments/{comment_id}/like [delete]
func (h *LikeHandler) UnlikeComment(w http.ResponseWriter, r *http.Request) {
	commentID, ok := parseUUIDParam(r, "comment_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的评论 ID")
		return
	}
	userID := jwt.GetUserIDFromContext(r)

	var resp likeStatusResponse
	err := h.store.ExecTx(r.Context(), func(q *db.Queries) error {
		rows, err := q.RemoveCommentLike(r.Context(), db.RemoveCommentLikeParams{UserID: userID, CommentID: commentID})
		if err != nil {
			return err
		}
		if rows > 0 {
			if err := q.DecrementCommentLikeCount(r.Context(), commentID); err != nil {
				return err
			}
		}
		comment, err := q.GetCommentByID(r.Context(), commentID)
		if err != nil {
			return err
		}
		liked, err := q.IsCommentLiked(r.Context(), db.IsCommentLikedParams{UserID: userID, CommentID: commentID})
		if err != nil {
			return err
		}
		resp = likeStatusResponse{ViewerLiked: liked, LikeCount: int64(comment.LikeCount)}
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			render.Error(w, http.StatusNotFound, "评论不存在")
			return
		}
		log.Error().Err(err).Msg("取消点赞评论失败")
		render.Error(w, http.StatusInternalServerError, "取消点赞失败")
		return
	}
	render.Success(w, "取消点赞成功", resp)
}
