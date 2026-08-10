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
	"github.com/sanbei101/blue-book/internal/pkg/render"
)

var ErrCommentNotInPost = errors.New("parent comment does not belong to the post")

type CommentHandler struct {
	store *db.Store
}

func NewCommentHandler(store *db.Store) *CommentHandler {
	return &CommentHandler{store: store}
}

// ---- 创建评论 ----

type createCommentRequest struct {
	// 帖子唯一标识 ID
	PostID uuid.UUID `json:"post_id" validate:"required"`
	// 父评论 ID,如果是顶级评论则为 nil
	ParentID *uuid.UUID `json:"parent_id"`
	// 评论内容
	Content string `json:"content" validate:"required,max=1000"`
}

type createCommentResponse struct {
	// 评论 ID
	ID uuid.UUID `json:"id"`
}

// 创建评论
//
//	@Summary	创建评论
//	@Tags		comments
//	@Security	BearerAuth
//	@Param		body	body		createCommentRequest	true	"评论内容"
//	@Success	200		{object}	render.Response[createCommentResponse]
//	@Failure	400		{object}	render.errorResponse
//	@Failure	404		{object}	render.errorResponse
//	@Failure	500		{object}	render.errorResponse
//	@Router		/comments [post]
func (h *CommentHandler) Create(w http.ResponseWriter, r *http.Request) {
	body, err := render.ReadBody[createCommentRequest](w, r)
	if err != nil {
		return
	}
	currentUserID := jwt.GetUserIDFromContext(r)

	var comment db.Comment
	err = h.store.ExecTx(r.Context(), func(q *db.Queries) error {
		if _, err := q.GetPostByID(r.Context(), body.PostID); err != nil {
			return err
		}
		if body.ParentID != nil {
			parent, err := q.GetCommentByID(r.Context(), *body.ParentID)
			if err != nil {
				return err
			}
			if parent.PostID != body.PostID {
				return ErrCommentNotInPost
			}
		}
		comment, err = q.CreateComment(r.Context(), db.CreateCommentParams{
			ID:       uuid.Must(uuid.NewV7()),
			PostID:   body.PostID,
			UserID:   currentUserID,
			ParentID: body.ParentID,
			Content:  body.Content,
		})
		if err != nil {
			return err
		}
		return q.IncrementPostCommentCount(r.Context(), body.PostID)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			render.Error(w, http.StatusNotFound, "帖子或父评论不存在")
			return
		}
		if errors.Is(err, ErrCommentNotInPost) {
			render.Error(w, http.StatusBadRequest, "父评论不属于该帖子")
			return
		}
		log.Error().Err(err).Msg("创建评论失败")
		render.Error(w, http.StatusInternalServerError, "创建评论失败")
		return
	}

	render.Success(w, "评论成功", createCommentResponse{ID: comment.ID})
}

// 删除评论
//
//	@Summary	删除评论
//	@Tags		comments
//	@Security	BearerAuth
//	@Param		comment_id	path		string	true	"评论 ID"
//	@Success	204			{object}	render.ResponseWithoutData
//	@Failure	400			{object}	render.errorResponse
//	@Failure	403			{object}	render.errorResponse
//	@Failure	404			{object}	render.errorResponse
//	@Failure	500			{object}	render.errorResponse
//	@Router		/comments/{comment_id} [delete]
func (h *CommentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	commentID, ok := parseUUIDParam(r, "comment_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的评论 ID")
		return
	}
	currentUserID := jwt.GetUserIDFromContext(r)

	comment, err := h.store.GetCommentByID(r.Context(), commentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			render.Error(w, http.StatusNotFound, "评论不存在")
			return
		}
		log.Error().Err(err).Msg("获取评论失败")
		render.Error(w, http.StatusInternalServerError, "删除评论失败")
		return
	}
	if comment.UserID != currentUserID {
		render.Error(w, http.StatusForbidden, "只能删除自己的评论")
		return
	}

	err = h.store.ExecTx(r.Context(), func(q *db.Queries) error {
		rows, err := q.DeleteComment(r.Context(), db.DeleteCommentParams{
			ID:     commentID,
			UserID: currentUserID,
		})
		if err != nil {
			return err
		}
		if rows == 0 {
			return pgx.ErrNoRows
		}
		return q.DecrementPostCommentCount(r.Context(), comment.PostID)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			render.Error(w, http.StatusNotFound, "评论不存在")
			return
		}
		log.Error().Err(err).Msg("删除评论失败")
		render.Error(w, http.StatusInternalServerError, "删除评论失败")
		return
	}

	render.SuccessNoData(w, "删除成功")
}

// ---- 帖子评论列表 ----

type commentResponse struct {
	// 评论 ID
	ID uuid.UUID `json:"id"`
	// 帖子 ID
	PostID uuid.UUID `json:"post_id"`
	// 用户 ID
	UserID uuid.UUID `json:"user_id"`
	// 父评论 ID,顶级评论为 nil
	ParentID *uuid.UUID `json:"parent_id,omitempty"`
	// 评论内容
	Content string `json:"content"`
	// 点赞数
	LikeCount int32 `json:"like_count"`
	// 作者用户名
	AuthorUsername string `json:"author_username"`
	// 作者头像地址
	AuthorAvatar string `json:"author_avatar,omitempty"`
	// 创建时间
	CreatedAt time.Time `json:"created_at"`
}

// 获取帖子评论列表
//
//	@Summary	获取帖子评论列表
//	@Tags		comments
//	@Param		post_id		path		string	true	"帖子 ID"
//	@Param		page		query		int		false	"页码"	default(1)
//	@Param		page_size	query		int		false	"每页数量"	default(20)
//	@Success	200			{object}	render.Response[[]commentResponse]
//	@Failure	400			{object}	render.errorResponse
//	@Failure	500			{object}	render.errorResponse
//	@Router		/posts/{post_id}/comments [get]
func (h *CommentHandler) ListByPost(w http.ResponseWriter, r *http.Request) {
	postID, ok := parseUUIDParam(r, "post_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的帖子 ID")
		return
	}

	offset, pageSize := Pagination(r, 1, 20, 50)

	rows, err := h.store.ListCommentsByPostID(r.Context(), db.ListCommentsByPostIDParams{
		PostID:      postID,
		OffsetCount: int32(offset),
		LimitCount:  int32(pageSize),
	})
	if err != nil {
		log.Error().Err(err).Msg("获取评论列表失败")
		render.Error(w, http.StatusInternalServerError, "获取评论列表失败")
		return
	}

	comments := make([]commentResponse, 0, len(rows))
	for i := range rows {
		c := commentResponse{
			ID:             rows[i].ID,
			PostID:         rows[i].PostID,
			UserID:         rows[i].UserID,
			Content:        rows[i].Content,
			LikeCount:      rows[i].LikeCount,
			AuthorUsername: rows[i].AuthorUsername,
			CreatedAt:      rows[i].CreatedAt,
		}
		if rows[i].ParentID != nil {
			c.ParentID = rows[i].ParentID
		}
		if rows[i].AuthorAvatar.Valid {
			c.AuthorAvatar = rows[i].AuthorAvatar.String
		}
		comments = append(comments, c)
	}

	render.Success(w, "查询成功", comments)
}
