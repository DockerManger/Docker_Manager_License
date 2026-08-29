// Package auth 管理员认证:argon2id 密码哈希 + HS256 JWT(token_version 机制) + TOTP 2FA。
//
// 安全要点:
//   - 密码一律 argon2id PHC 字符串,禁止 MD5/SHA1/明文
//   - JWT 携带 token_version:修改密码/吊销 token 后自增,旧 token 立即失效
//     (吸取 Docker_Manager_Go 早期"改密码后旧 JWT 继续有效"问题的教训)
//   - 登录接口有 IP 级限流(见 ratelimit.go)
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/argon2"
)

// ---------- 密码 (argon2id) ----------

// HashPassword 生成 argon2id PHC 字符串(参数与 Docker_Manager_Go 一致,便于迁移)。
func HashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(pw), salt, 2, 19456, 1, 32)
	return fmt.Sprintf("$argon2id$v=19$m=19456,t=2,p=1$%s$%s",
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash)), nil
}

// VerifyPassword 校验 argon2id PHC 字符串。
func VerifyPassword(pw, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	params := strings.Split(parts[3], ",") // ["m=19456","t=2","p=1"]
	mem, _ := strconv.Atoi(strings.TrimPrefix(params[0], "m="))
	timev, _ := strconv.Atoi(strings.TrimPrefix(params[1], "t="))
	threads, _ := strconv.Atoi(strings.TrimPrefix(params[2], "p="))
	got := argon2.IDKey([]byte(pw), salt, uint32(timev), uint32(mem), uint8(threads), uint32(len(want)))
	return hmac.Equal(got, want)
}

// ---------- JWT (HS256,携带 token_version) ----------

// Claims JWT 载荷。
type Claims struct {
	Username     string `json:"username"`
	TokenVersion int    `json:"tv"` // 与 admins.token_version 比对,落后即拒绝
	IssuedAt     int64  `json:"iat"`
	ExpiresAt    int64  `json:"exp"`
}

// MakeToken 签发 JWT(HS256,手写实现,无外部依赖)。
func MakeToken(secret, username string, tokenVersion int, ttl time.Duration) (string, error) {
	now := time.Now().Unix()
	claims, err := json.Marshal(Claims{
		Username:     username,
		TokenVersion: tokenVersion,
		IssuedAt:     now,
		ExpiresAt:    now + int64(ttl.Seconds()),
	})
	if err != nil {
		return "", err
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString(claims)
	sig := hmacSHA256(secret, header+"."+payload)
	return header + "." + payload + "." + sig, nil
}

// VerifyToken 校验 JWT,返回 Claims。失败原因区分:签名/过期/格式。
func VerifyToken(secret, token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}
	want := hmacSHA256(secret, parts[0]+"."+parts[1])
	if !hmac.Equal([]byte(want), []byte(parts[2])) {
		return nil, errors.New("invalid signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("invalid payload encoding")
	}
	var c Claims
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, errors.New("invalid payload")
	}
	if c.ExpiresAt < time.Now().Unix() {
		return nil, errors.New("token expired")
	}
	return &c, nil
}

func hmacSHA256(secret, data string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// ---------- TOTP 2FA ----------

// GenerateTOTPSecret 生成新 TOTP 密钥(返回 secret 与 otpauth URL)。
func GenerateTOTPSecret(issuer, account string) (string, string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: account,
		Period:      30,
		Digits:      6,
		SecretSize:  20,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// VerifyTOTP 校验 6 位动态码。
func VerifyTOTP(secret, code string) bool {
	if secret == "" || code == "" {
		return false
	}
	return totp.Validate(code, secret)
}
