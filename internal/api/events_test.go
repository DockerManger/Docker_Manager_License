package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ---------- V3 SSE 事件流测试(Event-Driven 主动同步) ----------
//
// 覆盖:
//   - SSE 认证(错误 token / 错误设备 → 401)
//   - 实时事件推送(管理端解绑 → activation.unbound 事件即时到达)
//   - Last-Event-ID Replay(断线期间事件不丢失)
//   - RESYNC_REQUIRED(事件中间缺口无法补齐)
//   - 事件持久化(Event Store 可查询,不丢事件)

// sseClient 建立 SSE 订阅,返回响应体 reader + 取消函数。
func sseClient(t *testing.T, srvURL, token, deviceID, lastEventID string) (*http.Response, *bufio.Reader, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, "GET", srvURL+"/api/v3/events", nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Device-ID", deviceID)
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("sse connect: %v", err)
	}
	if resp.StatusCode != 200 {
		cancel()
		t.Fatalf("sse connect status: %d", resp.StatusCode)
	}
	return resp, bufio.NewReader(resp.Body), cancel
}

// readSSEEvent 读取下一条 SSE 事件块(event: ... / id: ... / data: ...),返回 (eventName, id, dataJSON)。
func readSSEEvent(t *testing.T, r *bufio.Reader, timeout time.Duration) (string, string, string) {
	t.Helper()
	done := make(chan struct {
		evName, id, data string
	}, 1)
	go func() {
		var evName, id, data string
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				done <- struct{ evName, id, data string }{evName, id, data}
				return
			}
			line = strings.TrimRight(line, "\r\n")
			switch {
			case strings.HasPrefix(line, "event: "):
				evName = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "id: "):
				id = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "data: "):
				data += strings.TrimPrefix(line, "data: ")
			case line == "":
				if evName != "" || id != "" || data != "" {
					done <- struct{ evName, id, data string }{evName, id, data}
					return
				}
			}
		}
	}()
	select {
	case got := <-done:
		return got.evName, got.id, got.data
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for SSE event")
		return "", "", ""
	}
}

// sseAuthDenied 错误凭据必须 401。
func TestSSEAuthDenied(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	srv := httptest.NewServer(r)
	defer srv.Close()

	// 错误 token → 401
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/v3/events", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	req.Header.Set("X-Device-ID", "dev-x")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("wrong token must 401, got %d", resp.StatusCode)
	}
}

