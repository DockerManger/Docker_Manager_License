// license-server — Docker_Manager_License 签发服务端。
//
// 子命令:
//
//	license-server           启动服务(自动迁移数据库;私钥缺失时自动生成)
//	license-server migrate   仅执行数据库迁移
//	license-server keygen    生成 Ed25519 密钥对(私钥 0600 + 公钥 PEM)
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/DockerManger/Docker_Manager_License/internal/api"
	"github.com/DockerManger/Docker_Manager_License/internal/auth"
	"github.com/DockerManger/Docker_Manager_License/internal/config"
	"github.com/DockerManger/Docker_Manager_License/internal/crypto"
	"github.com/DockerManger/Docker_Manager_License/internal/database"
	"github.com/DockerManger/Docker_Manager_License/internal/service"
)

// 构建信息:发布时由 ldflags 注入(Dockerfile/Makefile 的 -X 参数对应),
// 本地开发/未打 tag 构建为 unknown。
var (
	Version   = "unknown"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)

	switch {
	case len(os.Args) >= 2 && os.Args[1] == "migrate":
		os.Exit(runMigrate())
	case len(os.Args) >= 2 && os.Args[1] == "keygen":
		os.Exit(runKeygen(os.Args[2:]))
	case len(os.Args) >= 2 && os.Args[1] == "pubkey":
		os.Exit(runPubkey())
	case len(os.Args) >= 2 && (os.Args[1] == "-h" || os.Args[1] == "--help"):
		usage()
		os.Exit(0)
	default:
		os.Exit(runServer())
	}
}

func usage() {
	fmt.Print(`Docker Manager License — License 签发服务端

用法:
  license-server            启动服务(自动迁移;私钥缺失时自动生成)
  license-server migrate    仅执行数据库迁移
  license-server keygen     生成 Ed25519 密钥对到 private/license.key + private/license.pub
  license-server keygen -o <dir>
  license-server pubkey     打印当前公钥(PEM,集成到 Docker_Manager_Go 用)

环境变量:
  DATABASE_URL              必填,PostgreSQL DSN
  JWT_SECRET                留空自动生成(持久化 DATA_DIR/jwt_secret)
  LICENSE_KEY_ID            签发密钥标识(默认 2026-01)
  LICENSE_PRIVATE_KEY_PATH  私钥路径(默认 private/license.key)
  SERVER_ADDR               监听地址(默认 :3000)
  JWT_TTL                   JWT 有效期(默认 12h)
  ADMIN_USERNAME            管理员用户名(默认 admin)
  ADMIN_PASSWORD            留空自动生成并打印在日志里
  DATA_DIR                  数据目录(默认 /data)
`)
}

