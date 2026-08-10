package main

import (
	"context"
	"errors"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/phuslu/log"

	"github.com/sanbei101/blue-book/internal/api"
	"github.com/sanbei101/blue-book/internal/db"
	"github.com/sanbei101/blue-book/internal/pkg/jwt"
	"github.com/sanbei101/blue-book/internal/pkg/media"
)

func main() {
	ctx := context.Background()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://postgres:password@localhost:5432/blue_book?sslmode=disable"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "dev-only-jwt-secret-please-override-in-production!"
	}
	if err := jwt.Configure(jwtSecret); err != nil {
		log.Error().Err(err).Msg("初始化 JWT 失败")
		return
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Error().Err(err).Msg("无法连接数据库")
		return
	}
	log.Info().Msg("数据库连接成功")
	defer pool.Close()

	store := db.NewStore(pool)

	presigner, err := media.NewPresigner()
	if err != nil {
		if errors.Is(err, media.ErrNotConfigured) {
			log.Warn().Msg("对象存储未配置,媒体上传接口不可用")
			presigner = nil
		} else {
			log.Error().Err(err).Msg("初始化对象存储失败")
			return
		}
	}

	router := api.RegisterRoutesWithMedia(store, presigner)

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	log.Info().Str("addr", addr).Msg("服务启动")

	if err := http.ListenAndServe(addr, router); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		log.Error().Err(err).Msg("服务异常关闭")
	}
}
