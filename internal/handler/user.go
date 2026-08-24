package handler

import (
	"net/http"
	"strconv"

	"inskill/internal/middleware"
	"inskill/internal/pkg/errs"
	"inskill/internal/service"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	svc     service.UserService
	infoSvc service.UserInfoService
}

func NewUserHandler(svc service.UserService, infoSvc service.UserInfoService) *UserHandler {
	return &UserHandler{svc: svc, infoSvc: infoSvc}
}

// Register 挂载到 /user 路由组（Java 版 UserController 的全部端点）。
func (h *UserHandler) Register(r *gin.RouterGroup) {
	r.POST("/code", h.sendCode)
	r.POST("/login", h.login)
	r.GET("/me", middleware.RequireAuth(), h.me)
	r.GET("/info/:id", h.info)
	r.GET("/:id", h.queryUser)
	r.POST("/sign", middleware.RequireAuth(), h.sign)
	r.GET("/sign/count", middleware.RequireAuth(), h.signCount)
}

func (h *UserHandler) sendCode(c *gin.Context) {
	if err := h.svc.SendCode(c.Request.Context(), c.Query("phone")); err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK())
}

func (h *UserHandler) login(c *gin.Context) {
	var req struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errs.Fail("参数错误"))
		return
	}
	token, err := h.svc.Login(c.Request.Context(), req.Phone, req.Code)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK(token))
}

func (h *UserHandler) me(c *gin.Context) {
	u, _ := middleware.UserFromContext(c)
	c.JSON(http.StatusOK, errs.OK(u))
}

// info 用户详情；无记录返回空 data（对齐 Java info 端点首次查看行为）。
func (h *UserHandler) info(c *gin.Context) {
	userID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	info, err := h.infoSvc.Get(c.Request.Context(), userID)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK(info))
}

func (h *UserHandler) queryUser(c *gin.Context) {
	userID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	dto, err := h.svc.GetDTO(c.Request.Context(), userID)
	if err != nil || dto == nil {
		c.JSON(http.StatusOK, errs.OK(nil))
		return
	}
	c.JSON(http.StatusOK, errs.OK(dto))
}

func (h *UserHandler) sign(c *gin.Context) {
	u, _ := middleware.UserFromContext(c)
	if err := h.svc.Sign(c.Request.Context(), u.ID); err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK())
}

func (h *UserHandler) signCount(c *gin.Context) {
	u, _ := middleware.UserFromContext(c)
	n, err := h.svc.SignCount(c.Request.Context(), u.ID)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK(n))
}
