package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/MinimaxFlora/Docker_Manager_License/internal/crypto"
	"github.com/MinimaxFlora/Docker_Manager_License/internal/license"
	"github.com/MinimaxFlora/Docker_Manager_License/internal/model"
)

// LicenseService License 业务核心:签发 / 查询 / 延期 / 吊销 / 在线激活验证。
// 签名只在这里发生(依赖 Ed25519 私钥),API 层只做参数传递。
type LicenseService struct {
	repo           *LicenseRepo
	activationRepo *ActivationRepo
	signingKeys    *SigningKeyRepo
	audit          *AuditRepo
	keyPair        *crypto.KeyPair
	keyID          string
}

// NewLicenseService 构造。
func NewLicenseService(repo *LicenseRepo, activationRepo *ActivationRepo, signingKeys *SigningKeyRepo, audit *AuditRepo, keyPair *crypto.KeyPair, keyID string) *LicenseService {
	return &LicenseService{
		repo: repo, activationRepo: activationRepo, signingKeys: signingKeys,
		audit: audit, keyPair: keyPair, keyID: keyID,
	}
}

// IssueRequest 签发请求(API 层解析后传入)。
type IssueRequest struct {
	Customer   string
	Plan       string
	Features   []string
	ExpiresAt  int64 // Unix 秒
	MaxDevices int
	Notes      string
	CreatedBy  string
	IP         string
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
	// 校验 features 与 plan 合法性
	if !containsStr(license.Plans, req.Plan) {
		return nil, fmt.Errorf("unsupported plan: %q", req.Plan)
	}
	for _, f := range req.Features {
		if !containsStr(license.FeatureRegistry, f) {
			return nil, fmt.Errorf("unknown feature: %q", f)
		}
	}

	now := Now()
	payload := &license.Payload{
		Version:    license.CurrentVersion,
		KeyID:      s.keyID,
		LicenseID:  license.NewLicenseID(),
		Product:    license.ProductDMG,
		Plan:       req.Plan,
		Features:   req.Features,
		Customer:   req.Customer,
		IssuedAt:   now,
		ExpiresAt:  req.ExpiresAt,
		MaxDevices: req.MaxDevices,
	}
	key, err := license.EncodeKey(payload, s.keyPair.Private)
	if err != nil {
		return nil, err
	}

	l := &model.License{
		LicenseID:  payload.LicenseID,
		KeyID:      s.keyID,
		Product:    license.ProductDMG,
		Plan:       req.Plan,
		Features:   req.Features,
		Customer:   req.Customer,
		IssuedAt:   now,
		ExpiresAt:  req.ExpiresAt,
		MaxDevices: req.MaxDevices,
		Status:     model.StatusActive,
		Notes:      req.Notes,
	}
	if err := s.repo.Create(ctx, l); err != nil {
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

// Get 按展示 ID 查询(附带最新修订 Key,用于导出)。
func (s *LicenseService) Get(ctx context.Context, licenseID string) (*model.License, error) {
	return s.repo.GetByLicenseID(ctx, licenseID)
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
	l, err := s.repo.GetByLicenseID(ctx, licenseID)
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
	if err := s.auditLog(ctx, by, ip, "license.extend", "license", l.LicenseID,
		map[string]any{"days": days, "expires_at": newExp}); err != nil {
		return nil, err
	}
	l.ExpiresAt = newExp
	return &IssueResult{License: l, Key: key, Payload: string(payload.CanonicalJSON())}, nil
}

// Revoke 吊销(软删除,保留记录与修订)。
func (s *LicenseService) Revoke(ctx context.Context, licenseID, reason, by, ip string) (*model.License, error) {
	l, err := s.repo.GetByLicenseID(ctx, licenseID)
	if err != nil {
		return nil, err
	}
	if l.Status == model.StatusRevoked {
		return nil, fmt.Errorf("%w: already revoked", ErrConflict)
	}
	now := Now()
	if err := s.repo.UpdateStatus(ctx, l.ID, model.StatusRevoked, &now, reason); err != nil {
		return nil, err
	}
	if err := s.auditLog(ctx, by, ip, "license.revoke", "license", l.LicenseID,
		map[string]any{"reason": reason}); err != nil {
		return nil, err
	}
	l.Status = model.StatusRevoked
	l.RevokedAt = &now
	l.RevokedReason = reason
	return l, nil
}

// Revisions 修订历史。
func (s *LicenseService) Revisions(ctx context.Context, licenseID string) ([]*model.LicenseRevision, error) {
	l, err := s.repo.GetByLicenseID(ctx, licenseID)
	if err != nil {
		return nil, err
	}
	return s.repo.Revisions(ctx, l.ID)
}

// Stats 概览统计。
func (s *LicenseService) Stats(ctx context.Context) (map[string]any, error) {
	return s.repo.Stats(ctx)
}

// PublicKey 返回当前签发公钥(供测试与集成文档输出)。
func (s *LicenseService) PublicKey() []byte {
	return s.keyPair.Public
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
