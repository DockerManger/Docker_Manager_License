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

// ActivationRepo 设备激活记录存取。
type ActivationRepo struct{ pool *pgxpool.Pool }

// NewActivationRepo 构造。
func NewActivationRepo(pool *pgxpool.Pool) *ActivationRepo { return &ActivationRepo{pool: pool} }

// activationCols 查询列(join licenses 取展示 ID,表别名固定 a/l)。
const activationCols = `a.id, l.license_id, a.activation_id, a.device_id, a.device_name, a.device_fingerprint,
	a.platform, a.architecture, a.product_version, a.status, a.activated_at, a.last_seen_at,
	a.deactivated_at, a.expires_at, a.revoked_at, a.ip, a.metadata`

func scanActivation(row pgx.Row) (*model.Activation, error) {
	var x model.Activation
	err := row.Scan(&x.ID, &x.LicenseID, &x.ActivationID, &x.DeviceID, &x.DeviceName, &x.DeviceFingerprint,
		&x.Platform, &x.Architecture, &x.ProductVersion, &x.Status, &x.ActivatedAt, &x.LastSeenAt,
		&x.DeactivatedAt, &x.ExpiresAt, &x.RevokedAt, &x.IP, &x.Metadata)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &x, nil
}

// ActivateDevice 事务内原子激活:
//
//	行锁 license(同 License 并发激活串行化)→ 复核状态(吊销/过期)→ 检查设备上限
//	→ upsert 激活记录 → 吊销旧 token → 签发新 token(只存 hash)。
//
// 语义:
//   - 该设备已有 active 记录 → 幂等刷新(返回现有激活,签发新 token,旧 token 吊销)
//   - 该设备已有 deactivated 记录 → 重新激活(恢复 active,新 token)
//   - 全新设备 → 活跃数 < maxDevices 才插入
//
// 参数:activationID 为展示 ID(ACT-<ULID>);tokenHash 为 SHA-256(token)(hex);
// licenseExpiresAt 为 License 过期时间(激活有效期随 License)。
func (r *ActivationRepo) ActivateDevice(ctx context.Context, licenseDBID string, maxDevices int,
	deviceID, deviceName, productVersion, deviceFingerprint, platform, architecture, ip,
	activationID, tokenHash string, licenseExpiresAt time.Time, now time.Time) (*model.Activation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// 行锁:并发激活同一 License 时串行化,从根上防止突破设备上限
	var status string
	var expiresAt int64
	if err := tx.QueryRow(ctx,
		`SELECT status, expires_at FROM licenses WHERE id = $1 FOR UPDATE`, licenseDBID,
	).Scan(&status, &expiresAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if status == model.StatusRevoked || status == model.StatusSuspended {
		return nil, ErrLicenseRevoked
	}
	if expiresAt < now.Unix() {
		return nil, ErrLicenseExpired
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
			// 幂等:已激活的设备再次激活 → 刷新心跳信息与设备指纹
			if _, err := tx.Exec(ctx, `
				UPDATE activations SET last_seen_at = $2, ip = $3, product_version = $4, device_name = $5,
					device_fingerprint = $6, platform = $7, architecture = $8, expires_at = $9
				WHERE id = $1`, existingID, now, ip, productVersion, deviceName,
				deviceFingerprint, platform, architecture, licenseExpiresAt); err != nil {
				return nil, err
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
				return nil, err
			}
		}
	case errors.Is(err, pgx.ErrNoRows):
		// 全新设备:检查活跃数上限后插入
		var activeCount int
		if err := tx.QueryRow(ctx,
			`SELECT COUNT(*) FROM activations WHERE license_id = $1 AND status = 'active'`,
			licenseDBID,
		).Scan(&activeCount); err != nil {
			return nil, err
		}
		if activeCount >= maxDevices {
			return nil, ErrDeviceLimit
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activations (license_id, activation_id, device_id, device_name, device_fingerprint,
				platform, architecture, product_version, status, expires_at, ip)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'active',$9,$10)`,
			licenseDBID, activationID, deviceID, deviceName, deviceFingerprint,
			platform, architecture, productVersion, licenseExpiresAt, ip); err != nil {
			return nil, err
		}
		existingID, err = activationIDOf(ctx, tx, licenseDBID, deviceID)
		if err != nil {
			return nil, err
		}
	default:
		return nil, err
	}

	// 吊销该激活的全部旧 token,签发新 token(只存 hash)
	if _, err := tx.Exec(ctx,
		`UPDATE activation_tokens SET revoked_at = $2 WHERE activation_id = $1 AND revoked_at IS NULL`,
		existingID, now); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activation_tokens (activation_id, token_hash, expires_at)
		VALUES ($1, $2, $3)`, existingID, tokenHash, now.Add(tokenTTL)); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.GetActiveByDevice(ctx, licenseDBID, deviceID)
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
// 同时吊销该激活的 token 与激活记录状态。
func (r *ActivationRepo) DeactivateByToken(ctx context.Context, licenseDBID, deviceID, tokenHash string, now time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
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
			return ErrActivationMismatch
		}
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE activation_tokens SET revoked_at = $2 WHERE activation_id = $1 AND revoked_at IS NULL`,
		actID, now); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx,
		`UPDATE activations SET status = 'deactivated', deactivated_at = $2
		WHERE id = $1 AND status = 'active'`, actID, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrActivationMismatch
	}
	return tx.Commit(ctx)
}

// DeactivateByID 管理员按激活记录 ID 单个解绑(同时吊销 token)。
func (r *ActivationRepo) DeactivateByID(ctx context.Context, activationID int64, now time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`UPDATE activation_tokens SET revoked_at = $2 WHERE activation_id = $1 AND revoked_at IS NULL`,
		activationID, now); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx,
		`UPDATE activations SET status = 'deactivated', deactivated_at = $2
		WHERE id = $1 AND status = 'active'`, activationID, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

// ResetDevices 管理员重置某 License 全部设备(解绑所有激活 + 吊销全部 token)。
// 返回受影响行数。
func (r *ActivationRepo) ResetDevices(ctx context.Context, licenseDBID string, now time.Time) (int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE activation_tokens SET revoked_at = $2
		WHERE revoked_at IS NULL AND activation_id IN (
			SELECT id FROM activations WHERE license_id = $1 AND status = 'active')`,
		licenseDBID, now); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx,
		`UPDATE activations SET status = 'deactivated', deactivated_at = $2
		WHERE license_id = $1 AND status = 'active'`, licenseDBID, now)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
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
