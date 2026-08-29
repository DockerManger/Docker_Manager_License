package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestEmbeddedFrontend 单二进制嵌入的前端可访问(根路径/SPA 路由)。
func TestEmbeddedFrontend(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 静态服务不依赖 DB/认证,零值 Deps 即可
	r := Router(&Deps{})

	cases := []struct {
		path string
		want int
	}{
		{"/", 200},
		{"/login", 200},           // SPA 路由 → index.html
		{"/dashboard", 200},       // SPA 路由 → index.html
		{"/api/v1/admin/me", 401}, // API 未认证 → 统一 JSON 401(而非 HTML 404)
		{"/api/v1/nope", 404},     // API 未知路径 → JSON 404
		{"/healthz", 200},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, c.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != c.want {
			t.Fatalf("%s: want %d, got %d", c.path, c.want, w.Code)
		}
		if strings.HasPrefix(c.path, "/api/") && !strings.Contains(w.Body.String(), "error") {
			t.Fatalf("%s: API 错误应返回 JSON error,got %s", c.path, w.Body.String()[:min(80, len(w.Body.String()))])
		}
	}
	// 根路径应包含前端 HTML
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "<!doctype html>") && !strings.Contains(w.Body.String(), "<html") {
		t.Fatalf("root must serve index.html, got: %s", w.Body.String()[:min(120, len(w.Body.String()))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
