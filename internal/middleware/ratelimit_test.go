package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func TestRateLimitAllowsThenBlocks(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.GET("/limited", RateLimit(rdb, "limit:test:", time.Minute, 2, func(c *gin.Context) string { return "ip1" }),
		func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/limited", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want 200", i+1, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/limited", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third request status = %d, want 429", rec.Code)
	}
}

func TestRateLimitDifferentDimensionsIndependent(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.GET("/limited", RateLimit(rdb, "limit:test:", time.Minute, 1, func(c *gin.Context) string { return c.GetHeader("X-U") }),
		func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req1 := httptest.NewRequest(http.MethodGet, "/limited", nil)
	req1.Header.Set("X-U", "user1")
	req2 := httptest.NewRequest(http.MethodGet, "/limited", nil)
	req2.Header.Set("X-U", "user2")

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req1)
	if rec.Code != http.StatusOK {
		t.Fatalf("user1 first status = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req2)
	if rec.Code != http.StatusOK {
		t.Fatalf("user2 first status = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req1)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("user1 second status = %d, want 429", rec.Code)
	}
}
