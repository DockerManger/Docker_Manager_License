package api

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/DockerManger/Docker_Manager_License/web"
)

// Router 注册全部路由。
//
// 路由划分(对外经反代 /license-api/ 剥离前缀后进入,内部无 /license-api 前缀):
//   - GET  /health(z)            健康检查(Caddy / Docker healthcheck 用)
//   - /api/v1/public/*           公开 API(激活/验证/解绑,供 Docker_Manager_Go 客户端)
//   - /api/v1/admin/*            管理 API(全部需要 JWT 认证)
//
// 对外规范 Base URL(生产固定):
//
//	https://manager.kejizero.xyz/license-api
//
// 客户端请求 = Base + "/api/v1/public/activate|verify|deactivate"
// 反代(Caddy/nginx)负责剥离 /license-api 前缀。
//
// 管理 API 匿名不可访问,且与公开 API 物理分离。
func Router(d *Deps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	// 基础信息(健康检查):/healthz 兼容旧部署,/health 为规格书规范路径
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 公开 API 与管理 API 前缀均可由环境变量覆盖(默认 /api/v1/public、/api/v1/admin)
	v1 := r.Group("/api/v1")

	// ---------- 公开(在线授权闭环,供 Docker_Manager_Go 客户端) ----------
	pub := v1.Group("/public")
	{
		pub.POST("/activate", publicActivate(d))
		pub.POST("/verify", publicVerify(d))
		pub.POST("/deactivate", publicDeactivate(d))
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
		authed.GET("/licenses/:id/activations", adminListActivations(d))
		authed.POST("/licenses/:id/activations/:aid/deactivate", adminDeactivateActivation(d))
		authed.POST("/licenses/:id/reset-devices", adminResetDevices(d))

		authed.GET("/signing-keys", adminListSigningKeys(d))

		authed.GET("/audit-logs", adminAuditLogs(d))

		// ---------- V3:客户 / 订阅 / 安全事件 / 服务器配置 ----------
		authed.POST("/customers", adminCreateCustomer(d))
		authed.GET("/customers", adminListCustomers(d))
		authed.POST("/subscriptions", adminCreateSubscription(d))
		authed.GET("/subscriptions", adminListSubscriptions(d))
		authed.POST("/subscriptions/:id/status", adminUpdateSubscriptionStatus(d))
		authed.GET("/security-events", adminSecurityEvents(d))
		authed.GET("/settings", adminSettings(d))
		authed.PUT("/settings", adminUpdateSettings(d))
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
		// 与 vite preview 一致:no-cache 防止浏览器启发式缓存旧版本资源
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, contentType(name), raw)
		return
	}
	// SPA fallback
	raw, err := fs.ReadFile(distFS, "index.html")
	if err != nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "text/html; charset=utf-8", raw)
}

func contentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".js"):
		return "text/javascript"
	case strings.HasSuffix(name, ".css"):
		return "text/css"
	case strings.HasSuffix(name, ".html"):
		return "text/html"
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
