package license

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/MinimaxFlora/Docker_Manager_License/internal/crypto"
)

func testKeyPair(t *testing.T) *crypto.KeyPair {
	t.Helper()
	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair: %v", err)
	}
	return kp
}

func testPayload() *Payload {
	return &Payload{
		Version:    CurrentVersion,
		KeyID:      "2026-01",
		LicenseID:  "DMG-01JTEST000000000000000000",
		Product:    ProductDMG,
		Plan:       "pro",
		Features:   []string{"compose", "container_create"},
		Customer:   "Zhao",
		IssuedAt:   time.Now().Unix() - 100,
		ExpiresAt:  time.Now().Unix() + 86400*365,
		MaxDevices: 3,
	}
}

// TestEncodeVerifyRoundtrip 签名 → 验证 → 字段一致。
func TestEncodeVerifyRoundtrip(t *testing.T) {
	kp := testKeyPair(t)
	p := testPayload()
	key, err := EncodeKey(p, kp.Private)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !strings.Contains(key, ".") {
		t.Fatal("key must contain separator")
	}
	got, ok := VerifyKey(key, kp.Public)
	if !ok {
		t.Fatal("verify should pass")
	}
	if got.LicenseID != p.LicenseID || got.Plan != p.Plan || got.Customer != p.Customer {
		t.Fatalf("roundtrip mismatch: %+v vs %+v", got, p)
	}
	if got.ExpiresAt != p.ExpiresAt || got.MaxDevices != p.MaxDevices {
		t.Fatalf("roundtrip expiry/devices mismatch")
	}
	if len(got.Features) != 2 || !got.HasFeature("compose") || !got.HasFeature("container_create") {
		t.Fatalf("features mismatch: %v", got.Features)
	}
}

// TestVerifyTamperedPayload 篡改 payload(改有效期/features/max_devices)→ 必须失败。
func TestVerifyTamperedPayload(t *testing.T) {
	kp := testKeyPair(t)
	p := testPayload()
	key, err := EncodeKey(p, kp.Private)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	raw, sig, err := DecodeKey(key)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	cases := map[string]func(*Payload){
		"extend expiry": func(x *Payload) { x.ExpiresAt += 86400 * 3650 },
		"add feature":   func(x *Payload) { x.Features = append(x.Features, "appstore") },
		"bump devices":  func(x *Payload) { x.MaxDevices = 99 },
		"change plan":   func(x *Payload) { x.Plan = "enterprise" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			var tampered Payload
			if err := json.Unmarshal(raw, &tampered); err != nil {
				t.Fatal(err)
			}
			mutate(&tampered)
			tamperedJSON, _ := json.Marshal(tampered)
			evilKey := b64urlNoPad(tamperedJSON) + "." + b64urlNoPad(sig)
			if _, ok := VerifyKey(evilKey, kp.Public); ok {
				t.Fatalf("tampered %q must fail verification", name)
			}
		})
	}
}

// TestVerifyTamperedSignature 篡改签名 → 必须失败。
func TestVerifyTamperedSignature(t *testing.T) {
	kp := testKeyPair(t)
	p := testPayload()
	key, err := EncodeKey(p, kp.Private)
	if err != nil {
		t.Fatal(err)
	}
	raw, sig, _ := DecodeKey(key)
	sig[0] ^= 0xFF // 翻转第一个字节
	evilKey := b64urlNoPad(raw) + "." + b64urlNoPad(sig)
	if _, ok := VerifyKey(evilKey, kp.Public); ok {
		t.Fatal("tampered signature must fail")
	}
}

// TestVerifyWrongPublicKey 错误公钥 → 必须失败。
func TestVerifyWrongPublicKey(t *testing.T) {
	kp := testKeyPair(t)
	other := testKeyPair(t)
	p := testPayload()
	key, err := EncodeKey(p, kp.Private)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := VerifyKey(key, other.Public); ok {
		t.Fatal("wrong public key must fail")
	}
}

// TestRejectUnsupportedVersion 未来版本(version=99)→ 必须明确拒绝。
func TestRejectUnsupportedVersion(t *testing.T) {
	kp := testKeyPair(t)
	p := testPayload()
	p.Version = 99
	if _, err := EncodeKey(p, kp.Private); err == nil {
		t.Fatal("version 99 must be rejected at encode")
	} else if !strings.Contains(err.Error(), "unsupported license version") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestValidateUnknownFeature 未知 feature → 拒绝(防止两端 feature 名分叉)。
func TestValidateUnknownFeature(t *testing.T) {
	p := testPayload()
	p.Features = []string{"advanced-compose"} // 与 Docker_Manager_Go 命名不一致
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "unknown feature") {
		t.Fatalf("unknown feature must be rejected, got %v", err)
	}
}

// TestStatusExpiry 过期状态判断。
func TestStatusExpiry(t *testing.T) {
	p := testPayload()
	now := time.Now().Unix()
	if p.Status(now) != "active" {
		t.Fatal("should be active")
	}
	p.ExpiresAt = now - 1
	if p.Status(now) != "expired" {
		t.Fatal("should be expired")
	}
	if !p.IsExpired(now) {
		t.Fatal("IsExpired should be true")
	}
}

// TestLicenseIDFormat 生成的 ID 格式:DMG- + 26 字符 ULID。
func TestLicenseIDFormat(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := NewLicenseID()
		if !strings.HasPrefix(id, "DMG-") || len(id) != 4+ulidLen {
			t.Fatalf("bad license id: %q", id)
		}
		for _, ch := range id[4:] {
			if !strings.ContainsRune(ulidAlphabet, ch) {
				t.Fatalf("bad ulid char %q in %q", ch, id)
			}
		}
		if seen[id] {
			t.Fatalf("duplicate id: %s", id)
		}
		seen[id] = true
	}
}

// TestCanonicalJSONStable 规范 JSON 序列化顺序稳定(签名可靠的前提)。
func TestCanonicalJSONStable(t *testing.T) {
	p := testPayload()
	a := string(p.CanonicalJSON())
	b := string(p.CanonicalJSON())
	if a != b {
		t.Fatal("canonical JSON must be stable")
	}
	// 关键字段顺序(与 Docker_Manager_Go 消费端解析一致)
	if !strings.Contains(a, `"version":`) || !strings.Contains(a, `"key_id":`) ||
		!strings.Contains(a, `"license_id":`) {
		t.Fatalf("unexpected payload shape: %s", a)
	}
}

func b64urlNoPad(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}
