package api

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/phuslu/log"
	"net/http"
	"uuid"

	"github.com/sanbei101/blue-book/internal/db"
	"github.com/sanbei101/blue-book/internal/pkg/jwt"
	"github.com/sanbei101/blue-book/internal/pkg/render"
)

type FollowHandler struct {
	store *db.Store
}

func NewFollowHandler(store *db.Store) *FollowHandler {
	return &FollowHandler{store: store}
}

type followStatusResponse struct {
	// 是否已关注
	ViewerFollowing bool `json:"viewer_following" validate:"required"`
	// 该用户粉丝数
	FollowerCount int64 `json:"follower_count" validate:"required,min=0"`
}

// 关注用户
//
//	@Summary	关注用户
//	@Tags		follows
//	@Security	BearerAuth
//	@Param		user_id	path		string	true	"目标用户 ID"
//	@Success	200		{object}	render.Response[followStatusResponse]
//	@Failure	400		{object}	render.errorResponse
//	@Failure	404		{object}	render.errorResponse
//	@Failure	500		{object}	render.errorResponse
//	@Router		/users/{user_id}/follow [put]
func (h *FollowHandler) Follow(w http.ResponseWriter, r *http.Request) {
	targetID, ok := parseUUIDParam(r, "user_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的用户 ID")
		return
	}
	currentUserID := jwt.GetUserIDFromContext(r)

	if currentUserID == targetID {
		render.Error(w, http.StatusBadRequest, "不能关注自己")
		return
	}

	var resp followStatusResponse
	err := h.store.ExecTx(r.Context(), func(q *db.Queries) error {
		if _, err := q.GetUserByID(r.Context(), targetID); err != nil {
			return err
		}
		rows, err := q.AddFollow(r.Context(), db.AddFollowParams{
			FollowerID:  currentUserID,
			FollowingID: targetID,
		})
		if err != nil {
			return err
		}
		if rows > 0 {
			if err := q.CreateFollowNotification(r.Context(), db.CreateFollowNotificationParams{
				ID: uuid.NewV7(), RecipientID: targetID, ActorID: currentUserID,
			}); err != nil {
				return err
			}
		}
		following, err := q.IsFollowing(r.Context(), db.IsFollowingParams{
			FollowerID:  currentUserID,
			FollowingID: targetID,
		})
		if err != nil {
			return err
		}
		followerCount, err := q.GetFollowerCount(r.Context(), targetID)
		if err != nil {
			return err
		}
		resp = followStatusResponse{ViewerFollowing: following, FollowerCount: followerCount}
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			render.Error(w, http.StatusNotFound, "用户不存在")
			return
		}
		log.Error().Err(err).Msg("关注失败")
		render.Error(w, http.StatusInternalServerError, "关注失败")
		return
	}
	render.Success(w, "关注成功", resp)
}

// 取消关注
//
//	@Summary	取消关注
//	@Tags		follows
//	@Security	BearerAuth
//	@Param		user_id	path		string	true	"目标用户 ID"
//	@Success	200		{object}	render.Response[followStatusResponse]
//	@Failure	400		{object}	render.errorResponse
//	@Failure	404		{object}	render.errorResponse
//	@Failure	500		{object}	render.errorResponse
//	@Router		/users/{user_id}/follow [delete]
func (h *FollowHandler) Unfollow(w http.ResponseWriter, r *http.Request) {
	targetID, ok := parseUUIDParam(r, "user_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的用户 ID")
		return
	}
	currentUserID := jwt.GetUserIDFromContext(r)

	var resp followStatusResponse
	err := h.store.ExecTx(r.Context(), func(q *db.Queries) error {
		if _, err := q.GetUserByID(r.Context(), targetID); err != nil {
			return err
		}
		if _, err := q.RemoveFollow(r.Context(), db.RemoveFollowParams{
			FollowerID:  currentUserID,
			FollowingID: targetID,
		}); err != nil {
			return err
		}
		following, err := q.IsFollowing(r.Context(), db.IsFollowingParams{
			FollowerID:  currentUserID,
			FollowingID: targetID,
		})
		if err != nil {
			return err
		}
		followerCount, err := q.GetFollowerCount(r.Context(), targetID)
		if err != nil {
			return err
		}
		resp = followStatusResponse{ViewerFollowing: following, FollowerCount: followerCount}
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			render.Error(w, http.StatusNotFound, "用户不存在")
			return
		}
		log.Error().Err(err).Msg("取消关注失败")
		render.Error(w, http.StatusInternalServerError, "取消关注失败")
		return
	}
	render.Success(w, "取消关注成功", resp)
}

// ---- 粉丝列表 ----

type followUserResponse struct {
	// 用户 ID
	ID uuid.UUID `json:"id" validate:"required"`
	// 用户名
	Username string `json:"username" validate:"required"`
	// 头像地址
	AvatarURL string `json:"avatar_url" validate:"required"`
	// 个人简介
	Bio string `json:"bio" validate:"required"`
	// 当前用户是否已关注
	ViewerFollowing bool `json:"viewer_following" validate:"required"`
}

