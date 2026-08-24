package handler

import (
	"errors"
	"net/http"

	"inskill/internal/pkg/errs"
	"inskill/internal/service"

	"github.com/gin-gonic/gin"
)

// PayCallbackHandler 模拟第三方支付回调（Java 版无此接口，按 README 设计补齐）。
type PayCallbackHandler struct{ svc service.OrderLifecycleService }

func NewPayCallbackHandler(svc service.OrderLifecycleService) *PayCallbackHandler {
	return &PayCallbackHandler{svc: svc}
}

func (h *PayCallbackHandler) Register(r *gin.RouterGroup) {
	r.POST("/pay-callback", h.callback)
}

func (h *PayCallbackHandler) callback(c *gin.Context) {
	var req struct {
		OrderID int64 `json:"orderId"`
		Success bool  `json:"success"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || !req.Success {
		c.JSON(http.StatusBadRequest, errs.Fail("参数错误"))
		return
	}
	if err := h.svc.PayCallback(c.Request.Context(), req.OrderID); err != nil {
		if errors.Is(err, errs.ErrOrderClosed) {
			c.JSON(http.StatusOK, errs.Fail("订单已关闭，退款流程已触发"))
			return
		}
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK())
}
