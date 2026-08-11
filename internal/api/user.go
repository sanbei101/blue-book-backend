package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/phuslu/log"
	"golang.org/x/crypto/bcrypt"

	"github.com/sanbei101/blue-book/internal/db"
	"github.com/sanbei101/blue-book/internal/pkg/jwt"
	"github.com/sanbei101/blue-book/internal/pkg/render"
)

type UserHandler struct {
	store *db.Store
}

func NewUserHandler(store *db.Store) *UserHandler {
	return &UserHandler{store: store}
}

// ---- 注册 ----

type registerRequest struct {
	// 用户名
	Username string `json:"username" validate:"required,min=3,max=32"`
	// 密码
	Password string `json:"password" validate:"required,min=6,max=128"`
}

type authResponse struct {
	// Access token
	AccessToken string `json:"access_token"`
	// Refresh token
	RefreshToken string `json:"refresh_token"`
	// Access token 有效期,单位为秒
	ExpiresIn int64 `json:"expires_in"`
	// 用户信息
	User userResponse `json:"user"`
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (h *UserHandler) issueTokens(ctx context.Context, userID uuid.UUID) (authResponse, error) {
	accessToken, err := jwt.GenerateToken(userID)
	if err != nil {
		return authResponse{}, err
	}
	refreshToken, err := jwt.GenerateRefreshToken()
	if err != nil {
		return authResponse{}, err
	}
	err = h.store.ExecTx(ctx, func(q *db.Queries) error {
		_, err := q.CreateSession(ctx, db.CreateSessionParams{
			ID:               uuid.Must(uuid.NewV7()),
			UserID:           userID,
			RefreshTokenHash: jwt.HashRefreshToken(refreshToken),
			ExpiresAt:        time.Now().Add(jwt.RefreshTokenTTL),
		})
		return err
	})
	if err != nil {
		return authResponse{}, err
	}
	return authResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(jwt.AccessTokenTTL / time.Second),
	}, nil
}

type userResponse struct {
	// 用户 ID
	ID uuid.UUID `json:"id"`
	// 用户名
	Username string `json:"username"`
	// 头像地址
	AvatarURL string `json:"avatar_url,omitempty"`
	// 个人简介
	Bio string `json:"bio,omitempty"`
}

type userProfileResponse struct {
	ID                          uuid.UUID `json:"id"                              validate:"required"`
	Username                    string    `json:"username"                        validate:"required"`
	AvatarURL                   string    `json:"avatar_url"                      validate:"required"`
	Bio                         string    `json:"bio"                             validate:"required"`
	PostCount                   int64     `json:"post_count"                      validate:"required,min=0"`
	FollowerCount               int64     `json:"follower_count"                  validate:"required,min=0"`
	FollowingCount              int64     `json:"following_count"                 validate:"required,min=0"`
	ReceivedLikeAndCollectCount int64     `json:"received_like_and_collect_count" validate:"required,min=0"`
	ViewerFollowing             bool      `json:"viewer_following"                validate:"required"`
}

func toUserResponse(u *db.User) userResponse {
	resp := userResponse{
		ID:       u.ID,
		Username: u.Username,
	}
	if u.AvatarURL.Valid {
		resp.AvatarURL = u.AvatarURL.String
	}
	if u.Bio.Valid {
		resp.Bio = u.Bio.String
	}
	return resp
}

