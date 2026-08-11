package seed

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"github.com/sanbei101/blue-book/internal/pkg/media"
)

const cardEngineURL = "http://localhost:5174/proto.cardengine.v1.CardEngineService/CardsList"

// 所有可用模板 ID
var allTemplateIDs = []string{
	// 清新文艺
	"question-blue",
	"green-notebook",
	"beige-paper",
	"lavender-soft",
	// 活力醒目
	"orange-burst",
	"red-stamp",
	"pink-bubble",
	// 沉稳专业
	"editorial-black",
	"blue-grid",
	"mono-frame",
	// 深色主题
	"midnight-stars",
	// 复古怀旧
	"yellow-memo",
}

// cardEngineRequest 卡片引擎 API 请求体
type cardEngineRequest struct {
	Title       string   `json:"title,omitempty"`
	Keyword     string   `json:"keyword,omitempty"`
	TemplateIDs []string `json:"templateIds,omitempty"`
}

// cardEngineResponse 卡片引擎 API 响应体
type cardEngineResponse struct {
	Templates []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"templates"`
}

type cardAsset struct {
	ObjectKey string
	Width     int32
	Height    int32
}

// cardCache 卡片 URL 缓存
type cardCache struct {
	mu    sync.RWMutex
	cache map[string][]cardAsset
}

var cards = &cardCache{
	cache: make(map[string][]cardAsset),
}

// fetchCardAssets 调用卡片引擎 API 并读取对象存储中的图片尺寸。
func (c *cardCache) fetchCardAssets(
	ctx context.Context,
	rng *rand.Rand,
	presigner *media.Presigner,
	title string,
) ([]cardAsset, error) {
	if presigner == nil {
		return nil, media.ErrNotConfigured
	}

	// 检查缓存
	c.mu.RLock()
	if urls, ok := c.cache[title]; ok {
		c.mu.RUnlock()
		return urls, nil
	}
	c.mu.RUnlock()

	// 构建请求
	reqBody := cardEngineRequest{
		Title:       title,
		TemplateIDs: []string{allTemplateIDs[rng.IntN(len(allTemplateIDs))]},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// 发送请求
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cardEngineURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result cardEngineResponse
	if err := json.UnmarshalRead(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Templates) == 0 {
		return nil, errors.New("no templates returned")
	}

	assets := make([]cardAsset, 0, len(result.Templates))
	for _, t := range result.Templates {
		if t.URL != "" {
			objectKey, err := presigner.ObjectKeyFromURL(t.URL)
			if err != nil {
				return nil, fmt.Errorf("parse card object URL: %w", err)
			}
			width, height, err := presigner.ImageDimensions(ctx, objectKey)
			if err != nil {
				return nil, fmt.Errorf("decode card image dimensions: %w", err)
			}
			assets = append(assets, cardAsset{ObjectKey: objectKey, Width: width, Height: height})
		}
	}
	if len(assets) == 0 {
		return nil, errors.New("no card images returned")
	}

	// 缓存结果
	c.mu.Lock()
	c.cache[title] = assets
	c.mu.Unlock()

	return assets, nil
}
