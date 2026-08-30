package service

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DockerManger/Docker_Manager_License/internal/model"
)

// ---------- ActivationRepository ----------
//
// 在线激活闭环的设备记录存取。激活/解绑必须在事务内完成:
// 行锁 license 串行化同一 License 的并发激活,防止突破 max_devices。
// 激活凭据(Skill §11):数据库只存 SHA-256 hash(activation_tokens 表),明文只随响应下发一次。
//
// V3 Event-Driven:所有状态变更在**同一事务**内同时写入 license_events(Event Store),
// 提交后由 service 层 Publish 到 SSE。绝不允许"状态已改但事件没持久化"或反之。

// ActivationRepo 设备激活记录存取。
type ActivationRepo struct {
	pool   *pgxpool.Pool
	events *EventRepo
}

// NewActivationRepo 构造。
func NewActivationRepo(pool *pgxpool.Pool) *ActivationRepo {
	return &ActivationRepo{pool: pool, events: &EventRepo{pool: pool}}
}

// activationCols 查询列(join licenses 取展示 ID,表别名固定 a/l)。
const activationCols = `a.id, l.license_id, a.activation_id, a.device_id, a.device_name, a.device_fingerprint,
	a.platform, a.architecture, a.product_version, a.status, a.activated_at, a.last_seen_at,
	a.deactivated_at, a.expires_at, a.revoked_at, a.ip, a.metadata, a.state_version`

func scanActivation(row pgx.Row) (*model.Activation, error) {
	var x model.Activation
	err := row.Scan(&x.ID, &x.LicenseID, &x.ActivationID, &x.DeviceID, &x.DeviceName, &x.DeviceFingerprint,
		&x.Platform, &x.Architecture, &x.ProductVersion, &x.Status, &x.ActivatedAt, &x.LastSeenAt,
		&x.DeactivatedAt, &x.ExpiresAt, &x.RevokedAt, &x.IP, &x.Metadata, &x.StateVersion)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &x, nil
}

// bumpVersionTx 事务内 state_version +1,返回新版本。
func bumpVersionTx(ctx context.Context, tx pgx.Tx, actID int64) (int64, error) {
	var v int64
	err := tx.QueryRow(ctx,
		`UPDATE activations SET state_version = state_version + 1 WHERE id = $1 RETURNING state_version`,
		actID).Scan(&v)
	return v, err
}

// activationDisplayTx 事务内取激活记录展示信息(license 展示 ID + activation 展示 ID + device_id)。
func activationDisplayTx(ctx context.Context, tx pgx.Tx, actID int64) (licenseID, activationID, deviceID string, err error) {
	err = tx.QueryRow(ctx, `
		SELECT l.license_id, a.activation_id, a.device_id
		FROM activations a JOIN licenses l ON l.id = a.license_id
		WHERE a.id = $1`, actID).Scan(&licenseID, &activationID, &deviceID)
	return
}

