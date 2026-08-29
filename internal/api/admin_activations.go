package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/DockerManger/Docker_Manager_License/internal/model"
)

// ---------- 设备管理(Admin,全部需 JWT) ----------

// adminListActivations GET /api/v1/admin/licenses/:id/activations
// 返回该 License 全部设备激活记录(含已解绑,激活时间倒序)。
func adminListActivations(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := d.LicenseSvc.ListActivations(c.Request.Context(), c.Param("id"))
		if err != nil {
			handleError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

// adminDeactivateActivation POST /api/v1/admin/licenses/:id/activations/:aid/deactivate
// 按激活记录 ID 单个解绑(设备换机/异常占用时人工释放)。
func adminDeactivateActivation(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		aid, err := strconv.ParseInt(c.Param("aid"), 10, 64)
		if err != nil {
			abort(c, http.StatusBadRequest, "BAD_REQUEST", "invalid activation id")
			return
		}
		if err := d.LicenseSvc.DeactivateActivation(c.Request.Context(), c.Param("id"), aid,
			c.GetString(ctxAdminKey), clientIP(c)); err != nil {
			handleError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// adminResetDevices POST /api/v1/admin/licenses/:id/reset-devices
// 重置该 License 全部设备(全部解绑)。审计记录:谁/何时/解绑了几台。
func adminResetDevices(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		n, err := d.LicenseSvc.ResetDevices(c.Request.Context(), c.Param("id"),
			c.GetString(ctxAdminKey), clientIP(c))
		if err != nil {
			handleError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "deactivated": n})
	}
}

// ---------- 签名密钥注册表(Admin) ----------

// adminListSigningKeys GET /api/v1/admin/signing-keys
// 签名密钥注册表:key rotation 的基础,旧公钥永不删除(旧 License 仍需验证)。
func adminListSigningKeys(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		keys, err := d.LicenseSvc.ListSigningKeys(c.Request.Context())
		if err != nil {
			handleError(c, err)
			return
		}
		if keys == nil {
			keys = []*model.SigningKey{} // 避免 null(前端防御)
		}
		c.JSON(http.StatusOK, gin.H{"items": keys})
	}
}
