package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/DockerManger/Docker_Manager_License/internal/crypto"
	"github.com/DockerManger/Docker_Manager_License/internal/license"
	"github.com/DockerManger/Docker_Manager_License/internal/model"
)

// NextVerifyAfter 客户端建议的下次验证间隔(秒)。配合客户端本地 Grace Period 使用。
const NextVerifyAfter = 86400 // 24h

// 激活 token 字节数(hex 编码 → 64 位字符)。
const activationTokenBytes = 32

// tokenTTL 激活凭据有效期(随 License 续期重签;License 过期由 license 层判定)。
const tokenTTL = 365 * 24 * time.Hour

// ReplayWindow 请求时间戳允许偏差(±5 分钟,Skill §16)。
const ReplayWindow = 5 * time.Minute

// NonceMaxAge nonce 保留时间(超过即视为可复用;窗口内已占用即重放)。
const NonceMaxAge = time.Hour

// ---------- 在线激活闭环(V3) ----------
//
// 安全模型(与 Docker_Manager_Go 客户端集成文档一致):
//   - 本地 Ed25519 验签证明"License 是官方签发且未被篡改"
//   - License Key 只用于首次激活/重新激活(Skill §13)
//   - 正常运行使用 Activation Token:数据库只存 SHA-256 hash(Skill §11)
//   - verify/deactivate 携带 timestamp + nonce 防重放(Skill §16)
//   - 所有响应带 server_time,客户端据此计算 clock_offset(Skill §14)

// ActivateRequest 激活请求(API 层解析后传入)。
type ActivateRequest struct {
	Key               string
	DeviceID          string
	DeviceName        string
	ProductVersion    string
	DeviceFingerprint string
	Platform          string
	Architecture      string
	IP                string
}

// ActivateResult 激活成功结果。
type ActivateResult struct {
	Activation *model.Activation
	Token      string // 明文 Activation Token(仅本次响应返回;数据库只存 hash)
	LicenseID  string
	ExpiresAt  int64
	Features   []string
	MaxDevices int
}

// Activate 在线激活(V3):
// 验签 → 查 License → 事务内设备绑定(行锁保证并发不破上限)→ 签发 token(hash 存储)→ 审计。
// 返回明文 token 供客户端保存,数据库只存 SHA-256。
func (s *LicenseService) Activate(ctx context.Context, req ActivateRequest) (*ActivateResult, error) {
	if req.DeviceID == "" {
		return nil, ErrActivationMissing
	}
	payload, ok := s.verifyKey(ctx, req.Key)
	if !ok {
		s.securityLog(ctx, SecInvalidSignature, "", "", req.DeviceID, req.IP, "")
		return nil, ErrInvalidSignature
	}
	l, err := s.repo.GetByLicenseID(ctx, payload.LicenseID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrLicenseNotFound
		}
		return nil, err
	}
	// 生成展示 ID 与明文 token(token 只存 hash)
	activationID := license.NewActivationID()
	token, err := newActivationToken()
	if err != nil {
		return nil, err
	}
	tokenHash := sha256Hex(token)
	now := time.Unix(Now(), 0)
	act, err := s.activationRepo.ActivateDevice(ctx, l.ID, l.MaxDevices,
		req.DeviceID, req.DeviceName, req.ProductVersion, req.DeviceFingerprint,
		req.Platform, req.Architecture, req.IP, activationID, tokenHash, time.Unix(l.ExpiresAt, 0), now)
	if err != nil {
		if errors.Is(err, ErrDeviceLimit) {
			s.securityLog(ctx, SecDeviceLimitExceeded, l.LicenseID, activationID, req.DeviceID, req.IP, "")
		}
		return nil, err
	}
	_ = s.auditLog(ctx, "", req.IP, "license.activate", "license", l.LicenseID,
		map[string]any{"activation_id": act.ActivationID, "device_id": req.DeviceID, "product_version": req.ProductVersion})
	return &ActivateResult{
		Activation: act,
		Token:      token,
		LicenseID:  l.LicenseID,
		ExpiresAt:  l.ExpiresAt,
		Features:   l.Features,
		MaxDevices: l.MaxDevices,
	}, nil
}

