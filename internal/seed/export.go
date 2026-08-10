package seed

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sanbei101/blue-book/internal/db"
)

// ExportSQL 导出种子数据为 SQL 文件
func ExportSQL(
	filename string,
	users []db.User,
	posts []db.Post,
	media []db.PostMedium,
	comments []db.Comment,
	likes []db.Like,
	follows []db.Follow,
) error {
	var sb strings.Builder

	sb.WriteString("-- Seed data generated at " + time.Now().Format(time.RFC3339) + "\n")
	sb.WriteString("-- This file can be imported directly into PostgreSQL\n\n")

	// 禁用外键检查以允许乱序插入
	sb.WriteString("SET session_replication_role = 'replica';\n\n")

	// Users
	sb.WriteString("-- Users\n")
	sb.WriteString("INSERT INTO users (id, username, password_hash, avatar_url, bio, created_at, updated_at) VALUES\n")
	for i := range users {
		u := &users[i]
		fmt.Fprintf(&sb, "    ('%s', '%s', '%s', '%s', '%s', '%s', '%s')",
			u.ID.String(),
			escapeSQL(u.Username),
			escapeSQL(u.PasswordHash),
			escapeSQL(u.AvatarURL.String),
			escapeSQL(u.Bio.String),
			u.CreatedAt.Format(time.RFC3339),
			u.UpdatedAt.Format(time.RFC3339))
		if i < len(users)-1 {
			sb.WriteString(",\n")
		} else {
			sb.WriteString("\n")
		}
	}
	sb.WriteString("ON CONFLICT (id) DO NOTHING;\n\n")

	// Posts
	sb.WriteString("-- Posts\n")
	sb.WriteString("INSERT INTO posts (id, user_id, title, content, view_count, created_at, updated_at) VALUES\n")
	for i := range posts {
		p := &posts[i]
		fmt.Fprintf(&sb, "    ('%s', '%s', '%s', '%s', %d, '%s', '%s')",
			p.ID.String(),
			p.UserID.String(),
			escapeSQL(p.Title),
			escapeSQL(p.Content),
			p.ViewCount,
			p.CreatedAt.Format(time.RFC3339),
			p.UpdatedAt.Format(time.RFC3339))
		if i < len(posts)-1 {
			sb.WriteString(",\n")
		} else {
			sb.WriteString("\n")
		}
	}
	sb.WriteString("ON CONFLICT (id) DO NOTHING;\n\n")

	// Post Media
	sb.WriteString("-- Post Media\n")
	sb.WriteString(
		"INSERT INTO post_media (id, post_id, media_key, media_type, width, height, sort_order, created_at) VALUES\n",
	)
	for i := range media {
		m := &media[i]
		fmt.Fprintf(&sb, "    ('%s', '%s', '%s', '%s', %d, %d, %d, '%s')",
			m.ID.String(),
			m.PostID.String(),
			escapeSQL(m.MediaKey),
			string(m.MediaType),
			m.Width,
			m.Height,
			m.SortOrder,
			m.CreatedAt.Format(time.RFC3339))
		if i < len(media)-1 {
			sb.WriteString(",\n")
		} else {
			sb.WriteString("\n")
		}
	}
	sb.WriteString("ON CONFLICT (id) DO NOTHING;\n\n")

	// Comments
	sb.WriteString("-- Comments\n")
	sb.WriteString("INSERT INTO comments (id, post_id, user_id, parent_id, content, like_count, created_at) VALUES\n")
	for i := range comments {
		c := &comments[i]
		parentID := "NULL"
		if c.ParentID != nil {
			parentID = "'" + c.ParentID.String() + "'"
		}
		fmt.Fprintf(&sb, "    ('%s', '%s', '%s', %s, '%s', %d, '%s')",
			c.ID.String(),
			c.PostID.String(),
			c.UserID.String(),
			parentID,
			escapeSQL(c.Content),
			c.LikeCount,
			c.CreatedAt.Format(time.RFC3339))
		if i < len(comments)-1 {
			sb.WriteString(",\n")
		} else {
			sb.WriteString("\n")
		}
	}
	sb.WriteString("ON CONFLICT (id) DO NOTHING;\n\n")

	// Likes
	sb.WriteString("-- Likes\n")
	sb.WriteString("INSERT INTO likes (id, user_id, target_id, target_type, created_at) VALUES\n")
	for i := range likes {
		l := &likes[i]
		fmt.Fprintf(&sb, "    ('%s', '%s', '%s', %d, '%s')",
			l.ID.String(),
			l.UserID.String(),
			l.TargetID.String(),
			l.TargetType,
			l.CreatedAt.Format(time.RFC3339))
		if i < len(likes)-1 {
			sb.WriteString(",\n")
		} else {
			sb.WriteString("\n")
		}
	}
	sb.WriteString("ON CONFLICT (user_id, target_id, target_type) DO NOTHING;\n\n")

	// Follows
	sb.WriteString("-- Follows\n")
	sb.WriteString("INSERT INTO follows (follower_id, following_id, created_at) VALUES\n")
	for i := range follows {
		f := &follows[i]
		fmt.Fprintf(&sb, "    ('%s', '%s', '%s')",
			f.FollowerID.String(),
			f.FollowingID.String(),
			f.CreatedAt.Format(time.RFC3339))
		if i < len(follows)-1 {
			sb.WriteString(",\n")
		} else {
			sb.WriteString("\n")
		}
	}
	sb.WriteString("ON CONFLICT (follower_id, following_id) DO NOTHING;\n\n")

	// 恢复外键检查
	sb.WriteString("SET session_replication_role = 'origin';\n")

	// 写入文件
	return os.WriteFile(filename, []byte(sb.String()), 0o644)
}

// escapeSQL 转义 SQL 字符串中的特殊字符
func escapeSQL(s string) string {
	s = strings.ReplaceAll(s, "'", "''")
	s = strings.ReplaceAll(s, "\\", "\\\\")
	return s
}
