package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/DockOrae/DockOrae-Auth/internal/auth"
	"github.com/DockOrae/DockOrae-Auth/internal/service"
)

// ---------- 在线授权闭环(V3 公开 API,供 Docker_Manager_Go 客户端调用) ----------
//
// 契约(与 Docker_Manager_Go 集成文档同步):
//   - activate:携带完整 License Key(仅首次激活/重新激活用)+ 设备信息 → 返回 activation_id +
//     activation_token(明文,数据库只存 SHA-256)+ server_time
//   - verify/deactivate:只携带 activation_token + device_id + timestamp + nonce
//     (timestamp ±5min 窗口 + nonce 唯一 = 重放防护)
//   - events:SSE 长连接,License 状态变化由 Server 主动推送(Event-Driven),
//     客户端收到事件后调用 verify 获取权威状态
//   - 所有成功响应含 server_time,客户端据此计算 clock_offset(防本地时间作弊)
//   - 全部接口有 IP 限流(防 Key 爆破 / 滥用)
//
// V2/Legacy 已完全移除:不存在 key + activation_id 的兼容路径,不存在 V3→V2 fallback。

// publicAllow 固定窗口限流:每次请求计数,超限锁定。
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
	Key               string `json:"key"`
	DeviceID          string `json:"device_id"`
	DeviceName        string `json:"device_name,omitempty"`
	ProductVersion    string `json:"product_version,omitempty"`
	DeviceFingerprint string `json:"device_fingerprint,omitempty"`
	Platform          string `json:"platform,omitempty"`
	Architecture      string `json:"architecture,omitempty"`
}

// publicActivate POST /api/v3/activate
// 成功: {status:"active", activation_id, activation_token, license_id, expires_at, features,
//
//	max_devices, server_time, state_version}
//
// 失败(统一错误体): INVALID_SIGNATURE / LICENSE_NOT_FOUND / LICENSE_REVOKED / LICENSE_EXPIRED / DEVICE_LIMIT_REACHED
func publicActivate(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := clientIP(c)
		if !publicAllow(d.ActivateLim, ip) {
			d.Security.Log(c.Request.Context(), secEvent(service.SecRateLimitExceeded, "", "", "", ip, "activate rate limited"))
			abort(c, http.StatusTooManyRequests, "RATE_LIMITED", "too many activation attempts, try later")
			return
		}
		var req activateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			handleError(c, err)
			return
		}
		res, err := d.LicenseSvc.Activate(c.Request.Context(), service.ActivateRequest{
			Key:               strings.TrimSpace(req.Key),
			DeviceID:          strings.TrimSpace(req.DeviceID),
			DeviceName:        strings.TrimSpace(req.DeviceName),
			ProductVersion:    strings.TrimSpace(req.ProductVersion),
			DeviceFingerprint: strings.TrimSpace(req.DeviceFingerprint),
			Platform:          strings.TrimSpace(req.Platform),
			Architecture:      strings.TrimSpace(req.Architecture),
			IP:                ip,
		})
		if err != nil {
			handleError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status":           "active",
			"activation_id":    res.Activation.ActivationID,
			"activation_token": res.Token,
			"license_id":       res.LicenseID,
			"expires_at":       res.ExpiresAt,
			"features":         res.Features,
			"max_devices":      res.MaxDevices,
			"server_time":      service.Now(),
			"state_version":    res.Activation.StateVersion,
		})
	}
}

// ---------- verify ----------

// verifyRequest V3 主格式(activation_token + device_id + timestamp + nonce)。
type verifyRequest struct {
	Token          string `json:"activation_token,omitempty"`
	DeviceID       string `json:"device_id,omitempty"`
	ProductVersion string `json:"product_version,omitempty"`
	Timestamp      int64  `json:"timestamp,omitempty"`
	Nonce          string `json:"nonce,omitempty"`
}

// publicVerify POST /api/v3/verify
// 返回 {status: valid|revoked|expired|invalid|blocked, server_time, state_version, minimum_client_version?, ...}。
func publicVerify(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := clientIP(c)
		if !publicAllow(d.VerifyLim, ip) {
			d.Security.Log(c.Request.Context(), secEvent(service.SecRateLimitExceeded, "", "", "", ip, "verify rate limited"))
			abort(c, http.StatusTooManyRequests, "RATE_LIMITED", "too many verify requests, try later")
			return
		}
		var req verifyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			handleError(c, err)
			return
		}
		trimmed := verifyRequest{
			Token:          strings.TrimSpace(req.Token),
			DeviceID:       strings.TrimSpace(req.DeviceID),
			ProductVersion: strings.TrimSpace(req.ProductVersion),
			Timestamp:      req.Timestamp,
			Nonce:          strings.TrimSpace(req.Nonce),
		}
		if trimmed.Token == "" {
			abort(c, http.StatusBadRequest, "BAD_REQUEST", "activation_token is required")
			return
		}
		out, err := d.LicenseSvc.Verify(c.Request.Context(), service.VerifyRequest{
			Token:          trimmed.Token,
			DeviceID:       trimmed.DeviceID,
			ProductVersion: trimmed.ProductVersion,
			Timestamp:      trimmed.Timestamp,
			Nonce:          trimmed.Nonce,
			IP:             ip,
			UserAgent:      c.Request.UserAgent(),
		})
		if err != nil {
			handleError(c, err)
			return
		}
		c.JSON(http.StatusOK, out)
	}
}

// ---------- deactivate ----------

type deactivateRequest struct {
	Token     string `json:"activation_token,omitempty"`
	DeviceID  string `json:"device_id"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Nonce     string `json:"nonce,omitempty"`
}

// publicDeactivate POST /api/v3/deactivate
// 必须携带 activation_token + device_id + timestamp + nonce,
// 凭据必须匹配,防 Device A 解绑 Device B。
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
		if err := d.LicenseSvc.Deactivate(c.Request.Context(), service.DeactivateRequest{
			Token:     strings.TrimSpace(req.Token),
			DeviceID:  strings.TrimSpace(req.DeviceID),
			Timestamp: req.Timestamp,
			Nonce:     strings.TrimSpace(req.Nonce),
			IP:        ip,
			UserAgent: c.Request.UserAgent(),
		}); err != nil {
			handleError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "server_time": service.Now()})
	}
}