func runServer() int {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// JWT secret:未设置时自动生成并持久化到 DataDir/jwt_secret(重启不失效)
	jwtSecret, err := loadOrCreateJWTSecret(cfg)
	if err != nil {
		log.Fatalf("jwt secret: %v", err)
	}
	cfg.JWTSecret = jwtSecret

	// Ed25519 私钥:不存在则生成(首次部署);存在则加载
	kp, err := crypto.LoadPrivateKey(cfg.LicensePrivKeyPath)
	if err != nil {
		if errors.Is(err, crypto.ErrKeyNotFound) {
			log.Printf("private key not found at %s, generating new key pair...", cfg.LicensePrivKeyPath)
			kp, err = crypto.GenerateKeyPair()
			if err != nil {
				log.Fatalf("generate key: %v", err)
			}
			if err := crypto.SavePrivateKey(cfg.LicensePrivKeyPath, kp.Private); err != nil {
				log.Fatalf("save private key: %v", err)
			}
			log.Printf("private key saved to %s (0600)", cfg.LicensePrivKeyPath)
			log.Printf("PUBLIC KEY (集成到 Docker_Manager_Go):\n%s", crypto.PublicKeyPEM(kp.Public))
		} else {
			log.Fatalf("load private key: %v", err)
		}
	} else {
		log.Printf("loaded private key from %s", cfg.LicensePrivKeyPath)
	}
	log.Printf("signing key id: %s", cfg.LicenseKeyID)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// PostgreSQL + 迁移
	pool, err := database.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Print("database migrated")

	// 初始化管理员(仅首次:admins 表为空时;密码未设置则自动生成并打印日志)
	adminRepo := service.NewAdminRepo(pool)
	if err := ensureAdmin(ctx, cfg, adminRepo); err != nil {
		log.Fatalf("ensure admin: %v", err)
	}

	// 组装依赖
	licenseSvc := service.NewLicenseService(
		service.NewLicenseRepo(pool),
		service.NewActivationRepo(pool),
		service.NewSigningKeyRepo(pool),
		service.NewAuditRepo(pool),
		kp, cfg.LicenseKeyID,
	)
	// 注册当前签名密钥到注册表(key rotation 基础;旧公钥永不删除)
	if err := licenseSvc.EnsureSigningKey(ctx); err != nil {
		log.Fatalf("register signing key: %v", err)
	}
	deps := &api.Deps{
		AdminRepo:      service.NewAdminRepo(pool),
		LicenseSvc:     licenseSvc,
		AuditRepo:      service.NewAuditRepo(pool),
		ActivationRepo: service.NewActivationRepo(pool),
		SigningKeyRepo: service.NewSigningKeyRepo(pool),
		JWTSecret:      cfg.JWTSecret,
		JWTTTL:         cfg.JWTTTL,
		Limiter:        auth.NewLoginLimiter(15*time.Minute, 10, 15*time.Minute),
		ActivateLim:    auth.NewLoginLimiter(15*time.Minute, 20, 15*time.Minute),  // 激活/解绑:15min 20 次,防 Key 爆破
		VerifyLim:      auth.NewLoginLimiter(15*time.Minute, 120, 15*time.Minute), // 验证:15min 120 次(24h/设备 足够宽松)
	}

	gin.SetMode(gin.ReleaseMode)
	r := api.Router(deps)
	srv := &http.Server{
		Addr:              cfg.ServerAddr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("license server listening on %s", cfg.ServerAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	log.Print("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	return 0
}

// loadOrCreateJWTSecret 优先用环境变量;否则读取/生成 DataDir/jwt_secret(0600)。
func loadOrCreateJWTSecret(cfg *config.Config) (string, error) {
	if cfg.JWTSecret != "" {
		return cfg.JWTSecret, nil
	}
	path := filepath.Join(cfg.DataDir, "jwt_secret")
	if raw, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(raw)) != "" {
		return strings.TrimSpace(string(raw)), nil
	}
	secret, err := randomHex(32)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(secret), 0o600); err != nil {
		return "", err
	}
	log.Printf("generated JWT secret, saved to %s (0600)", path)
	return secret, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ensureAdmin 首次启动时创建管理员。
// 密码来源:ADMIN_PASSWORD 环境变量;未设置则自动生成 16 位随机密码并打印到日志。
func ensureAdmin(ctx context.Context, cfg *config.Config, repo *service.AdminRepo) error {
	n, err := repo.Count(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	password := cfg.AdminPassword
	if password == "" {
		password, err = randomPassword()
		if err != nil {
			return err
		}
	}
	if len(password) < 8 {
		return fmt.Errorf("ADMIN_PASSWORD must be at least 8 characters")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	if err := repo.Create(ctx, cfg.AdminUsername, hash); err != nil {
		return err
	}
	log.Printf("==============================================")
	log.Printf("初始管理员已创建,请立即登录并修改密码:")
	log.Printf("  地址: http://<服务器IP>%s", cfg.ServerAddr)
	log.Printf("  用户名: %s", cfg.AdminUsername)
	log.Printf("  密码: %s", password)
	log.Printf("==============================================")
	return nil
}

// randomPassword 生成 16 位随机密码(字母+数字,易抄写)。
func randomPassword() (string, error) {
	const chars = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789"
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b), nil
}

// ---------- migrate 子命令 ----------

func runMigrate() int {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := database.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Print("migrations applied")
	return 0
}

// ---------- keygen 子命令 ----------

// runPubkey 打印当前私钥对应的公钥(集成到 Docker_Manager_Go 用)。
func runPubkey() int {
	path := os.Getenv("LICENSE_PRIVATE_KEY_PATH")
	if path == "" {
		path = "private/license.key"
	}
	kp, err := crypto.LoadPrivateKey(path)
	if err != nil {
		log.Fatalf("load private key: %v (路径见 LICENSE_PRIVATE_KEY_PATH)", err)
	}
	fmt.Println(crypto.PublicKeyPEM(kp.Public))
	return 0
}

func runKeygen(args []string) int {
	outDir := "private"
	for i := 0; i < len(args); i++ {
		if args[i] == "-o" && i+1 < len(args) {
			outDir = args[i+1]
			i++
		}
	}
	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		log.Fatalf("generate: %v", err)
	}
	privPath := outDir + "/license.key"
	if err := crypto.SavePrivateKey(privPath, kp.Private); err != nil {
		log.Fatalf("save private key: %v", err)
	}
	pubPath := outDir + "/license.pub"
	if err := os.WriteFile(pubPath, []byte(crypto.PublicKeyPEM(kp.Public)), 0o644); err != nil {
		log.Fatalf("save public key: %v", err)
	}
	fmt.Printf("private key: %s (0600, 绝不可进入 Git/镜像)\n", privPath)
	fmt.Printf("public key:  %s (集成到 Docker_Manager_Go)\n", pubPath)
	fmt.Printf("\n公钥内容:\n%s", crypto.PublicKeyPEM(kp.Public))
	return 0
}
