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
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/MinimaxFlora/Docker_Manager_License/internal/api"
	"github.com/MinimaxFlora/Docker_Manager_License/internal/auth"
	"github.com/MinimaxFlora/Docker_Manager_License/internal/config"
	"github.com/MinimaxFlora/Docker_Manager_License/internal/crypto"
	"github.com/MinimaxFlora/Docker_Manager_License/internal/database"
	"github.com/MinimaxFlora/Docker_Manager_License/internal/service"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)

	switch {
	case len(os.Args) >= 2 && os.Args[1] == "migrate":
		os.Exit(runMigrate())
	case len(os.Args) >= 2 && os.Args[1] == "keygen":
		os.Exit(runKeygen(os.Args[2:]))
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

环境变量:
  DATABASE_URL              必填,PostgreSQL DSN
  JWT_SECRET                必填,管理端 JWT 签名密钥
  LICENSE_KEY_ID            必填,当前签发密钥标识(如 2026-01)
  LICENSE_PRIVATE_KEY_PATH  私钥路径(默认 private/license.key)
  SERVER_ADDR               监听地址(默认 :3000)
  JWT_TTL                   JWT 有效期(默认 12h)
  ADMIN_USERNAME            首次初始化管理员用户名
  ADMIN_PASSWORD            首次初始化管理员密码(仅首次生效)
`)
}

func runServer() int {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

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

	// 初始化管理员(仅首次:admins 表为空时)
	adminRepo := service.NewAdminRepo(pool)
	if err := ensureAdmin(ctx, cfg, adminRepo); err != nil {
		log.Fatalf("ensure admin: %v", err)
	}

	// 组装依赖
	deps := &api.Deps{
		AdminRepo:  adminRepo,
		LicenseSvc: service.NewLicenseService(service.NewLicenseRepo(pool), service.NewAuditRepo(pool), kp, cfg.LicenseKeyID),
		AuditRepo:  service.NewAuditRepo(pool),
		JWTSecret:  cfg.JWTSecret,
		JWTTTL:     cfg.JWTTTL,
		Limiter:    auth.NewLoginLimiter(15*time.Minute, 10, 15*time.Minute),
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

// ensureAdmin 首次启动时用环境变量创建管理员。
func ensureAdmin(ctx context.Context, cfg *config.Config, repo *service.AdminRepo) error {
	n, err := repo.Count(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if cfg.AdminUsername == "" || cfg.AdminPassword == "" {
		return fmt.Errorf("admins table is empty; set ADMIN_USERNAME and ADMIN_PASSWORD to bootstrap the first admin")
	}
	if len(cfg.AdminPassword) < 8 {
		return fmt.Errorf("ADMIN_PASSWORD must be at least 8 characters")
	}
	hash, err := auth.HashPassword(cfg.AdminPassword)
	if err != nil {
		return err
	}
	if err := repo.Create(ctx, cfg.AdminUsername, hash); err != nil {
		return err
	}
	log.Printf("initial admin %q created (bootstrap)", cfg.AdminUsername)
	return nil
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
