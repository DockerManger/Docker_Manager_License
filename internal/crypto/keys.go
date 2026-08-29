// Package crypto Ed25519 密钥管理(签发端)。
//
// 安全原则:
//   - 私钥只存在于 filesystem(0600),绝不进入代码/Git/镜像/Dockerfile
//   - 私钥绝不通过 API 返回(即使管理员登录)
//   - key_id 随每次签发写入 License payload,支持未来密钥轮换
package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// KeyPair 一对 Ed25519 密钥。
type KeyPair struct {
	Private ed25519.PrivateKey
	Public  ed25519.PublicKey
}

// GenerateKeyPair 生成新密钥对(测试/初始化用)。
func GenerateKeyPair() (*KeyPair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519 key: %w", err)
	}
	return &KeyPair{Private: priv, Public: pub}, nil
}

// PEM 编码私钥(PKCS8,与 Go 标准库互操作)。
func marshalPrivateKey(priv ed25519.PrivateKey) []byte {
	der, _ := marshalPKCS8PrivateKey(priv)
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

// SavePrivateKey 写私钥到文件,权限强制 0600。
func SavePrivateKey(path string, priv ed25519.PrivateKey) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, marshalPrivateKey(priv), 0o600); err != nil {
		return fmt.Errorf("write key: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return fmt.Errorf("chmod key: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename key: %w", err)
	}
	return nil
}

// LoadPrivateKey 从文件加载私钥。文件不存在返回 ErrKeyNotFound(调用方决定是否初始化)。
func LoadPrivateKey(path string) (*KeyPair, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrKeyNotFound
		}
		return nil, fmt.Errorf("read key %s: %w", path, err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("invalid PEM in key file")
	}
	key, err := parsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	priv, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("key file is not an Ed25519 private key")
	}
	pub := priv.Public().(ed25519.PublicKey)
	return &KeyPair{Private: priv, Public: pub}, nil
}

// ErrKeyNotFound 私钥文件不存在。
var ErrKeyNotFound = errors.New("private key not found")

// PublicKeyPEM 输出公钥 PEM(供 Docker_Manager_Go 集成时嵌入)。
func PublicKeyPEM(pub ed25519.PublicKey) string {
	der, _ := marshalPKIXPublicKey(pub)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}
