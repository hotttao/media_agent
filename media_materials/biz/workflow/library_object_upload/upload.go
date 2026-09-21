package library_object_upload

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"media_agent/media_materials/biz/shared"
	storageclient "media_agent/media_materials/biz/shared/client/media_storage"
)

const MaxUploadBytes int64 = 4 << 20

var (
	ErrInvalid     = errors.New("invalid library object upload")
	ErrTooLarge    = errors.New("library object exceeds size limit")
	ErrConflict    = errors.New("library object upload conflict")
	ErrUnavailable = errors.New("library object storage unavailable")
	segmentPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	contentTypes   = map[string]string{
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/webp": ".webp",
		"image/gif":  ".gif",
	}
	purposes = map[string]bool{
		"material": true, "virtual_ip_avatar": true, "virtual_ip_full_body": true,
		"virtual_ip_three_view": true, "virtual_ip_nine_view": true, "product_image": true,
	}
)

type Storage interface {
	Put(context.Context, storageclient.PutInput) (storageclient.ObjectInfo, error)
}

type Command struct {
	OrganizationID string
	Scope          string
	Purpose        string
	CallerID       string
	IdempotencyKey string
	SizeBytes      int64
	Body           io.Reader
}

type Workflow struct{ storage Storage }

func New(storage Storage) *Workflow { return &Workflow{storage: storage} }

func (w *Workflow) Upload(ctx context.Context, command Command) (storageclient.ObjectInfo, error) {
	if w == nil || w.storage == nil || !validSegment(command.OrganizationID) || !shared.IsKnownScope(command.Scope) ||
		!purposes[command.Purpose] || command.CallerID == "" || len(command.IdempotencyKey) == 0 || len(command.IdempotencyKey) > 128 || command.Body == nil {
		return storageclient.ObjectInfo{}, ErrInvalid
	}
	if command.SizeBytes <= 0 {
		return storageclient.ObjectInfo{}, ErrInvalid
	}
	if command.SizeBytes > MaxUploadBytes {
		return storageclient.ObjectInfo{}, ErrTooLarge
	}
	payload, err := io.ReadAll(io.LimitReader(command.Body, MaxUploadBytes+1))
	if err != nil {
		return storageclient.ObjectInfo{}, fmt.Errorf("%w: read file", ErrInvalid)
	}
	if int64(len(payload)) > MaxUploadBytes {
		return storageclient.ObjectInfo{}, ErrTooLarge
	}
	if len(payload) == 0 || int64(len(payload)) != command.SizeBytes {
		return storageclient.ObjectInfo{}, ErrInvalid
	}
	contentType := http.DetectContentType(payload)
	extension, ok := contentTypes[contentType]
	if !ok {
		return storageclient.ObjectInfo{}, ErrInvalid
	}
	digest := sha256.Sum256([]byte(strings.Join([]string{command.CallerID, command.IdempotencyKey}, "\x00")))
	objectPath := fmt.Sprintf("materials/%s/%s/%s/%x%s", command.OrganizationID, command.Scope, command.Purpose, digest[:16], extension)
	result, err := w.storage.Put(ctx, storageclient.PutInput{ObjectPath: objectPath, ContentType: contentType,
		SizeBytes: int64(len(payload)), Body: bytes.NewReader(payload)})
	if errors.Is(err, storageclient.ErrConflict) {
		return storageclient.ObjectInfo{}, ErrConflict
	}
	if err != nil {
		return storageclient.ObjectInfo{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return result, nil
}

func validSegment(value string) bool { return segmentPattern.MatchString(value) }
