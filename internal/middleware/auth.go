package middleware

import (
	"net/http"
	"strconv"
	"time"

	"inskill/internal/model"
	"inskill/internal/pkg/errs"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const (
	loginTokenKey = "login:token:"
	loginTokenTTL = 30 * time.Minute
	userCtxKey    = "inskill.user"
)

// RefreshToken 挂全部路由：有 token 且 Redis 中存在则注入 UserDTO 并续期；无 token 放行。
func RefreshToken(rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("authorization")
		if token == "" {
			c.Next()
			return
		}
		key := loginTokenKey + token
		m, err := rdb.HGetAll(c.Request.Context(), key).Result()
		if err != nil || len(m) == 0 {
			c.Next()
			return
		}
		id, _ := strconv.ParseInt(m["id"], 10, 64)
		user := &model.UserDTO{ID: id, NickName: m["nickName"], Icon: m["icon"]}
		c.Set(userCtxKey, user)
		_ = rdb.Expire(c.Request.Context(), key, loginTokenTTL).Err() // 续期失败不影响本次请求
		c.Next()
	}
}

// RequireAuth 挂受限路由组：context 无用户则 401。
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := UserFromContext(c); !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, errs.Fail("未登录"))
			return
		}
		c.Next()
	}
}

// UserFromContext 从 gin context 取出登录用户。
func UserFromContext(c *gin.Context) (*model.UserDTO, bool) {
	v, ok := c.Get(userCtxKey)
	if !ok {
		return nil, false
	}
	u, ok := v.(*model.UserDTO)
	return u, ok
}
