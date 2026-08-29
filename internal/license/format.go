// Package license 定义 License V2 格式契约(签发端与 Docker_Manager_Go 消费端共享)。
//
// 契约要点(两端必须完全一致,修改必须同步):
//   - Payload 字段名与 JSON 顺序(规范 JSON 序列化,保证签名稳定)
//   - Key 编码: base64url(payload_json) + "." + base64url(ed25519_signature)
//   - Feature Registry(FeatureRegistry 变量)与 Docker_Manager_Go 门控点一一对应
//
// 历史:V1 为 HMAC-SHA256 共享密钥(secret 硬编码在 Docker_Manager_Go 源码,
// 已公开泄露),已于 2026-08 完全移除。V2 改用 Ed25519:签发端持私钥,消费端只持公钥,
// 源码公开也无法伪造。消费端只接受 V2,未知 version 必须拒绝(UNSUPPORTED_LICENSE_VERSION)。
//
// V2.1 演进(向后兼容):Payload 新增可选字段 customer_id / subscription_id(omitempty),
// 存量 V2 Key(无新字段)继续有效;消费端 JSON 解析天然忽略未知/缺失字段。
package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// CurrentVersion 当前 License 格式版本。
// Docker_Manager_Go 遇到高于其支持的版本必须明确拒绝(UNSUPPORTED_LICENSE_VERSION),不得静默接受。
const CurrentVersion = 2

// ProductDMG 目标产品标识(与 Docker_Manager_Go 约定)。
const ProductDMG = "docker-manager-go"

// FeatureRegistry 统一 Feature 定义 —— 与 Docker_Manager_Go 的授权门控点一一对应。
// 严禁两端各自起名(如 advanced-compose vs compose_advanced),否则授权失效。
// 未注册的 feature 一律拒绝签发与验证(防契约漂移)。
var FeatureRegistry = []string{
	"compose",          // Docker Compose 部署(compose.go license.required)
	"container_create", // 容器创建(container.go license.required)
	"appstore",         // 应用商店安装(appstore.go license.required)
}

// PlannedFeatures 规划中的 Feature(文档/后续版本路线图)。
// 注意:未在 FeatureRegistry 注册前不可签发 —— 客户端门控点尚未实现,签了等于"买了不能用"。
var PlannedFeatures = []string{
	"terminal", "backup", "monitor", "multi_node", "api", "automation",
}

// Plan 套餐定义(统一套餐注册表,套餐逻辑不允许散落在业务代码里)。
type Plan struct {
	Name     string   // 套餐标识(写入 payload.plan)
	Features []string // 该套餐包含的 Feature 全集
	Enabled  bool     // 是否可签发(依赖的 feature 未全部落地时置 false 预留)
}

// PlanRegistry 统一套餐注册表。
// free 不需要 License(消费端无 License 时天然成立),不可签发;
// 签发端只签发 Enabled 套餐,EnabledPlanNames 与消费端校验集合保持一致。
var PlanRegistry = []Plan{
	{Name: "free", Features: nil, Enabled: false}, // 免费:无需 License,不可签发
	{Name: "pro", Features: FeatureRegistry, Enabled: true},
	// 以下预留:multi_node/api/audit/sso 等 feature 落地并注册后置 Enabled=true
	{Name: "business", Features: []string{"compose", "container_create", "appstore", "multi_node", "api"}, Enabled: false},
	{Name: "enterprise", Features: []string{"compose", "container_create", "appstore", "multi_node", "api", "audit", "sso"}, Enabled: false},
}

// EnabledPlanNames 当前可签发/可验证的套餐集合(消费端同步校验)。
var EnabledPlanNames = enabledPlanNames()

func enabledPlanNames() []string {
	out := make([]string, 0, len(PlanRegistry))
	for _, p := range PlanRegistry {
		if p.Enabled {
			out = append(out, p.Name)
		}
	}
	return out
}

// Payload License 签名载荷。字段顺序即规范 JSON 顺序,修改字段必须同步
// Docker_Manager_Go 的消费端解析。
type Payload struct {
	Version        int      `json:"version"`
	KeyID          string   `json:"key_id"`                    // 签发密钥标识(轮换/吊销用),如 "2026-01"
	LicenseID      string   `json:"license_id"`                // 展示用唯一 ID,如 DMG-01JXXXXXXXXXXXX
	Product        string   `json:"product"`                   // docker-manager-go
	Plan           string   `json:"plan"`                      // 必须属于 EnabledPlanNames
	Features       []string `json:"features,omitempty"`        // FeatureRegistry 子集
	Customer       string   `json:"customer"`                  // 客户名(展示用)
	CustomerID     string   `json:"customer_id,omitempty"`     // V2.1 新增:customers.customer_id 关联(可选,向后兼容)
	SubscriptionID string   `json:"subscription_id,omitempty"` // V2.1 新增:subscriptions.subscription_id 关联(可选,向后兼容)
	IssuedAt       int64    `json:"issued_at"`                 // Unix 秒
	ExpiresAt      int64    `json:"expires_at"`                // Unix 秒
	MaxDevices     int      `json:"max_devices"`               // 允许绑定设备数
}