func (h *UserHandler) profileResponse(
	ctx context.Context,
	userID, viewerID uuid.UUID,
) (userProfileResponse, error) {
	user, err := h.store.GetUserByID(ctx, userID)
	if err != nil {
		return userProfileResponse{}, err
	}
	postCount, err := h.store.CountPostsByUser(ctx, userID)
	if err != nil {
		return userProfileResponse{}, err
	}
	followerCount, err := h.store.GetFollowerCount(ctx, userID)
	if err != nil {
		return userProfileResponse{}, err
	}
	followingCount, err := h.store.GetFollowingCount(ctx, userID)
	if err != nil {
		return userProfileResponse{}, err
	}
	receivedCount, err := h.store.CountReceivedLikeAndCollectByUser(ctx, userID)
	if err != nil {
		return userProfileResponse{}, err
	}
	resp := userProfileResponse{
		ID:                          user.ID,
		Username:                    user.Username,
		PostCount:                   postCount,
		FollowerCount:               followerCount,
		FollowingCount:              followingCount,
		ReceivedLikeAndCollectCount: receivedCount,
	}
	if user.AvatarURL.Valid {
		resp.AvatarURL = user.AvatarURL.String
	}
	if user.Bio.Valid {
		resp.Bio = user.Bio.String
	}
	if viewerID != uuid.Nil {
		resp.ViewerFollowing, err = h.store.IsFollowing(ctx, db.IsFollowingParams{
			FollowerID:  viewerID,
			FollowingID: userID,
		})
		if err != nil {
			return userProfileResponse{}, err
		}
	}
	return resp, nil
}

// 用户注册
//
//	@Summary	用户注册
//	@Tags		auth
//	@Param		body	body		registerRequest	true	"注册信息"
//	@Success	200		{object}	render.Response[authResponse]
//	@Failure	409		{object}	render.errorResponse
//	@Failure	500		{object}	render.errorResponse
//	@Router		/auth/register [post]
func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	body, err := render.ReadBody[registerRequest](w, r)
	if err != nil {
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Error().Err(err).Msg("生成密码哈希失败")
		render.Error(w, http.StatusInternalServerError, "注册失败")
		return
	}

	user, err := h.store.CreateUser(r.Context(), db.CreateUserParams{
		ID:           uuid.Must(uuid.NewV7()),
		Username:     body.Username,
		PasswordHash: string(passwordHash),
		AvatarURL:    pgtype.Text{},
		Bio:          pgtype.Text{},
	})
	if err != nil {
		log.Error().Err(err).Msg("注册用户失败")
		if isUniqueViolation(err) {
			render.Error(w, http.StatusConflict, "用户名已存在")
			return
		}
		render.Error(w, http.StatusInternalServerError, "注册失败")
		return
	}

	auth, err := h.issueTokens(r.Context(), user.ID)
	if err != nil {
		log.Error().Err(err).Msg("创建登录会话失败")
		render.Error(w, http.StatusInternalServerError, "注册失败")
		return
	}

	auth.User = toUserResponse(&user)
	render.Success(w, "注册成功", auth)
}

// ---- 登录 ----

type loginRequest struct {
	// 用户名
	Username string `json:"username" validate:"required"`
	// 密码
	Password string `json:"password" validate:"required"`
}

// 用户登录
//
//	@Summary	用户登录
//	@Tags		auth
//	@Param		body	body		loginRequest	true	"登录信息"
//	@Success	200		{object}	render.Response[authResponse]
//	@Failure	401		{object}	render.errorResponse
//	@Failure	500		{object}	render.errorResponse
//	@Router		/auth/login [post]
func (h *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
	body, err := render.ReadBody[loginRequest](w, r)
	if err != nil {
		return
	}

	user, err := h.store.GetUserByUsername(r.Context(), body.Username)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.Password)); err != nil {
		render.Error(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}

	auth, err := h.issueTokens(r.Context(), user.ID)
	if err != nil {
		log.Error().Err(err).Msg("创建登录会话失败")
		render.Error(w, http.StatusInternalServerError, "登录失败")
		return
	}

	auth.User = toUserResponse(&user)
	render.Success(w, "登录成功", auth)
}

type refreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// 刷新登录凭证
//
//	@Summary	刷新登录凭证
//	@Tags		auth
//	@Param		body	body		refreshTokenRequest	true	"刷新凭证"
//	@Success	200		{object}	render.Response[authResponse]
//	@Failure	400		{object}	render.errorResponse
//	@Failure	401		{object}	render.errorResponse
//	@Router		/auth/refresh [post]
func (h *UserHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	body, err := render.ReadBody[refreshTokenRequest](w, r)
	if err != nil {
		return
	}

	session, err := h.store.GetActiveSessionByTokenHash(r.Context(), jwt.HashRefreshToken(body.RefreshToken))
	if err != nil {
		render.Error(w, http.StatusUnauthorized, "刷新凭证无效或已过期")
		return
	}
	var user db.User
	var auth authResponse
	err = h.store.ExecTx(r.Context(), func(q *db.Queries) error {
		u, err := q.GetUserByID(r.Context(), session.UserID)
		if err != nil {
			return err
		}
		user = u

		accessToken, err := jwt.GenerateToken(user.ID)
		if err != nil {
			return err
		}
		refreshToken, err := jwt.GenerateRefreshToken()
		if err != nil {
			return err
		}
		if err := q.RevokeSession(r.Context(), session.ID); err != nil {
			return err
		}
		_, err = q.CreateSession(r.Context(), db.CreateSessionParams{
			ID:               uuid.Must(uuid.NewV7()),
			UserID:           user.ID,
			RefreshTokenHash: jwt.HashRefreshToken(refreshToken),
			ExpiresAt:        time.Now().Add(jwt.RefreshTokenTTL),
		})
		if err != nil {
			return err
		}
		auth = authResponse{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			ExpiresIn:    int64(jwt.AccessTokenTTL / time.Second),
		}
		return nil
	})
	if err != nil {
		log.Error().Err(err).Msg("刷新登录会话失败")
		render.Error(w, http.StatusUnauthorized, "刷新凭证无效或已过期")
		return
	}
	auth.User = toUserResponse(&user)
	render.Success(w, "刷新成功", auth)
}

// 获取当前登录用户
//
//	@Summary	获取当前登录用户
//	@Tags		auth
//	@Security	BearerAuth
//	@Success	200	{object}	render.Response[userResponse]
//	@Failure	401	{object}	render.errorResponse
//	@Router		/auth/me [get]
func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := jwt.GetUserIDFromContext(r)
	user, err := h.store.GetUserByID(r.Context(), userID)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, "用户不存在")
		return
	}
	render.Success(w, "查询成功", toUserResponse(&user))
}

// 退出当前账号的所有会话
//
//	@Summary	退出登录
//	@Tags		auth
//	@Security	BearerAuth
//	@Success	204	{object}	render.ResponseWithoutData
//	@Failure	500	{object}	render.errorResponse
//	@Router		/auth/logout [post]
func (h *UserHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if err := h.store.RevokeAllUserSessions(r.Context(), jwt.GetUserIDFromContext(r)); err != nil {
		log.Error().Err(err).Msg("退出登录失败")
		render.Error(w, http.StatusInternalServerError, "退出登录失败")
		return
	}
	render.SuccessNoData(w, "退出成功")
}

type changePasswordRequest struct {
	// 当前密码
	OldPassword string `json:"old_password" validate:"required"`
	// 新密码
	NewPassword string `json:"new_password" validate:"required,min=6,max=128"`
}

// 修改密码
//
//	@Summary	修改密码
//	@Tags		auth
//	@Security	BearerAuth
//	@Param		body	body		changePasswordRequest	true	"密码信息"
//	@Success	204		{object}	render.ResponseWithoutData
//	@Failure	400		{object}	render.errorResponse
//	@Failure	401		{object}	render.errorResponse
//	@Failure	500		{object}	render.errorResponse
//	@Router		/auth/password [put]
func (h *UserHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID := jwt.GetUserIDFromContext(r)
	body, err := render.ReadBody[changePasswordRequest](w, r)
	if err != nil {
		return
	}

	user, err := h.store.GetUserByID(r.Context(), userID)
	if err != nil {
		render.Error(w, http.StatusUnauthorized, "用户不存在")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.OldPassword)); err != nil {
		render.Error(w, http.StatusUnauthorized, "当前密码错误")
		return
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Error().Err(err).Msg("生成密码哈希失败")
		render.Error(w, http.StatusInternalServerError, "修改密码失败")
		return
	}
	if err := h.store.ExecTx(r.Context(), func(q *db.Queries) error {
		rows, err := q.UpdateUserPassword(r.Context(), db.UpdateUserPasswordParams{
			ID:           userID,
			PasswordHash: string(passwordHash),
		})
		if err != nil {
			return err
		}
		if rows == 0 {
			return errors.New("user not found")
		}
		return q.RevokeAllUserSessions(r.Context(), userID)
	}); err != nil {
		log.Error().Err(err).Msg("修改密码失败")
		render.Error(w, http.StatusInternalServerError, "修改密码失败")
		return
	}

	render.SuccessNoData(w, "修改成功")
}

