package service

import (
	"errors"
)

// 在线激活/验证相关的业务错误。
// API 层负责映射为统一错误码(LICENSE_REVOKED / LICENSE_EXPIRED / DEVICE_LIMIT_REACHED ...)。
var (
	// ErrInvalidSignature Key 验签失败(伪造/篡改/错误密钥)。
	ErrInvalidSignature = errors.New("invalid license signature")
	// ErrLicenseRevoked License 已被吊销。
	ErrLicenseRevoked = errors.New("license revoked")
	// ErrLicenseExpired License 已过期。
	ErrLicenseExpired = errors.New("license expired")
	// ErrLicenseNotFound License 不存在(与 ErrNotFound 区分以便映射 LICENSE_NOT_FOUND)。
	ErrLicenseNotFound = errors.New("license not found")
	// ErrDeviceLimit 活跃设备数已达上限。
	ErrDeviceLimit = errors.New("device limit reached")
	// ErrActivationMismatch 激活凭据与 License/设备不匹配(防跨设备解绑)。
	ErrActivationMismatch = errors.New("activation not found or does not match device")
	// ErrActivationMissing 未提供 device_id。
	ErrActivationMissing = errors.New("device_id is required")
	// ErrReplayDetected 请求重放(timestamp 越界或 nonce 已用)。
	ErrReplayDetected = errors.New("replay detected")
	// ErrTokenInvalid 激活凭据无效。
	ErrTokenInvalid = errors.New("invalid activation token")
	// ErrTokenExpired 激活凭据已过期。
	ErrTokenExpired = errors.New("activation token expired")
	// ErrClientVersionBlocked 客户端版本已被服务端封禁。
	ErrClientVersionBlocked = errors.New("client version blocked")
	// ErrClockRollback 客户端检测到本地时钟回退(客户端本地状态)。
	ErrClockRollback = errors.New("clock rollback detected")
)
