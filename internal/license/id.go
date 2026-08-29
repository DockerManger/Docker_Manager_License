package license

import (
	"crypto/rand"
	"encoding/binary"
	"strings"
	"time"
)

// ---------- License ID 生成 ----------
//
// 格式: DMG-<Crockford base32 26 字符 ULID>(16 字节:前 6 字节毫秒时间戳 + 后 10 字节随机)。
// 要求:全局唯一、不泄露数据库自增 ID、可安全展示、可排序(按签发时间)、可作客服查询凭据。
// 标准库自带 crypto/rand,不引入外部 ULID 依赖。

const (
	ulidAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ" // Crockford base32(去掉 I L O U)
	ulidLen      = 26
)

// NewLicenseID 生成 DMG-<ULID>。
func NewLicenseID() string {
	return "DMG-" + newULID()
}

// newULID 生成 26 字符 Crockford base32 ULID(标准 ULID 布局:48bit 毫秒时间戳 + 80bit 随机)。
func newULID() string {
	var b [16]byte
	ms := uint64(time.Now().UnixMilli())
	binary.BigEndian.PutUint64(b[:8], ms) // 高 48 bit 为时间戳
	if _, err := rand.Read(b[6:]); err != nil {
		panic(err) // crypto/rand 失败为致命错误
	}
	var sb strings.Builder
	sb.Grow(ulidLen)
	// 128 bit → 26 字符(每 5 bit 一个字符,高位补齐到 130 bit 后取前 26)
	// 标准 ULID 编码:从最高位开始每 5 bit 取一个字符
	for i := 0; i < ulidLen; i++ {
		// 取 bit 范围 [125-5i, 130-5i) —— 等效于把 128bit 左移后逐段取
		shift := 128 - 5*(i+1)
		var idx byte
		if shift >= 0 {
			idx = byte(b[shift/8] >> (shift % 8))
		} else {
			// 不足 5 bit 时跨字节取(第 26 字符只需 3 bit)
			idx = byte(b[0] << (-shift % 8))
		}
		idx &= 0x1F
		sb.WriteByte(ulidAlphabet[idx])
	}
	return sb.String()
}
