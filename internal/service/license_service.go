package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/DockOrae/DockOrae-Auth/internal/crypto"
	"github.com/DockOrae/DockOrae-Auth/internal/events"
	"github.com/DockOrae/DockOrae-Auth/internal/license"
	"github.com/DockOrae/DockOrae-Auth/internal/model"
)

// uuidRe 标准 UUID 格式(8-4-4-4-12 十六进制),用于区分数据库 UUID 与其他 ID 字符串。
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// LicenseService License 业务核心:签发 / 查询 / 延期 / 吊销 / 在线激活验证。
// 签名只在这里发生(依赖 Ed25519 私钥),API 层只做参数传递。
//
// V3 Event-Driven:状态变更在 repo 层同事务持久化事件(Event Store),
// 提交后由 PublishEvents 扇出到 SSE 订阅者(Broker)。
type LicenseService struct {
	repo           *LicenseRepo
	activationRepo *ActivationRepo
	signingKeys    *SigningKeyRepo
	audit          *AuditRepo
	tokens         *ActivationTokenRepo
	security       *SecurityEventRepo
	nonces         *NonceRepo
	settings       *ServerSettingsRepo
	customers      *CustomerRepo
	subscriptions  *SubscriptionRepo
	eventRepo      *EventRepo
	broker         *events.Broker
	keyPair        *crypto.KeyPair
	keyID          string
}

// NewLicenseService 构造。
func NewLicenseService(repo *LicenseRepo, activationRepo *ActivationRepo, signingKeys *SigningKeyRepo,
	audit *AuditRepo, tokens *ActivationTokenRepo, security *SecurityEventRepo,
	nonces *NonceRepo, settings *ServerSettingsRepo, customers *CustomerRepo,
	subscriptions *SubscriptionRepo, eventRepo *EventRepo, broker *events.Broker,
	keyPair *crypto.KeyPair, keyID string) *LicenseService {
	return &LicenseService{
		repo: repo, activationRepo: activationRepo, signingKeys: signingKeys,
		audit: audit, tokens: tokens, security: security, nonces: nonces,
		settings: settings, customers: customers, subscriptions: subscriptions,
		eventRepo: eventRepo, broker: broker,
		keyPair: keyPair, keyID: keyID,
	}
}

// PublishEvents 提交后扇出已持久化事件到 SSE 订阅者(尽力而为,不阻塞主流程)。
func (s *LicenseService) PublishEvents(events []*model.LicenseEvent) {
	if s.broker == nil || len(events) == 0 {
		return
	}
	for _, ev := range events {
		s.broker.Publish(ev)
	}
}

// PublishGlobal 持久化并广播一条全局事件(activation_id=”,所有订阅者收到)。
// 用于 version_policy.changed 等不针对单个激活的事件。
func (s *LicenseService) PublishGlobal(ctx context.Context, evType string, payload map[string]any) error {
	ev := newEvent(evType, "", "", "", 0, payload)
	// 全局事件:直接持久化(不在业务事务内;业务已先提交)
	tx, err := s.eventRepo.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	ev, err = s.eventRepo.InsertTx(ctx, tx, ev)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.broker.Publish(ev)
	return nil
}

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

// IssueRequest 签发请求(API 层解析后传入)。
type IssueRequest struct {
	Customer       string
	CustomerID     string // V3:CUS-*(可选,关联 customers 表)
	SubscriptionID string // V3:SUB-*(可选,关联 subscriptions 表)
	Plan           string
	Features       []string
	ExpiresAt      int64 // Unix 秒
	MaxDevices     int
	Notes          string
	CreatedBy      string
	IP             string
}

// IssueResult 签发结果。
type IssueResult struct {
	License *model.License
	Key     string // 完整 Key 字符串(仅签发/导出时返回,列表不返回)
	Payload string
}

