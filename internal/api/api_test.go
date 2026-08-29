package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MinimaxFlora/Docker_Manager_License/internal/auth"
	"github.com/MinimaxFlora/Docker_Manager_License/internal/crypto"
	"github.com/MinimaxFlora/Docker_Manager_License/internal/database"
	"github.com/MinimaxFlora/Docker_Manager_License/internal/license"
	"github.com/MinimaxFlora/Docker_Manager_License/internal/service"
)

// 集成测试需要真实 PostgreSQL:
//
//	go test -run TestAPI ./internal/api/ 需要 TEST_DATABASE_URL 环境变量
//	(CI 用 service container 提供;本地无 PG 时自动跳过)
func testDeps(t *testing.T) (*Deps, func()) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping DB integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// 清空测试数据(保留 schema)
	for _, tbl := range []string{"audit_logs", "activations", "license_revisions", "licenses", "admins"} {
		if _, err := pool.Exec(ctx, "DELETE FROM "+tbl); err != nil {
			t.Fatalf("clean %s: %v", tbl, err)
		}
	}
	// 种子管理员
	hash, _ := auth.HashPassword("testpass123")
	if err := service.NewAdminRepo(pool).Create(ctx, "admin", hash); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	kp, _ := crypto.GenerateKeyPair()
	d := &Deps{
		AdminRepo:  service.NewAdminRepo(pool),
		LicenseSvc: service.NewLicenseService(service.NewLicenseRepo(pool), service.NewAuditRepo(pool), kp, "test-key"),
		AuditRepo:  service.NewAuditRepo(pool),
		JWTSecret:  "test-jwt-secret",
		JWTTTL:     time.Hour,
		Limiter:    auth.NewLoginLimiter(time.Minute, 1000, time.Minute), // 测试放宽限流
	}
	return d, func() { pool.Close() }
}

func testRouter(d *Deps) *gin.Engine {
	gin.SetMode(gin.TestMode)
	return Router(d)
}

