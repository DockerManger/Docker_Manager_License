package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/DockOrae/DockOrae-Auth/internal/model"
)

// ---------- V3 SSE 事件流(主动同步核心) ----------
//
// GET /api/v3/events
//
// 认证:Authorization: Bearer <activation_token> + X-Device-ID: <device_id>
// (Device A 只能订阅 Device A 的激活事件,不能订阅其他 Activation)。
//
// Replay:Last-Event-ID: evt_<sequence>(标准 SSE 头;也接受 ?last_event_id=)。
//   - 能补齐 → 先 Replay 缺失事件,再进入实时流
//   - 无法补齐(事件已清理/客户端落后太多)→ 发送 resync_required 事件,
//     客户端收到后立即 V3 Verify 获取最新权威状态
//
// 事件格式:
//
//	event: license.changed
//	id: evt_123
//	data: {"event_id":"evt_123","event_type":"license.changed",...}
//
// Keep-alive:每 20s 发送注释行(SSE 长连接保活,不是 Heartbeat,不触发 Verify)。

// sseKeepAlive 保活间隔(HTTP/SSE 长连接保活,禁止实现成 Heartbeat→Verify)。
const sseKeepAlive = 20 * time.Second

// sseReplayLimit 单次重放上限(防止事件积压时一次发送过多)。
const sseReplayLimit = 200

// sseEventStream GET /api/v3/events
func sseEventStream(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ---------- 认证(SSE 绑定当前 Activation) ----------
		token := bearerToken(c)
		deviceID := strings.TrimSpace(c.GetHeader("X-Device-ID"))
		if token == "" {
			token = strings.TrimSpace(c.Query("activation_token"))
		}
		act, ok := d.LicenseSvc.ValidateActivationToken(c.Request.Context(), token, deviceID)
		if !ok {
			abort(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid activation credentials")
			return
		}

		// ---------- SSE 头 ----------
		w := c.Writer
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)

		writeEvent := func(evName, id, data string) bool {
			if id != "" {
				if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
					return false
				}
			}
			if evName != "" {
				if _, err := fmt.Fprintf(w, "event: %s\n", evName); err != nil {
					return false
				}
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return false
			}
			if flusher != nil {
				flusher.Flush()
			}
			return true
		}

		// ---------- Replay(Last-Event-ID) ----------
		afterSeq := parseLastEventID(c)
		if afterSeq > 0 {
			replayed, err := d.EventRepo.ListAfter(c.Request.Context(), act.ActivationID, afterSeq, sseReplayLimit)
			if err != nil {
				return // 客户端断开或 DB 错误,直接结束
			}
			gap := false
			if len(replayed) == 0 {
				// 无事件可补:若该激活保留的最小事件序号 > afterSeq,
				// 说明客户端落后到事件已被清理/丢失 → 无法补齐 → RESYNC
				minSeq, _ := d.EventRepo.MinSequenceFor(c.Request.Context(), act.ActivationID)
				gap = minSeq > afterSeq
			} else if replayed[0].SequenceID > afterSeq+1 {
				// 有事件但开头即跳号(中间事件被清理)→ 无法完整补齐 → RESYNC
				gap = true
			}
			if gap {
				maxSeq, _ := d.EventRepo.MaxSequence(c.Request.Context())
				data, _ := json.Marshal(map[string]any{
					"reason": "gap", "last_event_id": "evt_" + strconv.FormatInt(afterSeq, 10),
					"server_sequence": maxSeq,
				})
				if !writeEvent("resync_required", "", string(data)) {
					return
				}
			} else if len(replayed) > 0 {
				// 正常 Replay:按 sequence 升序补发
				for _, ev := range replayed {
					if !writeEvent(ev.EventType, ev.EventID, sseEventData(ev)) {
						return
					}
				}
			}
		}

		// ---------- 实时订阅 ----------
		ch := d.Events.Subscribe(act.ActivationID)
		defer d.Events.Unsubscribe(act.ActivationID, ch)
		ctx := c.Request.Context()

		keepAlive := time.NewTicker(sseKeepAlive)
		defer keepAlive.Stop()

		for {
			select {
			case <-ctx.Done():
				return // 客户端断开:取消订阅,goroutine 退出,无泄漏
			case ev := <-ch:
				if !writeEvent(ev.EventType, ev.EventID, sseEventData(ev)) {
					return
				}
			case <-keepAlive.C:
				// SSE 长连接保活注释行(HTTP 层;不是 Heartbeat,不触发 Verify)
				if _, err := fmt.Fprintf(w, ": keep-alive\n\n"); err != nil {
					return
				}
				if flusher != nil {
					flusher.Flush()
				}
			}
		}
	}
}

// sseEventData 事件 SSE data 载荷(客户端处理所需的全部字段)。
func sseEventData(ev *model.LicenseEvent) string {
	m := map[string]any{
		"event_id":      ev.EventID,
		"sequence_id":   ev.SequenceID,
		"event_type":    ev.EventType,
		"license_id":    ev.LicenseID,
		"activation_id": ev.ActivationID,
		"device_id":     ev.DeviceID,
		"state_version": ev.StateVersion,
		"created_at":    ev.CreatedAt.Unix(),
	}
	if len(ev.Payload) > 0 && string(ev.Payload) != "{}" {
		var p map[string]any
		if json.Unmarshal(ev.Payload, &p) == nil {
			m["payload"] = p
		}
	}
	raw, _ := json.Marshal(m)
	return string(raw)
}

// parseLastEventID 解析 Last-Event-ID(evt_123 → 123;也支持 ?last_event_id=evt_123)。
func parseLastEventID(c *gin.Context) int64 {
	raw := strings.TrimSpace(c.GetHeader("Last-Event-ID"))
	if raw == "" {
		raw = strings.TrimSpace(c.Query("last_event_id"))
	}
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "evt_"))
	if raw == "" {
		return 0
	}
	n, _ := strconv.ParseInt(raw, 10, 64)
	return n
}
