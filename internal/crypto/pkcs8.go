package crypto

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

// 基于 x509 + encoding/pem 标准库的 PKCS8/PKIX 编解码。

func marshalPKCS8PrivateKey(priv ed25519.PrivateKey) ([]byte, error) {
	return x509.MarshalPKCS8PrivateKey(priv)
}

func parsePKCS8PrivateKey(der []byte) (any, error) {
	return x509.ParsePKCS8PrivateKey(der)
}

func marshalPKIXPublicKey(pub ed25519.PublicKey) ([]byte, error) {
	return x509.MarshalPKIXPublicKey(pub)
}

// PEMPublicKey 解析 PEM 公钥(供测试/集成端使用)。
func PEMPublicKey(raw string) (ed25519.PublicKey, error) {
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, fmt.Errorf("invalid PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	p, ok := pub.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an Ed25519 public key")
	}
	return p, nil
}