func doJSON(t *testing.T, r *gin.Engine, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		buf.Write(b)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func login(t *testing.T, r *gin.Engine) string {
	t.Helper()
	w := doJSON(t, r, "POST", "/api/v1/admin/login", "", map[string]any{
		"username": "admin", "password": "testpass123",
	})
	if w.Code != 200 {
		t.Fatalf("login failed: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Token == "" {
		t.Fatal("no token in login response")
	}
	return resp.Token
}

func issueLicense(t *testing.T, r *gin.Engine, token string) string {
	t.Helper()
	w := doJSON(t, r, "POST", "/api/v1/admin/licenses", token, map[string]any{
		"customer":    "Zhao",
		"plan":        "pro",
		"features":    []string{"compose", "container_create"},
		"expire_days": 365,
		"max_devices": 3,
	})
	if w.Code != 201 {
		t.Fatalf("issue failed: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		License struct {
			LicenseID string `json:"license_id"`
		} `json:"license"`
		Key string `json:"key"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.License.LicenseID == "" || resp.Key == "" {
		t.Fatalf("issue response incomplete: %s", w.Body.String())
	}
	return resp.License.LicenseID
}

// ---------- 认证与安全 ----------

func TestLoginAndTokenLifecycle(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)

	// 正确登录
	token := login(t, r)
	if token == "" {
		t.Fatal("login should return token")
	}
	// /me 可用
	if w := doJSON(t, r, "GET", "/api/v1/admin/me", token, nil); w.Code != 200 {
		t.Fatalf("me: %d", w.Code)
	}
	// 错误密码 401
	if w := doJSON(t, r, "POST", "/api/v1/admin/login", "", map[string]any{
		"username": "admin", "password": "wrong",
	}); w.Code != 401 {
		t.Fatalf("wrong password must 401, got %d", w.Code)
	}
	// 改密码后旧 token 失效(token_version 机制)
	w := doJSON(t, r, "POST", "/api/v1/admin/change-password", token, map[string]any{
		"old_password": "testpass123", "new_password": "newpass456",
	})
	if w.Code != 200 {
		t.Fatalf("change password: %d %s", w.Code, w.Body.String())
	}
	if w := doJSON(t, r, "GET", "/api/v1/admin/me", token, nil); w.Code != 401 {
		t.Fatalf("old token must be revoked after password change, got %d", w.Code)
	}
	// 新密码可登录
	if w := doJSON(t, r, "POST", "/api/v1/admin/login", "", map[string]any{
		"username": "admin", "password": "newpass456",
	}); w.Code != 200 {
		t.Fatalf("new password must login, got %d", w.Code)
	}
}

func TestUnauthorizedAccess(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)

	// 匿名访问管理 API → 401
	for _, path := range []string{
		"/api/v1/admin/licenses",
		"/api/v1/admin/audit-logs",
		"/api/v1/admin/stats",
		"/api/v1/admin/me",
	} {
		if w := doJSON(t, r, "GET", path, "", nil); w.Code != 401 {
			t.Fatalf("%s must 401 without token, got %d", path, w.Code)
		}
	}
	// 伪造 token → 401
	if w := doJSON(t, r, "GET", "/api/v1/admin/licenses", "fake.token.here", nil); w.Code != 401 {
		t.Fatalf("forged token must 401, got %d", w.Code)
	}
}

func TestLoginRateLimit(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	// 收紧限流:3 次失败即锁定
	d.Limiter = auth.NewLoginLimiter(time.Minute, 3, time.Minute)
	r := testRouter(d)

	for i := 0; i < 3; i++ {
		doJSON(t, r, "POST", "/api/v1/admin/login", "", map[string]any{
			"username": "admin", "password": "wrong",
		})
	}
	w := doJSON(t, r, "POST", "/api/v1/admin/login", "", map[string]any{
		"username": "admin", "password": "testpass123", // 即使密码正确也被锁
	})
	if w.Code != 429 {
		t.Fatalf("rate limit must 429, got %d", w.Code)
	}
}

// ---------- License 生命周期 ----------

func TestLicenseLifecycle(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	token := login(t, r)

	// 签发
	licenseID := issueLicense(t, r, token)

	// 查询
	if w := doJSON(t, r, "GET", "/api/v1/admin/licenses/"+licenseID, token, nil); w.Code != 200 {
		t.Fatalf("get: %d %s", w.Code, w.Body.String())
	}

	// 列表
	w := doJSON(t, r, "GET", "/api/v1/admin/licenses?page=1&page_size=10", token, nil)
	if w.Code != 200 {
		t.Fatalf("list: %d", w.Code)
	}
	var list struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if list.Total < 1 {
		t.Fatalf("list total: %d", list.Total)
	}

	// 延期 → 新修订
	w = doJSON(t, r, "POST", "/api/v1/admin/licenses/"+licenseID+"/extend", token, map[string]any{
		"days": 30, "reason": "renewal",
	})
	if w.Code != 200 {
		t.Fatalf("extend: %d %s", w.Code, w.Body.String())
	}
	var ext struct {
		License struct {
			ExpiresAt int64 `json:"expires_at"`
		} `json:"license"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &ext)
	if ext.License.ExpiresAt == 0 {
		t.Fatal("extend should return new expiry")
	}

	// 修订历史 ≥ 2
	w = doJSON(t, r, "GET", "/api/v1/admin/licenses/"+licenseID+"/revisions", token, nil)
	if w.Code != 200 {
		t.Fatalf("revisions: %d", w.Code)
	}
	var revs struct {
		Items []any `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &revs)
	if len(revs.Items) < 2 {
		t.Fatalf("expect >=2 revisions, got %d", len(revs.Items))
	}

	// 吊销
	w = doJSON(t, r, "POST", "/api/v1/admin/licenses/"+licenseID+"/revoke", token, map[string]any{
		"reason": "refund",
	})
	if w.Code != 200 {
		t.Fatalf("revoke: %d %s", w.Code, w.Body.String())
	}
	// 重复吊销 → 409
	if w = doJSON(t, r, "POST", "/api/v1/admin/licenses/"+licenseID+"/revoke", token, map[string]any{}); w.Code != 409 {
		t.Fatalf("double revoke must 409, got %d", w.Code)
	}
	// 已吊销再延期 → 409
	if w = doJSON(t, r, "POST", "/api/v1/admin/licenses/"+licenseID+"/extend", token, map[string]any{"days": 1}); w.Code != 409 {
		t.Fatalf("extend revoked must 409, got %d", w.Code)
	}

	// 404
	if w := doJSON(t, r, "GET", "/api/v1/admin/licenses/DMG-NOTEXIST", token, nil); w.Code != 404 {
		t.Fatalf("missing license must 404, got %d", w.Code)
	}

	// 400:过期时间在过去
	if w := doJSON(t, r, "POST", "/api/v1/admin/licenses", token, map[string]any{
		"customer": "X", "expire_days": -1,
	}); w.Code != 400 {
		t.Fatalf("bad expiry must 400, got %d", w.Code)
	}
	// 400:未知 feature
	if w := doJSON(t, r, "POST", "/api/v1/admin/licenses", token, map[string]any{
		"customer": "X", "features": []string{"advanced-compose"}, "expire_days": 30,
	}); w.Code != 400 {
		t.Fatalf("unknown feature must 400, got %d", w.Code)
	}
}

func TestPublicVerify(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	token := login(t, r)

	// 签发后在线验证 → valid
	w := doJSON(t, r, "POST", "/api/v1/admin/licenses", token, map[string]any{
		"customer": "Zhao", "plan": "pro", "features": []string{"compose"}, "expire_days": 30,
	})
	var resp struct {
		Key string `json:"key"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	w = doJSON(t, r, "POST", "/api/v1/public/verify", "", map[string]any{"key": resp.Key})
	if w.Code != 200 {
		t.Fatalf("verify: %d", w.Code)
	}
	var v struct {
		Valid bool   `json:"valid"`
		Plan  string `json:"plan"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	if !v.Valid || v.Plan != "pro" {
		t.Fatalf("verify response: %s", w.Body.String())
	}

	// 垃圾 key → valid=false
	w = doJSON(t, r, "POST", "/api/v1/public/verify", "", map[string]any{"key": "garbage"})
	var v2 struct {
		Valid bool `json:"valid"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &v2)
	if v2.Valid {
		t.Fatal("garbage key must be invalid")
	}
}

// ---------- 安全审计 ----------

// TestSecurityNoSecretsLeak 关键安全测试:
// 私钥/密码/JWT secret 绝不出现在任何 API 响应中。
func TestSecurityNoSecretsLeak(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	token := login(t, r)

	// 签发并收集所有响应
	responses := []string{}
	licenseID := issueLicense(t, r, token)
	for _, call := range []struct {
		method, path string
		body         any
	}{
		{"GET", "/api/v1/admin/me", nil},
		{"GET", "/api/v1/admin/stats", nil},
		{"GET", "/api/v1/admin/licenses", nil},
		{"GET", "/api/v1/admin/licenses/" + licenseID, nil},
		{"GET", "/api/v1/admin/licenses/" + licenseID + "/revisions", nil},
		{"GET", "/api/v1/admin/licenses/" + licenseID + "/export-json", nil},
		{"GET", "/api/v1/admin/audit-logs", nil},
		{"POST", "/api/v1/admin/change-password", map[string]any{"old_password": "testpass123", "new_password": "anotherpass99"}},
	} {
		w := doJSON(t, r, call.method, call.path, token, call.body)
		responses = append(responses, w.Body.String())
	}
	all := strings.Join(responses, "\n")

	for _, secret := range []string{
		"test-jwt-secret", // JWT secret
		"private",         // 私钥路径关键字
		"-----BEGIN",      // PEM
		"password_hash",   // 密码哈希字段(API 序列化时被排除)
		"testpass123",     // 明文密码
	} {
		if strings.Contains(all, secret) {
			t.Fatalf("API response leaked secret keyword: %q", secret)
		}
	}
}

// TestKeyExportRoundtrip 导出 Key 可被公钥验证(核心闭环)。
func TestKeyExportRoundtrip(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	token := login(t, r)
	licenseID := issueLicense(t, r, token)

	w := doJSON(t, r, "GET", "/api/v1/admin/licenses/"+licenseID+"/export", token, nil)
	if w.Code != 200 {
		t.Fatalf("export: %d", w.Code)
	}
	key := strings.TrimSpace(w.Body.String())
	if !strings.Contains(key, ".") {
		t.Fatal("exported key must be a V2 key")
	}
	// 用服务端公钥离线验证
	p, ok := license.VerifyKey(key, d.LicenseSvc.PublicKey())
	if !ok {
		t.Fatal("exported key must verify with server public key")
	}
	if p.LicenseID != licenseID {
		t.Fatalf("license id mismatch: %s vs %s", p.LicenseID, licenseID)
	}
}
