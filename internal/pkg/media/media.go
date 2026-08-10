package media

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/purus-dev/aqua"
)

// ErrNotConfigured 表示对象存储未配置,无法生成预签名地址
var ErrNotConfigured = errors.New("object storage is not configured")

// Presigner 负责为 S3 兼容存储生成预签名上传地址
type Presigner struct {
	client *aqua.Client
}

// NewPresigner 从环境变量读取对象存储配置并初始化客户端。
// 支持 S3_ACCESS_KEY_ID、S3_ACCESS_KEY_SECRET、S3_BUCKET、S3_ENDPOINT、
// S3_REGION、S3_USE_SSL、S3_USE_PATH_STYLE。
// 关键配置缺失时返回 ErrNotConfigured。
func NewPresigner() (*Presigner, error) {
	var config aqua.Config
	if err := config.FromEnv(); err != nil {
		return nil, ErrNotConfigured
	}
	if err := config.Validate(); err != nil {
		return nil, ErrNotConfigured
	}
	return &Presigner{client: aqua.NewClient(&config)}, nil
}

// PresignPutObject 返回一个用于直传对象的上传 URL 和对应的对象 key。
func (p *Presigner) PresignPutObject(
	_ context.Context,
	contentType string,
	ttl time.Duration,
) (url, key string, err error) {
	key = uuid.Must(uuid.NewV7()).String()

	uploadURL, err := p.client.PutObjectPresign(key, int64(ttl.Seconds()), contentType)
	if err != nil {
		return "", "", fmt.Errorf("presign put object: %w", err)
	}

	return uploadURL, key, nil
}

// ImageDimensions returns the dimensions of an uploaded object without downloading
// the complete image. The object key is resolved through a short-lived GET URL.
func (p *Presigner) ImageDimensions(ctx context.Context, objectKey string) (width, height int32, err error) {
	getURL, err := p.client.GetPresignedURL(objectKey, int64((5 * time.Minute).Seconds()))
	if err != nil {
		return 0, 0, fmt.Errorf("presign get object: %w", err)
	}

	return ImageDimensions(ctx, getURL)
}
