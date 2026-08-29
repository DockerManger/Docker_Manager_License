// sign-demo 生成 V2 demo key(开发构建专用,2100 年过期,全功能)。
// 用法: go run ./cmd/sign-demo
package main

import (
	"fmt"
	"time"

	"github.com/DockerManger/Docker_Manager_License/internal/crypto"
	"github.com/DockerManger/Docker_Manager_License/internal/license"
)

func main() {
	priv, err := crypto.LoadPrivateKey("private/license.key")
	if err != nil {
		panic(err)
	}
	p := &license.Payload{
		Version:    license.CurrentVersion,
		KeyID:      "2026-01",
		LicenseID:  "DMG-DEMO0000000000000001",
		Product:    license.ProductDMG,
		Plan:       "pro",
		Features:   []string{"compose", "container_create", "appstore"},
		Customer:   "demo",
		IssuedAt:   time.Now().Unix(),
		ExpiresAt:  4102444800, // 2100-01-01
		MaxDevices: 9999,
	}
	key, err := license.EncodeKey(p, priv.Private)
	if err != nil {
		panic(err)
	}
	fmt.Println(key)
}