// Issue 签发流程:
// 校验请求 → 生成 License ID → 构建 Payload → Ed25519 签名 → 保存主记录+修订 → 审计 → 返回 Key。
func (s *LicenseService) Issue(ctx context.Context, req IssueRequest) (*IssueResult, error) {
	if req.Customer == "" {
		return nil, errors.New("customer is required")
	}
	if req.Plan == "" {
		req.Plan = "pro"
	}
	if req.ExpiresAt <= Now() {
		return nil, errors.New("expires_at must be in the future")
	}
	if req.MaxDevices < 1 {
		req.MaxDevices = 1
	}
	// 校验 features 与 plan 合法性(plan 必须属于注册表且 Enabled)
	if !containsStr(license.EnabledPlanNames, req.Plan) {
		return nil, fmt.Errorf("unsupported plan: %q", req.Plan)
	}
	for _, f := range req.Features {
		if !containsStr(license.FeatureRegistry, f) {
			return nil, fmt.Errorf("unknown feature: %q", f)
		}
	}

	// V3:客户/订阅关联(可选)。指定时必须存在,写 licenses 外键与 payload。
	var customerRef, subscriptionRef string
	if req.CustomerID != "" {
		c, err := s.customers.GetByCustomerID(ctx, req.CustomerID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, fmt.Errorf("customer not found: %s", req.CustomerID)
			}
			return nil, err
		}
		customerRef = c.ID
	}
	if req.SubscriptionID != "" {
		ref, err := s.subscriptions.GetDBIDBySubscriptionID(ctx, req.SubscriptionID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, fmt.Errorf("subscription not found: %s", req.SubscriptionID)
			}
			return nil, err
		}
		subscriptionRef = ref
	}

	now := Now()
	payload := &license.Payload{
		Version:        license.CurrentVersion,
		KeyID:          s.keyID,
		LicenseID:      license.NewLicenseID(),
		Product:        license.ProductDMG,
		Plan:           req.Plan,
		Features:       req.Features,
		Customer:       req.Customer,
		CustomerID:     req.CustomerID,
		SubscriptionID: req.SubscriptionID,
		IssuedAt:       now,
		ExpiresAt:      req.ExpiresAt,
		MaxDevices:     req.MaxDevices,
	}
	key, err := license.EncodeKey(payload, s.keyPair.Private)
	if err != nil {
		return nil, err
	}

	l := &model.License{
		LicenseID:      payload.LicenseID,
		KeyID:          s.keyID,
		Product:        license.ProductDMG,
		Plan:           req.Plan,
		Features:       req.Features,
		Customer:       req.Customer,
		CustomerID:     req.CustomerID,
		SubscriptionID: req.SubscriptionID,
		IssuedAt:       now,
		ExpiresAt:      req.ExpiresAt,
		MaxDevices:     req.MaxDevices,
		Status:         model.StatusActive,
		Notes:          req.Notes,
	}
	if err := s.repo.Create(ctx, l, customerRef, subscriptionRef); err != nil {
		return nil, err
	}
	if err := s.saveRevision(ctx, l, payload, key, "issue", req.CreatedBy); err != nil {
		return nil, err
	}
	if err := s.auditLog(ctx, req.CreatedBy, req.IP, "license.issue", "license", l.LicenseID,
		map[string]any{"plan": l.Plan, "expires_at": l.ExpiresAt}); err != nil {
		return nil, err
	}
	return &IssueResult{License: l, Key: key, Payload: string(payload.CanonicalJSON())}, nil
}

