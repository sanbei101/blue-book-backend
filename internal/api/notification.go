package api

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/phuslu/log"

	"github.com/sanbei101/blue-book/internal/db"
	"github.com/sanbei101/blue-book/internal/pkg/jwt"
	"github.com/sanbei101/blue-book/internal/pkg/render"
)

type NotificationHandler struct {
	store *db.Store
}

func NewNotificationHandler(store *db.Store) *NotificationHandler {
	return &NotificationHandler{store: store}
}

type notificationActorResponse struct {
	ID        uuid.UUID `json:"id"         validate:"required"`
	Username  string    `json:"username"   validate:"required"`
	AvatarURL string    `json:"avatar_url" validate:"required"`
}

type notificationResponse struct {
	ID               uuid.UUID                 `json:"id"                validate:"required"`
	NotificationType string                    `json:"notification_type" validate:"required"`
	PostID           *uuid.UUID                `json:"post_id,omitempty"`
	CommentID        *uuid.UUID                `json:"comment_id,omitempty"`
	Actor            notificationActorResponse `json:"actor"             validate:"required"`
	CreatedAt        time.Time                 `json:"created_at"        validate:"required"`
	ReadAt           *time.Time                `json:"read_at,omitempty"`
}

type unreadNotificationCountResponse struct {
	Count int64 `json:"count" validate:"required,min=0"`
}

// 获取通知列表
//
//	@Summary	获取通知列表
//	@Tags		notifications
//	@Security	BearerAuth
//	@Param		page		query		int	false	"页码"	default(1)
//	@Param		page_size	query		int	false	"每页数量"	default(20)
//	@Success	200			{object}	render.Response[pageResponse[notificationResponse]]
//	@Failure	500			{object}	render.errorResponse
//	@Router		/notifications [get]
func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	offset, pageSize := Pagination(r, 1, 20, 50)
	userID := jwt.GetUserIDFromContext(r)
	rows, err := h.store.ListNotifications(r.Context(), db.ListNotificationsParams{
		RecipientID: userID, OffsetCount: int32(offset), LimitCount: int32(pageSize),
	})
	if err != nil {
		log.Error().Err(err).Msg("获取通知列表失败")
		render.Error(w, http.StatusInternalServerError, "获取通知列表失败")
		return
	}
	total, err := h.store.CountNotifications(r.Context(), userID)
	if err != nil {
		log.Error().Err(err).Msg("统计通知列表失败")
		render.Error(w, http.StatusInternalServerError, "获取通知列表失败")
		return
	}
	items := make([]notificationResponse, 0, len(rows))
	for i := range rows {
		item := notificationResponse{
			ID: rows[i].ID, NotificationType: rows[i].NotificationType,
			PostID: rows[i].PostID, CommentID: rows[i].CommentID,
			Actor:     notificationActorResponse{ID: rows[i].ActorID, Username: rows[i].ActorUsername},
			CreatedAt: rows[i].CreatedAt,
		}
		if rows[i].ActorAvatar.Valid {
			item.Actor.AvatarURL = rows[i].ActorAvatar.String
		}
		if rows[i].ReadAt.Valid {
			readAt := rows[i].ReadAt.Time
			item.ReadAt = &readAt
		}
		items = append(items, item)
	}
	render.Success(w, "查询成功", newPageResponse(items, offset, pageSize, total))
}

// 获取未读通知数量
//
//	@Summary	获取未读通知数量
//	@Tags		notifications
//	@Security	BearerAuth
//	@Success	200	{object}	render.Response[unreadNotificationCountResponse]
//	@Failure	500	{object}	render.errorResponse
//	@Router		/notifications/unread-count [get]
func (h *NotificationHandler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	count, err := h.store.CountUnreadNotifications(r.Context(), jwt.GetUserIDFromContext(r))
	if err != nil {
		log.Error().Err(err).Msg("统计未读通知失败")
		render.Error(w, http.StatusInternalServerError, "获取未读通知数量失败")
		return
	}
	render.Success(w, "查询成功", unreadNotificationCountResponse{Count: count})
}

// 标记通知为已读
//
//	@Summary	标记通知为已读
//	@Tags		notifications
//	@Security	BearerAuth
//	@Param		notification_id	path		string	true	"通知 ID"
//	@Success	204				{object}	render.ResponseWithoutData
//	@Failure	400				{object}	render.errorResponse
//	@Failure	404				{object}	render.errorResponse
//	@Failure	500				{object}	render.errorResponse
//	@Router		/notifications/{notification_id}/read [patch]
func (h *NotificationHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	notificationID, ok := parseUUIDParam(r, "notification_id")
	if !ok {
		render.Error(w, http.StatusBadRequest, "无效的通知 ID")
		return
	}
	rows, err := h.store.MarkNotificationRead(r.Context(), db.MarkNotificationReadParams{
		ID: notificationID, RecipientID: jwt.GetUserIDFromContext(r),
	})
	if err != nil {
		log.Error().Err(err).Msg("标记通知已读失败")
		render.Error(w, http.StatusInternalServerError, "标记通知已读失败")
		return
	}
	if rows == 0 {
		render.Error(w, http.StatusNotFound, "通知不存在")
		return
	}
	render.SuccessNoData(w, "标记成功")
}

// 标记全部通知为已读
//
//	@Summary	标记全部通知为已读
//	@Tags		notifications
//	@Security	BearerAuth
//	@Success	204	{object}	render.ResponseWithoutData
//	@Failure	500	{object}	render.errorResponse
//	@Router		/notifications/read-all [post]
func (h *NotificationHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	if err := h.store.MarkAllNotificationsRead(r.Context(), jwt.GetUserIDFromContext(r)); err != nil {
		log.Error().Err(err).Msg("标记全部通知已读失败")
		render.Error(w, http.StatusInternalServerError, "标记全部通知已读失败")
		return
	}
	render.SuccessNoData(w, "标记成功")
}
