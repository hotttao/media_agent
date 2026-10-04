package platformcipher

import (
	"crypto/rand"
	"encoding/hex"
)

func GenerateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", ErrUnavailable
	}
	return hex.EncodeToString(b), nil
}

// GenerateCredential does not use the legacy Service Token hash verifier.
func GenerateCredential() (string, string, error) {
	id, err := GenerateID()
	if err != nil {
		return "", "", err
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", ErrUnavailable
	}
	return id, "tos_" + hex.EncodeToString(b), nil
}
