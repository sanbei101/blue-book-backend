package seed

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/phuslu/log"

	"github.com/sanbei101/blue-book/internal/db"
	"github.com/sanbei101/blue-book/internal/pkg/media"
)

// seedPosts 创建种子帖子和媒体
func (s *Seeder) seedPosts(ctx context.Context, users []db.User) ([]db.Post, []db.PostMedium, error) {
	if s.presigner == nil {
		return nil, nil, media.ErrNotConfigured
	}

	posts := make([]db.Post, 0, len(postSeeds))
	allMedia := make([]db.PostMedium, 0, len(postSeeds)*3)

	// 预先获取并上传所有帖子的卡片图片
	total := len(postSeeds)
	log.Printf("Fetching card images from card engine... (0/%d)", total)
	cardAssets := make([][]cardAsset, 0, total)
	for i, p := range postSeeds {
		log.Printf("[%d/%d] Fetching cards for %q...", i+1, total, p.Title)
		assets, err := cards.fetchCardAssets(ctx, s.rng, s.presigner, p.Title)
		if err != nil {
			return nil, nil, fmt.Errorf("fetch cards for post %d: %w", i, err)
		}
		cardAssets = append(cardAssets, assets)
		log.Printf("[%d/%d] Done ✓", i+1, total)
	}
	log.Printf("Card images uploaded successfully (%d/%d)", total, total)

	for i, p := range postSeeds {
		author := users[s.rng.IntN(len(users))]

		post, err := s.store.CreatePost(ctx, db.CreatePostParams{
			ID:      uuid.Must(uuid.NewV7()),
			UserID:  author.ID,
			Title:   p.Title,
			Content: p.Content,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("create post %d: %w", i, err)
		}

		// 添加媒体图片
		assets := cardAssets[i]
		mediaCount := min(len(assets), 3) // 最多使用 3 张卡片图片

		mediaParams := make([]db.CreatePostMediaParams, 0, mediaCount)
		for j := range mediaCount {
			asset := assets[j]
			mediaParams = append(mediaParams, db.CreatePostMediaParams{
				ID:        uuid.Must(uuid.NewV7()),
				PostID:    post.ID,
				MediaKey:  asset.ObjectKey,
				MediaType: db.MediaTypeEnumImage,
				Width:     asset.Width,
				Height:    asset.Height,
				SortOrder: int16(j),
			})
		}

		if len(mediaParams) > 0 {
			if _, err := s.store.CreatePostMedia(ctx, mediaParams); err != nil {
				return nil, nil, fmt.Errorf("create post media for post %d: %w", i, err)
			}
			// 收集媒体数据用于导出
			for _, param := range mediaParams {
				allMedia = append(allMedia, db.PostMedium{
					ID:        param.ID,
					PostID:    param.PostID,
					MediaKey:  param.MediaKey,
					MediaType: param.MediaType,
					Width:     param.Width,
					Height:    param.Height,
					SortOrder: param.SortOrder,
				})
			}
		}

		posts = append(posts, post)
	}

	return posts, allMedia, nil
}
