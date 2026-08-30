package service

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DockerManger/Docker_Manager_License/internal/model"
)

// ---------- License 事件类型(V3 Event-Driven,SSE 主动同步) ----------
//
// 约定:事件只是 Trigger(通知客户端"状态可能变化"),客户端收到后必须 V3 Verify
// 获取 Server 权威状态 —— Event 本身不带授权结论。
const (
	EvtLicenseChanged       = "license.changed"        // 许可证状态/到期时间/plan/features 变化
	EvtLicenseRevoked       = "license.revoked"        // 许可证被吊销(全局广播 + 逐激活)
	EvtLicenseExpired       = "license.expired"        // 许可证过期(全局广播)
	EvtLicenseDisabled      = "license.disabled"       // 许可证被挂起(全局广播)
	EvtLicenseEnabled       = "license.enabled"        // 许可证恢复(全局广播)
	EvtActivationCreated    = "activation.created"     // 新设备激活
	EvtActivationRevoked    = "activation.revoked"     // 激活随许可证吊销被撤销
	EvtActivationUnbound    = "activation.unbound"     // 设备解绑(管理员/客户端)
	EvtActivationRebound    = "activation.rebound"     // 设备重新激活(含幂等刷新 token)
	EvtFeatureChanged       = "feature.changed"        // 功能集合变化
	EvtVersionPolicyChanged = "version_policy.changed" // 版本策略变化(minimum/blocked)
)

// ---------- Event Store(持久化,PostgreSQL) ----------

// EventRepo license_events 表存取:事务内持久化 + Replay 查询 + Gap 检测。
type EventRepo struct{ pool *pgxpool.Pool }

// NewEventRepo 构造。
func NewEventRepo(pool *pgxpool.Pool) *EventRepo { return &EventRepo{pool: pool} }

// Pool 暴露连接池(供 PublishGlobal 等需要独立事务的场景)。
func (r *EventRepo) Pool() *pgxpool.Pool { return r.pool }

// InsertTx 在既有事务内持久化一条事件(与状态变更同事务,保证一致性)。
// event_id 统一为 'evt_' || id(不落库,避免与 id 的 BIGSERIAL 共用序列产生错位)。
// 返回补齐 SequenceID / EventID / CreatedAt 的事件对象。
func (r *EventRepo) InsertTx(ctx context.Context, tx pgx.Tx, ev *model.LicenseEvent) (*model.LicenseEvent, error) {
	payload := []byte("{}")
	if len(ev.Payload) > 0 {
		payload = ev.Payload
	}
	var id int64
	var eventID string
	err := tx.QueryRow(ctx, `
		INSERT INTO license_events (event_type, license_id, activation_id, device_id, state_version, payload)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, 'evt_' || id::text, created_at`,
		ev.EventType, ev.LicenseID, ev.ActivationID, ev.DeviceID, ev.StateVersion, payload,
	).Scan(&id, &eventID, &ev.CreatedAt)
	if err != nil {
		return nil, err
	}
	ev.SequenceID = id
	ev.EventID = eventID
	return ev, nil
}

// ListAfter 返回某 Activation(或全局广播,activation_id=”)在 afterSeq 之后的事件,
// 按 sequence 升序(Replay 顺序)。limit 用于防止单次重放过长。
func (r *EventRepo) ListAfter(ctx context.Context, activationID string, afterSeq int64, limit int) ([]*model.LicenseEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, 'evt_' || id::text, event_type, license_id, activation_id, device_id, state_version, payload, created_at
		FROM license_events
		WHERE id > $1 AND (activation_id = $2 OR activation_id = '')
		ORDER BY id ASC
		LIMIT $3`, afterSeq, activationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

// MinSequenceFor 该 Activation(含全局广播)保留的最小 sequence。
// 无任何事件返回 0。
func (r *EventRepo) MinSequenceFor(ctx context.Context, activationID string) (int64, error) {
	var min int64
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(MIN(id), 0) FROM license_events
		WHERE activation_id = $1 OR activation_id = ''`, activationID).Scan(&min)
	return min, err
}

// MaxSequence 全局最大 sequence(用于 resync 时让客户端快进 Last-Event-ID)。
func (r *EventRepo) MaxSequence(ctx context.Context) (int64, error) {
	var max int64
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(MAX(id), 0) FROM license_events`).Scan(&max)
	return max, err
}

// ListForLicense 某 License 的全部事件(管理端审计/测试用,时间倒序)。
func (r *EventRepo) ListForLicense(ctx context.Context, licenseID string, limit int) ([]*model.LicenseEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, 'evt_' || id::text, event_type, license_id, activation_id, device_id, state_version, payload, created_at
		FROM license_events
		WHERE license_id = $1
		ORDER BY id DESC
		LIMIT $2`, licenseID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

func scanEvents(rows pgx.Rows) ([]*model.LicenseEvent, error) {
	out := make([]*model.LicenseEvent, 0)
	for rows.Next() {
		var ev model.LicenseEvent
		if err := rows.Scan(&ev.SequenceID, &ev.EventID, &ev.EventType, &ev.LicenseID,
			&ev.ActivationID, &ev.DeviceID, &ev.StateVersion, &ev.Payload, &ev.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &ev)
	}
	return out, rows.Err()
}

// ---------- 事件构造辅助 ----------

// newEvent 构造事件对象(带 payload map)。
func newEvent(eventType, licenseID, activationID, deviceID string, stateVersion int64, payload map[string]any) *model.LicenseEvent {
	raw := []byte("{}")
	if payload != nil {
		if b, err := json.Marshal(payload); err == nil {
			raw = b
		}
	}
	return &model.LicenseEvent{
		EventType:    eventType,
		LicenseID:    licenseID,
		ActivationID: activationID,
		DeviceID:     deviceID,
		StateVersion: stateVersion,
		Payload:      raw,
	}
}

// licenseStatusPayload 通用事件载荷:当前 License 状态摘要(不含 key/token 等敏感信息)。
func licenseStatusPayload(l *model.License, status string) map[string]any {
	if l == nil {
		return map[string]any{"status": status}
	}
	return map[string]any{
		"status":      status,
		"plan":        l.Plan,
		"features":    l.Features,
		"expires_at":  l.ExpiresAt,
		"max_devices": l.MaxDevices,
	}
}
