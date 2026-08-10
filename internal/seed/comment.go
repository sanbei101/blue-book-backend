package seed

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/sanbei101/blue-book/internal/db"
)

// seedComments 创建种子评论(包含嵌套回复)
func (s *Seeder) seedComments(ctx context.Context, users []db.User, posts []db.Post) ([]db.Comment, error) {
	comments := make([]db.Comment, 0, 120)

	for i := range posts {
		post := &posts[i]
		// 每个帖子 2-4 条一级评论
		topLevelCount := s.rng.IntN(3) + 2
		for range topLevelCount {
			commenter := users[s.rng.IntN(len(users))]

			comment, err := s.store.CreateComment(ctx, db.CreateCommentParams{
				ID:      uuid.Must(uuid.NewV7()),
				PostID:  post.ID,
				UserID:  commenter.ID,
				Content: commentSeeds[s.rng.IntN(len(commentSeeds))],
			})
			if err != nil {
				return nil, fmt.Errorf("create comment: %w", err)
			}

			comments = append(comments, comment)

			// 50% 概率生成嵌套回复(1-2 条)
			if s.rng.IntN(2) == 0 {
				replyCount := s.rng.IntN(2) + 1
				for range replyCount {
					replier := users[s.rng.IntN(len(users))]

					reply, err := s.store.CreateComment(ctx, db.CreateCommentParams{
						ID:       uuid.Must(uuid.NewV7()),
						PostID:   post.ID,
						UserID:   replier.ID,
						ParentID: &comment.ID,
						Content:  replySeeds[s.rng.IntN(len(replySeeds))],
					})
					if err != nil {
						return nil, fmt.Errorf("create reply: %w", err)
					}

					comments = append(comments, reply)
				}
			}
		}
	}

	return comments, nil
}
