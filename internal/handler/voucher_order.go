package handler

import (
	"errors"
	"net/http"
	"strconv"

	"inskill/internal/middleware"
	"inskill/internal/pkg/errs"
	"inskill/internal/service"

	"github.com/gin-gonic/gin"
)

type VoucherOrderHandler struct{ svc service.VoucherOrderService }

func NewVoucherOrderHandler(svc service.VoucherOrderService) *VoucherOrderHandler {
	return &VoucherOrderHandler{svc: svc}
}

func (h *VoucherOrderHandler) Register(r *gin.RouterGroup) {
	r.POST("/seckill/:id", middleware.RequireAuth(), h.seckill)
}

func (h *VoucherOrderHandler) seckill(c *gin.Context) {
	voucherID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, errs.Fail("参数错误"))
		return
	}
	u, _ := middleware.UserFromContext(c)
	orderID, err := h.svc.Seckill(c.Request.Context(), u.ID, voucherID)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrStockEmpty):
			c.JSON(http.StatusOK, errs.Fail("库存不足"))
		case errors.Is(err, errs.ErrDuplicatedOrder):
			c.JSON(http.StatusOK, errs.Fail("不能重复下单"))
		default:
			c.JSON(http.StatusOK, errs.Fail("秒杀活动不存在"))
		}
		return
	}
	c.JSON(http.StatusOK, errs.OK(orderID))
}
