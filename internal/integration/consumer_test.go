// Package integration 跨端集成验证 —— 本项目最重要的测试。
//
// 场景:模拟 Docker_Manager_Go 消费端的行为:
//
//	签发端(Docker_Manager_License)用私钥签发 Key
//	→ 消费端(模拟 Docker_Manager_Go)只持公钥,离线本地验证
//	→ 真伪判断、过期判断、feature 判断、篡改拒绝
//
// 消费端验证代码刻意写成独立于签发端实现的方式(只 import 公钥 + 契约格式),
// 以发现两端契约分叉。Docker_Manager_Go 改造完成后,此测试的验证逻辑
// 应与 Docker_Manager_Go 内部实现的验证逻辑完全一致。
package integration

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/MinimaxFlora/Docker_Manager_License/internal/crypto"
	"github.com/MinimaxFlora/Docker_Manager_License/internal/license"
)

func ed25519Verify(pub, msg, sig []byte) bool {
	return ed25519.Verify(ed25519.PublicKey(pub), msg, sig)
}

// consumerPublicKey 模拟"Docker_Manager_Go 内嵌的公钥"(生产上由集成文档提供)。
// 测试用临时密钥对生成。
var consumerPublicKey = func() func() []byte {
	var kp *crypto.KeyPair
	return func() []byte {
		if kp == nil {
			kp, _ = crypto.GenerateKeyPair()
		}
		return kp.Public
	}
}()

// consumerVerify 模拟 Docker_Manager_Go 消费端离线验证(独立实现,不用签发端 VerifyKey):
// 解码 → 解析 payload → 校验契约 → 公钥验签 → 过期检查。
// Docker_Manager_Go 改造时必须保持此逻辑一致(或直接复用 internal/license.VerifyKey)。
func consumerVerify(key string, pub []byte, now int64) (payload map[string]any, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(key), ".", 2)
	if len(parts) != 2 {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	var p map[string]any
	if json.Unmarshal(raw, &p) != nil {
		return nil, false
	}
	// 契约校验:版本必须支持
	if v, _ := p["version"].(float64); int(v) != license.CurrentVersion {
		return nil, false
	}
	// 验签
	if !ed25519Verify(pub, raw, sig) {
		return nil, false
	}
	// 过期
	if exp, _ := p["expires_at"].(float64); int64(exp) < now {
		p["status"] = "expired"
	} else {
		p["status"] = "active"
	}
	return p, true
}

// TestConsumerVerifiesIssuedKey 核心兼容性测试:
// 签发端生成 → 消费端(Docker_Manager_Go 模拟)离线验证通过。
func TestConsumerVerifiesIssuedKey(t *testing.T) {
	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	payload := &license.Payload{
		Version:    license.CurrentVersion,
		KeyID:      "2026-01",
		LicenseID:  license.NewLicenseID(),
		Product:    license.ProductDMG,
		Plan:       "pro",
		Features:   []string{"compose", "container_create"},
		Customer:   "Zhao",
		IssuedAt:   time.Now().Unix(),
		ExpiresAt:  time.Now().Unix() + 86400*365,
		MaxDevices: 3,
	}
	key, err := license.EncodeKey(payload, kp.Private)
	if err != nil {
		t.Fatal(err)
	}

	// 消费端只有公钥,离线验证
	info, ok := consumerVerify(key, kp.Public, time.Now().Unix())
	if !ok {
		t.Fatal("consumer must accept a legitimately issued key")
	}
	if info["status"] != "active" || info["license_id"] != payload.LicenseID {
		t.Fatalf("consumer sees wrong info: %+v", info)
	}
	// feature 判断(消费端门控)
	if !hasFeature(info, "compose") || !hasFeature(info, "container_create") {
		t.Fatalf("features must be honored: %+v", info["features"])
	}
}

// TestConsumerRejectsTamperedExpiry 篡改有效期 → 消费端拒绝。
func TestConsumerRejectsTamperedExpiry(t *testing.T) {
	kp, _ := crypto.GenerateKeyPair()
	payload := &license.Payload{
		Version:    license.CurrentVersion,
		KeyID:      "2026-01",
		LicenseID:  license.NewLicenseID(),
		Product:    license.ProductDMG,
		Plan:       "pro",
		Features:   []string{"compose"},
		Customer:   "Zhao",
		IssuedAt:   time.Now().Unix(),
		ExpiresAt:  time.Now().Unix() + 365*86400,
		MaxDevices: 1,
	}
	key, _ := license.EncodeKey(payload, kp.Private)
	raw, sig, _ := license.DecodeKey(key)

	// 篡改 expires_at → +10 年
	var tampered map[string]any
	_ = json.Unmarshal(raw, &tampered)
	tampered["expires_at"] = int64(tampered["expires_at"].(float64)) + 10*365*86400
	tamperedRaw, _ := json.Marshal(tampered)
	evilKey := base64.RawURLEncoding.EncodeToString(tamperedRaw) + "." +
		base64.RawURLEncoding.EncodeToString(sig)

	if _, ok := consumerVerify(evilKey, kp.Public, time.Now().Unix()); ok {
		t.Fatal("consumer must reject tampered expiry")
	}
}

