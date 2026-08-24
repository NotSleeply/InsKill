package handler

import (
	"net/http"
	"strconv"

	"inskill/internal/middleware"
	"inskill/internal/pkg/errs"
	"inskill/internal/service"

	"github.com/gin-gonic/gin"
)

type FollowHandler struct{ svc service.FollowService }

func NewFollowHandler(svc service.FollowService) *FollowHandler { return &FollowHandler{svc: svc} }

func (h *FollowHandler) Register(r *gin.RouterGroup) {
	r.PUT("/:id/:isFollow", middleware.RequireAuth(), h.follow)
	r.GET("/or/not/:id", middleware.RequireAuth(), h.isFollow)
	r.GET("/common/:id", middleware.RequireAuth(), h.commons)
}

func (h *FollowHandler) follow(c *gin.Context) {
	followUserID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	isFollow, _ := strconv.ParseBool(c.Param("isFollow"))
	u, _ := middleware.UserFromContext(c)
	if err := h.svc.Follow(c.Request.Context(), u.ID, followUserID, isFollow); err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK())
}

func (h *FollowHandler) isFollow(c *gin.Context) {
	followUserID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	u, _ := middleware.UserFromContext(c)
	isFollow, err := h.svc.IsFollow(c.Request.Context(), u.ID, followUserID)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK(isFollow))
}

func (h *FollowHandler) commons(c *gin.Context) {
	otherID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	u, _ := middleware.UserFromContext(c)
	users, err := h.svc.Commons(c.Request.Context(), u.ID, otherID)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK(users))
}