// Validate 校验 Payload 字段合法性(签发前/消费端解析后都应调用)。
func (p *Payload) Validate() error {
	if p.Version != CurrentVersion {
		return fmt.Errorf("unsupported license version: %d (supported: %d)", p.Version, CurrentVersion)
	}
	if p.KeyID == "" {
		return errors.New("key_id is required")
	}
	if p.LicenseID == "" {
		return errors.New("license_id is required")
	}
	if p.Product != ProductDMG {
		return fmt.Errorf("unexpected product: %q", p.Product)
	}
	if !contains(EnabledPlanNames, p.Plan) {
		return fmt.Errorf("unsupported plan: %q", p.Plan)
	}
	if p.IssuedAt <= 0 {
		return errors.New("issued_at is required")
	}
	if p.ExpiresAt <= 0 {
		return errors.New("expires_at is required")
	}
	if p.IssuedAt > p.ExpiresAt {
		return errors.New("issued_at must not be after expires_at")
	}
	if p.MaxDevices < 1 {
		return errors.New("max_devices must be >= 1")
	}
	for _, f := range p.Features {
		if !contains(FeatureRegistry, f) {
			return fmt.Errorf("unknown feature: %q", f)
		}
	}
	return nil
}

// HasFeature 判断是否包含某 Feature(供消费端门控)。
func (p *Payload) HasFeature(name string) bool {
	return contains(p.Features, name)
}

// CanonicalJSON 返回规范 JSON 字节(payload 按 struct 字段序序列化,签名对象)。
func (p *Payload) CanonicalJSON() []byte {
	b, err := json.Marshal(p)
	if err != nil {
		// Payload 字段全部可序列化,理论上不可能失败
		panic(err)
	}
	return b
}

// ParsePayload 解析规范 JSON 到 Payload(消费端与签发端共用)。
func ParsePayload(raw []byte) (*Payload, error) {
	var p Payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("parse payload: %w", err)
	}
	return &p, nil
}

// DecodePayloadOnly 仅解码 Key 字符串中的 payload(不验签,供提取 key_id 等元数据)。
// 解析失败返回 false。
func DecodePayloadOnly(key string) (*Payload, bool) {
	raw, _, err := DecodeKey(key)
	if err != nil {
		return nil, false
	}
	p, err := ParsePayload(raw)
	if err != nil {
		return nil, false
	}
	return p, true
}

// ---------- Key 编码 ----------
//
// Key 字符串格式(与 V1 同样以 "." 分隔两段,便于 Docker_Manager_Go 统一入口分派):
//
//	<base64url(canonical payload json)>.<base64url(ed25519 signature 64 bytes)>
//
// V1 的第二段是 32 位 hex(HMAC 截断),V2 是 88 位 base64url —— 长度不同,消费端可无歧义分派。
// 消费端分派规则:第二段长度 32 → V1(HMAC);其他 → V2(Ed25519)。

// EncodeKey 由 Payload 与私钥生成完整 Key 字符串。
func EncodeKey(p *Payload, priv ed25519.PrivateKey) (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	raw := p.CanonicalJSON()
	sig := ed25519.Sign(priv, raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." +
		base64.RawURLEncoding.EncodeToString(sig), nil
}

// DecodeKey 解码 Key 字符串为 (payloadJSON, signature, err)。
// 只做解码与基础分派,不验签 —— 验签由消费端用公钥完成。
func DecodeKey(key string) ([]byte, []byte, error) {
	key = strings.TrimSpace(key)
	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 {
		return nil, nil, errors.New("invalid license key format")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, fmt.Errorf("decode payload: %w", err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, fmt.Errorf("decode signature: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return nil, nil, fmt.Errorf("invalid signature length: %d", len(sig))
	}
	return raw, sig, nil
}

// VerifyKey 用公钥验证完整 Key:解码 → 校验 Payload 合法性 → 验签。
// 这是消费端(Docker_Manager_Go)离线验证的基础。
func VerifyKey(key string, pub ed25519.PublicKey) (*Payload, bool) {
	raw, sig, err := DecodeKey(key)
	if err != nil {
		return nil, false
	}
	p, err := ParsePayload(raw)
	if err != nil {
		return nil, false
	}
	if err := p.Validate(); err != nil {
		return nil, false
	}
	if !ed25519.Verify(pub, raw, sig) {
		return nil, false
	}
	return p, true
}

// IsExpired 按当前时间判断是否过期(消费端激活时用)。
func (p *Payload) IsExpired(now int64) bool {
	return p.ExpiresAt > 0 && p.ExpiresAt < now
}

// Status 返回当前生命周期状态字符串:active / expired。
func (p *Payload) Status(now int64) string {
	if p.IsExpired(now) {
		return "expired"
	}
	return "active"
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
