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

func HashRefreshToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

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

func GetUserIDFromContext(r *http.Request) uuid.UUID {
	id, _ := r.Context().Value(userIDKey).(uuid.UUID)
	return id
}
