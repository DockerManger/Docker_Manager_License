package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/MinimaxFlora/Docker_Manager_License/internal/crypto"
	"github.com/MinimaxFlora/Docker_Manager_License/internal/license"
	"github.com/MinimaxFlora/Docker_Manager_License/internal/model"
)

// NextVerifyAfter 客户端建议的下次验证间隔(秒)。配合客户端本地 Grace Period 使用。
const NextVerifyAfter = 86400 // 24h

// 激活码长度(hex 字节数 → 64 位字符)。
const activationCodeBytes = 32

// ---------- 在线激活闭环 ----------
//
// 职责划分(与 Docker_Manager_Go 客户端集成文档一致):
//   - 本地 Ed25519 验签证明"License 是官方签发且未被篡改"
//   - 本服务端在线闭环负责:设备绑定、设备上限、吊销/过期状态、心跳
//   - 客户端请求一律携带完整 License Key(服务端自行验签解析),
//     不暴露 license_id 查询凭据,防枚举

// ActivateRequest 激活请求(API 层解析后传入)。
type ActivateRequest struct {
	Key            string
	DeviceID       string
	DeviceName     string
	ProductVersion string
	IP             string
}

// ActivateResult 激活成功结果。
type ActivateResult struct {
	Activation *model.Activation
	LicenseID  string
	ExpiresAt  int64
	Features   []string
	MaxDevices int
}

// Activate 在线激活:
// 验签 → 查 License → 事务内设备绑定(行锁保证并发不破上限)→ 审计。
func (s *LicenseService) Activate(ctx context.Context, req ActivateRequest) (*ActivateResult, error) {
	if req.DeviceID == "" {
		return nil, ErrActivationMissing
	}
	payload, ok := s.verifyKey(ctx, req.Key)
	if !ok {
		return nil, ErrInvalidSignature
	}
	l, err := s.repo.GetByLicenseID(ctx, payload.LicenseID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrLicenseNotFound
		}
		return nil, err
	}
	code, err := newActivationCode()
	if err != nil {
		return nil, err
	}
	act, err := s.activationRepo.ActivateDevice(ctx, l.ID, l.MaxDevices,
		req.DeviceID, req.DeviceName, req.ProductVersion, req.IP, code, time.Unix(Now(), 0))
	if err != nil {
		return nil, err
	}
	_ = s.auditLog(ctx, "", req.IP, "license.activate", "license", l.LicenseID,
		map[string]any{"device_id": req.DeviceID, "product_version": req.ProductVersion})
	return &ActivateResult{
		Activation: act,
		LicenseID:  l.LicenseID,
		ExpiresAt:  l.ExpiresAt,
		Features:   l.Features,
		MaxDevices: l.MaxDevices,
	}, nil
}

// Deactivate 客户端解绑:必须携带激活时返回的 activation_id(activation_code)。
// 吊销/过期的 License 也允许解绑(客户端清理),但凭据必须匹配,防跨设备操作。
func (s *LicenseService) Deactivate(ctx context.Context, key, activationID, deviceID, ip string) error {
	if deviceID == "" || activationID == "" {
		return ErrActivationMissing
	}
	payload, ok := s.verifyKey(ctx, key)
	if !ok {
		return ErrInvalidSignature
	}
	l, err := s.repo.GetByLicenseID(ctx, payload.LicenseID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrLicenseNotFound
		}
		return err
	}
	if err := s.activationRepo.DeactivateByCode(ctx, l.ID, deviceID, activationID, time.Unix(Now(), 0)); err != nil {
		return err
	}
	_ = s.auditLog(ctx, "", ip, "license.deactivate", "license", l.LicenseID,
		map[string]any{"device_id": deviceID, "activation_id": activationID})
	return nil
}

// Verify 定期在线验证(客户端每 24h 调用):
//   - 验签失败 / License 不存在 / 设备未激活或凭据不匹配 → status=invalid(不泄露 License 存在性)
//   - 吊销/过期 → status=revoked/expired(客户端应禁用 Pro)
//   - 有效 → 更新心跳(last_seen_at)并返回状态与 features
//
// 兼容旧调用(不传 device_id):仅返回 License 在线状态,不更新心跳。
func (s *LicenseService) Verify(ctx context.Context, key, activationID, deviceID, productVersion, ip string) (map[string]any, error) {
	payload, ok := s.verifyKey(ctx, key)
	if !ok {
		return verifyResult("invalid", nil), nil
	}
	l, err := s.repo.GetByLicenseID(ctx, payload.LicenseID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return verifyResult("invalid", nil), nil
		}
		return nil, err
	}
	if l.Status == model.StatusRevoked || l.Status == model.StatusSuspended {
		return verifyResult("revoked", l), nil
	}
	if l.ExpiresAt < Now() {
		return verifyResult("expired", l), nil
	}

	if deviceID == "" {
		// 旧格式调用:仅在线状态查询,不校验设备
		return verifyResult("valid", l), nil
	}

	// 设备校验:未激活或凭据不匹配 → invalid(客户端不能继续使用 Pro)
	act, err := s.activationRepo.GetActiveByDevice(ctx, l.ID, deviceID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return verifyResult("invalid", nil), nil
		}
		return nil, err
	}
	if activationID != "" && act.ActivationCode != activationID {
		return verifyResult("invalid", nil), nil
	}
	// 心跳:更新 last_seen_at / ip / product_version
	_, _ = s.activationRepo.TouchLastSeen(ctx, l.ID, deviceID, ip, productVersion, time.Unix(Now(), 0))
	return verifyResult("valid", l), nil
}

