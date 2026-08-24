package handler

import (
	"errors"
	"net/http"

	"inskill/internal/pkg/errs"

	"github.com/gin-gonic/gin"
)

// writeErr 错误 → 响应映射（对齐 Java WebExceptionAdvice + 业务文案）。
func writeErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errs.ErrInvalidPhone):
		c.JSON(http.StatusOK, errs.Fail("手机号格式错误"))
	case errors.Is(err, errs.ErrCodeMismatch):
		c.JSON(http.StatusOK, errs.Fail("验证码不一致，请重新输入"))
	case errors.Is(err, errs.ErrNotFound):
		c.JSON(http.StatusOK, errs.Fail("数据不存在"))
	default:
		c.JSON(http.StatusInternalServerError, errs.Fail("服务器异常"))
	}
}
