package seed

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/sanbei101/blue-book/internal/db"
)

// seedLikes 创建种子点赞(帖子点赞 + 评论点赞)
func (s *Seeder) seedLikes(
	ctx context.Context,
	users []db.User,
	posts []db.Post,
	comments []db.Comment,
) ([]db.Like, error) {
	likes := make([]db.Like, 0)
	seen := make(map[string]bool)

	// 帖子点赞:每个帖子 3-8 个赞
	for i := range posts {
		post := &posts[i]
		likeCount := s.rng.IntN(6) + 3
		for range likeCount {
			user := users[s.rng.IntN(len(users))]
			key := user.ID.String() + post.ID.String() + "1"
			if seen[key] {
				continue
			}
			seen[key] = true

			likeID := uuid.Must(uuid.NewV7())
			_, err := s.store.AddPostLike(ctx, db.AddPostLikeParams{
				ID:     likeID,
				UserID: user.ID,
				PostID: post.ID,
			})
			if err != nil {
				return nil, fmt.Errorf("like post: %w", err)
			}
			if err := s.store.IncrementPostLikeCount(ctx, post.ID); err != nil {
				return nil, fmt.Errorf("increment post like count: %w", err)
			}
			likes = append(likes, db.Like{
				ID:         likeID,
				UserID:     user.ID,
				TargetID:   post.ID,
				TargetType: 1,
			})
		}
	}

	// 评论点赞:每个评论 0-3 个赞
	for i := range comments {
		comment := &comments[i]
		likeCount := s.rng.IntN(4)
		for range likeCount {
			user := users[s.rng.IntN(len(users))]
			key := user.ID.String() + comment.ID.String() + "2"
			if seen[key] {
				continue
			}
			seen[key] = true

			likeID := uuid.Must(uuid.NewV7())
			_, err := s.store.AddCommentLike(ctx, db.AddCommentLikeParams{
				ID:        likeID,
				UserID:    user.ID,
				CommentID: comment.ID,
			})
			if err != nil {
				return nil, fmt.Errorf("like comment: %w", err)
			}
			if err := s.store.IncrementCommentLikeCount(ctx, comment.ID); err != nil {
				return nil, fmt.Errorf("increment comment like count: %w", err)
			}
			likes = append(likes, db.Like{
				ID:         likeID,
				UserID:     user.ID,
				TargetID:   comment.ID,
				TargetType: 2,
			})
		}
	}

	return likes, nil
}

// seedFollows 创建种子关注关系
func (s *Seeder) seedFollows(ctx context.Context, users []db.User) ([]db.Follow, error) {
	follows := make([]db.Follow, 0)
	seen := make(map[string]bool)

	// 每个用户关注 3-8 个其他用户
	for i := range users {
		follower := &users[i]
		followCount := s.rng.IntN(6) + 3
		for range followCount {
			following := &users[s.rng.IntN(len(users))]
			if follower.ID == following.ID {
				continue
			}

			key := follower.ID.String() + following.ID.String()
			if seen[key] {
				continue
			}
			seen[key] = true

			_, err := s.store.AddFollow(ctx, db.AddFollowParams{
				FollowerID:  follower.ID,
				FollowingID: following.ID,
			})
			if err != nil {
				return nil, fmt.Errorf("follow: %w", err)
			}
			follows = append(follows, db.Follow{
				FollowerID:  follower.ID,
				FollowingID: following.ID,
			})
		}
	}

	return follows, nil
}