// 获取用户资料
//
//	@Summary	获取用户资料
//	@Tags		users
//	@Param		user_id	path		string	true	"用户 ID"
//	@Success	200		{object}	render.Response[userProfileResponse]
//	@Failure	400		{object}	render.errorResponse
//	@Failure	404		{object}	render.errorResponse
//	@Router		/users/{user_id} [get]
func (h *UserHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userIDStr := chi.URLParam(r, "user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		render.Error(w, http.StatusBadRequest, "无效的用户 ID")
		return
	}

	profile, err := h.profileResponse(r.Context(), userID, jwt.GetUserIDFromContext(r))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			render.Error(w, http.StatusNotFound, "用户不存在")
			return
		}
		log.Error().Err(err).Msg("获取用户资料失败")
		render.Error(w, http.StatusInternalServerError, "获取用户资料失败")
		return
	}

	render.Success(w, "查询成功", profile)
}

// 获取当前用户资料
//
//	@Summary	获取当前用户资料
//	@Tags		users
//	@Security	BearerAuth
//	@Success	200	{object}	render.Response[userProfileResponse]
//	@Failure	401	{object}	render.errorResponse
//	@Failure	500	{object}	render.errorResponse
//	@Router		/me/profile [get]
func (h *UserHandler) MyProfile(w http.ResponseWriter, r *http.Request) {
	profile, err := h.profileResponse(r.Context(), jwt.GetUserIDFromContext(r), jwt.GetUserIDFromContext(r))
	if err != nil {
		log.Error().Err(err).Msg("获取当前用户资料失败")
		render.Error(w, http.StatusInternalServerError, "获取用户资料失败")
		return
	}
	render.Success(w, "查询成功", profile)
}

// ---- 更新资料 ----

type updateProfileRequest struct {
	// 用户名
	Username string `json:"username" validate:"required,min=3,max=32"`
	// 头像 URL
	AvatarURL string `json:"avatar_url"`
	// 个人简介
	Bio string `json:"bio" validate:"max=200"`
}

// 更新用户资料
//
//	@Summary	更新用户资料
//	@Tags		users
//	@Security	BearerAuth
//	@Param		body	body		updateProfileRequest	true	"更新信息"
//	@Success	200		{object}	render.Response[userResponse]
//	@Failure	400		{object}	render.errorResponse
//	@Failure	500		{object}	render.errorResponse
//	@Router		/users/profile [put]
func (h *UserHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	currentUserID := jwt.GetUserIDFromContext(r)

	body, err := render.ReadBody[updateProfileRequest](w, r)
	if err != nil {
		return
	}

	avatar := pgtype.Text{}
	if body.AvatarURL != "" {
		avatar = pgtype.Text{String: body.AvatarURL, Valid: true}
	}
	bio := pgtype.Text{}
	if body.Bio != "" {
		bio = pgtype.Text{String: body.Bio, Valid: true}
	}

	user, err := h.store.UpdateUser(r.Context(), db.UpdateUserParams{
		ID:        currentUserID,
		Username:  body.Username,
		AvatarURL: avatar,
		Bio:       bio,
	})
	if err != nil {
		log.Error().Err(err).Msg("更新用户资料失败")
		if isUniqueViolation(err) {
			render.Error(w, http.StatusConflict, "用户名已存在")
			return
		}
		render.Error(w, http.StatusInternalServerError, "更新失败")
		return
	}

	render.Success(w, "更新成功", toUserResponse(&user))
}
