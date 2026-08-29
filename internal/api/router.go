package api

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/MinimaxFlora/Docker_Manager_License/web"
)

// Router 注册全部路由。
//
// 路由划分:
//   - /api/v1/public/*   公开 API(当前仅 verify)
//   - /api/v1/admin/*    管理 API(全部需要 JWT 认证)
//
// 管理 API 匿名不可访问,且与公开 API 物理分离。
func Router(d *Deps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	// 基础信息(健康检查)
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := r.Group("/api/v1")

	// ---------- 公开 ----------
	pub := v1.Group("/public")
	{
		pub.POST("/verify", publicVerify(d))
	}

	// ---------- 管理 ----------
	admin := v1.Group("/admin")
	{
		admin.POST("/login", adminLogin(d))
	}

	authed := v1.Group("/admin", adminAuth(d))
	{
		authed.POST("/logout", adminLogout(d))
		authed.GET("/me", adminMe(d))
		authed.POST("/change-password", adminChangePassword(d))
		authed.POST("/setup-totp", adminSetupTOTP(d))
		authed.POST("/confirm-totp", adminConfirmTOTP(d))
		authed.POST("/disable-totp", adminDisableTOTP(d))

		authed.GET("/stats", adminLicenseStats(d))

		authed.GET("/licenses", adminListLicenses(d))
		authed.POST("/licenses", adminIssueLicense(d))
		authed.GET("/licenses/:id", adminGetLicense(d))
		authed.GET("/licenses/:id/revisions", adminLicenseRevisions(d))
		authed.GET("/licenses/:id/export", adminExportLicense(d))
		authed.GET("/licenses/:id/export-json", adminExportLicenseJSON(d))
		authed.POST("/licenses/:id/extend", adminExtendLicense(d))
		authed.POST("/licenses/:id/revoke", adminRevokeLicense(d))

		authed.GET("/audit-logs", adminAuditLogs(d))
	}

	// ---------- 前端(单二进制嵌入) ----------
	distFS, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		// 构建期已保证 dist 存在(embed all:dist);此处失败属于打包错误
		panic("web/dist not embedded: " + err.Error())
	}
	// SPA 回退:未匹配路由时,API 路径返回 JSON 404,其余返回前端页面。
	// 用 http.ServeContent 直接服务(不用 http.FileServer:Go 1.27 对
	// /index.html 请求强制 301 到 ./ 的重定向,会破坏 SPA fallback)
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/") || p == "/healthz" {
			c.JSON(http.StatusNotFound, ErrorBody{Error: ErrorDetail{Code: "NOT_FOUND", Message: "not found"}})
			return
		}
		serveFromDist(c, distFS, strings.TrimPrefix(p, "/"))
	})

	return r
}

// serveFromDist 从嵌入的 dist 提供文件;不存在时回退 index.html(SPA 路由)。
// 前端产物体积小(几 KB~百 KB),直接读内存 ServeContent。
func serveFromDist(c *gin.Context, distFS fs.FS, name string) {
	if name == "" {
		name = "index.html"
	}
	if raw, err := fs.ReadFile(distFS, name); err == nil {
		c.Data(http.StatusOK, contentType(name), raw)
		return
	}
	// SPA fallback
	raw, err := fs.ReadFile(distFS, "index.html")
	if err != nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", raw)
}

func contentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".js"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".ico"):
		return "image/x-icon"
	case strings.HasSuffix(name, ".json"):
		return "application/json"
	default:
		return "application/octet-stream"
	}
}
