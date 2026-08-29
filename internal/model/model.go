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

// ActivationStatus 设备激活状态。
const (
	ActivationActive      = "active"
	ActivationDeactivated = "deactivated"
	ActivationRevoked     = "revoked"
	ActivationExpired     = "expired"
)

// SigningKeyStatus 签名密钥状态。
const (
	SigningKeyActive  = "active"
	SigningKeyRetired = "retired"
	SigningKeyRevoked = "revoked"
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
	ID             string    `json:"id"`         // 数据库 UUID
	LicenseID      string    `json:"license_id"` // 展示用 DMG-<ULID>,全局唯一
	KeyID          string    `json:"key_id"`     // 签发密钥标识
	Product        string    `json:"product"`
	Plan           string    `json:"plan"`
	Features       []string  `json:"features"`
	Customer       string    `json:"customer"`                  // 客户名(展示用,冗余)
	CustomerID     string    `json:"customer_id,omitempty"`     // CUS-<ULID>(V3:关联 customers 表,查询时 join 带出)
	SubscriptionID string    `json:"subscription_id,omitempty"` // SUB-<ULID>(V3:关联 subscriptions 表)
	IssuedAt       int64     `json:"issued_at"`
	ExpiresAt      int64     `json:"expires_at"`
	MaxDevices     int       `json:"max_devices"`
	ActiveDevices  int       `json:"active_devices"` // 当前激活设备数(查询时带出)
	Status         string    `json:"status"`
	RevokedAt      *int64    `json:"revoked_at,omitempty"`
	RevokedReason  string    `json:"revoked_reason,omitempty"`
	RevokedBy      string    `json:"revoked_by,omitempty"`
	Notes          string    `json:"notes,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
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

// Activation 设备激活记录(在线激活闭环:激活/解绑/心跳由服务端权威记录)。
type Activation struct {
	ID                int64      `json:"id"`
	LicenseID         string     `json:"license_id"`    // 展示用 DMG-*(查询时由 licenses 表 join 填充)
	ActivationID      string     `json:"activation_id"` // 展示 ID(ACT-<ULID>)
	ActivationCode    string     `json:"-"`             // 历史明文凭据(003 迁移后已清空,不再读写)
	DeviceID          string     `json:"device_id"`
	DeviceName        string     `json:"device_name,omitempty"`
	DeviceFingerprint string     `json:"device_fingerprint,omitempty"`
	Platform          string     `json:"platform,omitempty"`
	Architecture      string     `json:"architecture,omitempty"`
	ProductVersion    string     `json:"product_version,omitempty"`
	Status            string     `json:"status"` // active / deactivated / revoked / expired
	ActivatedAt       time.Time  `json:"activated_at"`
	LastSeenAt        time.Time  `json:"last_seen_at"`
	DeactivatedAt     *time.Time `json:"deactivated_at,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"` // 激活有效期(随 License 过期)
	RevokedAt         *time.Time `json:"revoked_at,omitempty"`
	IP                string     `json:"ip,omitempty"`
	Metadata          string     `json:"metadata,omitempty"`
}

// ActivationToken 激活凭据记录(只存 SHA-256 hash,数据库绝不明文)。
type ActivationToken struct {
	ID           int64      `json:"id"`
	ActivationID int64      `json:"activation_id"` // activations.id
	TokenHash    string     `json:"-"`             // SHA-256(hex),绝不返回
	CreatedAt    time.Time  `json:"created_at"`
	ExpiresAt    time.Time  `json:"expires_at"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
}

// Customer 客户(V3:License 与客户身份分离)。
type Customer struct {
	ID         string    `json:"id"`
	CustomerID string    `json:"customer_id"` // CUS-<ULID>
	Name       string    `json:"name"`
	Email      string    `json:"email,omitempty"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Subscription 订阅(V3:支持月付/年付/永久/升级/降级/续费)。
type Subscription struct {
	ID             string    `json:"id"`
	SubscriptionID string    `json:"subscription_id"` // SUB-<ULID>
	CustomerID     string    `json:"customer_id"`     // CUS-<ULID>(查询时 join 带出)
	Plan           string    `json:"plan"`
	Status         string    `json:"status"` // active / expired / cancelled / suspended
	StartsAt       int64     `json:"starts_at"`
	ExpiresAt      int64     `json:"expires_at"`
	AutoRenew      bool      `json:"auto_renew"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// SecurityEvent 安全事件(只记录非敏感标识,不记录 key/token/私钥)。
type SecurityEvent struct {
	ID           int64     `json:"id"`
	EventType    string    `json:"event_type"`
	LicenseID    string    `json:"license_id,omitempty"`
	ActivationID string    `json:"activation_id,omitempty"`
	DeviceID     string    `json:"device_id,omitempty"`
	IP           string    `json:"ip,omitempty"`
	UserAgent    string    `json:"user_agent,omitempty"`
	Details      string    `json:"details,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// ServerSetting 服务器配置键值。
type ServerSetting struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SigningKey 签名密钥注册表记录。
type SigningKey struct {
	KeyID     string     `json:"key_id"`
	Algorithm string     `json:"algorithm"`
	PublicKey string     `json:"public_key"` // base64url(ed25519 32 字节)
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	RetiredAt *time.Time `json:"retired_at,omitempty"`
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
