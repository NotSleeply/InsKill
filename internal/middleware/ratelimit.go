package middleware

import (
	_ "embed"
	"net/http"
	"strconv"
	"time"

	"inskill/internal/pkg/errs"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

//go:embed rate_limit.lua
var rateLimitLua string

var rateLimitScript = redis.NewScript(rateLimitLua)

// RateLimit 滑动窗口限流（Redis ZSet + Lua 原子执行）。
// dimension 决定限流 key 的归属：IP / 用户 ID / 全局。对齐 CityHub README 设计。
func RateLimit(rdb *redis.Client, prefix string, window time.Duration, limit int, dimension func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := prefix + dimension(c)
		now := time.Now().UnixMilli()
		member := strconv.FormatInt(now, 10) + "-" + strconv.FormatInt(time.Now().UnixNano()%1e6, 10)
		allowed, err := rateLimitScript.Run(c.Request.Context(), rdb,
			[]string{key}, window.Milliseconds(), limit, now, member).Int()
		if err != nil {
			// Redis 故障时放行（限流不阻塞业务，对齐 README「保证可用性」）
			c.Next()
			return
		}
		if allowed == 0 {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, errs.Fail("请求过于频繁，请稍后再试"))
			return
		}
		c.Next()
	}
}

// IPDimension 按客户端 IP 限流。
func IPDimension(c *gin.Context) string { return c.ClientIP() }

// UserDimension 按登录用户限流；未登录退回 IP。
func UserDimension(c *gin.Context) string {
	if u, ok := UserFromContext(c); ok {
		return "u" + strconv.FormatInt(u.ID, 10)
	}
	return "ip-" + c.ClientIP()
}
