package jwt

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/cristalhq/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/sanbei101/blue-book/internal/db"
	"github.com/sanbei101/blue-book/internal/pkg/render"
)

var (
	jwtSigner   jwt.Signer
	jwtVerifier jwt.Verifier
)

const (
	AccessTokenTTL  = 15 * time.Minute
	RefreshTokenTTL = 30 * 24 * time.Hour
)

type contextKey string

const userIDKey contextKey = "user_id"

type userClaims struct {
	UserID uuid.UUID `json:"user_id"`
	jwt.RegisteredClaims
}

func Configure(secret string) error {
	if len(secret) < 32 {
		return errors.New("JWT_SECRET must contain at least 32 characters")
	}

	var err error
	jwtSigner, err = jwt.NewSignerHS(jwt.HS256, []byte(secret))
	if err != nil {
		return err
	}
	jwtVerifier, err = jwt.NewVerifierHS(jwt.HS256, []byte(secret))
	if err != nil {
		return err
	}
	return nil
}

func GenerateToken(userID uuid.UUID) (string, error) {
	if jwtSigner == nil {
		return "", errors.New("JWT is not configured")
	}
	c := userClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(AccessTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	builder := jwt.NewBuilder(jwtSigner)
	token, err := builder.Build(c)
	if err != nil {
		return "", err
	}
	return token.String(), nil
}

func GenerateRefreshToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

const apiKeyPrefix = "bbk_"

// GenerateAPIKey returns a long-lived credential whose plaintext is only safe to show once.
func GenerateAPIKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return apiKeyPrefix + base64.RawURLEncoding.EncodeToString(bytes), nil
}

func HashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

func IsAPIKey(key string) bool {
	return strings.HasPrefix(key, apiKeyPrefix)
}

func HashRefreshToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

// AuthMiddleware only accepts short-lived access tokens. Use it for account
// security operations that a delegated API key must not perform.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if jwtVerifier == nil {
			render.Error(w, http.StatusInternalServerError, "认证服务未配置")
			return
		}
		auth := r.Header.Get("Authorization")
		if auth == "" {
			render.Error(w, http.StatusUnauthorized, "未登录")
			return
		}
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			render.Error(w, http.StatusUnauthorized, "无效的认证格式")
			return
		}

		token, err := jwt.Parse([]byte(parts[1]), jwtVerifier)
		if err != nil {
			render.Error(w, http.StatusUnauthorized, "无效的登录凭证")
			return
		}

		var c userClaims
		if err := json.Unmarshal(token.Claims(), &c); err != nil {
			render.Error(w, http.StatusUnauthorized, "凭证数据解析失败")
			return
		}
		if c.UserID == uuid.Nil {
			render.Error(w, http.StatusUnauthorized, "无效的登录凭证")
			return
		}

		if !c.IsValidAt(time.Now()) {
			render.Error(w, http.StatusUnauthorized, "登录已过期")
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, c.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func authenticateAPIKey(r *http.Request, store *db.Store) (uuid.UUID, bool, error) {
	auth := r.Header.Get("Authorization")
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" || !IsAPIKey(parts[1]) {
		return uuid.Nil, false, nil
	}
	key, err := store.GetActiveAPIKeyByHash(r.Context(), HashAPIKey(parts[1]))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, false, nil
		}
		return uuid.Nil, false, err
	}
	if err := store.TouchAPIKey(r.Context(), key.ID); err != nil {
		return uuid.Nil, false, err
	}
	return key.UserID, true, nil
}

// DelegatedAuthMiddleware accepts either a short-lived JWT or a user's delegated API key.
func DelegatedAuthMiddleware(store *db.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok, err := authenticateAPIKey(r, store)
			if err != nil {
				render.Error(w, http.StatusInternalServerError, "API Key 验证失败")
				return
			}
			if ok {
				ctx := context.WithValue(r.Context(), userIDKey, userID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			AuthMiddleware(next).ServeHTTP(w, r)
		})
	}
}

// OptionalAuthMiddleware 在凭证有效时注入用户 ID,凭证缺失或无效时继续访问公开接口。
func OptionalAuthMiddleware(store *db.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok, err := authenticateAPIKey(r, store)
			if err != nil {
				render.Error(w, http.StatusInternalServerError, "API Key 验证失败")
				return
			}
			if ok {
				ctx := context.WithValue(r.Context(), userIDKey, userID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			if jwtVerifier == nil {
				next.ServeHTTP(w, r)
				return
			}
			parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				next.ServeHTTP(w, r)
				return
			}

			token, err := jwt.Parse([]byte(parts[1]), jwtVerifier)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			var claims userClaims
			if err := json.Unmarshal(token.Claims(), &claims); err != nil ||
				claims.UserID == uuid.Nil || !claims.IsValidAt(time.Now()) {
				next.ServeHTTP(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetUserIDFromContext(r *http.Request) uuid.UUID {
	id, _ := r.Context().Value(userIDKey).(uuid.UUID)
	return id
}