// Deactivate 客户端解绑:
//   - 新路径:activation_token + device_id(凭据必须匹配,防 Device A 解绑 Device B)
//   - 旧路径(升级窗口期兼容):key + activation_id(=旧 code,已迁移为 token hash)+ device_id
//
// 吊销/过期的 License 也允许解绑(客户端清理)。
func (s *LicenseService) Deactivate(ctx context.Context, req DeactivateRequest) error {
	now := time.Unix(Now(), 0)
	// 新路径:token 凭据
	if req.Token != "" {
		hash := sha256Hex(req.Token)
		_, act, err := s.tokens.FindByTokenHash(ctx, hash)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				s.securityLog(ctx, SecInvalidToken, "", "", req.DeviceID, req.IP, "")
				return ErrActivationMismatch
			}
			return err
		}
		if req.DeviceID != "" && act.DeviceID != req.DeviceID {
			return ErrActivationMismatch
		}
		l, err := s.repo.GetByLicenseID(ctx, act.LicenseID)
		if err != nil {
			return err
		}
		if err := s.activationRepo.DeactivateByToken(ctx, l.ID, act.DeviceID, hash, now); err != nil {
			return err
		}
		_ = s.auditLog(ctx, "", req.IP, "license.deactivate", "license", l.LicenseID,
			map[string]any{"activation_id": act.ActivationID, "device_id": act.DeviceID})
		return nil
	}
	// 旧路径:key + activation_id + device_id(兼容升级窗口期)
	if req.Key == "" || req.ActivationID == "" || req.DeviceID == "" {
		return ErrActivationMissing
	}
	payload, ok := s.verifyKey(ctx, req.Key)
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
	// 旧凭据解绑(兼容两种格式):
	//   - ACT-* 新展示 ID:按激活记录解绑(校验设备匹配)
	//   - 旧 code(升级前客户端):已迁移为 token hash,按 hash 解绑
	if strings.HasPrefix(req.ActivationID, "ACT-") {
		act, err := s.activationRepo.GetActiveByActivationID(ctx, req.ActivationID)
		if err != nil || act.DeviceID != req.DeviceID {
			return ErrActivationMismatch
		}
		if err := s.activationRepo.DeactivateByID(ctx, act.ID, now); err != nil {
			return err
		}
	} else {
		if err := s.activationRepo.DeactivateByToken(ctx, l.ID, req.DeviceID, sha256Hex(req.ActivationID), now); err != nil {
			return err
		}
	}
	_ = s.auditLog(ctx, "", req.IP, "license.deactivate", "license", l.LicenseID,
		map[string]any{"device_id": req.DeviceID})
	return nil
}

// DeactivateRequest 解绑请求(双路径)。
type DeactivateRequest struct {
	Key          string // 旧路径兼容
	Token        string // 新路径
	ActivationID string // 旧路径兼容(旧 code,已 hash 迁移)
	DeviceID     string
	Timestamp    int64
	Nonce        string
	IP           string
	UserAgent    string
}

// VerifyRequest 定期验证请求(V3:不带完整 License Key)。
type VerifyRequest struct {
	Token          string // 新路径
	Key            string // 旧路径兼容(升级窗口期)
	ActivationID   string // 旧路径兼容
	DeviceID       string
	ProductVersion string
	Timestamp      int64
	Nonce          string
	IP             string
	UserAgent      string
}

