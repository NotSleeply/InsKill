package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func newAuthRouter(t *testing.T) (*gin.Engine, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.Use(RefreshToken(rdb))
	e.GET("/public", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	e.GET("/private", RequireAuth(), func(c *gin.Context) {
		u, _ := UserFromContext(c)
		c.JSON(200, gin.H{"id": u.ID})
	})
	return e, rdb
}

func TestRequireAuthWithoutToken(t *testing.T) {
	e, _ := newAuthRouter(t)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/private", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRefreshTokenInjectsUser(t *testing.T) {
	e, rdb := newAuthRouter(t)
	ctx := context.Background()
	rdb.HSet(ctx, "login:token:t1", "id", "42", "nickName", "小明", "icon", "")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set("authorization", "t1")
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != `{"id":42}` {
		t.Errorf("body = %s", rec.Body.String())
	}
}
