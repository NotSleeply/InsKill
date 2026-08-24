package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"inskill/internal/pkg/errs"

	"github.com/gin-gonic/gin"
)

// Recovery 捕获 panic，返回 Java 版 WebExceptionAdvice 对齐的「服务器异常」响应。
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic recovered", "panic", r, "stack", string(debug.Stack()))
				c.AbortWithStatusJSON(http.StatusInternalServerError, errs.Fail("服务器异常"))
			}
		}()
		c.Next()
	}
}