func newFollowUserResponse(
	id uuid.UUID,
	username string,
	avatarURL, bio pgtype.Text,
) followUserResponse {
	resp := followUserResponse{
		ID:       id,
		Username: username,
	}
	if avatarURL.Valid {
		resp.AvatarURL = avatarURL.String
	}
	if bio.Valid {
		resp.Bio = bio.String
	}
	return resp
}

func applyViewerFollowingStates(
	ctx context.Context,
	store *db.Store,
	viewerID uuid.UUID,
	users []followUserResponse,
) error {
	if viewerID == uuid.Nil() || len(users) == 0 {
		return nil
	}
	userIDs := make([]uuid.UUID, 0, len(users))
	for i := range users {
		userIDs = append(userIDs, users[i].ID)
	}
	rows, err := store.ListViewerFollowingStates(ctx, db.ListViewerFollowingStatesParams{
		ViewerID: viewerID,
		UserIds:  userIDs,
	})
	if err != nil {
		return err
	}
	states := make(map[uuid.UUID]bool, len(rows))
	for i := range rows {
		states[rows[i].UserID] = rows[i].ViewerFollowing
	}
	for i := range users {
		users[i].ViewerFollowing = states[users[i].ID]
	}
	return nil
}

// 获取粉丝列表
//
//	@Summary	获取粉丝列表
//	@Tags		follows
//	@Param		user_id		path		string	true	"用户 ID"
//	@Param		page		query		int		false	"页码"	default(1)
//	@Param		page_size	query		int		false	"每页数量"	default(20)
//	@Success	200			{object}	render.Response[pageResponse[followUserResponse]]
//	@Failure	400			{object}	render.errorResponse
//	@Failure	500			{object}	render.errorResponse
//	@Router		/users/{user_id}/followers [get]
func (h *FollowHandler) ListFollowers(w http.ResponseWriter, r *http.Request) {
	userID, ok := parseUUIDParam(r, "user_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的用户 ID")
		return
	}

	offset, pageSize := Pagination(r, 1, 20, 50)

	rows, err := h.store.ListFollowers(r.Context(), db.ListFollowersParams{
		FollowingID: userID,
		OffsetCount: int32(offset),
		LimitCount:  int32(pageSize),
	})
	if err != nil {
		log.Error().Err(err).Msg("获取粉丝列表失败")
		render.Error(w, http.StatusInternalServerError, "获取粉丝列表失败")
		return
	}
	total, err := h.store.CountFollowers(r.Context(), userID)
	if err != nil {
		log.Error().Err(err).Msg("统计粉丝列表失败")
		render.Error(w, http.StatusInternalServerError, "获取粉丝列表失败")
		return
	}

	users := make([]followUserResponse, 0, len(rows))
	for i := range rows {
		user := newFollowUserResponse(
			rows[i].ID,
			rows[i].Username,
			rows[i].AvatarURL,
			rows[i].Bio,
		)
		users = append(users, user)
	}
	if err := applyViewerFollowingStates(r.Context(), h.store, jwt.GetUserIDFromContext(r), users); err != nil {
		log.Error().Err(err).Msg("获取粉丝查看者状态失败")
		render.Error(w, http.StatusInternalServerError, "获取粉丝列表失败")
		return
	}

	render.Success(w, "查询成功", newPageResponse(users, offset, pageSize, total))
}

// 获取关注列表
//
//	@Summary	获取关注列表
//	@Tags		follows
//	@Param		user_id		path		string	true	"用户 ID"
//	@Param		page		query		int		false	"页码"	default(1)
//	@Param		page_size	query		int		false	"每页数量"	default(20)
//	@Success	200			{object}	render.Response[pageResponse[followUserResponse]]
//	@Failure	400			{object}	render.errorResponse
//	@Failure	500			{object}	render.errorResponse
//	@Router		/users/{user_id}/following [get]
func (h *FollowHandler) ListFollowing(w http.ResponseWriter, r *http.Request) {
	userID, ok := parseUUIDParam(r, "user_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的用户 ID")
		return
	}

	offset, pageSize := Pagination(r, 1, 20, 50)

	rows, err := h.store.ListFollowing(r.Context(), db.ListFollowingParams{
		FollowerID:  userID,
		OffsetCount: int32(offset),
		LimitCount:  int32(pageSize),
	})
	if err != nil {
		log.Error().Err(err).Msg("获取关注列表失败")
		render.Error(w, http.StatusInternalServerError, "获取关注列表失败")
		return
	}
	total, err := h.store.CountFollowing(r.Context(), userID)
	if err != nil {
		log.Error().Err(err).Msg("统计关注列表失败")
		render.Error(w, http.StatusInternalServerError, "获取关注列表失败")
		return
	}

	users := make([]followUserResponse, 0, len(rows))
	for i := range rows {
		user := newFollowUserResponse(
			rows[i].ID,
			rows[i].Username,
			rows[i].AvatarURL,
			rows[i].Bio,
		)
		users = append(users, user)
	}
	if err := applyViewerFollowingStates(r.Context(), h.store, jwt.GetUserIDFromContext(r), users); err != nil {
		log.Error().Err(err).Msg("获取关注查看者状态失败")
		render.Error(w, http.StatusInternalServerError, "获取关注列表失败")
		return
	}

	render.Success(w, "查询成功", newPageResponse(users, offset, pageSize, total))
}
