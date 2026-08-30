package service

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DockOrae/DockOrae-Auth/internal/model"
)

// ---------- ActivationTokenRepository ----------
//
// 激活凭据只存 SHA-256 hash,数据库绝不明文(Skill §11)。
// 客户端获得明文 token,服务器验证时 hash 后比对。

// ActivationTokenRepo 激活凭据存取。
type ActivationTokenRepo struct{ pool *pgxpool.Pool }

// NewActivationTokenRepo 构造。
func NewActivationTokenRepo(pool *pgxpool.Pool) *ActivationTokenRepo {
	return &ActivationTokenRepo{pool: pool}
}

// Create 为某激活记录签发新 token(旧 token 全部吊销,支持设备重新激活)。
// 返回新 token 记录。tokenHash 为 SHA-256(hex)。
func (r *ActivationTokenRepo) Create(ctx context.Context, activationDBID int64, tokenHash string, ttl time.Duration, now time.Time) (*model.ActivationToken, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	// 吊销该激活的所有旧 token(防旧凭据继续使用)
	if _, err := tx.Exec(ctx,
		`UPDATE activation_tokens SET revoked_at = $2 WHERE activation_id = $1 AND revoked_at IS NULL`,
		activationDBID, now); err != nil {
		return nil, err
	}
	var t model.ActivationToken
	err = tx.QueryRow(ctx, `
		INSERT INTO activation_tokens (activation_id, token_hash, expires_at)
		VALUES ($1, $2, $3) RETURNING id, created_at, expires_at`,
		activationDBID, tokenHash, now.Add(ttl),
	).Scan(&t.ID, &t.CreatedAt, &t.ExpiresAt)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	t.ActivationID = activationDBID
	t.TokenHash = tokenHash
	return &t, nil
}

// FindByTokenHash 按 token hash 查找未吊销、未过期的 token(join activation 与 license)。
// 返回 token 记录 + 激活记录(含 license 展示 ID)。
func (r *ActivationTokenRepo) FindByTokenHash(ctx context.Context, tokenHash string) (*model.ActivationToken, *model.Activation, error) {
	return r.findByTokenHash(ctx, tokenHash, true)
}

// FindByTokenHashAny 按 token hash 查找 token(含已吊销/已过期,不参与激活判定)。
// 用于 verify 时区分"解绑(activation.deactivated,License 仍 active)"与"吊销":
// 解绑后 token 被吊销 → FindByTokenHash 找不到 → 用本方法定位原激活记录,
// 由 service 层根据 activation.status / license.status 返回 unbound / revoked。
func (r *ActivationTokenRepo) FindByTokenHashAny(ctx context.Context, tokenHash string) (*model.ActivationToken, *model.Activation, error) {
	return r.findByTokenHash(ctx, tokenHash, false)
}

func (r *ActivationTokenRepo) findByTokenHash(ctx context.Context, tokenHash string, onlyLive bool) (*model.ActivationToken, *model.Activation, error) {
	live := ` AND t.revoked_at IS NULL AND t.expires_at > now()`
	if !onlyLive {
		live = ``
	}
	var t model.ActivationToken
	var act model.Activation
	err := r.pool.QueryRow(ctx, `
		SELECT t.id, t.activation_id, t.token_hash, t.created_at, t.expires_at, t.last_used_at, t.revoked_at,
		       a.id, l.license_id, a.activation_id, a.device_id, a.device_name, a.device_fingerprint,
		       a.platform, a.architecture, a.product_version, a.status, a.state_version, a.activated_at, a.last_seen_at,
		       a.deactivated_at, a.expires_at, a.revoked_at, a.ip, a.metadata
		FROM activation_tokens t
		JOIN activations a ON a.id = t.activation_id
		JOIN licenses l ON l.id = a.license_id
		WHERE t.token_hash = $1`+live,
		tokenHash,
	).Scan(
		&t.ID, &t.ActivationID, &t.TokenHash, &t.CreatedAt, &t.ExpiresAt, &t.LastUsedAt, &t.RevokedAt,
		&act.ID, &act.LicenseID, &act.ActivationID, &act.DeviceID, &act.DeviceName, &act.DeviceFingerprint,
		&act.Platform, &act.Architecture, &act.ProductVersion, &act.Status, &act.StateVersion, &act.ActivatedAt, &act.LastSeenAt,
		&act.DeactivatedAt, &act.ExpiresAt, &act.RevokedAt, &act.IP, &act.Metadata,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	return &t, &act, nil
}

// RevokeAll 吊销某激活记录的全部 token(管理端解绑/重置设备时调用)。
func (r *ActivationTokenRepo) RevokeAll(ctx context.Context, activationDBID int64, now time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE activation_tokens SET revoked_at = $2 WHERE activation_id = $1 AND revoked_at IS NULL`,
		activationDBID, now)
	return err
}

// TouchLastUsed 更新 token 最近使用时间(verify 时调用)。
func (r *ActivationTokenRepo) TouchLastUsed(ctx context.Context, tokenID int64, now time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE activation_tokens SET last_used_at = $2 WHERE id = $1`, tokenID, now)
	return err
}

