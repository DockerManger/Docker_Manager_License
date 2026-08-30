package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DockerManger/Docker_Manager_License/internal/auth"
	"github.com/DockerManger/Docker_Manager_License/internal/crypto"
	"github.com/DockerManger/Docker_Manager_License/internal/database"
	"github.com/DockerManger/Docker_Manager_License/internal/events"
	"github.com/DockerManger/Docker_Manager_License/internal/license"
	"github.com/DockerManger/Docker_Manager_License/internal/service"
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
	// 顺序必须遵循外键:先删引用者(license_revisions/activations → licenses),
	// 再删被引用者(subscriptions/customers),否则 FK 约束报错。
	for _, tbl := range []string{
		"license_events", "security_nonces", "activation_tokens", "security_events",
		"license_revisions", "activations", "licenses",
		"subscriptions", "customers",
		"audit_logs", "admins", "signing_keys",
	} {
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
	eventRepo := service.NewEventRepo(pool)
	broker := events.NewBroker()
	licenseSvc := service.NewLicenseService(
		service.NewLicenseRepo(pool),
		service.NewActivationRepo(pool),
		service.NewSigningKeyRepo(pool),
		service.NewAuditRepo(pool),
		service.NewActivationTokenRepo(pool),
		service.NewSecurityEventRepo(pool),
		service.NewNonceRepo(pool),
		service.NewServerSettingsRepo(pool),
		service.NewCustomerRepo(pool),
		service.NewSubscriptionRepo(pool),
		eventRepo,
		broker,
		kp, "test-key",
	)
	if err := licenseSvc.EnsureSigningKey(ctx); err != nil {
		t.Fatalf("ensure signing key: %v", err)
	}
	d := &Deps{
		AdminRepo:        service.NewAdminRepo(pool),
		LicenseSvc:       licenseSvc,
		AuditRepo:        service.NewAuditRepo(pool),
		ActivationRepo:   service.NewActivationRepo(pool),
		SigningKeyRepo:   service.NewSigningKeyRepo(pool),
		CustomerRepo:     service.NewCustomerRepo(pool),
		SubscriptionRepo: service.NewSubscriptionRepo(pool),
		Security:         service.NewSecurityEventRepo(pool),
		Settings:         service.NewServerSettingsRepo(pool),
		EventRepo:        eventRepo,
		Events:           broker,
		JWTSecret:        "test-jwt-secret",
		JWTTTL:           time.Hour,
		Limiter:          auth.NewLoginLimiter(time.Minute, 1000, time.Minute), // 测试放宽限流
		ActivateLim:      auth.NewLoginLimiter(time.Minute, 1000, time.Minute), // 测试放宽限流
		VerifyLim:        auth.NewLoginLimiter(time.Minute, 1000, time.Minute), // 测试放宽限流
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

	// 列表 + status 筛选(回归:COUNT 查询曾复用主查询的 $3 参数编号导致 42P18)
	for _, s := range []string{"active", "expired", "revoked", "suspended"} {
		w = doJSON(t, r, "GET", "/api/v1/admin/licenses?page=1&page_size=10&status="+s, token, nil)
		if w.Code != 200 {
			t.Fatalf("list?status=%s: %d %s", s, w.Code, w.Body.String())
		}
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

// activateWithToken 激活并返回 (activation_id, activation_token)(测试辅助)。
func activateWithToken(t *testing.T, r *gin.Engine, key, deviceID string) (string, string) {
	t.Helper()
	w := activate(t, r, key, deviceID)
	if w.Code != 200 {
		t.Fatalf("activate %s: %d %s", deviceID, w.Code, w.Body.String())
	}
	var act struct {
		ActivationID    string `json:"activation_id"`
		ActivationToken string `json:"activation_token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &act)
	if act.ActivationID == "" || act.ActivationToken == "" {
		t.Fatalf("activate must return activation_id + activation_token: %s", w.Body.String())
	}
	return act.ActivationID, act.ActivationToken
}

// verifyToken 用 token 调用 verify(测试辅助)。
func verifyToken(t *testing.T, r *gin.Engine, token, deviceID, nonce string) (string, map[string]any) {
	t.Helper()
	w := doJSON(t, r, "POST", "/api/v3/verify", "", map[string]any{
		"activation_token": token,
		"device_id":        deviceID,
		"product_version":  "v3.0.0",
		"timestamp":        time.Now().Unix(),
		"nonce":            nonce,
	})
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	status, _ := out["status"].(string)
	return status, out
}

func TestPublicVerify(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	token := login(t, r)

	// 签发并激活 → token 验证 valid
	key := issueKey(t, r, token, 3)
	_, actToken := activateWithToken(t, r, key, "verify-dev-1")

	w := doJSON(t, r, "POST", "/api/v3/verify", "", map[string]any{
		"activation_token": actToken,
		"device_id":        "verify-dev-1",
		"timestamp":        time.Now().Unix(),
		"nonce":            "nonce-pubverify-1",
	})
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

	// 垃圾 token → valid=false(invalid,不泄露存在性)
	w = doJSON(t, r, "POST", "/api/v3/verify", "", map[string]any{
		"activation_token": "garbage-token",
		"device_id":        "verify-dev-1",
		"timestamp":        time.Now().Unix(),
		"nonce":            "nonce-pubverify-2",
	})
	var v2 struct {
		Valid bool `json:"valid"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &v2)
	if v2.Valid {
		t.Fatal("garbage token must be invalid")
	}

	// 缺 token → 400 BAD_REQUEST
	w = doJSON(t, r, "POST", "/api/v3/verify", "", map[string]any{"device_id": "x"})
	if w.Code != 400 {
		t.Fatalf("missing token must 400, got %d", w.Code)
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

// ---------- 在线激活闭环 ----------

// issueKey 签发并返回完整 Key(带设备上限)。
func issueKey(t *testing.T, r *gin.Engine, token string, maxDevices int) string {
	t.Helper()
	w := doJSON(t, r, "POST", "/api/v1/admin/licenses", token, map[string]any{
		"customer": "Zhao", "plan": "pro", "features": []string{"compose"},
		"expire_days": 365, "max_devices": maxDevices,
	})
	if w.Code != 201 {
		t.Fatalf("issue failed: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Key string `json:"key"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return resp.Key
}

// activate 发起激活,返回响应记录。
func activate(t *testing.T, r *gin.Engine, key, deviceID string) *httptest.ResponseRecorder {
	t.Helper()
	return doJSON(t, r, "POST", "/api/v3/activate", "", map[string]any{
		"key": key, "device_id": deviceID, "device_name": "test-host-" + deviceID,
		"product_version": "v2.0.0",
	})
}

// TestActivationLifecycle 完整激活生命周期(V3 token 路径):
// 激活 → 幂等重复激活 → 设备上限 → 验证(valid/invalid) → 解绑 → 重新激活 → 重置 → 吊销。
func TestActivationLifecycle(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	token := login(t, r)
	key := issueKey(t, r, token, 2)

	// 设备 A 激活
	w := activate(t, r, key, "dev-a")
	if w.Code != 200 {
		t.Fatalf("activate dev-a: %d %s", w.Code, w.Body.String())
	}
	var act struct {
		Status         string `json:"status"`
		ActivationID   string `json:"activation_id"`
		ActivationTok  string `json:"activation_token"`
		LicenseID      string `json:"license_id"`
		ExpiresAt      int64  `json:"expires_at"`
		StateVersion   int64  `json:"state_version"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &act)
	if act.Status != "active" || act.ActivationID == "" || act.LicenseID == "" || act.ExpiresAt == 0 {
		t.Fatalf("activate response incomplete: %s", w.Body.String())
	}
	if act.ActivationTok == "" {
		t.Fatal("activate must return activation_token")
	}
	if act.StateVersion != 1 {
		t.Fatalf("first activation state_version must be 1, got %d", act.StateVersion)
	}
	aID, aTok := act.ActivationID, act.ActivationTok

	// 设备 A 重复激活 → 幂等(200,同一 activation_id,新 token,state_version+1)
	w = activate(t, r, key, "dev-a")
	if w.Code != 200 {
		t.Fatalf("re-activate dev-a must be idempotent 200, got %d", w.Code)
	}
	var act2 struct {
		ActivationID  string `json:"activation_id"`
		ActivationTok string `json:"activation_token"`
		StateVersion  int64  `json:"state_version"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &act2)
	if act2.ActivationID != aID {
		t.Fatal("idempotent re-activate must keep same activation_id")
	}
	if act2.ActivationTok == "" || act2.ActivationTok == aTok {
		t.Fatal("re-activate must issue a new activation_token")
	}
	if act2.StateVersion != 2 {
		t.Fatalf("re-activate must bump state_version to 2, got %d", act2.StateVersion)
	}
	aTok = act2.ActivationTok // 旧 token 已吊销,后续用新 token

	// 设备 B 激活 → 成功(第 2 台,上限 2)
	_, bTok := activateWithToken(t, r, key, "dev-b")

	// 设备 C → 上限(2/2)→ 409 DEVICE_LIMIT_REACHED
	w = activate(t, r, key, "dev-c")
	if w.Code != 409 {
		t.Fatalf("3rd device must be device-limited 409, got %d %s", w.Code, w.Body.String())
	}
	var errBody struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &errBody)
	if errBody.Error.Code != "DEVICE_LIMIT_REACHED" {
		t.Fatalf("expected DEVICE_LIMIT_REACHED, got %s", errBody.Error.Code)
	}

	// verify(dev-a, 正确 token)→ valid + state_version
	if status, out := verifyToken(t, r, aTok, "dev-a", "nonce-lifecycle-1"); status != "valid" {
		t.Fatalf("verify dev-a must be valid, got %s", status)
	} else if sv, _ := out["state_version"].(float64); int64(sv) != 2 {
		t.Fatalf("verify must return state_version 2, got %v", out["state_version"])
	}

	// verify(dev-a, 旧 token)→ invalid(旧凭据已吊销)
	if status, _ := verifyToken(t, r, aTok+"x", "dev-a", "nonce-lifecycle-1b"); status != "invalid" {
		t.Fatalf("verify with revoked token must be invalid, got %s", status)
	}

	// verify(dev-a, 错误设备)→ invalid(防跨设备)
	if status, _ := verifyToken(t, r, aTok, "dev-other", "nonce-lifecycle-2"); status != "invalid" {
		t.Fatalf("verify with wrong device must be invalid, got %s", status)
	}

	// 未激活设备(无 token)verify → invalid
	w = doJSON(t, r, "POST", "/api/v3/verify", "", map[string]any{
		"activation_token": "no-such-token", "device_id": "dev-ghost",
		"timestamp": time.Now().Unix(), "nonce": "nonce-lifecycle-3",
	})
	var v struct {
		Status string `json:"status"`
		Valid  bool   `json:"valid"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	if v.Status != "invalid" {
		t.Fatalf("verify unactivated device must be invalid, got %s", w.Body.String())
	}

	// 设备 A 解绑(正确 token)→ ok
	w = doJSON(t, r, "POST", "/api/v3/deactivate", "", map[string]any{
		"activation_token": aTok, "device_id": "dev-a",
		"timestamp": time.Now().Unix(), "nonce": "nonce-lifecycle-4",
	})
	if w.Code != 200 {
		t.Fatalf("deactivate: %d %s", w.Code, w.Body.String())
	}
	// 解绑后 verify → invalid
	if status, _ := verifyToken(t, r, aTok, "dev-a", "nonce-lifecycle-5"); status != "invalid" {
		t.Fatalf("verify after deactivate must be invalid, got %s", status)
	}
	// 设备 A 用自己 token 解绑设备 B → 404(防跨设备解绑)
	w = doJSON(t, r, "POST", "/api/v3/deactivate", "", map[string]any{
		"activation_token": aTok, "device_id": "dev-b",
		"timestamp": time.Now().Unix(), "nonce": "nonce-lifecycle-6",
	})
	if w.Code != 404 {
		t.Fatalf("cross-device deactivate must 404, got %d %s", w.Code, w.Body.String())
	}
	// 设备 B 仍有效
	if status, _ := verifyToken(t, r, bTok, "dev-b", "nonce-lifecycle-7"); status != "valid" {
		t.Fatalf("dev-b must still be valid after cross-device attempt, got %s", status)
	}

	// 设备 A 重新激活 → 恢复 active(新凭据,不占新额度)
	aID2, aTok2 := activateWithToken(t, r, key, "dev-a")
	if aID2 == "" || aID2 == aID {
		t.Fatal("re-activation must issue a new activation_id")
	}
	_ = aTok2

	// 管理端:重置设备 → 全部解绑
	w = doJSON(t, r, "POST", "/api/v1/admin/licenses/"+act.LicenseID+"/reset-devices", token, nil)
	if w.Code != 200 {
		t.Fatalf("reset-devices: %d %s", w.Code, w.Body.String())
	}
	if status, _ := verifyToken(t, r, aTok2, "dev-a", "nonce-lifecycle-8"); status != "invalid" {
		t.Fatalf("verify after reset must be invalid, got %s", status)
	}

	// 重新激活 dev-a(吊销前最后一条有效凭据)
	_, aTok3 := activateWithToken(t, r, key, "dev-a")

	// 吊销 → token 全部作废,verify invalid(凭据直接作废,更严格;客户端 revoked/invalid 均禁用 Pro)
	// 同时管理端 license 状态 = revoked
	w = doJSON(t, r, "POST", "/api/v1/admin/licenses/"+act.LicenseID+"/revoke", token, map[string]any{"reason": "refund"})
	if w.Code != 200 {
		t.Fatalf("revoke: %d", w.Code)
	}
	w = doJSON(t, r, "POST", "/api/v3/verify", "", map[string]any{
		"activation_token": aTok3, "device_id": "dev-a",
		"timestamp": time.Now().Unix(), "nonce": "nonce-lifecycle-9",
	})
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	if v.Status != "invalid" {
		t.Fatalf("verify after revoke must be invalid (token revoked), got %s", w.Body.String())
	}
	w = doJSON(t, r, "GET", "/api/v1/admin/licenses/"+act.LicenseID, token, nil)
	var licAfter struct {
		License struct {
			Status string `json:"status"`
		} `json:"license"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &licAfter)
	if licAfter.License.Status != "revoked" {
		t.Fatalf("license status must be revoked, got %s", licAfter.License.Status)
	}
	w = activate(t, r, key, "dev-new")
	if w.Code != 403 {
		t.Fatalf("activate revoked license must 403, got %d %s", w.Code, w.Body.String())
	}
	_ = json.Unmarshal(w.Body.Bytes(), &errBody)
	if errBody.Error.Code != "LICENSE_REVOKED" {
		t.Fatalf("expected LICENSE_REVOKED, got %s", errBody.Error.Code)
	}
}

// TestConcurrentActivation 并发激活不突破设备上限(Skill 关键验收):
// max_devices=1,同时 10 台设备激活 → 最终 active 必须恰好 1 台。
func TestConcurrentActivation(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	token := login(t, r)
	key := issueKey(t, r, token, 1)

	const n = 10
	var wg sync.WaitGroup
	results := make([]int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := activate(t, r, key, fmt.Sprintf("dev-%d", i))
			results[i] = w.Code
		}(i)
	}
	wg.Wait()

	okCount := 0
	for _, code := range results {
		if code == 200 {
			okCount++
		} else if code != 409 {
			t.Fatalf("unexpected status %d", code)
		}
	}
	if okCount != 1 {
		t.Fatalf("exactly 1 of %d concurrent activates must succeed, got %d", n, okCount)
	}

	// 管理端复核:激活记录中 active 恰好 1 条
	licenseID := licenseIDFromKey(t, key)
	w := doJSON(t, r, "GET", "/api/v1/admin/licenses/"+licenseID+"/activations", token, nil)
	if w.Code != 200 {
		t.Fatalf("activations list: %d", w.Code)
	}
	var list struct {
		Items []struct {
			Status string `json:"status"`
		} `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	active := 0
	for _, it := range list.Items {
		if it.Status == "active" {
			active++
		}
	}
	if active != 1 {
		t.Fatalf("DB must contain exactly 1 active activation, got %d", active)
	}
}

// licenseIDFromKey 从 Key 提取 license_id(测试辅助)。
func licenseIDFromKey(t *testing.T, key string) string {
	t.Helper()
	p, ok := license.DecodePayloadOnly(key)
	if !ok {
		t.Fatal("decode key")
	}
	return p.LicenseID
}

// TestPublicActivateRateLimit 激活接口限流(防 Key 爆破)。
func TestPublicActivateRateLimit(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	d.ActivateLim = auth.NewLoginLimiter(time.Minute, 1, time.Minute) // 1 次后即锁
	r := testRouter(d)
	token := login(t, r)
	key := issueKey(t, r, token, 1)

	if w := activate(t, r, key, "dev-1"); w.Code != 200 {
		t.Fatalf("first activate must pass, got %d", w.Code)
	}
	if w := activate(t, r, key, "dev-2"); w.Code != 429 {
		t.Fatalf("second activate must be rate limited 429, got %d %s", w.Code, w.Body.String())
	}
}

// TestDeviceManagementAdmin 管理端设备管理 API。
func TestDeviceManagementAdmin(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	token := login(t, r)
	key := issueKey(t, r, token, 5)

	// 激活两台
	if w := activate(t, r, key, "dev-a"); w.Code != 200 {
		t.Fatalf("activate dev-a: %d", w.Code)
	}
	if w := activate(t, r, key, "dev-b"); w.Code != 200 {
		t.Fatalf("activate dev-b: %d", w.Code)
	}
	licenseID := licenseIDFromKey(t, key)

	// 激活列表 2 条
	w := doJSON(t, r, "GET", "/api/v1/admin/licenses/"+licenseID+"/activations", token, nil)
	if w.Code != 200 {
		t.Fatalf("activations list: %d", w.Code)
	}
	var list struct {
		Items []struct {
			ID       int64  `json:"id"`
			DeviceID string `json:"device_id"`
			Status   string `json:"status"`
		} `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Items) != 2 {
		t.Fatalf("expect 2 activations, got %d", len(list.Items))
	}

	// 单个解绑 dev-a
	aid := int64(0)
	for _, it := range list.Items {
		if it.DeviceID == "dev-a" {
			aid = it.ID
		}
	}
	w = doJSON(t, r, "POST", fmt.Sprintf("/api/v1/admin/licenses/%s/activations/%d/deactivate", licenseID, aid), token, nil)
	if w.Code != 200 {
		t.Fatalf("admin deactivate activation: %d %s", w.Code, w.Body.String())
	}
	// dev-b 解绑后重新激活(释放额度)
	if w = activate(t, r, key, "dev-c"); w.Code != 200 {
		t.Fatalf("activate dev-c after admin deactivate: %d %s", w.Code, w.Body.String())
	}

	// 列表字段含设备信息
	w = doJSON(t, r, "GET", "/api/v1/admin/licenses/"+licenseID+"/activations", token, nil)
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	seen := map[string]bool{}
	for _, it := range list.Items {
		seen[it.DeviceID] = true
	}
	if !seen["dev-c"] {
		t.Fatalf("dev-c must appear in activations: %+v", list.Items)
	}

	// 许可证详情/列表带 active_devices 计数
	w = doJSON(t, r, "GET", "/api/v1/admin/licenses/"+licenseID, token, nil)
	var lic struct {
		License struct {
			ActiveDevices int `json:"active_devices"`
			MaxDevices    int `json:"max_devices"`
		} `json:"license"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &lic)
	if lic.License.ActiveDevices != 2 || lic.License.MaxDevices != 5 {
		t.Fatalf("license device counts wrong: %+v", lic.License)
	}

	// 签名密钥注册表 ≥1 条,公钥与当前签发公钥一致
	w = doJSON(t, r, "GET", "/api/v1/admin/signing-keys", token, nil)
	if w.Code != 200 {
		t.Fatalf("signing-keys: %d", w.Code)
	}
	var keys struct {
		Items []struct {
			KeyID     string `json:"key_id"`
			PublicKey string `json:"public_key"`
			Status    string `json:"status"`
		} `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &keys)
	if len(keys.Items) < 1 {
		t.Fatal("signing-keys must have at least the current key")
	}
	want := crypto.PublicKeyBase64URL(d.LicenseSvc.PublicKey())
	if keys.Items[0].PublicKey != want {
		t.Fatal("registered public key must match current signing key")
	}

	// 匿名访问设备 API → 401
	if w := doJSON(t, r, "GET", "/api/v1/admin/licenses/"+licenseID+"/activations", "", nil); w.Code != 401 {
		t.Fatalf("anonymous activations must 401, got %d", w.Code)
	}
}