// TestConsumerRejectsExpiredKey 过期 Key → 消费端 status=expired。
func TestConsumerRejectsExpiredKey(t *testing.T) {
	kp, _ := crypto.GenerateKeyPair()
	payload := &license.Payload{
		Version:    license.CurrentVersion,
		KeyID:      "2026-01",
		LicenseID:  license.NewLicenseID(),
		Product:    license.ProductDMG,
		Plan:       "pro",
		Features:   []string{"compose"},
		Customer:   "Zhao",
		IssuedAt:   time.Now().Unix() - 1000,
		ExpiresAt:  time.Now().Unix() - 1, // 已过期
		MaxDevices: 1,
	}
	key, _ := license.EncodeKey(payload, kp.Private)
	info, ok := consumerVerify(key, kp.Public, time.Now().Unix())
	if !ok {
		t.Fatal("expired but validly signed key must still parse")
	}
	if info["status"] != "expired" {
		t.Fatalf("consumer must see expired status, got %v", info["status"])
	}
}

// TestConsumerRejectsUnknownVersion 未来版本 → 消费端明确拒绝(不静默接受)。
func TestConsumerRejectsUnknownVersion(t *testing.T) {
	kp, _ := crypto.GenerateKeyPair()
	payload := &license.Payload{
		Version:    99, // 未来版本
		KeyID:      "2026-01",
		LicenseID:  license.NewLicenseID(),
		Product:    license.ProductDMG,
		Plan:       "pro",
		Features:   []string{"compose"},
		Customer:   "Zhao",
		IssuedAt:   time.Now().Unix(),
		ExpiresAt:  time.Now().Unix() + 86400,
		MaxDevices: 1,
	}
	key, err := license.EncodeKey(payload, kp.Private)
	if err == nil {
		// 签发端已拒绝;若签发端放行,消费端也必须拒绝
		if _, ok := consumerVerify(key, kp.Public, time.Now().Unix()); ok {
			t.Fatal("consumer must reject unsupported license version")
		}
	}
}

// TestConsumerPublicKeyIsEnough 攻击者持有全部源码+公钥,也无法伪造合法 Key。
func TestConsumerPublicKeyIsEnough(t *testing.T) {
	kp, _ := crypto.GenerateKeyPair()
	// 攻击者只有公钥(源码公开),试图自签 → 伪造的签名必须被拒绝
	fake := &license.Payload{
		Version:    license.CurrentVersion,
		KeyID:      "2026-01",
		LicenseID:  license.NewLicenseID(),
		Product:    license.ProductDMG,
		Plan:       "pro",
		Features:   []string{"compose", "container_create", "appstore"},
		Customer:   "attacker",
		IssuedAt:   time.Now().Unix(),
		ExpiresAt:  time.Now().Unix() + 100*365*86400,
		MaxDevices: 999,
	}
	// 用攻击者自己生成的密钥对签名(他无法拿到签发端私钥)
	attackerKP, _ := crypto.GenerateKeyPair()
	fakeKey, _ := license.EncodeKey(fake, attackerKP.Private)
	if _, ok := consumerVerify(fakeKey, kp.Public, time.Now().Unix()); ok {
		t.Fatal("attacker-forged key must be rejected by consumer")
	}
}

// TestContractFieldsAligned 契约字段对齐:签发端 FeatureRegistry 与消费端预期一致。
func TestContractFieldsAligned(t *testing.T) {
	// 消费端(Docker_Manager_Go)预期 feature 集合 —— 与 internal/license.FeatureRegistry 完全一致
	expected := []string{"compose", "container_create", "appstore"}
	got := license.FeatureRegistry
	if len(got) != len(expected) {
		t.Fatalf("feature registry mismatch: %v vs %v", got, expected)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("feature registry mismatch at %d: %v vs %v", i, got, expected)
		}
	}
}

func hasFeature(p map[string]any, name string) bool {
	feats, _ := p["features"].([]any)
	for _, f := range feats {
		if f == name {
			return true
		}
	}
	return false
}
