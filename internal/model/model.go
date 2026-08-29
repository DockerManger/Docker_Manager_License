// Package model 数据库实体(与 migrations 中的表结构对应)。
package model

import "time"

// LicenseStatus License 生命周期状态。
const (
	StatusActive    = "active"
	StatusExpired   = "expired"
	StatusRevoked   = "revoked"
	StatusSuspended = "suspended"
)

// Admin 管理端账号。
type Admin struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	TokenVersion int       `json:"-"` // 修改密码/吊销 token 后自增,旧 JWT 立即失效
	TOTPSecret   string    `json:"-"` // 已启用 2FA 时非空
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// License 许可证主记录。
type License struct {
	ID            string    `json:"id"`         // 数据库 UUID
	LicenseID     string    `json:"license_id"` // 展示用 DMG-<ULID>,全局唯一
	KeyID         string    `json:"key_id"`     // 签发密钥标识
	Product       string    `json:"product"`
	Plan          string    `json:"plan"`
	Features      []string  `json:"features"`
	Customer      string    `json:"customer"`
	IssuedAt      int64     `json:"issued_at"`
	ExpiresAt     int64     `json:"expires_at"`
	MaxDevices    int       `json:"max_devices"`
	Status        string    `json:"status"`
	RevokedAt     *int64    `json:"revoked_at,omitempty"`
	RevokedReason string    `json:"revoked_reason,omitempty"`
	Notes         string    `json:"notes,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// LicenseRevision License 历史修订(每次签发/延期/变更都生成新修订,不覆盖历史)。
type LicenseRevision struct {
	ID        int64     `json:"id"`
	LicenseID string    `json:"license_id"` // 外键(数据库 UUID)
	Revision  int       `json:"revision"`
	Payload   string    `json:"payload"`   // 规范 JSON(签名前内容)
	Signature string    `json:"signature"` // base64url 签名
	Key       string    `json:"key"`       // 完整 Key 字符串
	Reason    string    `json:"reason,omitempty"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// Activation 设备激活记录(第一版预留:离线验证由消费端本地绑定,
// 服务端记录仅用于统计/未来在线激活)。
type Activation struct {
	ID            int64      `json:"id"`
	LicenseID     string     `json:"license_id"`
	DeviceID      string     `json:"device_id"`
	DeviceName    string     `json:"device_name,omitempty"`
	ActivatedAt   time.Time  `json:"activated_at"`
	LastSeenAt    time.Time  `json:"last_seen_at"`
	DeactivatedAt *time.Time `json:"deactivated_at,omitempty"`
	IP            string     `json:"ip,omitempty"`
	Metadata      string     `json:"metadata,omitempty"`
}

// AuditLog 审计日志。
type AuditLog struct {
	ID           int64     `json:"id"`
	Admin        string    `json:"admin"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type,omitempty"`
	ResourceID   string    `json:"resource_id,omitempty"`
	IP           string    `json:"ip,omitempty"`
	Metadata     string    `json:"metadata,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}