// resolveLicense 按展示 ID(DMG-...)或数据库 UUID 解析许可证。
// 管理 API 的 :id 参数兼容两种标识:列表返回的 id(UUID)与 license_id(DMG-...)都能查。
// 非 UUID 格式的未知 ID 直接返回 ErrNotFound(避免把非法字符串丢给 PG 的 uuid 列报 500)。
func (s *LicenseService) resolveLicense(ctx context.Context, id string) (*model.License, error) {
	l, err := s.repo.GetByLicenseID(ctx, id)
	if err == nil {
		return l, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if !uuidRe.MatchString(id) {
		return nil, ErrNotFound
	}
	return s.repo.GetByDBID(ctx, id)
}

// Get 按展示 ID 查询(附带最新修订 Key,用于导出)。
func (s *LicenseService) Get(ctx context.Context, licenseID string) (*model.License, error) {
	return s.resolveLicense(ctx, licenseID)
}

// List 分页列表。
func (s *LicenseService) List(ctx context.Context, offset, limit int, status string) ([]*model.License, int, error) {
	return s.repo.List(ctx, offset, limit, status)
}

// Extend 延期:+days 天,生成新修订并重新签名,不覆盖历史。
func (s *LicenseService) Extend(ctx context.Context, licenseID string, days int, reason, by, ip string) (*IssueResult, error) {
	if days <= 0 {
		return nil, errors.New("days must be positive")
	}
	l, err := s.resolveLicense(ctx, licenseID)
	if err != nil {
		return nil, err
	}
	if l.Status != model.StatusActive {
		return nil, fmt.Errorf("%w: license status is %s", ErrConflict, l.Status)
	}
	newExp := l.ExpiresAt + int64(days)*86400
	if newExp < Now() {
		return nil, errors.New("extended expiry would already be in the past")
	}

	payload := &license.Payload{
		Version:    license.CurrentVersion,
		KeyID:      s.keyID,
		LicenseID:  l.LicenseID,
		Product:    l.Product,
		Plan:       l.Plan,
		Features:   l.Features,
		Customer:   l.Customer,
		IssuedAt:   Now(),
		ExpiresAt:  newExp,
		MaxDevices: l.MaxDevices,
	}
	key, err := license.EncodeKey(payload, s.keyPair.Private)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateExpiry(ctx, l.ID, newExp); err != nil {
		return nil, err
	}
	if err := s.saveRevision(ctx, l, payload, key, reason, by); err != nil {
		return nil, err
	}
	// V3:延期影响授权状态 → 活跃激活各发一条 license.changed(客户端收到后自动 Verify)
	if events, err := s.activationRepo.LicenseStateChanged(ctx, l.ID, EvtLicenseChanged, map[string]any{
		"status": l.Status, "expires_at": newExp, "plan": l.Plan, "features": l.Features,
	}); err != nil {
		return nil, err
	} else {
		s.PublishEvents(events)
	}
	if err := s.auditLog(ctx, by, ip, "license.extend", "license", l.LicenseID,
		map[string]any{"days": days, "expires_at": newExp}); err != nil {
		return nil, err
	}
	l.ExpiresAt = newExp
	return &IssueResult{License: l, Key: key, Payload: string(payload.CanonicalJSON())}, nil
}

// Revoke 吊销(软删除,保留记录与修订)。revoked_by 记录操作管理员。
// V3 Event-Driven:吊销状态 + 全部激活 revoked + token 吊销 + 事件持久化在同一事务,
// 提交后 Publish 到 SSE —— 客户端无需等待任何周期检查即可实时触达。
func (s *LicenseService) Revoke(ctx context.Context, licenseID, reason, by, ip string) (*model.License, error) {
	l, err := s.resolveLicense(ctx, licenseID)
	if err != nil {
		return nil, err
	}
	if l.Status == model.StatusRevoked {
		return nil, fmt.Errorf("%w: already revoked", ErrConflict)
	}
	now := Now()
	// 同事务:license 状态 + 激活 revoked + token 吊销 + activation.revoked 事件
	_, events, err := s.activationRepo.RevokeLicenseActivations(ctx, l.ID, model.StatusRevoked, reason, by, time.Unix(now, 0))
	if err != nil {
		return nil, err
	}
	// 事务已提交:扇出事件到 SSE(客户端收到后立即 V3 Verify → revoked)
	s.PublishEvents(events)
	if err := s.auditLog(ctx, by, ip, "license.revoke", "license", l.LicenseID,
		map[string]any{"reason": reason}); err != nil {
		return nil, err
	}
	l.Status = model.StatusRevoked
	l.RevokedAt = &now
	l.RevokedReason = reason
	l.RevokedBy = by
	return l, nil
}

// Revisions 修订历史。
func (s *LicenseService) Revisions(ctx context.Context, licenseID string) ([]*model.LicenseRevision, error) {
	l, err := s.resolveLicense(ctx, licenseID)
	if err != nil {
		return nil, err
	}
	return s.repo.Revisions(ctx, l.ID)
}

// Delete 永久删除许可证(生命周期:ACTIVE → 吊销 → REVOKED → 允许删除)。
// 后端强制校验:只有 status == REVOKED 的许可证才允许删除。
// ACTIVE / EXPIRED / UNBOUND 一律拒绝(409)——即使绕过前端隐藏按钮也无法误删。
// 删除级联清理激活/凭据/修订;license_events 与审计日志保留(可追溯)。
func (s *LicenseService) Delete(ctx context.Context, licenseID, by, ip string) error {
	l, err := s.resolveLicense(ctx, licenseID)
	if err != nil {
		return err
	}
	if l.Status != model.StatusRevoked {
		return fmt.Errorf("%w: only revoked licenses can be deleted (current status: %s)", ErrConflict, l.Status)
	}
	if err := s.repo.Delete(ctx, l.ID); err != nil {
		return err
	}
	_ = s.auditLog(ctx, by, ip, "license.delete", "license", l.LicenseID,
		map[string]any{"status_before": l.Status})
	return nil
}

// Unbind 管理员强制解除绑定(License 粒度,解绑该 License 的全部活跃激活)。
// 语义:只删除 Binding,License 保持 ACTIVE,可重新激活 —— 绝不吊销。
// 幂等:无活跃激活时返回 0 且不报错(重复解绑不产生 500)。
// 每个受影响的激活持久化 activation.unbound(source=admin,reason=admin_unbound)事件并 SSE 推送。
// reason 为管理员填写的解除原因(可选,记录到审计)。
func (s *LicenseService) Unbind(ctx context.Context, licenseID, reason, by, ip string) (int, error) {
	l, err := s.resolveLicense(ctx, licenseID)
	if err != nil {
		return 0, err
	}
	n, events, err := s.activationRepo.ResetDevices(ctx, l.ID, time.Unix(Now(), 0))
	if err != nil {
		return 0, err
	}
	s.PublishEvents(events)
	meta := map[string]any{"unbound": n, "source": "admin", "reason": "admin_unbound"}
	if reason != "" {
		meta["note"] = reason
	}
	_ = s.auditLog(ctx, by, ip, "license.admin_unbind", "license", l.LicenseID, meta)
	return int(n), nil
}

// Stats 概览统计。
func (s *LicenseService) Stats(ctx context.Context) (map[string]any, error) {
	return s.repo.Stats(ctx)
}

// PublicKey 返回当前签发公钥(供测试与集成文档输出)。
func (s *LicenseService) PublicKey() []byte {
	return s.keyPair.Public
}

// ValidateActivationToken 校验 SSE 订阅凭据(activation_token + device_id 必须匹配激活)。
// 返回激活记录;凭据无效返回 ok=false(不泄露 License 存在性)。
func (s *LicenseService) ValidateActivationToken(ctx context.Context, token, deviceID string) (*model.Activation, bool) {
	if token == "" {
		return nil, false
	}
	_, act, err := s.tokens.FindByTokenHash(ctx, sha256Hex(token))
	if err != nil {
		return nil, false
	}
	if act.Status != model.ActivationActive {
		return nil, false
	}
	if deviceID != "" && act.DeviceID != deviceID {
		return nil, false
	}
	return act, true
}

// ListLicenseEvents 某 License 的事件历史(管理端审计,时间倒序;limit<=0 用默认)。
func (s *LicenseService) ListLicenseEvents(ctx context.Context, licenseID string, limit int) ([]*model.LicenseEvent, error) {
	l, err := s.resolveLicense(ctx, licenseID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrLicenseNotFound
		}
		return nil, err
	}
	return s.eventRepo.ListForLicense(ctx, l.LicenseID, limit)
}

// ---------- 内部 ----------

func (s *LicenseService) saveRevision(ctx context.Context, l *model.License, payload *license.Payload, key, reason, by string) error {
	revs, err := s.repo.Revisions(ctx, l.ID)
	if err != nil {
		return err
	}
	rev := &model.LicenseRevision{
		LicenseID: l.ID,
		Revision:  len(revs) + 1,
		Payload:   string(payload.CanonicalJSON()),
		Key:       key,
		Reason:    reason,
		CreatedBy: by,
	}
	// 分离签名以便审计可追溯(Key = payload.signature)
	_, sig, err := license.DecodeKey(key)
	if err != nil {
		return err
	}
	rev.Signature = base64.RawURLEncoding.EncodeToString(sig)
	return s.repo.SaveRevision(ctx, rev)
}

func (s *LicenseService) auditLog(ctx context.Context, admin, ip, action, resType, resID string, meta map[string]any) error {
	raw := ""
	if meta != nil {
		if b, err := json.Marshal(meta); err == nil {
			raw = string(b)
		}
	}
	return s.audit.Log(ctx, &model.AuditLog{
		Admin:        admin,
		Action:       action,
		ResourceType: resType,
		ResourceID:   resID,
		IP:           ip,
		Metadata:     raw,
	})
}
