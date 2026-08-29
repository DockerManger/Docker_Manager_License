// local-dev — 本地完整部署辅助工具(无需 Docker/PostgreSQL)
//
// 用法: go run ./cmd/local-dev
//  1. 启动嵌入式 PostgreSQL(临时实例,:5433,自动下载二进制)
//  2. 以子进程启动 license-server(连接该 PG)
//  3. 浏览器访问 http://localhost:3000
//
// 说明:仅用于本地开发/复现,生产部署用 deploy/docker-compose.yml。
package main

import (
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

func main() {
	// ---------- 1. 嵌入式 PostgreSQL ----------
	cacheDir := filepath.Join(os.TempDir(), "dml-pg-cache")
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Port(5433).
		Username("license").
		Password("licensepass").
		Database("license").
		CachePath(cacheDir).
		RuntimePath(filepath.Join(cacheDir, "runtime")).
		BinaryRepositoryURL("https://repo1.maven.org/maven2"))
	if err := pg.Start(); err != nil {
		log.Fatalf("embedded postgres start: %v (首次运行需下载 PG 二进制,网络慢请耐心等待)", err)
	}
	defer pg.Stop()
	log.Println("✓ embedded postgres ready on 127.0.0.1:5433 (db=license user=license)")

	// ---------- 2. 启动 license-server 子进程 ----------
	// 私钥用仓库 private/license.key(与 Docker_Manager_Go 内置公钥配对;部署时带同一份私钥)
	privKeyPath := filepath.Join("private", "license.key")
	env := append(os.Environ(),
		"DATABASE_URL=postgres://license:licensepass@127.0.0.1:5433/license?sslmode=disable",
		"JWT_SECRET=local-dev-secret",
		"LICENSE_KEY_ID=2026-01",
		"LICENSE_PRIVATE_KEY_PATH="+privKeyPath,
		"DATA_DIR="+filepath.Join(os.TempDir(), "dml-local"),
		"SERVER_ADDR=:3000",
	)
	cmd := exec.Command("go", "run", "./cmd/license-server")
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		log.Fatalf("license-server start: %v", err)
	}
	defer cmd.Process.Kill()

	log.Println("✓ license-server starting on http://localhost:3000")
	log.Println("  浏览器打开 http://localhost:3000 (管理员密码在 license-server 日志里)")

	// 等待 Ctrl+C
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	log.Println("stopping...")
}
