package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/phuslu/log"

	"github.com/sanbei101/blue-book/internal/pkg/media"
	"github.com/sanbei101/blue-book/internal/seed"
)

func main() {
	ctx := context.Background()

	pool, err := pgxpool.New(
		ctx,
		"postgres://dexteleop:password@localhost:5432/blue_book?sslmode=disable",
	)
	if err != nil {
		log.Error().Err(err).Msg("无法连接数据库")
		return
	}
	defer pool.Close()

	presigner, err := media.NewPresigner()
	if err != nil {
		log.Error().Err(err).Msg("无法初始化对象存储")
		return
	}

	seeder := seed.NewSeeder(pool, presigner)
	if err := seeder.Run(ctx); err != nil {
		log.Error().Err(err).Msg("种子数据注入失败")
		return
	}

	log.Info().Msg("种子数据注入完成")
}
