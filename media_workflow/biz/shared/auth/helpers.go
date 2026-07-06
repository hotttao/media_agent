package auth

import (
	"crypto/sha256"
	"encoding/base64"
)

func base64URLEncode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func newSHA256(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}
