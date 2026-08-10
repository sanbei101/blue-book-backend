package seed

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	"github.com/sanbei101/blue-book/internal/db"
)

// seedUsers 创建种子用户
func (s *Seeder) seedUsers(ctx context.Context) ([]db.User, error) {
	users := make([]db.User, 0, len(userSeeds))

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash seed password: %w", err)
	}

	for _, u := range userSeeds {
		user, err := s.store.CreateUser(ctx, db.CreateUserParams{
			ID:           uuid.Must(uuid.NewV7()),
			Username:     u.Username,
			PasswordHash: string(passwordHash),
			AvatarURL:    pgtype.Text{String: avatarURL(u.Username), Valid: true},
			Bio:          pgtype.Text{String: u.Bio, Valid: true},
		})
		if err != nil {
			return nil, fmt.Errorf("create user %s: %w", u.Username, err)
		}
		users = append(users, user)
	}

	return users, nil
}
