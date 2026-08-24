package handler

import (
	"net/http"
	"strconv"

	"inskill/internal/model"
	"inskill/internal/pkg/errs"
	"inskill/internal/service"

	"github.com/gin-gonic/gin"
)

type VoucherHandler struct{ svc service.VoucherService }

func NewVoucherHandler(svc service.VoucherService) *VoucherHandler { return &VoucherHandler{svc: svc} }

func (h *VoucherHandler) Register(r *gin.RouterGroup) {
	r.POST("", h.create)         // 普通券
	r.POST("/seckill", h.create) // 秒杀券（Java 版同名方法 addSeckillVoucher）
	r.GET("/list/:shopId", h.listByShop)
}

func (h *VoucherHandler) create(c *gin.Context) {
	var v model.Voucher
	if err := c.ShouldBindJSON(&v); err != nil {
		c.JSON(http.StatusBadRequest, errs.Fail("参数错误"))
		return
	}
	if err := h.svc.Create(c.Request.Context(), &v); err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK(v.ID))
}

func (h *VoucherHandler) listByShop(c *gin.Context) {
	shopID, _ := strconv.ParseInt(c.Param("shopId"), 10, 64)
	list, err := h.svc.ListByShop(c.Request.Context(), shopID)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK(list))
}
