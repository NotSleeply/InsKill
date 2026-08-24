package handler

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"inskill/internal/pkg/errs"

	"github.com/gin-gonic/gin"
)

// UploadHandler 图片上传：保存到本地目录，返回 /blogs/{d1}/{d2}/{uuid}.{ext}（对齐 Java 版）。
type UploadHandler struct{ dir string }

func NewUploadHandler(dir string) *UploadHandler { return &UploadHandler{dir: dir} }

func (h *UploadHandler) Register(r *gin.RouterGroup) {
	r.POST("/blog", h.upload)
	r.GET("/blog/delete", h.delete)
}

func (h *UploadHandler) upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, errs.Fail("文件上传失败"))
		return
	}
	name := newFileName(file.Filename)
	dst := filepath.Join(h.dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, errs.Fail("文件上传失败"))
		return
	}
	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.JSON(http.StatusInternalServerError, errs.Fail("文件上传失败"))
		return
	}
	c.JSON(http.StatusOK, errs.OK(name))
}

func (h *UploadHandler) delete(c *gin.Context) {
	name := c.Query("name")
	if name == "" || filepath.Base(name) != name {
		c.JSON(http.StatusOK, errs.Fail("错误的文件名称"))
		return
	}
	p := filepath.Join(h.dir, filepath.FromSlash(name))
	if err := os.Remove(p); err != nil {
		c.JSON(http.StatusOK, errs.Fail("错误的文件名称"))
		return
	}
	c.JSON(http.StatusOK, errs.OK())
}

// newFileName 生成 /blogs/{d1}/{d2}/{uuid}.{ext} 路径（hash 散列对齐 Java createNewFileName）。
func newFileName(original string) string {
	ext := filepath.Ext(original)
	sum := sha256.Sum256([]byte(original))
	d1 := sum[0] & 0xF
	d2 := sum[1] & 0xF
	id := fmt.Sprintf("%x", sum[0:8])
	return fmt.Sprintf("/blogs/%d/%d/%s%s", d1, d2, id, ext)
}
