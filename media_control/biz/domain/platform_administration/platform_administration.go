package platform_administration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

var (
	ErrNotFound  = errors.New("platform setting not found")
	ErrConflict  = errors.New("platform setting version conflict")
	ErrForbidden = errors.New("platform administrator role is required")
)

var settingKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,127}$`)

type Actor struct{ UserID, Role string }

func (a Actor) ValidateAdmin() error {
	if strings.TrimSpace(a.UserID) == "" || (a.Role != "admin" && a.Role != "system") {
		return ErrForbidden
	}
	return nil
}

type Setting struct {
	Key, ValueJSON, UpdatedBy string
	Version                   int64
	UpdatedAt                 time.Time
}

type ChangeEvent struct {
	ID, SettingKey, EventType, PayloadJSON string
	Version                                int64
	CreatedAt                              time.Time
}

type Repository interface {
	Create(context.Context, *Setting) error
	List(context.Context) ([]Setting, error)
	Get(context.Context, string) (*Setting, error)
	Update(context.Context, *Setting, int64) error
	Enqueue(context.Context, *ChangeEvent) error
}

func Validate(key, valueJSON string) (string, string, error) {
	key, valueJSON = strings.TrimSpace(key), strings.TrimSpace(valueJSON)
	if !settingKeyPattern.MatchString(key) {
		return "", "", errors.New("key must start with a lowercase letter and contain only lowercase letters, digits, dot, underscore or hyphen")
	}
	if valueJSON == "" || len(valueJSON) > 64*1024 || !json.Valid([]byte(valueJSON)) {
		return "", "", errors.New("value_json must be valid JSON no larger than 64 KiB")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(valueJSON)); err != nil {
		return "", "", err
	}
	return key, compact.String(), nil
}