// ActivateDevice 事务内原子激活:
//
//	行锁 license(同 License 并发激活串行化)→ 复核状态(吊销/过期)→ 检查设备上限
//	→ upsert 激活记录 → 吊销旧 token → 签发新 token(只存 hash)→ 持久化事件。
//
// 语义:
//   - 该设备已有 active 记录 → 幂等刷新(返回现有激活,签发新 token,旧 token 吊销;事件 activation.rebound)
//   - 该设备已有 deactivated 记录 → 重新激活(恢复 active,新 token;事件 activation.rebound)
//   - 全新设备 → 活跃数 < maxDevices 才插入(事件 activation.created)
//
// 参数:activationID 为展示 ID(ACT-<ULID>);tokenHash 为 SHA-256(token)(hex);
// licenseExpiresAt 为 License 过期时间(激活有效期随 License)。
// 返回:激活记录 + 事务内已持久化的事件(调用方提交后负责 Publish)。
func (r *ActivationRepo) ActivateDevice(ctx context.Context, licenseDBID string, maxDevices int,
	deviceID, deviceName, productVersion, deviceFingerprint, platform, architecture, ip,
	activationID, tokenHash string, licenseExpiresAt time.Time, now time.Time) (*model.Activation, []*model.LicenseEvent, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)

	// 行锁:并发激活同一 License 时串行化,从根上防止突破设备上限
	var status string
	var expiresAt int64
	var licenseDisplayID string
	if err := tx.QueryRow(ctx,
		`SELECT status, expires_at, license_id FROM licenses WHERE id = $1 FOR UPDATE`, licenseDBID,
	).Scan(&status, &expiresAt, &licenseDisplayID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	if status == model.StatusRevoked || status == model.StatusSuspended {
		return nil, nil, ErrLicenseRevoked
	}
	if expiresAt < now.Unix() {
		return nil, nil, ErrLicenseExpired
	}

	var events []*model.LicenseEvent
	emit := func(evType string, stateVersion int64, payload map[string]any) error {
		ev, err := r.events.InsertTx(ctx, tx, newEvent(evType, licenseDisplayID, activationID, deviceID, stateVersion, payload))
		if err != nil {
			return err
		}
		events = append(events, ev)
		return nil
	}

	// 已有该设备记录(锁行,防同设备并发激活产生重复)
	var existingID int64
	var existingStatus string
	err = tx.QueryRow(ctx,
		`SELECT id, status FROM activations WHERE license_id = $1 AND device_id = $2 FOR UPDATE`,
		licenseDBID, deviceID,
	).Scan(&existingID, &existingStatus)
	switch {
	case err == nil:
		if existingStatus == model.ActivationActive {
			// 幂等:已激活的设备再次激活 → 刷新心跳信息与设备指纹;重新签发 token(activation.rebound)
			if _, err := tx.Exec(ctx, `
				UPDATE activations SET last_seen_at = $2, ip = $3, product_version = $4, device_name = $5,
					device_fingerprint = $6, platform = $7, architecture = $8, expires_at = $9
				WHERE id = $1`, existingID, now, ip, productVersion, deviceName,
				deviceFingerprint, platform, architecture, licenseExpiresAt); err != nil {
				return nil, nil, err
			}
		} else {
			// 解绑过的设备重新激活
			if _, err := tx.Exec(ctx, `
				UPDATE activations SET status = 'active', activation_id = $2, activated_at = $3,
					last_seen_at = $3, deactivated_at = NULL, revoked_at = NULL, ip = $4,
					product_version = $5, device_name = $6, device_fingerprint = $7, platform = $8,
					architecture = $9, expires_at = $10
				WHERE id = $1`, existingID, activationID, now, ip, productVersion, deviceName,
				deviceFingerprint, platform, architecture, licenseExpiresAt); err != nil {
				return nil, nil, err
			}
		}
		v, err := bumpVersionTx(ctx, tx, existingID)
		if err != nil {
			return nil, nil, err
		}
		if err := emit(EvtActivationRebound, v, nil); err != nil {
			return nil, nil, err
		}
	case errors.Is(err, pgx.ErrNoRows):
		// 全新设备:检查活跃数上限后插入
		var activeCount int
		if err := tx.QueryRow(ctx,
			`SELECT COUNT(*) FROM activations WHERE license_id = $1 AND status = 'active'`,
			licenseDBID,
		).Scan(&activeCount); err != nil {
			return nil, nil, err
		}
		if activeCount >= maxDevices {
			return nil, nil, ErrDeviceLimit
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activations (license_id, activation_id, device_id, device_name, device_fingerprint,
				platform, architecture, product_version, status, expires_at, ip)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'active',$9,$10)`,
			licenseDBID, activationID, deviceID, deviceName, deviceFingerprint,
			platform, architecture, productVersion, licenseExpiresAt, ip); err != nil {
			return nil, nil, err
		}
		existingID, err = activationIDOf(ctx, tx, licenseDBID, deviceID)
		if err != nil {
			return nil, nil, err
		}
		// 新激活:state_version 初始 1,事件 version=1
		if err := emit(EvtActivationCreated, 1, nil); err != nil {
			return nil, nil, err
		}
	default:
		return nil, nil, err
	}

	// 吊销该激活的全部旧 token,签发新 token(只存 hash)
	if _, err := tx.Exec(ctx,
		`UPDATE activation_tokens SET revoked_at = $2 WHERE activation_id = $1 AND revoked_at IS NULL`,
		existingID, now); err != nil {
		return nil, nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activation_tokens (activation_id, token_hash, expires_at)
		VALUES ($1, $2, $3)`, existingID, tokenHash, now.Add(tokenTTL)); err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	act, err := r.GetActiveByDevice(ctx, licenseDBID, deviceID)
	return act, events, err
}

