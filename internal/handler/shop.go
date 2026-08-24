package handler

import (
	"errors"
	"net/http"
	"strconv"

	"inskill/internal/model"
	"inskill/internal/pkg/errs"
	"inskill/internal/service"

	"github.com/gin-gonic/gin"
)

type ShopHandler struct{ svc service.ShopService }

func NewShopHandler(svc service.ShopService) *ShopHandler { return &ShopHandler{svc: svc} }

func (h *ShopHandler) Register(r *gin.RouterGroup) {
	r.GET("/:id", h.getByID)
	r.POST("", h.create)
	r.PUT("", h.update)
	r.GET("/of/type", h.pageByType)
	r.GET("/of/name", h.pageByName)
}

func (h *ShopHandler) getByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, errs.Fail("店铺不存在！"))
		return
	}
	shop, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			c.JSON(http.StatusOK, errs.Fail("店铺不存在！"))
			return
		}
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK(shop))
}

func (h *ShopHandler) create(c *gin.Context) {
	var shop model.Shop
	if err := c.ShouldBindJSON(&shop); err != nil {
		c.JSON(http.StatusBadRequest, errs.Fail("参数错误"))
		return
	}
	if err := h.svc.Create(c.Request.Context(), &shop); err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK(shop.ID))
}

// update 更新并删缓存
func (h *ShopHandler) update(c *gin.Context) {
	var shop model.Shop
	if err := c.ShouldBindJSON(&shop); err != nil {
		c.JSON(http.StatusBadRequest, errs.Fail("参数错误"))
		return
	}
	if err := h.svc.Update(c.Request.Context(), &shop); err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK())
}

func (h *ShopHandler) pageByType(c *gin.Context) {
	typeID, _ := strconv.ParseInt(c.Query("typeId"), 10, 64)
	page := parseIntDefault(c.Query("current"), 1)
	var x, y *float64
	if vx, err := strconv.ParseFloat(c.Query("x"), 64); err == nil {
		x = &vx
	}
	if vy, err := strconv.ParseFloat(c.Query("y"), 64); err == nil {
		y = &vy
	}
	shops, err := h.svc.PageByType(c.Request.Context(), typeID, page, x, y)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK(shops))
}

func (h *ShopHandler) pageByName(c *gin.Context) {
	page := parseIntDefault(c.Query("current"), 1)
	shops, err := h.svc.PageByName(c.Request.Context(), c.Query("name"), page)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK(shops))
}

func parseIntDefault(s string, def int) int {
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return def
}
