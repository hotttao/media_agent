// Package ids 提供应用层 ID 生成器（users.id 等短随机字符串，对齐 control-plane generateId）。
//
// users.id 是 text 短随机串（非自增），由应用层在 upsert 时产生。
package ids

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
)

// New 生成 16 字节（32 hex 字符）的随机 ID。失败 panic（rand 故障属不可恢复）。
func New() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("ids: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// NewChecked is for recoverable background workflows. It never exposes the
// random source diagnostic or requires any credential/encryption dependency.
func NewChecked() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", errors.New("identifier generation unavailable")
	}
	return hex.EncodeToString(b), nil
}
