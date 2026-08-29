// Package service 业务层:LicenseService(签发/查询/延期/吊销)+ AdminService。
// API 层不直接写 SQL,统一经 repository(本包)访问 PostgreSQL。
package service

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DockerManger/Docker_Manager_License/internal/model"
)

// ErrNotFound 记录不存在。
var ErrNotFound = errors.New("not found")

// ErrConflict 状态冲突(如已吊销的 License 再次吊销)。
var ErrConflict = errors.New("conflict")

// ---------- AdminRepository ----------

// AdminRepo 管理端账号存取。
type AdminRepo struct{ pool *pgxpool.Pool }

// NewAdminRepo 构造。
func NewAdminRepo(pool *pgxpool.Pool) *AdminRepo { return &AdminRepo{pool: pool} }

// GetByUsername 按用户名查询。
func (r *AdminRepo) GetByUsername(ctx context.Context, username string) (*model.Admin, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, username, password_hash, token_version, totp_secret, created_at, updated_at
		FROM admins WHERE username = $1`, username)
	var a model.Admin
	if err := row.Scan(&a.ID, &a.Username, &a.PasswordHash, &a.TokenVersion, &a.TOTPSecret, &a.CreatedAt, &a.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}

// Create 创建管理员。
func (r *AdminRepo) Create(ctx context.Context, username, passwordHash string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO admins (username, password_hash) VALUES ($1, $2)`, username, passwordHash)
	return err
}

// Count 管理员数量(判断是否需要初始化)。
func (r *AdminRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM admins`).Scan(&n)
	return n, err
}

// UpdatePassword 修改密码并 token_version++(旧 JWT 全部失效)。
func (r *AdminRepo) UpdatePassword(ctx context.Context, id, passwordHash string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE admins SET password_hash = $2, token_version = token_version + 1, updated_at = now() WHERE id = $1`,
		id, passwordHash)
	return err
}

// UpdateTOTP 设置/关闭 TOTP 密钥并 token_version++。
func (r *AdminRepo) UpdateTOTP(ctx context.Context, id, secret string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE admins SET totp_secret = $2, token_version = token_version + 1, updated_at = now() WHERE id = $1`,
		id, secret)
	return err
}

// RevokeTokens 吊销该管理员所有 token(token_version++)。
func (r *AdminRepo) RevokeTokens(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE admins SET token_version = token_version + 1, updated_at = now() WHERE id = $1`, id)
	return err
}

// ---------- LicenseRepository ----------

// LicenseRepo License 主记录 + 修订 + 激活 存取。
type LicenseRepo struct{ pool *pgxpool.Pool }

// NewLicenseRepo 构造。
func NewLicenseRepo(pool *pgxpool.Pool) *LicenseRepo { return &LicenseRepo{pool: pool} }

// licenseCols 带 active_devices 子查询与 V3 关联展示 ID(所有 SELECT 共用,表别名固定为 l)。
const licenseCols = `l.id, l.license_id, l.key_id, l.product, l.plan, l.features, l.customer,
	COALESCE(c.customer_id, ''), COALESCE(s.subscription_id, ''),
	l.issued_at, l.expires_at, l.max_devices,
	(SELECT COUNT(*) FROM activations a WHERE a.license_id = l.id AND a.status = 'active') AS active_devices,
	l.status, l.revoked_at, l.revoked_reason, l.revoked_by, l.notes, l.created_at, l.updated_at`

// licenseJoin V3 关联表 join(LEFT JOIN,兼容无客户/订阅的存量 License)。
const licenseJoin = ` LEFT JOIN customers c ON c.id = l.customer_ref
	LEFT JOIN subscriptions s ON s.id = l.subscription_id`

func scanLicense(row pgx.Row) (*model.License, error) {
	var l model.License
	err := row.Scan(&l.ID, &l.LicenseID, &l.KeyID, &l.Product, &l.Plan, &l.Features, &l.Customer,
		&l.CustomerID, &l.SubscriptionID,
		&l.IssuedAt, &l.ExpiresAt, &l.MaxDevices, &l.ActiveDevices,
		&l.Status, &l.RevokedAt, &l.RevokedReason, &l.RevokedBy, &l.Notes, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &l, nil
}

// Create 创建 License 主记录(customerRef/subscriptionRef 可空,关联 V3 表)。
// 参数类型用 ::uuid 显式转换:NULLIF 与 ” 比较会让 PG 推断为 text,插 uuid 列报 42804。
func (r *LicenseRepo) Create(ctx context.Context, l *model.License, customerRef, subscriptionRef string) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO licenses (license_id, key_id, product, plan, features, customer,
			customer_ref, subscription_id, issued_at, expires_at, max_devices, status, notes)
		VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,'')::uuid,NULLIF($8,'')::uuid,$9,$10,$11,$12,$13)
		RETURNING id, created_at, updated_at`,
		l.LicenseID, l.KeyID, l.Product, l.Plan, l.Features, l.Customer,
		customerRef, subscriptionRef,
		l.IssuedAt, l.ExpiresAt, l.MaxDevices, l.Status, l.Notes,
	).Scan(&l.ID, &l.CreatedAt, &l.UpdatedAt)
}

// GetByLicenseID 按展示 ID 查询。
func (r *LicenseRepo) GetByLicenseID(ctx context.Context, licenseID string) (*model.License, error) {
	return scanLicense(r.pool.QueryRow(ctx,
		`SELECT `+licenseCols+` FROM licenses l`+licenseJoin+` WHERE l.license_id = $1`, licenseID))
}

// GetByDBID 按数据库 UUID 查询。
func (r *LicenseRepo) GetByDBID(ctx context.Context, id string) (*model.License, error) {
	return scanLicense(r.pool.QueryRow(ctx,
		`SELECT `+licenseCols+` FROM licenses l`+licenseJoin+` WHERE l.id = $1`, id))
}

// List 分页列表(按创建时间倒序)。
func (r *LicenseRepo) List(ctx context.Context, offset, limit int, status string) ([]*model.License, int, error) {
	var total int
	where := ""
	args := []any{limit, offset}
	if status != "" {
		where = " WHERE l.status = $3"
		args = append(args, status)
	}
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM licenses l`+where,
		append([]any{}, args[2:]...)...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+licenseCols+` FROM licenses l`+licenseJoin+where+
		` ORDER BY l.created_at DESC LIMIT $1 OFFSET $2`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]*model.License, 0)
	for rows.Next() {
		l, err := scanLicense(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, l)
	}
	return out, total, rows.Err()
}

