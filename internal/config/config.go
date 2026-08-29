// Package config 环境变量配置加载。
//
// 敏感配置(JWT_SECRET / DATABASE_URL / 初始管理员密码)一律来自环境变量,
// 绝不写入代码或配置文件。生产用 .env + 权限 0600。
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config 服务端配置。
type Config struct {
	ServerAddr  string // 监听地址,默认 :3000
	DatabaseURL string // PostgreSQL DSN
	JWTSecret   string // HS256 签名密钥(管理端 JWT)
	JWTTTL      time.Duration

	LicenseKeyID       string // 当前签发密钥标识,如 "2026-01"(写入每个 License payload)
	LicensePrivKeyPath string // Ed25519 私钥文件路径

	AdminUsername string // 仅首次初始化管理员时使用
	AdminPassword string // 仅首次初始化管理员时使用(初始化后立即清空)

	PublicBasePath string // 公开 API 前缀,默认 /api/v1/public
	AdminBasePath  string // 管理 API 前缀,默认 /api/v1/admin
}

// Load 从环境变量加载配置。requireSecrets=true 时校验 JWT_SECRET(生产/服务模式)。
func Load() (*Config, error) {
	c := &Config{
		ServerAddr:         envOr("SERVER_ADDR", ":3000"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		JWTTTL:             envDur("JWT_TTL", 12*time.Hour),
		LicenseKeyID:       os.Getenv("LICENSE_KEY_ID"),
		LicensePrivKeyPath: envOr("LICENSE_PRIVATE_KEY_PATH", "private/license.key"),
		AdminUsername:      os.Getenv("ADMIN_USERNAME"),
		AdminPassword:      os.Getenv("ADMIN_PASSWORD"),
		PublicBasePath:     envOr("PUBLIC_API_BASE", "/api/v1/public"),
		AdminBasePath:      envOr("ADMIN_API_BASE", "/api/v1/admin"),
	}
	var errs []string
	if c.DatabaseURL == "" {
		errs = append(errs, "DATABASE_URL is required")
	}
	if c.JWTSecret == "" {
		errs = append(errs, "JWT_SECRET is required")
	}
	if c.LicenseKeyID == "" {
		errs = append(errs, "LICENSE_KEY_ID is required (e.g. 2026-01)")
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("config: %s", strings.Join(errs, "; "))
	}
	return c, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
