package api

// ---------- License 生命周期测试(解绑 ≠ 吊销 / 删除规则 / 设备唯一绑定) ----------
//
// 需求(Skill 文档 §52-§55):
//   - 多个 License 可以同时存在(管理后台)
//   - 一个 Docker_Manager_Go(device_id)只能绑定一个 License
//   - 用户解绑 → License 保持 ACTIVE,Binding = UNBOUND(绝不 REVOKED)
//   - 管理员解绑 → License 保持 ACTIVE,Binding = UNBOUND(绝不 REVOKED)
//   - 解绑后 verify 返回 unbound(客户端可重新激活)
//   - 只有 REVOKED 的 License 才允许删除;ACTIVE(含 UNBOUND)删除一律 409
//   - 吊销 → REVOKED;吊销后不可重新激活
//   - 重新绑定:UNBOUND → Activate → BOUND
//   - 并发:同一设备同时激活两个 License → 最多一个成功(数据库约束兜底)

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// lifecycleIssue 签发并返回 (key, licenseID)。
func lifecycleIssue(t *testing.T, r *gin.Engine, token string, customer string) (string, string) {
	t.Helper()
	w := doJSON(t, r, "POST", "/api/v1/admin/licenses", token, map[string]any{
		"customer": customer, "plan": "pro", "features": []string{"compose", "container_create"},
		"expire_days": 365, "max_devices": 2,
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
	if resp.Key == "" || resp.License.LicenseID == "" {
		t.Fatalf("issue response incomplete: %s", w.Body.String())
	}
	return resp.Key, resp.License.LicenseID
}

// lifecycleActivate 激活并返回 (activation_id, activation_token)。
func lifecycleActivate(t *testing.T, r *gin.Engine, key, deviceID string) (string, string) {
	t.Helper()
	w := activate(t, r, key, deviceID)
	if w.Code != 200 {
		t.Fatalf("activate failed: %d %s", w.Code, w.Body.String())
	}
	var act struct {
		ActivationID    string `json:"activation_id"`
		ActivationToken string `json:"activation_token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &act)
	if act.ActivationID == "" || act.ActivationToken == "" {
		t.Fatalf("activate must return token: %s", w.Body.String())
	}
	return act.ActivationID, act.ActivationToken
}

// lifecycleVerify 调用 verify,返回响应体 map。
func lifecycleVerify(t *testing.T, r *gin.Engine, token, deviceID, nonce string) map[string]any {
	t.Helper()
	w := doJSON(t, r, "POST", "/api/v3/verify", "", map[string]any{
		"activation_token": token,
		"device_id":        deviceID,
		"timestamp":        time.Now().Unix(),
		"nonce":            nonce,
	})
	if w.Code != 200 {
		t.Fatalf("verify: %d %s", w.Code, w.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return out
}

// lifecycleGetLicense 管理端查 License 详情,返回 status。
func lifecycleGetStatus(t *testing.T, r *gin.Engine, token, licenseID string) string {
	t.Helper()
	w := doJSON(t, r, "GET", "/api/v1/admin/licenses/"+licenseID, token, nil)
	if w.Code != 200 {
		t.Fatalf("get license: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		License struct {
			Status string `json:"status"`
		} `json:"license"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return out.License.Status
}

// TestLifecycleMultipleLicenses 多个 License 同时存在(管理后台)。
func TestLifecycleMultipleLicenses(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	tok := login(t, r)

	// 创建 License A / B / C
	keys := make([]string, 0, 3)
	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		k, id := lifecycleIssue(t, r, tok, "customer-"+string(rune('A'+i)))
		keys = append(keys, k)
		ids = append(ids, id)
	}
	// 三个可同时存在
	w := doJSON(t, r, "GET", "/api/v1/admin/licenses?page=1&page_size=20", tok, nil)
	var out struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.Total < 3 {
		t.Fatalf("want >=3 licenses, got %d", out.Total)
	}
	// 每个都能独立激活(设备不同)
	for i, k := range keys {
		_, _ = lifecycleActivate(t, r, k, "multi-dev-"+string(rune('1'+i)))
	}
	_ = ids
}

// TestLifecycleDeviceSingleLicense 一个设备只能绑定一个 License:
// Device A 激活 License A 后,再激活 License B → 409 DEVICE_BOUND。
func TestLifecycleDeviceSingleLicense(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	tok := login(t, r)

	keyA, _ := lifecycleIssue(t, r, tok, "cust-a")
	keyB, _ := lifecycleIssue(t, r, tok, "cust-b")

	// Device A → License A:成功
	if w := activate(t, r, keyA, "device-A"); w.Code != 200 {
		t.Fatalf("activate A: %d %s", w.Code, w.Body.String())
	}
	// Device A → License B:必须拒绝(不覆盖 License A)
	w := activate(t, r, keyB, "device-A")
	if w.Code != 409 {
		t.Fatalf("second activate must be 409 DEVICE_BOUND, got %d %s", w.Code, w.Body.String())
	}
	var eb struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &eb)
	if eb.Error.Code != "DEVICE_BOUND" {
		t.Fatalf("want DEVICE_BOUND, got %s", eb.Error.Code)
	}
	// 先解绑 A → Device A 再激活 B:成功(历史清晰:解除 A 后再绑定 B)
	_, tokA := lifecycleActivate(t, r, keyA, "device-A") // 幂等拿 token
	w = doJSON(t, r, "POST", "/api/v3/deactivate", "", map[string]any{
		"activation_token": tokA, "device_id": "device-A",
		"timestamp": time.Now().Unix(), "nonce": "nonce-dev-1",
	})
	if w.Code != 200 {
		t.Fatalf("deactivate A: %d %s", w.Code, w.Body.String())
	}
	if w := activate(t, r, keyB, "device-A"); w.Code != 200 {
		t.Fatalf("activate B after unbind A must succeed: %d %s", w.Code, w.Body.String())
	}
}

// TestLifecycleUserUnbindKeepsActive 用户解绑:
// License A ACTIVE+BOUND → 用户解绑 → ACTIVE+UNBOUND(绝不 REVOKED)。
func TestLifecycleUserUnbindKeepsActive(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	tok := login(t, r)

	keyA, idA := lifecycleIssue(t, r, tok, "cust-a")
	_, tokAct := lifecycleActivate(t, r, keyA, "user-dev-1")
	if st := lifecycleGetStatus(t, r, tok, idA); st != "active" {
		t.Fatalf("after activate: want active, got %s", st)
	}

	// 用户解绑
	w := doJSON(t, r, "POST", "/api/v3/deactivate", "", map[string]any{
		"activation_token": tokAct, "device_id": "user-dev-1",
		"timestamp": time.Now().Unix(), "nonce": "nonce-ub-1",
	})
	if w.Code != 200 {
		t.Fatalf("user deactivate: %d %s", w.Code, w.Body.String())
	}
	// License 保持 ACTIVE —— 绝不 REVOKED
	if st := lifecycleGetStatus(t, r, tok, idA); st != "active" {
		t.Fatalf("after user unbind: license must stay ACTIVE, got %s", st)
	}
	// 解绑后 verify → unbound(不是 invalid/revoked),携带 license 信息
	out := lifecycleVerify(t, r, tokAct, "user-dev-1", "nonce-ub-2")
	if out["status"] != "unbound" {
		t.Fatalf("verify after unbind: want unbound, got %v", out["status"])
	}
	if out["license_id"] != idA {
		t.Fatalf("unbound verify must carry license_id, got %v", out["license_id"])
	}
}

// TestLifecycleAdminUnbindKeepsActive 管理员强制解绑:
// License A ACTIVE+BOUND → 管理员 unbind → ACTIVE+UNBOUND(绝不 REVOKED)。
func TestLifecycleAdminUnbindKeepsActive(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	tok := login(t, r)

	keyA, idA := lifecycleIssue(t, r, tok, "cust-a")
	lifecycleActivate(t, r, keyA, "admin-dev-1")
	if st := lifecycleGetStatus(t, r, tok, idA); st != "active" {
		t.Fatalf("after activate: want active, got %s", st)
	}

	// 管理员强制解绑 POST /admin/licenses/:id/unbind
	w := doJSON(t, r, "POST", "/api/v1/admin/licenses/"+idA+"/unbind", tok, nil)
	if w.Code != 200 {
		t.Fatalf("admin unbind: %d %s", w.Code, w.Body.String())
	}
	var ub struct {
		Unbound int `json:"unbound"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &ub)
	if ub.Unbound != 1 {
		t.Fatalf("unbound count want 1, got %d", ub.Unbound)
	}
	// License 保持 ACTIVE —— 绝不 REVOKED
	if st := lifecycleGetStatus(t, r, tok, idA); st != "active" {
		t.Fatalf("after admin unbind: license must stay ACTIVE, got %s", st)
	}
	// 幂等:再次 unbind 不报错(unbound=0)
	w = doJSON(t, r, "POST", "/api/v1/admin/licenses/"+idA+"/unbind", tok, nil)
	if w.Code != 200 {
		t.Fatalf("idempotent unbind must 200, got %d %s", w.Code, w.Body.String())
	}
	_ = json.Unmarshal(w.Body.Bytes(), &ub)
	if ub.Unbound != 0 {
		t.Fatalf("second unbind unbound count want 0, got %d", ub.Unbound)
	}
	// 解绑后重新激活(重新绑定)正常
	if w := activate(t, r, keyA, "admin-dev-1"); w.Code != 200 {
		t.Fatalf("re-activate after admin unbind: %d %s", w.Code, w.Body.String())
	}
	if st := lifecycleGetStatus(t, r, tok, idA); st != "active" {
		t.Fatalf("after re-activate: want active, got %s", st)
	}
}

// TestLifecycleRevokeAndDelete 吊销与删除规则:
//   - 吊销 → REVOKED;吊销后 verify → revoked;不可重新激活
//   - DELETE ACTIVE → 409;DELETE ACTIVE+UNBOUND → 409;DELETE REVOKED → 200
func TestLifecycleRevokeAndDelete(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	tok := login(t, r)

	keyA, idA := lifecycleIssue(t, r, tok, "cust-a")
	_, tokAct := lifecycleActivate(t, r, keyA, "rev-dev-1")

	// 吊销
	w := doJSON(t, r, "POST", "/api/v1/admin/licenses/"+idA+"/revoke", tok, map[string]any{
		"reason": "Fraud",
	})
	if w.Code != 200 {
		t.Fatalf("revoke: %d %s", w.Code, w.Body.String())
	}
	if st := lifecycleGetStatus(t, r, tok, idA); st != "revoked" {
		t.Fatalf("after revoke: want revoked, got %s", st)
	}
	// 吊销后 verify → revoked(不是 unbound)
	out := lifecycleVerify(t, r, tokAct, "rev-dev-1", "nonce-rev-1")
	if out["status"] != "revoked" {
		t.Fatalf("verify after revoke: want revoked, got %v", out["status"])
	}
	// 吊销后不可重新激活
	if w := activate(t, r, keyA, "rev-dev-1"); w.Code != 403 {
		t.Fatalf("re-activate revoked license must be 403, got %d %s", w.Code, w.Body.String())
	}

	// 删除 REVOKED → 成功
	w = doJSON(t, r, "DELETE", "/api/v1/admin/licenses/"+idA, tok, nil)
	if w.Code != 200 {
		t.Fatalf("delete revoked license must succeed, got %d %s", w.Code, w.Body.String())
	}
	// 删除后不存在
	if w := doJSON(t, r, "GET", "/api/v1/admin/licenses/"+idA, tok, nil); w.Code != 404 {
		t.Fatalf("deleted license must 404, got %d", w.Code)
	}
}

// TestLifecycleDeleteActiveRejected ACTIVE(含 UNBOUND)不允许删除:
//   - ACTIVE + BOUND → 409
//   - ACTIVE + UNBOUND(用户解绑后)→ 409
//   - EXPIRED → 409
func TestLifecycleDeleteActiveRejected(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	tok := login(t, r)

	// ACTIVE + BOUND
	keyB, idB := lifecycleIssue(t, r, tok, "cust-b")
	lifecycleActivate(t, r, keyB, "del-dev-b")
	w := doJSON(t, r, "DELETE", "/api/v1/admin/licenses/"+idB, tok, nil)
	if w.Code != 409 {
		t.Fatalf("delete ACTIVE+BOUND must be 409, got %d %s", w.Code, w.Body.String())
	}
	var eb struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &eb)
	if eb.Error.Code != "CONFLICT" {
		t.Fatalf("want CONFLICT, got %s", eb.Error.Code)
	}

	// ACTIVE + UNBOUND(用户解绑后)
	keyC, idC := lifecycleIssue(t, r, tok, "cust-c")
	_, tokC := lifecycleActivate(t, r, keyC, "del-dev-c")
	w = doJSON(t, r, "POST", "/api/v3/deactivate", "", map[string]any{
		"activation_token": tokC, "device_id": "del-dev-c",
		"timestamp": time.Now().Unix(), "nonce": "nonce-del-1",
	})
	if w.Code != 200 {
		t.Fatalf("deactivate: %d %s", w.Code, w.Body.String())
	}
	if st := lifecycleGetStatus(t, r, tok, idC); st != "active" {
		t.Fatalf("after unbind want active, got %s", st)
	}
	w = doJSON(t, r, "DELETE", "/api/v1/admin/licenses/"+idC, tok, nil)
	if w.Code != 409 {
		t.Fatalf("delete ACTIVE+UNBOUND must be 409, got %d %s", w.Code, w.Body.String())
	}
}

// TestLifecycleConcurrentSameDevice 并发:同一设备同时激活两个不同 License,
// 数据库 partial unique index 兜底 → 恰好一个成功,另一个 409 DEVICE_BOUND。
func TestLifecycleConcurrentSameDevice(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	tok := login(t, r)

	keyA, _ := lifecycleIssue(t, r, tok, "cust-a")
	keyB, _ := lifecycleIssue(t, r, tok, "cust-b")

	const dev = "concurrent-dev-1"
	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i, k := range []string{keyA, keyB} {
		wg.Add(1)
		go func(idx int, key string) {
			defer wg.Done()
			w := activate(t, r, key, dev)
			codes[idx] = w.Code
		}(i, k)
	}
	wg.Wait()
	ok, conflict := 0, 0
	for _, c := range codes {
		switch c {
		case 200:
			ok++
		case 409:
			conflict++
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("concurrent same-device activate: want exactly 1 ok + 1 conflict, got codes=%v", codes)
	}
	// 数据库最终一致性:该设备只有一条 active 激活
	adminTok := tok
	w := doJSON(t, r, "GET", "/api/v1/admin/licenses?page=1&page_size=50", adminTok, nil)
	var out struct {
		Items []struct {
			ActiveDevices int `json:"active_devices"`
		} `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	totalActive := 0
	for _, it := range out.Items {
		totalActive += it.ActiveDevices
	}
	if totalActive != 1 {
		t.Fatalf("total active activations must be exactly 1, got %d", totalActive)
	}
}

// TestLifecycleUnbindEventSource 解绑事件 payload 携带 source/reason(admin vs user)。
func TestLifecycleUnbindEventSource(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	tok := login(t, r)

	// 用户解绑
	keyU, idU := lifecycleIssue(t, r, tok, "cust-u")
	_, tokU := lifecycleActivate(t, r, keyU, "evt-dev-u")
	w := doJSON(t, r, "POST", "/api/v3/deactivate", "", map[string]any{
		"activation_token": tokU, "device_id": "evt-dev-u",
		"timestamp": time.Now().Unix(), "nonce": "nonce-evt-1",
	})
	if w.Code != 200 {
		t.Fatalf("deactivate: %d", w.Code)
	}
	// 管理员解绑
	keyA, idA := lifecycleIssue(t, r, tok, "cust-a")
	lifecycleActivate(t, r, keyA, "evt-dev-a")
	w = doJSON(t, r, "POST", "/api/v1/admin/licenses/"+idA+"/unbind", tok, nil)
	if w.Code != 200 {
		t.Fatalf("admin unbind: %d %s", w.Code, w.Body.String())
	}

	// 事件历史:找到两个 unbound 事件,source 分别正确
	seenUser, seenAdmin := false, false
	for _, id := range []string{idU, idA} {
		w = doJSON(t, r, "GET", "/api/v1/admin/licenses/"+id+"/events?limit=50", tok, nil)
		if w.Code != 200 {
			t.Fatalf("events: %d", w.Code)
		}
		var evs struct {
			Items []struct {
				EventType string         `json:"event_type"`
				Payload   map[string]any `json:"payload"`
			} `json:"items"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &evs)
		for _, ev := range evs.Items {
			if ev.EventType == "activation.unbound" {
				if ev.Payload["source"] == "user" {
					seenUser = true
				}
				if ev.Payload["source"] == "admin" {
					seenAdmin = true
				}
			}
		}
	}
	if !seenUser || !seenAdmin {
		t.Fatalf("want user+admin unbound events with source payload (user=%v admin=%v)", seenUser, seenAdmin)
	}
}

var _ = gin.Mode