// Verify 定期在线验证(客户端每 24h 调用,V3):
//   - 重放防护:timestamp ±5min + nonce 唯一(Skill §16)
//   - token 凭据验证(Skill §13):凭据无效 → status=invalid(不泄露 License 存在性)
//   - License 吊销/过期 → status=revoked/expired(客户端禁用 Pro)
//   - 版本控制:blocked_versions → CLIENT_VERSION_BLOCKED(Skill §21)
//   - 有效 → 更新心跳 + token last_used,返回 server_time / minimum_client_version
//
// 旧路径(key + activation_id)兼容升级窗口期,凭据按迁移后的 token hash 校验。
func (s *LicenseService) Verify(ctx context.Context, req VerifyRequest) (map[string]any, error) {
	// 重放防护(所有路径统一)
	if req.Timestamp > 0 || req.Nonce != "" {
		if ok, err := s.replayAllowed(ctx, req.Timestamp, req.Nonce, req.IP); err != nil {
			return nil, err
		} else if !ok {
			return s.verifyResult("invalid", nil), nil
		}
	}

	// ---------- 新路径:activation_token ----------
	if req.Token != "" {
		hash := sha256Hex(req.Token)
		tok, act, err := s.tokens.FindByTokenHash(ctx, hash)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				s.securityLog(ctx, SecInvalidToken, "", "", req.DeviceID, req.IP, "")
				return s.verifyResult("invalid", nil), nil
			}
			return nil, err
		}
		if req.DeviceID != "" && act.DeviceID != req.DeviceID {
			return s.verifyResult("invalid", nil), nil
		}
		if act.Status != model.ActivationActive {
			return s.verifyResult("invalid", nil), nil
		}
		l, err := s.repo.GetByLicenseID(ctx, act.LicenseID)
		if err != nil {
			return nil, err
		}
		// 版本控制:blocked 版本禁止 Pro
		if blocked, ver := s.versionBlocked(ctx, req.ProductVersion); blocked {
			s.securityLog(ctx, SecClientVersionBlocked, l.LicenseID, act.ActivationID, act.DeviceID, req.IP, ver)
			return s.verifyResultWith(l, "blocked"), nil
		}
		if l.Status == model.StatusRevoked || l.Status == model.StatusSuspended {
			return s.verifyResult("revoked", l), nil
		}
		if l.ExpiresAt < Now() {
			return s.verifyResult("expired", l), nil
		}
		// 心跳 + token 使用时间
		_, _ = s.activationRepo.TouchLastSeen(ctx, l.ID, act.DeviceID, req.IP, req.ProductVersion, time.Unix(Now(), 0))
		_ = s.tokens.TouchLastUsed(ctx, tok.ID, time.Unix(Now(), 0))
		return s.verifyResultWith(l, "valid"), nil
	}

	// ---------- 旧路径:key + activation_id(升级窗口期兼容) ----------
	if req.Key == "" {
		return s.verifyResult("invalid", nil), nil
	}
	payload, ok := s.verifyKey(ctx, req.Key)
	if !ok {
		s.securityLog(ctx, SecInvalidSignature, "", "", req.DeviceID, req.IP, "")
		return s.verifyResult("invalid", nil), nil
	}
	l, err := s.repo.GetByLicenseID(ctx, payload.LicenseID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return s.verifyResult("invalid", nil), nil
		}
		return nil, err
	}
	if blocked, _ := s.versionBlocked(ctx, req.ProductVersion); blocked {
		return s.verifyResultWith(l, "blocked"), nil
	}
	if l.Status == model.StatusRevoked || l.Status == model.StatusSuspended {
		return s.verifyResult("revoked", l), nil
	}
	if l.ExpiresAt < Now() {
		return s.verifyResult("expired", l), nil
	}
	if req.DeviceID == "" {
		// 最旧格式:仅在线状态查询,不校验设备
		return s.verifyResult("valid", l), nil
	}
	act, err := s.activationRepo.GetActiveByDevice(ctx, l.ID, req.DeviceID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return s.verifyResult("invalid", nil), nil
		}
		return nil, err
	}
	// 旧凭据校验(兼容两种格式):
	//   - ACT-* 新展示 ID:必须与该设备激活记录一致
	//   - 旧 code(升级前客户端):按迁移后的 token hash 匹配
	if req.ActivationID != "" {
		if strings.HasPrefix(req.ActivationID, "ACT-") {
			if act.ActivationID != req.ActivationID {
				return s.verifyResult("invalid", nil), nil
			}
		} else if _, tact, err := s.tokens.FindByTokenHash(ctx, sha256Hex(req.ActivationID)); err != nil || tact.ID != act.ID {
			return s.verifyResult("invalid", nil), nil
		}
	}
	_, _ = s.activationRepo.TouchLastSeen(ctx, l.ID, req.DeviceID, req.IP, req.ProductVersion, time.Unix(Now(), 0))
	return s.verifyResult("valid", l), nil
}

