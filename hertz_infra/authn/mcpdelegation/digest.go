package mcpdelegation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
)

// ArgumentsDigest rejects duplicate keys and trailing JSON before canonicalizing
// keys. Number spellings are retained; callers must sign the forwarded encoding.
func ArgumentsDigest(raw []byte) (string, error) {
	if len(raw) > 1<<20 {
		return "", ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	value, err := uniqueValue(d, 0)
	if err != nil {
		return "", ErrInvalid
	}
	if _, ok := value.(map[string]any); !ok {
		return "", ErrInvalid
	}
	if _, err = d.Token(); err != io.EOF {
		return "", ErrInvalid
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return "", ErrInvalid
	}
	sum := sha256.Sum256(normalized)
	return hex.EncodeToString(sum[:]), nil
}
func uniqueValue(d *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, ErrInvalid
	}
	t, err := d.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return t, nil
	}
	switch delim {
	case '{':
		m := map[string]any{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return nil, err
			}
			s, ok := key.(string)
			if !ok {
				return nil, ErrInvalid
			}
			if _, ok = m[s]; ok {
				return nil, ErrInvalid
			}
			v, err := uniqueValue(d, depth+1)
			if err != nil {
				return nil, err
			}
			m[s] = v
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return nil, ErrInvalid
		}
		return m, nil
	case '[':
		a := []any{}
		for d.More() {
			v, err := uniqueValue(d, depth+1)
			if err != nil {
				return nil, err
			}
			a = append(a, v)
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return nil, ErrInvalid
		}
		return a, nil
	default:
		return nil, ErrInvalid
	}
}
