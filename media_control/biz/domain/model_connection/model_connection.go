package model_connection

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrNotFound = errors.New("model connection not found")
var ErrConflict = errors.New("model connection version conflict")
var ErrInvalid = errors.New("invalid model connection or auth.json")
var ErrRevoked = errors.New("model connection is revoked")

type Connection struct {
	ID, OwnerUserID, Name, Kind, Status string
	Version                             int64
	EncryptedAuth                       []byte `json:"-"`
	CreatedAt, UpdatedAt                time.Time
}
type Secret struct {
	ID       string
	Version  int64
	AuthJSON string
}
type Repository interface {
	Get(context.Context, string) (*Connection, error)
	List(context.Context, string) ([]Connection, error)
	Save(context.Context, *Connection, int64) error
}

func ValidateAuth(raw string) error {
	if len(raw) == 0 || len(raw) > 65536 {
		return ErrInvalid
	}
	var doc map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &doc) != nil || doc == nil {
		return ErrInvalid
	}
	var key string
	_ = json.Unmarshal(doc["OPENAI_API_KEY"], &key)
	if strings.TrimSpace(key) != "" {
		return nil
	}
	var tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if json.Unmarshal(doc["tokens"], &tokens) != nil || strings.TrimSpace(tokens.AccessToken) == "" || strings.TrimSpace(tokens.RefreshToken) == "" {
		return ErrInvalid
	}
	return nil
}
