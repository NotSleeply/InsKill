package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// UVCount 页面独立访客统计（HyperLogLog）：PFADD uv:{page}:{yyyyMMdd} {userID|IP}。
func UVCount(rdb *redis.Client, page string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := "uv:" + page + ":" + time.Now().Format("20060102")
		member := c.ClientIP()
		if u, ok := UserFromContext(c); ok {
			member = "u" + strconv.FormatInt(u.ID, 10)
		}
		_ = rdb.PFAdd(c.Request.Context(), key, member).Err()
		c.Next()
	}
}