// activationIDOf 事务内按 (license, device) 取激活记录数据库 ID(新插入行)。
func activationIDOf(ctx context.Context, tx pgx.Tx, licenseDBID, deviceID string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx,
		`SELECT id FROM activations WHERE license_id = $1 AND device_id = $2`,
		licenseDBID, deviceID).Scan(&id)
	return id, err
}

// GetActiveByDevice 查询某 License 下某设备的 active 激活记录。
func (r *ActivationRepo) GetActiveByDevice(ctx context.Context, licenseDBID, deviceID string) (*model.Activation, error) {
	return scanActivation(r.pool.QueryRow(ctx, `
		SELECT `+activationCols+` FROM activations a
		JOIN licenses l ON l.id = a.license_id
		WHERE a.license_id = $1 AND a.device_id = $2 AND a.status = 'active'`, licenseDBID, deviceID))
}

// GetActiveByActivationID 按展示 ID(ACT-*)查询 active 激活记录。
func (r *ActivationRepo) GetActiveByActivationID(ctx context.Context, activationID string) (*model.Activation, error) {
	return scanActivation(r.pool.QueryRow(ctx, `
		SELECT `+activationCols+` FROM activations a
		JOIN licenses l ON l.id = a.license_id
		WHERE a.activation_id = $1 AND a.status = 'active'`, activationID))
}

// TouchLastSeen verify 心跳:更新最近在线时间/IP/产品版本。返回受影响行数。
// product_version 为空时不覆盖(客户端未上报时保留历史版本)。
// 注意:verify 是只读操作,不产生事件、不 bump state_version。
func (r *ActivationRepo) TouchLastSeen(ctx context.Context, licenseDBID, deviceID, ip, productVersion string, now time.Time) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE activations SET last_seen_at = $3, ip = $4,
			product_version = COALESCE(NULLIF($5, ''), product_version)
		WHERE license_id = $1 AND device_id = $2 AND status = 'active'`,
		licenseDBID, deviceID, now, ip, productVersion)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// DeactivateByToken 客户端解绑(token 路径):
// 凭据必须匹配该 License + 设备(防 Device A 用偷来的 token 解绑 Device B)。
// 同时吊销该激活的 token 与激活记录状态,同事务持久化 activation.unbound 事件。
func (r *ActivationRepo) DeactivateByToken(ctx context.Context, licenseDBID, deviceID, tokenHash string, now time.Time) ([]*model.LicenseEvent, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// 定位激活记录(token hash → activation,且属于该 license + 设备)
	var actID int64
	err = tx.QueryRow(ctx, `
		SELECT t.activation_id FROM activation_tokens t
		JOIN activations a ON a.id = t.activation_id
		WHERE t.token_hash = $1 AND t.revoked_at IS NULL AND t.expires_at > now()
		  AND a.license_id = $2 AND a.device_id = $3 AND a.status = 'active'`,
		tokenHash, licenseDBID, deviceID,
	).Scan(&actID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrActivationMismatch
		}
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE activation_tokens SET revoked_at = $2 WHERE activation_id = $1 AND revoked_at IS NULL`,
		actID, now); err != nil {
		return nil, err
	}
	tag, err := tx.Exec(ctx,
		`UPDATE activations SET status = 'deactivated', deactivated_at = $2
		WHERE id = $1 AND status = 'active'`, actID, now)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrActivationMismatch
	}
	v, err := bumpVersionTx(ctx, tx, actID)
	if err != nil {
		return nil, err
	}
	licenseDisplayID, activationDisplayID, devID, err := activationDisplayTx(ctx, tx, actID)
	if err != nil {
		return nil, err
	}
	ev, err := r.events.InsertTx(ctx, tx, newEvent(EvtActivationUnbound, licenseDisplayID, activationDisplayID, devID, v, nil))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return []*model.LicenseEvent{ev}, nil
}

