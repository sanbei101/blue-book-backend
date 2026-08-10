package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/phuslu/log"

	"github.com/sanbei101/blue-book/internal/pkg/media"
	"github.com/sanbei101/blue-book/internal/pkg/render"
)

type MediaHandler struct {
	presigner *media.Presigner
}

func NewMediaHandler(presigner *media.Presigner) *MediaHandler {
	return &MediaHandler{presigner: presigner}
}

type presignMediaRequest struct {
	// 文件 MIME 类型
	ContentType string `json:"content_type" validate:"required"`
}

type presignMediaResponse struct {
	// 上传地址
	UploadURL string `json:"upload_url"`
	// 对象存储 key,发布帖子时作为媒体 URL 使用
	ObjectKey string `json:"object_key"`
	// 上传地址有效期,单位为秒
	ExpiresIn int64 `json:"expires_in"`
}

// 获取媒体上传地址
//
//	@Summary	获取媒体上传地址
//	@Tags		media
//	@Security	BearerAuth
//	@Param		body	body		presignMediaRequest	true	"媒体信息"
//	@Success	200		{object}	render.Response[presignMediaResponse]
//	@Failure	400		{object}	render.errorResponse
//	@Failure	503		{object}	render.errorResponse
//	@Failure	500		{object}	render.errorResponse
//	@Router		/media/presign [post]
func (h *MediaHandler) Presign(w http.ResponseWriter, r *http.Request) {
	if h.presigner == nil {
		render.Error(w, http.StatusServiceUnavailable, "对象存储未配置")
		return
	}

	body, err := render.ReadBody[presignMediaRequest](w, r)
	if err != nil {
		return
	}

	ttl := 15 * time.Minute
	uploadURL, objectKey, err := h.presigner.PresignPutObject(r.Context(), body.ContentType, ttl)
	if err != nil {
		if errors.Is(err, media.ErrNotConfigured) {
			render.Error(w, http.StatusServiceUnavailable, "对象存储未配置")
			return
		}
		log.Error().Err(err).Msg("生成上传地址失败")
		render.Error(w, http.StatusInternalServerError, "生成上传地址失败")
		return
	}

	render.Success(w, "获取成功", presignMediaResponse{
		UploadURL: uploadURL,
		ObjectKey: objectKey,
		ExpiresIn: int64(ttl / time.Second),
	})
}
