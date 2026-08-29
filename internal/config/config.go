// Package config 环境变量配置加载。
//
// 敏感配置(JWT_SECRET / DATABASE_URL / 初始管理员密码)一律来自环境变量,
// 绝不写入代码或配置文件。生产用 .env + 权限 0600。
package config

import (
	"fmt"
	"os"
	"time"
)

// Config 服务端配置。
type Config struct {
	ServerAddr  string // 监听地址,默认 :3000
	DatabaseURL string // PostgreSQL DSN

	DataDir   string // 数据目录(持久化 JWT secret 等),默认 /data
	JWTSecret string // HS256 签名密钥;为空则自动生成并持久化到 DataDir/jwt_secret
	JWTTTL    time.Duration

	LicenseKeyID       string // 当前签发密钥标识,默认 2026-01
	LicensePrivKeyPath string // Ed25519 私钥文件路径

	AdminUsername string // 首次初始化管理员用户名,默认 admin
	AdminPassword string // 首次初始化管理员密码;为空则自动生成并打印到日志
}

// Load 从环境变量加载配置。除 DATABASE_URL 外均有默认值/自动生成,开箱即用。
func Load() (*Config, error) {
	c := &Config{
		ServerAddr:         envOr("SERVER_ADDR", ":3000"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		DataDir:            envOr("DATA_DIR", "/data"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		JWTTTL:             envDur("JWT_TTL", 12*time.Hour),
		LicenseKeyID:       envOr("LICENSE_KEY_ID", "2026-01"),
		LicensePrivKeyPath: envOr("LICENSE_PRIVATE_KEY_PATH", "private/license.key"),
		AdminUsername:      envOr("ADMIN_USERNAME", "admin"),
		AdminPassword:      os.Getenv("ADMIN_PASSWORD"),
	}
	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("config: DATABASE_URL is required")
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
