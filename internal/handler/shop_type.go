package handler

import (
	"net/http"

	"inskill/internal/pkg/errs"
	"inskill/internal/service"

	"github.com/gin-gonic/gin"
)

type ShopTypeHandler struct{ svc service.ShopTypeService }

func NewShopTypeHandler(svc service.ShopTypeService) *ShopTypeHandler { return &ShopTypeHandler{svc: svc} }

func (h *ShopTypeHandler) Register(r *gin.RouterGroup) {
	r.GET("/list", h.list)
}

func (h *ShopTypeHandler) list(c *gin.Context) {
	types, err := h.svc.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, errs.Fail("没有分类数据"))
		return
	}
	c.JSON(http.StatusOK, errs.OK(types))
}
