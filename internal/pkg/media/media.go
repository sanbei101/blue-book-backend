package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // Register GIF image decoding.
	_ "image/jpeg" // Register JPEG image decoding.
	_ "image/png"  // Register PNG image decoding.
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/purus-dev/aqua"
	_ "golang.org/x/image/webp" // Register WebP image decoding.
)

// ErrNotConfigured 表示对象存储未配置,无法生成预签名地址
var ErrNotConfigured = errors.New("object storage is not configured")

const (
	initialImageHeaderSize = 64 * 1024
	maxImageHeaderSize     = 1024 * 1024
)

// CDNURL returns the public URL for an object key. CDN_BASE_URL is optional for local development.
func CDNURL(objectKey string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("CDN_BASE_URL")), "/")
	if baseURL == "" || objectKey == "" {
		return objectKey
	}
	return baseURL + "/" + strings.TrimLeft(objectKey, "/")
}

// Presigner 负责为 S3 兼容存储生成预签名上传地址
type Presigner struct {
	client *aqua.Client
	bucket string
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
	return &Presigner{client: aqua.NewClient(&config), bucket: config.Bucket}, nil
}

// PresignPutObject 返回一个用于直传对象的上传 URL 和对应的对象 key。
func (p *Presigner) PresignPutObject(
	_ context.Context,
	contentType string,
	ttl time.Duration,
) (uploadURL, key string, err error) {
	key = uuid.Must(uuid.NewV7()).String()

	uploadURL, err = p.client.PutObjectPresign(key, int64(ttl.Seconds()), contentType)
	if err != nil {
		return "", "", fmt.Errorf("presign put object: %w", err)
	}

	return uploadURL, key, nil
}

// ImageDimensions returns the dimensions of an uploaded image without downloading
// the complete object.
func (p *Presigner) ImageDimensions(ctx context.Context, objectKey string) (width, height int32, err error) {
	var decodeErr error
	for size := initialImageHeaderSize; size <= maxImageHeaderSize; size *= 4 {
		data, err := p.client.DownloadFileWithRange(ctx, objectKey, fmt.Sprintf("bytes=0-%d", size-1))
		if err != nil {
			return 0, 0, fmt.Errorf("download image header: %w", err)
		}

		config, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err == nil {
			const maxInt32 = 1<<31 - 1
			if config.Width <= 0 || config.Height <= 0 || config.Width > maxInt32 || config.Height > maxInt32 {
				return 0, 0, errors.New("invalid image dimensions")
			}
			return int32(config.Width), int32(config.Height), nil
		}
		decodeErr = err
		if len(data) < size {
			break
		}
	}
	return 0, 0, fmt.Errorf("decode image dimensions: %w", decodeErr)
}

// ObjectKeyFromURL extracts the S3 object key from a path-style or virtual-hosted URL.
func (p *Presigner) ObjectKeyFromURL(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse object URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("object URL must be absolute")
	}

	objectKey, err := url.PathUnescape(strings.TrimPrefix(parsed.EscapedPath(), "/"))
	if err != nil {
		return "", fmt.Errorf("decode object key: %w", err)
	}
	if after, ok := strings.CutPrefix(objectKey, p.bucket+"/"); ok {
		objectKey = after
	}
	if objectKey == "" {
		return "", errors.New("object URL does not contain a key")
	}
	return objectKey, nil
}
