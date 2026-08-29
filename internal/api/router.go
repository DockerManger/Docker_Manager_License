package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
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

	return r
}
