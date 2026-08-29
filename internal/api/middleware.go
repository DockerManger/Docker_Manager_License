package api

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/MinimaxFlora/Docker_Manager_License/internal/auth"
	"github.com/MinimaxFlora/Docker_Manager_License/internal/service"
)

// Deps API 依赖集合(构造时注入)。
type Deps struct {
	AdminRepo      *service.AdminRepo
	LicenseSvc     *service.LicenseService
	AuditRepo      *service.AuditRepo
	ActivationRepo *service.ActivationRepo
	SigningKeyRepo *service.SigningKeyRepo
	JWTSecret      string
	JWTTTL         time.Duration
	Limiter        *auth.LoginLimiter // 登录限流
	ActivateLim    *auth.LoginLimiter // 激活/解绑限流(防爆破)
	VerifyLim      *auth.LoginLimiter // 在线验证限流(宽松)
}

// ctxAdmin 从上下文取当前管理员用户名。
const ctxAdminKey = "ctx_admin"

// adminAuth 管理 API 认证中间件:
//   - 校验 JWT 签名/过期
//   - 校验 token_version 与库中一致(token_version 落后 = 密码已改/已吊销 → 拒绝)
//   - 校验用户名仍存在
func adminAuth(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c)
		if token == "" {
			abort(c, 401, "UNAUTHORIZED", "missing token")
			return
		}
		claims, err := auth.VerifyToken(d.JWTSecret, token)
		if err != nil {
			abort(c, 401, "UNAUTHORIZED", "invalid or expired token")
			return
		}
		admin, err := d.AdminRepo.GetByUsername(c.Request.Context(), claims.Username)
		if err != nil {
			abort(c, 401, "UNAUTHORIZED", "account not found")
			return
		}
		// SEC:token_version 落后 → 修改密码/吊销后旧 token 立即失效
		if claims.TokenVersion != admin.TokenVersion {
			abort(c, 401, "UNAUTHORIZED", "token revoked, please login again")
			return
		}
		c.Set(ctxAdminKey, admin.Username)
		c.Set("ctx_admin_id", admin.ID)
		c.Next()
	}
}

// bearerToken 从 Authorization: Bearer <token> 提取。
func bearerToken(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return ""
}