// UpdateStatus 更新状态(吊销/挂起)。revokedBy 记录操作者。
func (r *LicenseRepo) UpdateStatus(ctx context.Context, id, status string, revokedAt *int64, reason, revokedBy string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE licenses SET status = $2, revoked_at = $3, revoked_reason = $4, revoked_by = $5, updated_at = now()
		WHERE id = $1`, id, status, revokedAt, reason, revokedBy)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateExpiry 更新过期时间(延期时主记录同步,不覆盖修订历史)。
func (r *LicenseRepo) UpdateExpiry(ctx context.Context, id string, expiresAt int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE licenses SET expires_at = $2, updated_at = now() WHERE id = $1`, id, expiresAt)
	return err
}

// SaveRevision 保存修订。
func (r *LicenseRepo) SaveRevision(ctx context.Context, rev *model.LicenseRevision) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO license_revisions (license_id, revision, payload, signature, key, reason, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id, created_at`,
		rev.LicenseID, rev.Revision, rev.Payload, rev.Signature, rev.Key, rev.Reason, rev.CreatedBy,
	).Scan(&rev.ID, &rev.CreatedAt)
}

// Revisions 某 License 的全部修订(升序)。
func (r *LicenseRepo) Revisions(ctx context.Context, licenseDBID string) ([]*model.LicenseRevision, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, license_id, revision, payload, signature, key, reason, created_by, created_at
		FROM license_revisions WHERE license_id = $1 ORDER BY revision ASC`, licenseDBID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*model.LicenseRevision, 0)
	for rows.Next() {
		var rev model.LicenseRevision
		if err := rows.Scan(&rev.ID, &rev.LicenseID, &rev.Revision, &rev.Payload,
			&rev.Signature, &rev.Key, &rev.Reason, &rev.CreatedBy, &rev.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &rev)
	}
	return out, rows.Err()
}

// Stats 概览统计。
func (r *LicenseRepo) Stats(ctx context.Context) (map[string]any, error) {
	type row struct {
		Status string `json:"status"`
		N      int    `json:"n"`
	}
	rows, err := r.pool.Query(ctx, `SELECT status, COUNT(*) FROM licenses GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]any{"total": 0, "by_status": map[string]int{}}
	byStatus := out["by_status"].(map[string]int)
	total := 0
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		byStatus[s] = n
		total += n
	}
	out["total"] = total
	return out, rows.Err()
}

// ---------- AuditRepository ----------

// AuditRepo 审计日志存取。
type AuditRepo struct{ pool *pgxpool.Pool }

// NewAuditRepo 构造。
func NewAuditRepo(pool *pgxpool.Pool) *AuditRepo { return &AuditRepo{pool: pool} }

// Log 写入一条审计日志。
func (r *AuditRepo) Log(ctx context.Context, e *model.AuditLog) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO audit_logs (admin, action, resource_type, resource_id, ip, metadata)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		e.Admin, e.Action, e.ResourceType, e.ResourceID, e.IP, e.Metadata)
	return err
}

// List 审计日志分页。
func (r *AuditRepo) List(ctx context.Context, offset, limit int) ([]*model.AuditLog, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, admin, action, resource_type, resource_id, ip, metadata, created_at
		FROM audit_logs ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]*model.AuditLog, 0)
	for rows.Next() {
		var e model.AuditLog
		if err := rows.Scan(&e.ID, &e.Admin, &e.Action, &e.ResourceType, &e.ResourceID,
			&e.IP, &e.Metadata, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, &e)
	}
	return out, total, rows.Err()
}

// ---------- helpers ----------

// Now 当前 Unix 秒(可注入测试)。
var Now = func() int64 { return time.Now().Unix() }

func containsStr(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