// ListActivations 某 License 的全部设备激活记录(管理端)。
func (s *LicenseService) ListActivations(ctx context.Context, licenseID string) ([]*model.Activation, error) {
	l, err := s.resolveLicense(ctx, licenseID)
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
	l, err := s.resolveLicense(ctx, licenseID)
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

// ResetDevices 管理端重置某 License 全部设备(解绑所有激活 + 吊销 token)。
func (s *LicenseService) ResetDevices(ctx context.Context, licenseID, by, ip string) (int, error) {
	l, err := s.resolveLicense(ctx, licenseID)
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

// replayAllowed 重放防护:timestamp 必须落在 ±ReplayWindow 内,nonce 必须未使用。
// 返回 (允许, 错误)。校验失败写 security event。
func (s *LicenseService) replayAllowed(ctx context.Context, timestamp int64, nonce, ip string) (bool, error) {
	now := Now()
	if timestamp == 0 || nonce == "" {
		// 协议要求带 timestamp+nonce;缺失按拒绝处理(记录 tampered_timestamp)
		s.securityLog(ctx, SecTamperedTimestamp, "", "", "", ip, "missing timestamp/nonce")
		return false, ErrReplayDetected
	}
	diff := now - timestamp
	if diff < 0 {
		diff = -diff
	}
	if diff > int64(ReplayWindow.Seconds()) {
		s.securityLog(ctx, SecTamperedTimestamp, "", "", "", ip, "timestamp outside window")
		return false, ErrReplayDetected
	}
	used, err := s.nonces.Use(ctx, sha256Hex(nonce), time.Unix(now, 0))
	if err != nil {
		return false, err
	}
	if !used {
		s.securityLog(ctx, SecReplayDetected, "", "", "", ip, "nonce reused")
		return false, ErrReplayDetected
	}
	return true, nil
}

// versionBlocked 检查产品版本是否在 blocked_versions(JSON 数组)。
func (s *LicenseService) versionBlocked(ctx context.Context, productVersion string) (bool, string) {
	if productVersion == "" {
		return false, ""
	}
	raw, err := s.settings.Get(ctx, "blocked_versions")
	if err != nil || raw == "" || raw == "[]" {
		return false, ""
	}
	var list []string
	if err := jsonUnmarshal(raw, &list); err != nil {
		return false, ""
	}
	for _, v := range list {
		if v == productVersion {
			return true, productVersion
		}
	}
	return false, ""
}

// verifyResult 组装 verify 响应(不带额外字段)。
// invalid 一律不携带 License 信息(不泄露 License 存在性)。
func (s *LicenseService) verifyResult(status string, l *model.License) map[string]any {
	return s.verifyResultWith(l, status)
}

// verifyResultWith 组装 verify 响应:基础字段 + server_time + minimum_client_version + next_verify_after。
func (s *LicenseService) verifyResultWith(l *model.License, status string) map[string]any {
	out := map[string]any{
		"status":      status,
		"valid":       status == "valid",
		"server_time": Now(),
	}
	if l != nil {
		out["license_id"] = l.LicenseID
		out["plan"] = l.Plan
		out["customer"] = l.Customer
		out["expires_at"] = l.ExpiresAt
		out["features"] = l.Features
	}
	if status == "valid" || status == "blocked" {
		out["next_verify_after"] = NextVerifyAfter
	}
	// 版本控制字段(Skill §21):客户端对比 minimum_client_version 判断 UPDATE_REQUIRED
	if s.settings != nil {
		if v, err := s.settings.Get(context.Background(), "minimum_client_version"); err == nil && v != "" {
			out["minimum_client_version"] = v
		}
	}
	return out
}

// newActivationToken 生成激活凭证明文(32 字节随机 hex,碰撞概率可忽略)。
func newActivationToken() (string, error) {
	b := make([]byte, activationTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// sha256Hex SHA-256 hex 摘要。
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// jsonUnmarshal 容错 JSON 解析(避免 import 冲突)。
func jsonUnmarshal(raw string, v any) error {
	return json.Unmarshal([]byte(raw), v)
}

// securityLog 记录安全事件(尽力而为)。
func (s *LicenseService) securityLog(ctx context.Context, eventType, licenseID, activationID, deviceID, ip, details string) {
	if s.security == nil {
		return
	}
	s.security.Log(ctx, &model.SecurityEvent{
		EventType:    eventType,
		LicenseID:    licenseID,
		ActivationID: activationID,
		DeviceID:     deviceID,
		IP:           ip,
		Details:      details,
	})
}