// DeactivateByID 管理员按激活记录 ID 单个解绑(同时吊销 token),持久化 activation.unbound 事件。
func (r *ActivationRepo) DeactivateByID(ctx context.Context, activationID int64, now time.Time) ([]*model.LicenseEvent, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`UPDATE activation_tokens SET revoked_at = $2 WHERE activation_id = $1 AND revoked_at IS NULL`,
		activationID, now); err != nil {
		return nil, err
	}
	tag, err := tx.Exec(ctx,
		`UPDATE activations SET status = 'deactivated', deactivated_at = $2
		WHERE id = $1 AND status = 'active'`, activationID, now)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	v, err := bumpVersionTx(ctx, tx, activationID)
	if err != nil {
		return nil, err
	}
	licenseDisplayID, activationDisplayID, deviceID, err := activationDisplayTx(ctx, tx, activationID)
	if err != nil {
		return nil, err
	}
	ev, err := r.events.InsertTx(ctx, tx, newEvent(EvtActivationUnbound, licenseDisplayID, activationDisplayID, deviceID, v, nil))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return []*model.LicenseEvent{ev}, nil
}

// ResetDevices 管理员重置某 License 全部设备(解绑所有激活 + 吊销全部 token)。
// 每个受影响的激活各持久化一条 activation.unbound 事件。
// 返回受影响行数与事件列表。
func (r *ActivationRepo) ResetDevices(ctx context.Context, licenseDBID string, now time.Time) (int64, []*model.LicenseEvent, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback(ctx)

	// 先取受影响激活(license 展示 ID / activation 展示 ID / device_id / 状态版本)
	type actRow struct {
		actID   int64
		licID   string
		actDisp string
		devID   string
	}
	rows, err := tx.Query(ctx, `
		SELECT a.id, l.license_id, a.activation_id, a.device_id
		FROM activations a JOIN licenses l ON l.id = a.license_id
		WHERE a.license_id = $1 AND a.status = 'active'`, licenseDBID)
	if err != nil {
		return 0, nil, err
	}
	var affected []actRow
	for rows.Next() {
		var x actRow
		if err := rows.Scan(&x.actID, &x.licID, &x.actDisp, &x.devID); err != nil {
			rows.Close()
			return 0, nil, err
		}
		affected = append(affected, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, nil, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE activation_tokens SET revoked_at = $2
		WHERE revoked_at IS NULL AND activation_id IN (
			SELECT id FROM activations WHERE license_id = $1 AND status = 'active')`,
		licenseDBID, now); err != nil {
		return 0, nil, err
	}
	tag, err := tx.Exec(ctx,
		`UPDATE activations SET status = 'deactivated', deactivated_at = $2
		WHERE license_id = $1 AND status = 'active'`, licenseDBID, now)
	if err != nil {
		return 0, nil, err
	}

	var events []*model.LicenseEvent
	for _, x := range affected {
		v, err := bumpVersionTx(ctx, tx, x.actID)
		if err != nil {
			return 0, nil, err
		}
		ev, err := r.events.InsertTx(ctx, tx, newEvent(EvtActivationUnbound, x.licID, x.actDisp, x.devID, v, nil))
		if err != nil {
			return 0, nil, err
		}
		events = append(events, ev)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, nil, err
	}
	return tag.RowsAffected(), events, nil
}

// RevokeLicenseActivations 许可证吊销:更新 license 状态 + 全部激活标记 revoked +
// 吊销全部 token + 每个激活一条 activation.revoked 事件(同事务)。
// 返回 (license 展示 ID, 事件列表, error)。
func (r *ActivationRepo) RevokeLicenseActivations(ctx context.Context, licenseDBID, status, reason, by string, now time.Time) (string, []*model.LicenseEvent, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback(ctx)

	var licenseDisplayID string
	if err := tx.QueryRow(ctx,
		`SELECT license_id FROM licenses WHERE id = $1 FOR UPDATE`, licenseDBID).Scan(&licenseDisplayID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil, ErrNotFound
		}
		return "", nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE licenses SET status = $2, revoked_at = $3, revoked_reason = $4, revoked_by = $5
		WHERE id = $1`, licenseDBID, status, now.Unix(), reason, by); err != nil {
		return "", nil, err
	}

	type actRow struct {
		actID   int64
		actDisp string
		devID   string
	}
	rows, err := tx.Query(ctx, `
		SELECT id, activation_id, device_id FROM activations WHERE license_id = $1 AND status = 'active'`,
		licenseDBID)
	if err != nil {
		return "", nil, err
	}
	var affected []actRow
	for rows.Next() {
		var x actRow
		if err := rows.Scan(&x.actID, &x.actDisp, &x.devID); err != nil {
			rows.Close()
			return "", nil, err
		}
		affected = append(affected, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", nil, err
	}

	// 吊销全部激活 token
	if _, err := tx.Exec(ctx, `
		UPDATE activation_tokens SET revoked_at = $2
		WHERE revoked_at IS NULL AND activation_id IN (
			SELECT id FROM activations WHERE license_id = $1 AND status = 'active')`,
		licenseDBID, now); err != nil {
		return "", nil, err
	}
	// 激活标记 revoked(区别于管理员解绑的 deactivated)
	if _, err := tx.Exec(ctx, `
		UPDATE activations SET status = 'revoked', revoked_at = $2
		WHERE license_id = $1 AND status = 'active'`, licenseDBID, now); err != nil {
		return "", nil, err
	}

	var events []*model.LicenseEvent
	for _, x := range affected {
		v, err := bumpVersionTx(ctx, tx, x.actID)
		if err != nil {
			return "", nil, err
		}
		ev, err := r.events.InsertTx(ctx, tx, newEvent(EvtActivationRevoked, licenseDisplayID, x.actDisp, x.devID, v, nil))
		if err != nil {
			return "", nil, err
		}
		events = append(events, ev)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", nil, err
	}
	return licenseDisplayID, events, nil
}

// LicenseStateChanged 许可证级状态变化(延期/功能变更/挂起/恢复):
// 该 License 全部活跃激活 state_version +1,各持久化一条事件(同事务)。
// payload 为事件载荷(不含敏感信息)。返回事件列表。
func (r *ActivationRepo) LicenseStateChanged(ctx context.Context, licenseDBID, evType string, payload map[string]any) ([]*model.LicenseEvent, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var licenseDisplayID string
	if err := tx.QueryRow(ctx,
		`SELECT license_id FROM licenses WHERE id = $1`, licenseDBID).Scan(&licenseDisplayID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	type actRow struct {
		actID   int64
		actDisp string
		devID   string
	}
	rows, err := tx.Query(ctx, `
		SELECT id, activation_id, device_id FROM activations WHERE license_id = $1 AND status = 'active'`,
		licenseDBID)
	if err != nil {
		return nil, err
	}
	var affected []actRow
	for rows.Next() {
		var x actRow
		if err := rows.Scan(&x.actID, &x.actDisp, &x.devID); err != nil {
			rows.Close()
			return nil, err
		}
		affected = append(affected, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var events []*model.LicenseEvent
	for _, x := range affected {
		v, err := bumpVersionTx(ctx, tx, x.actID)
		if err != nil {
			return nil, err
		}
		ev, err := r.events.InsertTx(ctx, tx, newEvent(evType, licenseDisplayID, x.actDisp, x.devID, v, payload))
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return events, nil
}

// ListByLicense 某 License 全部激活记录(激活时间倒序)。
func (r *ActivationRepo) ListByLicense(ctx context.Context, licenseDBID string) ([]*model.Activation, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+activationCols+` FROM activations a
		JOIN licenses l ON l.id = a.license_id
		WHERE a.license_id = $1 ORDER BY a.activated_at DESC`, licenseDBID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*model.Activation, 0)
	for rows.Next() {
		x, err := scanActivation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// CountActive 活跃设备数。
func (r *ActivationRepo) CountActive(ctx context.Context, licenseDBID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activations WHERE license_id = $1 AND status = 'active'`, licenseDBID).Scan(&n)
	return n, err
}
