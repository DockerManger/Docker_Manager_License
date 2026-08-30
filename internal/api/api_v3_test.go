package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// ---------- V3 协议测试(token 路径 / 重放防护 / 安全事件 / 版本控制 / 客户订阅) ----------
//
// 这些测试需要 TEST_DATABASE_URL(CI 提供 PostgreSQL),与 api_test.go 共用 testDeps。

// testIssueV3 签发并激活,返回 (licenseKey, activationID, activationToken)。
func testIssueV3(t *testing.T, r *gin.Engine, maxDevices int) (string, string, string) {
	t.Helper()
	key := issueKey(t, r, login(t, r), maxDevices)
	w := activate(t, r, key, "v3-device-1")
	var act struct {
		ActivationID    string `json:"activation_id"`
		ActivationToken string `json:"activation_token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &act); err != nil {
		t.Fatalf("activate parse: %v (body=%s)", err, w.Body.String())
	}
	if act.ActivationID == "" || act.ActivationToken == "" {
		t.Fatalf("activate must return activation_id + activation_token: %s", w.Body.String())
	}
	return key, act.ActivationID, act.ActivationToken
}

// TestV3TokenVerify 新协议:token 路径 verify 返回 valid + server_time,不带 key。
func TestV3TokenVerify(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	_, _, token := testIssueV3(t, r, 3)

	now := time.Now().Unix()
	w := doJSON(t, r, "POST", "/api/v3/verify", "", map[string]any{
		"activation_token": token,
		"device_id":        "v3-device-1",
		"product_version":  "v3.0.0",
		"timestamp":        now,
		"nonce":            "nonce-verify-1",
	})
	if w.Code != 200 {
		t.Fatalf("verify: %d %s", w.Code, w.Body.String())
	}
	var v struct {
		Status     string `json:"status"`
		Valid      bool   `json:"valid"`
		ServerTime int64  `json:"server_time"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	if !v.Valid || v.Status != "valid" {
		t.Fatalf("verify response: %s", w.Body.String())
	}
	if v.ServerTime == 0 {
		t.Fatal("verify must return server_time")
	}
}

// TestV3VerifyWrongDevice token 与 device_id 不匹配 → invalid。
func TestV3VerifyWrongDevice(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	_, _, token := testIssueV3(t, r, 3)

	w := doJSON(t, r, "POST", "/api/v3/verify", "", map[string]any{
		"activation_token": token,
		"device_id":        "other-device",
		"timestamp":        time.Now().Unix(),
		"nonce":            "nonce-wrong-dev",
	})
	var v struct {
		Status string `json:"status"`
		Valid  bool   `json:"valid"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	if v.Valid || v.Status != "invalid" {
		t.Fatalf("wrong device must be invalid: %s", w.Body.String())
	}
}

// TestV3ReplayProtection nonce 重用 → 400 REPLAY_DETECTED;时间戳越界 → 400。
func TestV3ReplayProtection(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	_, _, token := testIssueV3(t, r, 3)

	// 首次使用 nonce → 成功
	w := doJSON(t, r, "POST", "/api/v3/verify", "", map[string]any{
		"activation_token": token,
		"device_id":        "v3-device-1",
		"timestamp":        time.Now().Unix(),
		"nonce":            "nonce-replay-1",
	})
	if w.Code != 200 {
		t.Fatalf("first verify: %d", w.Code)
	}
	// 重用同一 nonce → REPLAY_DETECTED
	w = doJSON(t, r, "POST", "/api/v3/verify", "", map[string]any{
		"activation_token": token,
		"device_id":        "v3-device-1",
		"timestamp":        time.Now().Unix(),
		"nonce":            "nonce-replay-1",
	})
	if w.Code != 400 {
		t.Fatalf("reused nonce must be 400, got %d %s", w.Code, w.Body.String())
	}
	var eb struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &eb)
	if eb.Error.Code != "REPLAY_DETECTED" {
		t.Fatalf("want REPLAY_DETECTED, got %s", eb.Error.Code)
	}
	// 时间戳越界(10 分钟前)→ REPLAY_DETECTED
	w = doJSON(t, r, "POST", "/api/v3/verify", "", map[string]any{
		"activation_token": token,
		"device_id":        "v3-device-1",
		"timestamp":        time.Now().Unix() - 600,
		"nonce":            "nonce-stale-1",
	})
	if w.Code != 400 {
		t.Fatalf("stale timestamp must be 400, got %d", w.Code)
	}
}

// TestV3SecurityEvents 无效 token 记录 security event,admin 可查询。
func TestV3SecurityEvents(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	adminTok := login(t, r)

	// 无效 token 验证 → invalid + 记录 security event
	w := doJSON(t, r, "POST", "/api/v3/verify", "", map[string]any{
		"activation_token": "totally-invalid-token",
		"device_id":        "attacker-dev",
		"timestamp":        time.Now().Unix(),
		"nonce":            "nonce-sec-1",
	})
	if w.Code != 200 {
		t.Fatalf("invalid token verify must return 200 invalid, got %d", w.Code)
	}
	// admin 查询 security-events(带类型筛选,回归:COUNT 查询曾复用 $3 参数编号导致 42P18)
	for _, et := range []string{"invalid_token", "replay_detected", "rate_limit_exceeded", "device_limit_exceeded", "client_version_blocked", "tampered_timestamp", "invalid_signature"} {
		w = doJSON(t, r, "GET", "/api/v1/admin/security-events?type="+et, adminTok, nil)
		if w.Code != 200 {
			t.Fatalf("security-events?type=%s: %d %s", et, w.Code, w.Body.String())
		}
	}
	// 不带类型筛选
	w = doJSON(t, r, "GET", "/api/v1/admin/security-events", adminTok, nil)
	if w.Code != 200 {
		t.Fatalf("security-events: %d", w.Code)
	}
	var out struct {
		Items []struct {
			EventType string `json:"event_type"`
			DeviceID  string `json:"device_id"`
		} `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	found := false
	for _, it := range out.Items {
		if it.EventType == "invalid_token" && it.DeviceID == "attacker-dev" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("invalid_token security event not recorded: %s", w.Body.String())
	}
}

// TestV3VersionControl blocked_versions 封禁版本;minimum_client_version 随 verify 返回。
func TestV3VersionControl(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	adminTok := login(t, r)
	_, _, token := testIssueV3(t, r, 3)

	// 设置 blocked_versions + minimum_client_version
	w := doJSON(t, r, "PUT", "/api/v1/admin/settings", adminTok, map[string]any{
		"key": "blocked_versions", "value": `["1.0.0"]`,
	})
	if w.Code != 200 {
		t.Fatalf("set blocked: %d %s", w.Code, w.Body.String())
	}
	w = doJSON(t, r, "PUT", "/api/v1/admin/settings", adminTok, map[string]any{
		"key": "minimum_client_version", "value": "1.5.0",
	})
	if w.Code != 200 {
		t.Fatalf("set min version: %d", w.Code)
	}

	// blocked 版本 → status=blocked
	w = doJSON(t, r, "POST", "/api/v3/verify", "", map[string]any{
		"activation_token": token,
		"device_id":        "v3-device-1",
		"product_version":  "1.0.0",
		"timestamp":        time.Now().Unix(),
		"nonce":            "nonce-blocked-1",
	})
	var vb struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &vb)
	if vb.Status != "blocked" {
		t.Fatalf("blocked version must return blocked, got %s", w.Body.String())
	}

	// 正常版本 → valid + minimum_client_version
	w = doJSON(t, r, "POST", "/api/v3/verify", "", map[string]any{
		"activation_token": token,
		"device_id":        "v3-device-1",
		"product_version":  "2.0.0",
		"timestamp":        time.Now().Unix(),
		"nonce":            "nonce-min-1",
	})
	var vv struct {
		Status           string `json:"status"`
		MinimumClientVer string `json:"minimum_client_version"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &vv)
	if vv.Status != "valid" {
		t.Fatalf("normal version must be valid: %s", w.Body.String())
	}
	if vv.MinimumClientVer != "1.5.0" {
		t.Fatalf("minimum_client_version = %s, want 1.5.0", vv.MinimumClientVer)
	}

	// 清理配置(不影响其他测试)
	_ = doJSON(t, r, "PUT", "/api/v1/admin/settings", adminTok, map[string]any{
		"key": "blocked_versions", "value": "[]",
	})
	_ = doJSON(t, r, "PUT", "/api/v1/admin/settings", adminTok, map[string]any{
		"key": "minimum_client_version", "value": "",
	})
}

// TestV3TokenDeactivate token 解绑:凭据匹配 → 成功;错设备 → ACTIVATION_NOT_FOUND。
func TestV3TokenDeactivate(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	_, _, token := testIssueV3(t, r, 3)

	// 错设备解绑 → 404
	w := doJSON(t, r, "POST", "/api/v3/deactivate", "", map[string]any{
		"activation_token": token,
		"device_id":        "other-device",
		"timestamp":        time.Now().Unix(),
		"nonce":            "nonce-deact-1",
	})
	if w.Code != 404 {
		t.Fatalf("wrong device deactivate must be 404, got %d %s", w.Code, w.Body.String())
	}
	// 正确解绑 → ok
	w = doJSON(t, r, "POST", "/api/v3/deactivate", "", map[string]any{
		"activation_token": token,
		"device_id":        "v3-device-1",
		"timestamp":        time.Now().Unix(),
		"nonce":            "nonce-deact-2",
	})
	if w.Code != 200 {
		t.Fatalf("deactivate: %d %s", w.Code, w.Body.String())
	}
	// 解绑后 verify → unbound(License 保持 ACTIVE,客户端提示"请重新激活")
	// 注:旧行为返回 invalid;生命周期重构后解绑 ≠ 吊销,返回 unbound 携带 license 信息。
	w = doJSON(t, r, "POST", "/api/v3/verify", "", map[string]any{
		"activation_token": token,
		"device_id":        "v3-device-1",
		"timestamp":        time.Now().Unix(),
		"nonce":            "nonce-deact-3",
	})
	var v struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	if v.Status != "unbound" {
		t.Fatalf("verify after deactivate must be unbound: %s", w.Body.String())
	}
}

// TestCustomersSubscriptions 创建客户/订阅 → 签发带关联的 License → payload 含 customer_id/subscription_id。
func TestCustomersSubscriptions(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	adminTok := login(t, r)

	// 创建客户
	w := doJSON(t, r, "POST", "/api/v1/admin/customers", adminTok, map[string]any{
		"name": "Acme Corp", "email": "billing@acme.example",
	})
	if w.Code != 201 {
		t.Fatalf("create customer: %d %s", w.Code, w.Body.String())
	}
	var cust struct {
		Customer struct {
			CustomerID string `json:"customer_id"`
		} `json:"customer"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &cust)
	if cust.Customer.CustomerID == "" {
		t.Fatal("customer_id missing")
	}

	// 创建订阅
	w = doJSON(t, r, "POST", "/api/v1/admin/subscriptions", adminTok, map[string]any{
		"customer_id": cust.Customer.CustomerID,
		"plan":        "pro",
		"expires_at":  time.Now().Unix() + 86400*365,
		"auto_renew":  true,
	})
	if w.Code != 201 {
		t.Fatalf("create subscription: %d %s", w.Code, w.Body.String())
	}
	var sub struct {
		Subscription struct {
			SubscriptionID string `json:"subscription_id"`
		} `json:"subscription"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &sub)
	if sub.Subscription.SubscriptionID == "" {
		t.Fatal("subscription_id missing")
	}

	// 签发带关联的 License
	w = doJSON(t, r, "POST", "/api/v1/admin/licenses", adminTok, map[string]any{
		"customer":        "Acme Corp",
		"customer_id":     cust.Customer.CustomerID,
		"subscription_id": sub.Subscription.SubscriptionID,
		"plan":            "pro",
		"features":        []string{"compose"},
		"expire_days":     365,
		"max_devices":     2,
	})
	if w.Code != 201 {
		t.Fatalf("issue with customer: %d %s", w.Code, w.Body.String())
	}
	var issued struct {
		Payload string `json:"payload"`
		Key     string `json:"key"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &issued)
	var payload map[string]any
	_ = json.Unmarshal([]byte(issued.Payload), &payload)
	if payload["customer_id"] != cust.Customer.CustomerID {
		t.Fatalf("payload customer_id missing: %s", issued.Payload)
	}
	if payload["subscription_id"] != sub.Subscription.SubscriptionID {
		t.Fatalf("payload subscription_id missing: %s", issued.Payload)
	}
	// Key 可正常激活
	w = doJSON(t, r, "POST", "/api/v3/activate", "", map[string]any{
		"key": issued.Key, "device_id": "acme-dev-1",
	})
	if w.Code != 200 {
		t.Fatalf("activate issued key: %d %s", w.Code, w.Body.String())
	}
}
