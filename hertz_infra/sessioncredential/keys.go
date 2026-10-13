package sessioncredential

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
)

var ErrKeys = errors.New("invalid persistent credential keys")

type Ring struct {
	Active string
	keys   map[string][]byte
}

func New(active string, keys map[string][]byte) (*Ring, error) {
	copyKeys := map[string][]byte{}
	for id, key := range keys {
		if id == "" || len(id) > 64 || strings.ContainsAny(id, ".\x00\r\n") || len(key) < 32 {
			return nil, ErrKeys
		}
		copyKeys[id] = append([]byte(nil), key...)
	}
	if _, ok := copyKeys[active]; !ok {
		return nil, ErrKeys
	}
	return &Ring{active, copyKeys}, nil
}
func Load(path string) (*Ring, error) {
	stat, err := os.Lstat(path)
	if err != nil || !stat.Mode().IsRegular() || stat.Mode().Perm()&0077 != 0 || stat.Size() > 64<<10 {
		return nil, ErrKeys
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, ErrKeys
	}
	var value struct {
		Active string            `json:"active"`
		Keys   map[string]string `json:"keys"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return nil, ErrKeys
	}
	keys := map[string][]byte{}
	for id, text := range value.Keys {
		key, err := base64.RawURLEncoding.DecodeString(text)
		if err != nil {
			return nil, ErrKeys
		}
		keys[id] = key
	}
	return New(value.Active, keys)
}
func (r *Ring) Derive(purpose, keyID string, scope ...string) (string, error) {
	key, ok := r.keys[keyID]
	if !ok || purpose == "" {
		return "", ErrKeys
	}
	body, err := json.Marshal(append([]string{purpose, keyID}, scope...))
	if err != nil {
		return "", ErrKeys
	}
	hash := hmac.New(sha256.New, key)
	hash.Write(body)
	return purpose + "." + keyID + "." + base64.RawURLEncoding.EncodeToString(hash.Sum(nil)), nil
}
func Hash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
func Matches(token, hash string) bool {
	return subtle.ConstantTimeCompare([]byte(Hash(token)), []byte(hash)) == 1
}
