package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/DockerManger/Docker_Manager_License/internal/service"
)

// ---------- 签发 ----------

type issueLicenseRequest struct {
	Customer   string   `json:"customer"`
	Plan       string   `json:"plan"`
	Features   []string `json:"features"`
	ExpiresAt  int64    `json:"expires_at"`            // Unix 秒
	ExpireDays int      `json:"expire_days,omitempty"` // 便捷:相对天数(与 expires_at 二选一)
	MaxDevices int      `json:"max_devices"`
	Notes      string   `json:"notes"`
}

// adminIssueLicense POST /api/v1/admin/licenses
func adminIssueLicense(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req issueLicenseRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			handleError(c, err)
			return
		}
		if req.ExpiresAt == 0 && req.ExpireDays > 0 {
			req.ExpiresAt = service.Now() + int64(req.ExpireDays)*86400
		}
		res, err := d.LicenseSvc.Issue(c.Request.Context(), service.IssueRequest{
			Customer:   strings.TrimSpace(req.Customer),
			Plan:       req.Plan,
			Features:   req.Features,
			ExpiresAt:  req.ExpiresAt,
			MaxDevices: req.MaxDevices,
			Notes:      req.Notes,
			CreatedBy:  c.GetString(ctxAdminKey),
			IP:         clientIP(c),
		})
		if err != nil {
			handleError(c, err)
			return
		}
		c.JSON(http.StatusCreated, gin.H{
			"license": res.License,
			"key":     res.Key,
			"payload": res.Payload,
		})
	}
}

// adminListLicenses GET /api/v1/admin/licenses?page=&page_size=&status=
func adminListLicenses(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, pageSize := pagination(c)
		status := strings.TrimSpace(c.Query("status"))
		items, total, err := d.LicenseSvc.List(c.Request.Context(), (page-1)*pageSize, pageSize, status)
		if err != nil {
			handleError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
	}
}

// adminGetLicense GET /api/v1/admin/licenses/:id
func adminGetLicense(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		l, err := d.LicenseSvc.Get(c.Request.Context(), c.Param("id"))
		if err != nil {
			handleError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"license": l})
	}
}

// adminLicenseRevisions GET /api/v1/admin/licenses/:id/revisions
func adminLicenseRevisions(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		revs, err := d.LicenseSvc.Revisions(c.Request.Context(), c.Param("id"))
		if err != nil {
			handleError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": revs})
	}
}

// adminExportLicense GET /api/v1/admin/licenses/:id/export
// 导出最新修订的完整 Key(文本下载)。
func adminExportLicense(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		revs, err := d.LicenseSvc.Revisions(c.Request.Context(), c.Param("id"))
		if err != nil {
			handleError(c, err)
			return
		}
		if len(revs) == 0 {
			abort(c, http.StatusNotFound, "NOT_FOUND", "no revisions for license")
			return
		}
		latest := revs[len(revs)-1]
		// 输出 Key 文本 + payload 说明,便于用户粘贴/保存为 .lic 文件
		c.Header("Content-Disposition", `attachment; filename="`+latest.Key[:16]+`.lic"`)
		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.String(http.StatusOK, "%s\n", latest.Key)
	}
}

// adminExportLicenseJSON GET /api/v1/admin/licenses/:id/export-json
// 导出 license.json(结构化的 payload+signature,供文档/调试)。
func adminExportLicenseJSON(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		revs, err := d.LicenseSvc.Revisions(c.Request.Context(), c.Param("id"))
		if err != nil {
			handleError(c, err)
			return
		}
		if len(revs) == 0 {
			abort(c, http.StatusNotFound, "NOT_FOUND", "no revisions for license")
			return
		}
		latest := revs[len(revs)-1]
		var payload map[string]any
		_ = json.Unmarshal([]byte(latest.Payload), &payload)
		c.Header("Content-Type", "application/json")
		c.JSON(http.StatusOK, gin.H{"payload": payload, "signature": latest.Signature, "key": latest.Key})
	}
}

// adminExtendLicense POST /api/v1/admin/licenses/:id/extend
type extendLicenseRequest struct {
	Days   int    `json:"days"`
	Reason string `json:"reason"`
}

func adminExtendLicense(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req extendLicenseRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			handleError(c, err)
			return
		}
		res, err := d.LicenseSvc.Extend(c.Request.Context(), c.Param("id"), req.Days,
			req.Reason, c.GetString(ctxAdminKey), clientIP(c))
		if err != nil {
			handleError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"license": res.License, "key": res.Key})
	}
}

// adminRevokeLicense POST /api/v1/admin/licenses/:id/revoke
type revokeLicenseRequest struct {
	Reason string `json:"reason"`
}

func adminRevokeLicense(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req revokeLicenseRequest
		_ = c.ShouldBindJSON(&req)
		l, err := d.LicenseSvc.Revoke(c.Request.Context(), c.Param("id"),
			req.Reason, c.GetString(ctxAdminKey), clientIP(c))
		if err != nil {
			handleError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"license": l})
	}
}

// adminLicenseStats GET /api/v1/admin/stats
func adminLicenseStats(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		stats, err := d.LicenseSvc.Stats(c.Request.Context())
		if err != nil {
			handleError(c, err)
			return
		}
		c.JSON(http.StatusOK, stats)
	}
}

// ---------- 工具 ----------

// parseID 兼容参数中的数字 ID(预留)。
func parseID(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