// TestSSEUnbindPush 核心:管理端解绑 → 事件经 SSE 实时到达(不等待周期检查)。
func TestSSEUnbindPush(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	srv := httptest.NewServer(r)
	defer srv.Close()

	adminTok := login(t, r)
	key := issueKey(t, r, adminTok, 3)
	_, actToken := activateWithToken(t, r, key, "sse-dev-1")
	licenseID := licenseIDFromKey(t, key)

	// 订阅 SSE
	resp, reader, cancel := sseClient(t, srv.URL, actToken, "sse-dev-1", "")
	defer cancel()
	defer resp.Body.Close()

	// 管理端解绑该设备(需要激活记录 db id)
	w := doJSON(t, r, "GET", "/api/v1/admin/licenses/"+licenseID+"/activations", adminTok, nil)
	var list struct {
		Items []struct {
			ID       int64  `json:"id"`
			DeviceID string `json:"device_id"`
		} `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	actDBID := int64(0)
	for _, it := range list.Items {
		if it.DeviceID == "sse-dev-1" {
			actDBID = it.ID
		}
	}
	if actDBID == 0 {
		t.Fatal("activation not found")
	}
	w = doJSON(t, r, "POST", fmt.Sprintf("/api/v1/admin/licenses/%s/activations/%d/deactivate", licenseID, actDBID), adminTok, nil)
	if w.Code != 200 {
		t.Fatalf("admin deactivate: %d %s", w.Code, w.Body.String())
	}

	// 事件必须实时到达:activation.unbound
	evName, evID, evData := readSSEEvent(t, reader, 5*time.Second)
	if evName != "activation.unbound" {
		t.Fatalf("expect activation.unbound event, got %q (data=%s)", evName, evData)
	}
	if !strings.HasPrefix(evID, "evt_") {
		t.Fatalf("event id must be evt_N, got %q", evID)
	}
	var ev struct {
		EventType    string `json:"event_type"`
		ActivationID string `json:"activation_id"`
		StateVersion int64  `json:"state_version"`
	}
	_ = json.Unmarshal([]byte(evData), &ev)
	if ev.EventType != "activation.unbound" || ev.ActivationID == "" {
		t.Fatalf("event payload incomplete: %s", evData)
	}
	if ev.StateVersion < 2 {
		t.Fatalf("state_version must be >= 2 after unbind, got %d", ev.StateVersion)
	}

	// 解绑后 verify → invalid
	if status, _ := verifyToken(t, r, actToken, "sse-dev-1", "nonce-sse-1"); status != "invalid" {
		t.Fatalf("verify after unbind must be invalid, got %s", status)
	}
}

// TestSSELastEventIDReplay 断线期间事件不丢失:Last-Event-ID 重放。
func TestSSELastEventIDReplay(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	srv := httptest.NewServer(r)
	defer srv.Close()

	adminTok := login(t, r)
	key := issueKey(t, r, adminTok, 3)
	_, actToken := activateWithToken(t, r, key, "sse-dev-2")
	licenseID := licenseIDFromKey(t, key)

	// 第一次延期 → 产生 license.changed 事件(订阅中收到)
	resp1, reader1, cancel1 := sseClient(t, srv.URL, actToken, "sse-dev-2", "")
	w := doJSON(t, r, "POST", "/api/v1/admin/licenses/"+licenseID+"/extend", adminTok, map[string]any{"days": 30, "reason": "replay-test-1"})
	if w.Code != 200 {
		t.Fatalf("extend: %d %s", w.Code, w.Body.String())
	}
	evName, evID, _ := readSSEEvent(t, reader1, 5*time.Second)
	if evName != "license.changed" {
		t.Fatalf("expect license.changed, got %q", evName)
	}
	lastID := evID
	resp1.Body.Close()
	cancel1()
	resp1.Close = true

	// "断线"期间:第二次延期(事件 evt_+1)
	w = doJSON(t, r, "POST", "/api/v1/admin/licenses/"+licenseID+"/extend", adminTok, map[string]any{"days": 30, "reason": "replay-test-2"})
	if w.Code != 200 {
		t.Fatalf("extend2: %d %s", w.Code, w.Body.String())
	}

	// 重连:Last-Event-ID = 上次最后处理的事件 → 必须 Replay 断线期间的新事件
	resp2, reader2, cancel2 := sseClient(t, srv.URL, actToken, "sse-dev-2", lastID)
	defer cancel2()
	defer resp2.Body.Close()

	replayedName, replayedID, replayedData := readSSEEvent(t, reader2, 5*time.Second)
	if replayedName != "license.changed" {
		t.Fatalf("replay expect license.changed, got %q (data=%s)", replayedName, replayedData)
	}
	// 重放的事件序号必须大于 lastID(evt_<N>)
	lastSeq, _ := strconv.ParseInt(strings.TrimPrefix(lastID, "evt_"), 10, 64)
	replaySeq, _ := strconv.ParseInt(strings.TrimPrefix(replayedID, "evt_"), 10, 64)
	if replaySeq <= lastSeq {
		t.Fatalf("replayed event %s must be after %s", replayedID, lastID)
	}
}

// TestSSEResyncRequired 事件中间缺口(被清理)→ 无法补齐 → RESYNC_REQUIRED。
func TestSSEResyncRequired(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	srv := httptest.NewServer(r)
	defer srv.Close()

	adminTok := login(t, r)
	key := issueKey(t, r, adminTok, 3)
	_, actToken := activateWithToken(t, r, key, "sse-dev-3") // evt_1 = activation.created
	licenseID := licenseIDFromKey(t, key)

	// 第二次延期 → evt_3(先做一次延期得到 evt_2,然后删掉中间事件制造缺口)
	w := doJSON(t, r, "POST", "/api/v1/admin/licenses/"+licenseID+"/extend", adminTok, map[string]any{"days": 30, "reason": "gap-1"})
	if w.Code != 200 {
		t.Fatalf("extend1: %d", w.Code)
	}
	// 删除中间事件(模拟 Event Store 清理):保留 evt_1 与最新事件,删除缺口中的
	// 精确做法:删除全部事件后重新确认行为 —— 这里删除除最新一条外的全部(制造 min > afterSeq 场景)
	pool := d.EventRepo.Pool()
	if _, err := pool.Exec(context.Background(),
		`DELETE FROM license_events WHERE license_id = $1 AND id NOT IN (
			SELECT MAX(id) FROM license_events WHERE license_id = $1)`, licenseID); err != nil {
		t.Fatal(err)
	}

	// 客户端最后处理的是 evt_1(activation.created),但 store 已只剩最新事件 → 无法补齐
	resp, reader, cancel := sseClient(t, srv.URL, actToken, "sse-dev-3", "evt_1")
	defer cancel()
	defer resp.Body.Close()

	evName, _, evData := readSSEEvent(t, reader, 5*time.Second)
	if evName != "resync_required" {
		t.Fatalf("expect resync_required, got %q (data=%s)", evName, evData)
	}
	var payload struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal([]byte(evData), &payload)
	if payload.Reason != "gap" {
		t.Fatalf("resync reason must be gap, got %s", evData)
	}
}

// TestSSEEventStorePersisted 事件持久化:管理端可查询事件历史(Event Store 不丢事件)。
func TestSSEEventStorePersisted(t *testing.T) {
	d, cleanup := testDeps(t)
	defer cleanup()
	r := testRouter(d)
	adminTok := login(t, r)
	key := issueKey(t, r, adminTok, 3)
	_, _ = activateWithToken(t, r, key, "sse-dev-4")
	licenseID := licenseIDFromKey(t, key)

	// 事件列表包含 activation.created
	w := doJSON(t, r, "GET", "/api/v1/admin/licenses/"+licenseID+"/events", adminTok, nil)
	if w.Code != 200 {
		t.Fatalf("license events: %d %s", w.Code, w.Body.String())
	}
	var list struct {
		Items []struct {
			EventType string `json:"event_type"`
			EventID   string `json:"event_id"`
		} `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	found := false
	for _, it := range list.Items {
		if it.EventType == "activation.created" && strings.HasPrefix(it.EventID, "evt_") {
			found = true
		}
	}
	if !found {
		t.Fatalf("activation.created event must be persisted: %s", w.Body.String())
	}
}
