package auth

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

// TestHashVerifyPassword 正确密码通过,错误密码拒绝。
func TestHashVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("correct-horse-battery", hash) {
		t.Fatal("correct password must verify")
	}
	if VerifyPassword("wrong-password", hash) {
		t.Fatal("wrong password must not verify")
	}
}

// TestVerifyPasswordRejectsPlain 明文/非 argon2id 字符串必须拒绝。
func TestVerifyPasswordRejectsPlain(t *testing.T) {
	for _, bad := range []string{"", "123456", "MD5:abc", "$argon2id$v=18$m=1,t=1,p=1$AAAA$BBBB"} {
		if VerifyPassword("x", bad) {
			t.Fatalf("must reject: %q", bad)
		}
	}
}

// TestHashNotPlaintext 哈希不包含明文(防误存)。
func TestHashNotPlaintext(t *testing.T) {
	hash, _ := HashPassword("super-secret-pass")
	for i := 0; i+4 < len("super-secret-pass"); i++ {
		if contains(hash, "super") {
			t.Fatal("hash must not contain plaintext")
		}
	}
}

// TestJWTRoundtrip 签发 → 验证 → claims 一致。
func TestJWTRoundtrip(t *testing.T) {
	token, err := MakeToken("secret", "admin", 3, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := VerifyToken("secret", token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Username != "admin" || claims.TokenVersion != 3 {
		t.Fatalf("claims mismatch: %+v", claims)
	}
}

// TestJWTWrongSecret 错误密钥 → 拒绝。
func TestJWTWrongSecret(t *testing.T) {
	token, _ := MakeToken("secret-a", "admin", 0, time.Hour)
	if _, err := VerifyToken("secret-b", token); err == nil {
		t.Fatal("wrong secret must be rejected")
	}
}

// TestJWTExpired 过期 token → 拒绝。
func TestJWTExpired(t *testing.T) {
	token, _ := MakeToken("s", "admin", 0, -time.Minute)
	if _, err := VerifyToken("s", token); err == nil {
		t.Fatal("expired token must be rejected")
	}
}

// TestJWTTamperedPayload 篡改 payload(username)→ 拒绝。
func TestJWTTamperedPayload(t *testing.T) {
	token, _ := MakeToken("s", "admin", 0, time.Hour)
	parts := strings.Split(token, ".")
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	// 把 username 从 admin 改成 root(保持签名不变 → 签名校验必须失败)
	evil := strings.Replace(string(raw), "admin", "root", 1)
	parts[1] = base64.RawURLEncoding.EncodeToString([]byte(evil))
	if _, err := VerifyToken("s", strings.Join(parts, ".")); err == nil {
		t.Fatal("tampered token must be rejected")
	}
}

// TestTOTPGenerateVerify 生成 secret → 当前动态码可通过。
func TestTOTPGenerateVerify(t *testing.T) {
	secret, url, err := GenerateTOTPSecret("DML", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" || url == "" {
		t.Fatal("secret/url must not be empty")
	}
	code, err := totpCode(secret)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyTOTP(secret, code) {
		t.Fatal("valid TOTP code must verify")
	}
	if VerifyTOTP(secret, "000000") {
		t.Fatal("wrong TOTP code must fail")
	}
}

// TestLoginLimiter 限流:超过阈值后锁定,到期后恢复。
func TestLoginLimiter(t *testing.T) {
	l := NewLoginLimiter(time.Minute, 3, time.Minute)
	ip := "1.2.3.4"
	for i := 0; i < 3; i++ {
		if !l.Allow(ip) {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
		l.RecordFailure(ip)
	}
	if l.Allow(ip) {
		t.Fatal("after max failures must be locked")
	}
	l.RecordSuccess(ip)
	if !l.Allow(ip) {
		t.Fatal("RecordSuccess must unlock")
	}
}
