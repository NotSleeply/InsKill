package handler

import (
	"net/http"
	"strconv"
	"time"

	"inskill/internal/middleware"
	"inskill/internal/model"
	"inskill/internal/pkg/errs"
	"inskill/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type BlogHandler struct {
	svc service.BlogService
	rdb *redis.Client
}

func NewBlogHandler(svc service.BlogService, rdb *redis.Client) *BlogHandler {
	return &BlogHandler{svc: svc, rdb: rdb}
}

func (h *BlogHandler) Register(r *gin.RouterGroup) {
	r.POST("", middleware.RequireAuth(), h.create)
	r.PUT("/like/:id", middleware.RequireAuth(), h.like)
	r.GET("/hot", h.hot)
	r.GET("/uv", h.uv)
	r.GET("/of/me", middleware.RequireAuth(), h.myPage)
	r.GET("/of/user", h.userPage)
	r.GET("/of/follow", middleware.RequireAuth(), h.feed)
	r.GET("/likes/:id", h.likes)
	r.GET("/:id", h.getByID)
}

func (h *BlogHandler) create(c *gin.Context) {
	var blog model.Blog
	if err := c.ShouldBindJSON(&blog); err != nil {
		c.JSON(http.StatusBadRequest, errs.Fail("参数错误"))
		return
	}
	u, _ := middleware.UserFromContext(c)
	if err := h.svc.Create(c.Request.Context(), u.ID, &blog); err != nil {
		c.JSON(http.StatusOK, errs.Fail("新增笔记失败"))
		return
	}
	c.JSON(http.StatusOK, errs.OK(blog.ID))
}

func (h *BlogHandler) like(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	u, _ := middleware.UserFromContext(c)
	if err := h.svc.Like(c.Request.Context(), u.ID, id); err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK())
}

func (h *BlogHandler) hot(c *gin.Context) {
	page := parseIntDefault(c.Query("current"), 1)
	blogs, err := h.svc.HotPage(c.Request.Context(), page)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK(blogs))
}

// uv 今日博客页 UV（HyperLogLog PFCount）。
func (h *BlogHandler) uv(c *gin.Context) {
	key := "uv:blog:" + time.Now().Format("20060102")
	n, err := h.rdb.PFCount(c.Request.Context(), key).Result()
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK(n))
}

func (h *BlogHandler) myPage(c *gin.Context) {
	u, _ := middleware.UserFromContext(c)
	page := parseIntDefault(c.Query("current"), 1)
	blogs, err := h.svc.MyPage(c.Request.Context(), u.ID, page)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK(blogs))
}

func (h *BlogHandler) userPage(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Query("id"), 10, 64)
	page := parseIntDefault(c.Query("current"), 1)
	blogs, err := h.svc.UserPage(c.Request.Context(), id, page)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK(blogs))
}

func (h *BlogHandler) feed(c *gin.Context) {
	u, _ := middleware.UserFromContext(c)
	lastID, _ := strconv.ParseInt(c.Query("lastId"), 10, 64)
	offset, _ := strconv.Atoi(c.Query("offset"))
	res, err := h.svc.Feed(c.Request.Context(), u.ID, lastID, offset)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK(res))
}

func (h *BlogHandler) likes(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	users, err := h.svc.LikeUsers(c.Request.Context(), id)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, errs.OK(users))
}

func (h *BlogHandler) getByID(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	userID := int64(0)
	if u, ok := middleware.UserFromContext(c); ok {
		userID = u.ID
	}
	blog, err := h.svc.GetByID(c.Request.Context(), userID, id)
	if err != nil {
		c.JSON(http.StatusOK, errs.Fail("博客不存在"))
		return
	}
	c.JSON(http.StatusOK, errs.OK(blog))
}
