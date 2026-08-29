package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/MinimaxFlora/Docker_Manager_License/internal/auth"
	"github.com/MinimaxFlora/Docker_Manager_License/internal/service"
)

// ---------- 在线授权闭环(公开 API,供 Docker_Manager_Go 客户端调用) ----------
//
// 契约约定(与 Docker_Manager_Go 集成文档同步):
//   - 请求一律携带完整 License Key,服务端验签后自行解析 license_id
//     (不暴露 license_id 查询凭据,防枚举;客户端本地 Ed25519 验签仍是真伪判断的第一道关)
//   - 全部接口有 IP 限流(防 Key 爆破 / 滥用)
//   - 成功响应含 next_verify_after(秒),客户端据此安排下次定期验证

// publicAllow 固定窗口限流:每次请求计数,超限锁定。
// (语义上"窗口内最多 N 次请求",防爆破;对正常客户端(激活一次/24h 验证一次)足够宽松)
func publicAllow(l *auth.LoginLimiter, ip string) bool {
	if l == nil {
		return true
	}
	if !l.Allow(ip) {
		return false
	}
	l.RecordFailure(ip) // 计数(不限成败)
	return true
}

// ---------- activate ----------

type activateRequest struct {
	Key            string `json:"key"`
	DeviceID       string `json:"device_id"`
	DeviceName     string `json:"device_name,omitempty"`
	ProductVersion string `json:"product_version,omitempty"`
}

// publicActivate POST /api/v1/public/activate
// 成功: {status:"active", activation_id, license_id, expires_at, features, max_devices, next_verify_after}
// 失败(统一错误体): INVALID_SIGNATURE / LICENSE_NOT_FOUND / LICENSE_REVOKED / LICENSE_EXPIRED / DEVICE_LIMIT_REACHED
func publicActivate(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := clientIP(c)
		if !publicAllow(d.ActivateLim, ip) {
			abort(c, http.StatusTooManyRequests, "RATE_LIMITED", "too many activation attempts, try later")
			return
		}
		var req activateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			handleError(c, err)
			return
		}
		res, err := d.LicenseSvc.Activate(c.Request.Context(), service.ActivateRequest{
			Key:            strings.TrimSpace(req.Key),
			DeviceID:       strings.TrimSpace(req.DeviceID),
			DeviceName:     strings.TrimSpace(req.DeviceName),
			ProductVersion: strings.TrimSpace(req.ProductVersion),
			IP:             ip,
		})
		if err != nil {
			handleError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status":            "active",
			"activation_id":     res.Activation.ActivationCode,
			"license_id":        res.LicenseID,
			"expires_at":        res.ExpiresAt,
			"features":          res.Features,
			"max_devices":       res.MaxDevices,
			"next_verify_after": service.NextVerifyAfter,
		})
	}
}

// ---------- verify ----------

type verifyRequest struct {
	Key            string `json:"key"`
	ActivationID   string `json:"activation_id,omitempty"`
	DeviceID       string `json:"device_id,omitempty"`
	ProductVersion string `json:"product_version,omitempty"`
}

// publicVerify POST /api/v1/public/verify
// 新契约(传 device_id):返回 {status: valid|revoked|expired|invalid, ...} + next_verify_after,并更新心跳。
// 旧契约(仅传 key):兼容保留,返回 License 在线状态,不校验设备。
func publicVerify(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := clientIP(c)
		if !publicAllow(d.VerifyLim, ip) {
			abort(c, http.StatusTooManyRequests, "RATE_LIMITED", "too many verify requests, try later")
			return
		}
		var req verifyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			handleError(c, err)
			return
		}
		if strings.TrimSpace(req.Key) == "" {
			abort(c, http.StatusBadRequest, "BAD_REQUEST", "key is required")
			return
		}
		out, err := d.LicenseSvc.Verify(c.Request.Context(),
			strings.TrimSpace(req.Key),
			strings.TrimSpace(req.ActivationID),
			strings.TrimSpace(req.DeviceID),
			strings.TrimSpace(req.ProductVersion),
			ip)
		if err != nil {
			handleError(c, err)
			return
		}
		c.JSON(http.StatusOK, out)
	}
}

// ---------- deactivate ----------

type deactivateRequest struct {
	Key          string `json:"key"`
	ActivationID string `json:"activation_id"`
	DeviceID     string `json:"device_id"`
}

// publicDeactivate POST /api/v1/public/deactivate
// 客户端解绑:必须携带激活时返回的 activation_id,防 Device A 解绑 Device B。
// 吊销/过期的 License 也允许解绑(客户端清理)。
func publicDeactivate(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := clientIP(c)
		if !publicAllow(d.ActivateLim, ip) {
			abort(c, http.StatusTooManyRequests, "RATE_LIMITED", "too many deactivate attempts, try later")
			return
		}
		var req deactivateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			handleError(c, err)
			return
		}
		if err := d.LicenseSvc.Deactivate(c.Request.Context(),
			strings.TrimSpace(req.Key),
			strings.TrimSpace(req.ActivationID),
			strings.TrimSpace(req.DeviceID),
			ip); err != nil {
			handleError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
