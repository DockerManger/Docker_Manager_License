package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/DockerManger/Docker_Manager_License/internal/auth"
	"github.com/DockerManger/Docker_Manager_License/internal/model"
)

// ---------- 登录 ----------

// adminLoginRequest 登录请求(密码必填,TOTP 码可空 —— 未启用 2FA 时忽略)。
type adminLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TOTPCode string `json:"totp_code,omitempty"`
}

// adminLogin POST /api/v1/admin/login
// 流程:限流检查 → 查账号 → 校验密码(argon2id) → 校验 TOTP(如启用) → 签发 JWT。
func adminLogin(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := clientIP(c)
		if !d.Limiter.Allow(ip) {
			abort(c, http.StatusTooManyRequests, "RATE_LIMITED", "too many failed attempts, locked for 15 minutes")
			return
		}
		var req adminLoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			handleError(c, err)
			return
		}
		req.Username = strings.TrimSpace(req.Username)
		if req.Username == "" || req.Password == "" {
			abort(c, http.StatusBadRequest, "BAD_REQUEST", "username and password are required")
			return
		}
		admin, err := d.AdminRepo.GetByUsername(c.Request.Context(), req.Username)
		if err != nil || !auth.VerifyPassword(req.Password, admin.PasswordHash) {
			d.Limiter.RecordFailure(ip)
			_ = d.auditLog(c, "", ip, "admin.login_failed", "admin", req.Username, nil)
			abort(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid username or password")
			return
		}
		if admin.TOTPSecret != "" {
			if !auth.VerifyTOTP(admin.TOTPSecret, req.TOTPCode) {
				d.Limiter.RecordFailure(ip)
				_ = d.auditLog(c, admin.Username, ip, "admin.login_failed_totp", "admin", admin.Username, nil)
				abort(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid TOTP code")
				return
			}
		}
		d.Limiter.RecordSuccess(ip)
		token, err := auth.MakeToken(d.JWTSecret, admin.Username, admin.TokenVersion, d.JWTTTL)
		if err != nil {
			handleError(c, err)
			return
		}
		_ = d.auditLog(c, admin.Username, ip, "admin.login", "admin", admin.Username, nil)
		c.JSON(http.StatusOK, gin.H{"token": token, "username": admin.Username})
	}
}

// adminLogout POST /api/v1/admin/logout
// 无状态 JWT 无法单独撤销,logout 由前端丢弃 token;
// 提供可选 ?revoke_all=true 吊销该账号全部会话(token_version++)。
func adminLogout(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := c.GetString(ctxAdminKey)
		id := c.GetString("ctx_admin_id")
		if c.Query("revoke_all") == "true" {
			if err := d.AdminRepo.RevokeTokens(c.Request.Context(), id); err != nil {
				handleError(c, err)
				return
			}
		}
		_ = d.auditLog(c, username, clientIP(c), "admin.logout", "admin", username, nil)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// adminMe GET /api/v1/admin/me
func adminMe(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := c.GetString(ctxAdminKey)
		c.JSON(http.StatusOK, gin.H{"username": username})
	}
}

// adminChangePassword POST /api/v1/admin/change-password
// 修改密码后 token_version++ → 该账号所有旧 JWT 立即失效。
type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func adminChangePassword(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := c.GetString(ctxAdminKey)
		id := c.GetString("ctx_admin_id")
		var req changePasswordRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			handleError(c, err)
			return
		}
		if len(req.NewPassword) < 8 {
			abort(c, http.StatusBadRequest, "BAD_REQUEST", "new password must be at least 8 characters")
			return
		}
		admin, err := d.AdminRepo.GetByUsername(c.Request.Context(), username)
		if err != nil {
			handleError(c, err)
			return
		}
		if !auth.VerifyPassword(req.OldPassword, admin.PasswordHash) {
			abort(c, http.StatusUnauthorized, "UNAUTHORIZED", "old password is incorrect")
			return
		}
		hash, err := auth.HashPassword(req.NewPassword)
		if err != nil {
			handleError(c, err)
			return
		}
		if err := d.AdminRepo.UpdatePassword(c.Request.Context(), id, hash); err != nil {
			handleError(c, err)
			return
		}
		_ = d.auditLog(c, username, clientIP(c), "admin.password_change", "admin", username, nil)
		c.JSON(http.StatusOK, gin.H{"ok": true, "message": "password changed, all sessions revoked"})
	}
}

// adminSetupTOTP POST /api/v1/admin/setup-totp
// 启用 2FA:返回 otpauth URL(前端渲染二维码),verify 后生效。
func adminSetupTOTP(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := c.GetString(ctxAdminKey)
		secret, url, err := auth.GenerateTOTPSecret("Docker Manager License", username)
		if err != nil {
			handleError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"secret": secret, "otpauth_url": url})
	}
}

// adminConfirmTOTP POST /api/v1/admin/confirm-totp
// 提交 setup 返回的 secret + 当前动态码,校验通过则持久化并生效。
type confirmTOTPRequest struct {
	Secret string `json:"secret"`
	Code   string `json:"code"`
}

func adminConfirmTOTP(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := c.GetString(ctxAdminKey)
		id := c.GetString("ctx_admin_id")
		var req confirmTOTPRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			handleError(c, err)
			return
		}
		if !auth.VerifyTOTP(req.Secret, req.Code) {
			abort(c, http.StatusBadRequest, "BAD_REQUEST", "invalid TOTP code")
			return
		}
		if err := d.AdminRepo.UpdateTOTP(c.Request.Context(), id, req.Secret); err != nil {
			handleError(c, err)
			return
		}
		_ = d.auditLog(c, username, clientIP(c), "admin.totp_enabled", "admin", username, nil)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// adminDisableTOTP POST /api/v1/admin/disable-totp
func adminDisableTOTP(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := c.GetString(ctxAdminKey)
		id := c.GetString("ctx_admin_id")
		if err := d.AdminRepo.UpdateTOTP(c.Request.Context(), id, ""); err != nil {
			handleError(c, err)
			return
		}
		_ = d.auditLog(c, username, clientIP(c), "admin.totp_disabled", "admin", username, nil)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// ---------- 审计日志 ----------

// adminAuditLogs GET /api/v1/admin/audit-logs?page=&page_size=
func adminAuditLogs(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, pageSize := pagination(c)
		logs, total, err := d.AuditRepo.List(c.Request.Context(), (page-1)*pageSize, pageSize)
		if err != nil {
			handleError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": logs, "total": total, "page": page, "page_size": pageSize})
	}
}

// ---------- helpers ----------

func (d *Deps) auditLog(c *gin.Context, admin, ip, action, resType, resID string, meta map[string]any) error {
	return d.AuditRepo.Log(c.Request.Context(), &model.AuditLog{
		Admin: admin, Action: action, ResourceType: resType, ResourceID: resID,
		IP: ip, Metadata: jsonString(meta),
	})
}

func jsonString(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func clientIP(c *gin.Context) string {
	if ip := c.ClientIP(); ip != "" {
		return ip
	}
	return c.Request.RemoteAddr
}

func pagination(c *gin.Context) (page, pageSize int) {
	page = 1
	pageSize = 20
	if v := c.Query("page"); v != "" {
		if n := atoi(v); n > 0 {
			page = n
		}
	}
	if v := c.Query("page_size"); v != "" {
		if n := atoi(v); n > 0 && n <= 100 {
			pageSize = n
		}
	}
	return page, pageSize
}

func atoi(s string) int {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int(ch-'0')
	}
	return n
}
