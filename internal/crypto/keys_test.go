package crypto

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestGenerateSaveLoadRoundtrip 生成 → 保存 → 加载,公钥一致。
func TestGenerateSaveLoadRoundtrip(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "license.key")
	if err := SavePrivateKey(path, kp.Private); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows 无 POSIX 权限位(只有只读位),0600 检查仅在类 Unix 生效
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("private key file must be 0600, got %v", info.Mode().Perm())
	}
	loaded, err := LoadPrivateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(loaded.Public, kp.Public) {
		t.Fatal("loaded public key mismatch")
	}
}

// TestLoadMissingKey 文件不存在 → ErrKeyNotFound。
func TestLoadMissingKey(t *testing.T) {
	if _, err := LoadPrivateKey(filepath.Join(t.TempDir(), "nope.key")); err != ErrKeyNotFound {
		t.Fatalf("want ErrKeyNotFound, got %v", err)
	}
}

// TestPublicKeyPEMRoundtrip 公钥 PEM 可被重新解析且一致(Docker_Manager_Go 集成用)。
func TestPublicKeyPEMRoundtrip(t *testing.T) {
	kp, _ := GenerateKeyPair()
	pemStr := PublicKeyPEM(kp.Public)
	pub, err := PEMPublicKey(pemStr)
	if err != nil {
		t.Fatalf("parse PEM: %v", err)
	}
	if len(pub) != len(kp.Public) {
		t.Fatal("public key length mismatch")
	}
	for i := range pub {
		if pub[i] != kp.Public[i] {
			t.Fatal("public key bytes mismatch")
		}
	}
}

// TestRejectGarbageKey 非 PEM 内容必须拒绝。
func TestRejectGarbageKey(t *testing.T) {
	if _, err := LoadPrivateKey(writeTemp(t, "garbage")); err == nil {
		t.Fatal("garbage key file must fail to load")
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "k")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}
