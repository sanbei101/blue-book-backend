package api

import (
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/sanbei101/blue-book/internal/db"
	"github.com/sanbei101/blue-book/internal/pkg/jwt"
	"github.com/sanbei101/blue-book/internal/pkg/media"
)

// OpenAPI 定义
//
//	@title							小蓝书 API
//	@version						1.0
//	@description					小蓝书后端接口文档
//	@servers.url					http://localhost:8080/api/v1
//	@servers.description			本地开发环境
//	@securitydefinitions.bearerauth	BearerAuth
func RegisterRoutes(store *db.Store) *chi.Mux {
	return RegisterRoutesWithMedia(store, nil)
}

// RegisterRoutesWithMedia 注册全部路由,presigner 为 nil 时媒体接口返回 503
func RegisterRoutesWithMedia(store *db.Store, presigner *media.Presigner) *chi.Mux {
	r := chi.NewRouter()
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{
			"http://localhost:5173",
			"http://127.0.0.1:5173",
		},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	userHandler := NewUserHandler(store)
	postHandler := NewPostHandler(store, presigner)
	commentHandler := NewCommentHandler(store)
	likeHandler := NewLikeHandler(store)
	followHandler := NewFollowHandler(store)
	collectionHandler := NewCollectionHandler(store)
	mediaHandler := NewMediaHandler(presigner)
	discoveryHandler := NewDiscoveryHandler(store)

	r.Route("/api/v1", func(r chi.Router) {
		// 认证
		r.Post("/auth/register", userHandler.Register)
		r.Post("/auth/login", userHandler.Login)
		r.Post("/auth/refresh", userHandler.Refresh)
		r.With(jwt.OptionalAuthMiddleware(store)).Get("/search", discoveryHandler.Search)
		r.With(jwt.OptionalAuthMiddleware(store)).Get("/search/suggestions", discoveryHandler.Suggestions)
		r.Get("/search/trending", discoveryHandler.Trending)
		r.With(jwt.OptionalAuthMiddleware(store)).Get("/feed/recommended", discoveryHandler.Recommended)
		r.Get("/tags/{tag_id}", discoveryHandler.GetTag)
		r.With(jwt.OptionalAuthMiddleware(store)).Get("/tags/{tag_id}/posts", discoveryHandler.ListTagPosts)
		r.Get("/topics", discoveryHandler.ListTopics)
		r.Get("/topics/{topic_id}", discoveryHandler.GetTopic)
		r.With(jwt.OptionalAuthMiddleware(store)).Get("/topics/{topic_id}/posts", discoveryHandler.ListTopicPosts)

		// 公开路由
		r.With(jwt.OptionalAuthMiddleware(store)).Get("/users/{user_id}", userHandler.GetProfile)
		r.With(jwt.OptionalAuthMiddleware(store)).Get("/posts", postHandler.ListFeed)
		r.With(jwt.OptionalAuthMiddleware(store)).Get("/posts/{post_id}", postHandler.GetByID)
		r.With(jwt.OptionalAuthMiddleware(store)).Get("/posts/user/{user_id}", postHandler.ListByUser)
		r.With(jwt.OptionalAuthMiddleware(store)).Get("/users/{user_id}/posts", postHandler.ListByUser)
		r.With(jwt.OptionalAuthMiddleware(store)).Get("/posts/{post_id}/comments", commentHandler.ListByPost)
		r.With(jwt.OptionalAuthMiddleware(store)).Get("/users/{user_id}/followers", followHandler.ListFollowers)
		r.With(jwt.OptionalAuthMiddleware(store)).Get("/users/{user_id}/following", followHandler.ListFollowing)

		// API key lifecycle changes account access and therefore requires a JWT.
		r.Group(func(r chi.Router) {
			r.Use(jwt.AuthMiddleware)
			r.Post("/auth/api-keys", userHandler.CreateAPIKey)
			r.Get("/auth/api-keys", userHandler.ListAPIKeys)
			r.Delete("/auth/api-keys/{key_id}", userHandler.RevokeAPIKey)
			r.Post("/auth/logout", userHandler.Logout)
			r.Put("/auth/password", userHandler.ChangePassword)
		})

		// 需要认证的路由
		r.Group(func(r chi.Router) {
			r.Use(jwt.DelegatedAuthMiddleware(store))
			r.Get("/auth/me", userHandler.Me)
			r.Get("/me/profile", userHandler.MyProfile)
			r.Put("/users/profile", userHandler.UpdateProfile)
			r.Get("/me/search-history", discoveryHandler.ListHistory)
			r.Delete("/me/search-history", discoveryHandler.DeleteHistory)
			r.Post("/tags", discoveryHandler.CreateTag)

			r.Post("/media/presign", mediaHandler.Presign)

			r.Post("/posts", postHandler.Create)
			r.Patch("/posts/{post_id}", postHandler.Update)
			r.Delete("/posts/{post_id}", postHandler.Delete)

			r.Post("/comments", commentHandler.Create)
			r.Delete("/comments/{comment_id}", commentHandler.Delete)

			r.Put("/posts/{post_id}/like", likeHandler.LikePost)
			r.Delete("/posts/{post_id}/like", likeHandler.UnlikePost)
			r.Put("/comments/{comment_id}/like", likeHandler.LikeComment)
			r.Delete("/comments/{comment_id}/like", likeHandler.UnlikeComment)

			r.Put("/posts/{post_id}/collection", collectionHandler.Collect)
			r.Delete("/posts/{post_id}/collection", collectionHandler.Uncollect)
			r.Get("/me/collections", collectionHandler.List)
			r.Get("/me/collections/folders", collectionHandler.ListFolders)
			r.Post("/me/collections/folders", collectionHandler.CreateFolder)
			r.Patch("/me/collections/folders/{folder_id}", collectionHandler.UpdateFolder)
			r.Delete("/me/collections/folders/{folder_id}", collectionHandler.DeleteFolder)

			r.Put("/users/{user_id}/follow", followHandler.Follow)
			r.Delete("/users/{user_id}/follow", followHandler.Unfollow)
		})
	})

	return r
}