// ---------- SecurityEventRepository ----------

// SecurityEventRepo 安全事件存取(不记录 key/token/私钥等敏感信息)。
type SecurityEventRepo struct{ pool *pgxpool.Pool }

// NewSecurityEventRepo 构造。
func NewSecurityEventRepo(pool *pgxpool.Pool) *SecurityEventRepo {
	return &SecurityEventRepo{pool: pool}
}

// SecurityEventType 安全事件类型(与 Skill §20 对应)。
const (
	SecInvalidSignature     = "invalid_signature"
	SecInvalidToken         = "invalid_token"
	SecRateLimitExceeded    = "rate_limit_exceeded"
	SecReplayDetected       = "replay_detected"
	SecClockRollback        = "clock_rollback"
	SecDeviceLimitExceeded  = "device_limit_exceeded"
	SecSuspiciousActivation = "suspicious_activation"
	SecUnknownKeyID         = "unknown_key_id"
	SecUnsupportedVersion   = "unsupported_license_version"
	SecClientVersionBlocked = "client_version_blocked"
	SecTamperedTimestamp    = "tampered_timestamp"
)

// Log 写入一条安全事件(尽力而为,失败不阻断主流程)。
func (r *SecurityEventRepo) Log(ctx context.Context, e *model.SecurityEvent) {
	if r == nil || r.pool == nil {
		return
	}
	_, _ = r.pool.Exec(ctx, `
		INSERT INTO security_events (event_type, license_id, activation_id, device_id, ip, user_agent, details)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		e.EventType, e.LicenseID, e.ActivationID, e.DeviceID, e.IP, e.UserAgent, e.Details)
}

// List 安全事件分页(时间倒序)。eventType 空 = 全部。
func (r *SecurityEventRepo) List(ctx context.Context, offset, limit int, eventType string) ([]*model.SecurityEvent, int, error) {
	var total int
	where := ""
	args := []any{limit, offset}
	countWhere := ""
	countArgs := []any{}
	if eventType != "" {
		// 主查询:limit=$1 offset=$2 event_type=$3
		where = " WHERE event_type = $3"
		args = append(args, eventType)
		// COUNT 查询独立编号:参数从 $1 开始(与主查询共用 $3 会导致 42P18 参数无法绑定)
		countWhere = " WHERE event_type = $1"
		countArgs = append(countArgs, eventType)
	}
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM security_events`+countWhere, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, event_type, license_id, activation_id, device_id, ip, user_agent, details, created_at
		FROM security_events`+where+` ORDER BY created_at DESC LIMIT $1 OFFSET $2`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]*model.SecurityEvent, 0)
	for rows.Next() {
		var e model.SecurityEvent
		if err := rows.Scan(&e.ID, &e.EventType, &e.LicenseID, &e.ActivationID, &e.DeviceID,
			&e.IP, &e.UserAgent, &e.Details, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, &e)
	}
	return out, total, rows.Err()
}

// ---------- NonceRepository(重放防护) ----------

// NonceRepo 请求 nonce 存取(SHA-256 存储;定期清理过期记录)。
type NonceRepo struct{ pool *pgxpool.Pool }

// NewNonceRepo 构造。
func NewNonceRepo(pool *pgxpool.Pool) *NonceRepo { return &NonceRepo{pool: pool} }

// Use 尝试占用一个 nonce hash:已存在 → false(重放);不存在 → 插入并返回 true。
func (r *NonceRepo) Use(ctx context.Context, nonceHash string, now time.Time) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO security_nonces (nonce_hash, created_at) VALUES ($1, $2)
		ON CONFLICT (nonce_hash) DO NOTHING`, nonceHash, now)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// Cleanup 删除超过 maxAge 的 nonce(启动时与每小时调用)。
func (r *NonceRepo) Cleanup(ctx context.Context, maxAge time.Duration) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM security_nonces WHERE created_at < now() - $1::interval`,
		maxAge.String())
	return err
}

// ---------- ServerSettingsRepository(版本控制等) ----------

// ServerSettingsRepo 服务器配置存取。
type ServerSettingsRepo struct{ pool *pgxpool.Pool }

// NewServerSettingsRepo 构造。
func NewServerSettingsRepo(pool *pgxpool.Pool) *ServerSettingsRepo {
	return &ServerSettingsRepo{pool: pool}
}

// Get 读取配置(不存在返回空串,不报错)。
func (r *ServerSettingsRepo) Get(ctx context.Context, key string) (string, error) {
	var v string
	err := r.pool.QueryRow(ctx, `SELECT value FROM server_settings WHERE key = $1`, key).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// Set 写入配置(upsert)。
func (r *ServerSettingsRepo) Set(ctx context.Context, key, value string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO server_settings (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`,
		key, value)
	return err
}

// All 全部配置。
func (r *ServerSettingsRepo) All(ctx context.Context) ([]*model.ServerSetting, error) {
	rows, err := r.pool.Query(ctx, `SELECT key, value, updated_at FROM server_settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*model.ServerSetting, 0)
	for rows.Next() {
		var s model.ServerSetting
		if err := rows.Scan(&s.Key, &s.Value, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}