// ListActivations 某 License 的全部设备激活记录(管理端)。
func (s *LicenseService) ListActivations(ctx context.Context, licenseID string) ([]*model.Activation, error) {
	l, err := s.repo.GetByLicenseID(ctx, licenseID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrLicenseNotFound
		}
		return nil, err
	}
	return s.activationRepo.ListByLicense(ctx, l.ID)
}

// DeactivateActivation 管理端按激活记录 ID 单个解绑。
func (s *LicenseService) DeactivateActivation(ctx context.Context, licenseID string, activationID int64, by, ip string) error {
	l, err := s.repo.GetByLicenseID(ctx, licenseID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrLicenseNotFound
		}
		return err
	}
	if err := s.activationRepo.DeactivateByID(ctx, activationID, time.Unix(Now(), 0)); err != nil {
		return err
	}
	_ = s.auditLog(ctx, by, ip, "license.device_deactivate", "license", l.LicenseID,
		map[string]any{"activation_id": activationID})
	return nil
}

// ResetDevices 管理端重置某 License 全部设备(解绑所有激活)。
func (s *LicenseService) ResetDevices(ctx context.Context, licenseID, by, ip string) (int, error) {
	l, err := s.repo.GetByLicenseID(ctx, licenseID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return 0, ErrLicenseNotFound
		}
		return 0, err
	}
	n, err := s.activationRepo.ResetDevices(ctx, l.ID, time.Unix(Now(), 0))
	if err != nil {
		return 0, err
	}
	_ = s.auditLog(ctx, by, ip, "license.devices_reset", "license", l.LicenseID,
		map[string]any{"deactivated": n})
	return int(n), nil
}

// EnsureSigningKey 注册当前签发密钥到 signing_keys 表(启动时调用)。
func (s *LicenseService) EnsureSigningKey(ctx context.Context) error {
	return s.signingKeys.Ensure(ctx, s.keyID, "ed25519", crypto.PublicKeyBase64URL(s.keyPair.Public))
}

// ListSigningKeys 签名密钥注册表(管理端)。
func (s *LicenseService) ListSigningKeys(ctx context.Context) ([]*model.SigningKey, error) {
	return s.signingKeys.List(ctx)
}

// ---------- 内部 ----------

// verifyKey 按 payload.key_id 从注册表取公钥验签。
// 注册表查不到(理论上不会:启动即注册)时回退当前公钥,保证可用性。
func (s *LicenseService) verifyKey(ctx context.Context, key string) (*license.Payload, bool) {
	if p, ok := license.VerifyKey(key, s.keyPair.Public); ok {
		return p, true
	}
	// key_id 指定的公钥(旧密钥轮换后仍需能验证旧 License)
	if raw, err := s.signingKeys.GetPublicKey(ctx, keyIDOf(key)); err == nil {
		if pub, err := crypto.PublicKeyFromBase64URL(raw); err == nil {
			if p, ok := license.VerifyKey(key, pub); ok {
				return p, true
			}
		}
	}
	return nil, false
}

// keyIDOf 从 Key 中提取 key_id(仅用于查公钥;解析失败返回空串,走当前公钥回退)。
func keyIDOf(key string) string {
	p, ok := license.DecodePayloadOnly(key)
	if !ok {
		return ""
	}
	return p.KeyID
}

// verifyResult 组装 verify 响应。
// invalid 一律不携带 License 信息(不泄露 License 存在性)。
func verifyResult(status string, l *model.License) map[string]any {
	out := map[string]any{"status": status, "valid": status == "valid"}
	if l != nil {
		out["license_id"] = l.LicenseID
		out["plan"] = l.Plan
		out["customer"] = l.Customer
		out["expires_at"] = l.ExpiresAt
		out["features"] = l.Features
	}
	if status == "valid" {
		out["next_verify_after"] = NextVerifyAfter
	}
	return out
}

// newActivationCode 生成激活凭据(32 字节随机 hex,碰撞概率可忽略)。
func newActivationCode() (string, error) {
	b := make([]byte, activationCodeBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
