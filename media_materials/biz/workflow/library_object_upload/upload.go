package library_object_upload

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"media_agent/media_materials/biz/shared"
	"media_agent/media_materials/biz/shared/objectstorage"
)

const MaxUploadBytes int64 = 16 << 20

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
		"video/mp4":  ".mp4",
		"video/webm": ".webm",
	}
	purposes = map[string]bool{
		"material": true, "virtual_ip_avatar": true, "virtual_ip_full_body": true,
		"virtual_ip_three_view": true, "virtual_ip_nine_view": true, "product_image": true,
	}
)

type Storage interface {
	PutIfAbsent(context.Context, objectstorage.PutInput) (objectstorage.ObjectInfo, error)
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

func (w *Workflow) Upload(ctx context.Context, command Command) (objectstorage.ObjectInfo, error) {
	if w == nil || w.storage == nil || !validSegment(command.OrganizationID) || !shared.IsKnownScope(command.Scope) ||
		!purposes[command.Purpose] || command.CallerID == "" || len(command.IdempotencyKey) == 0 || len(command.IdempotencyKey) > 128 || command.Body == nil {
		return objectstorage.ObjectInfo{}, ErrInvalid
	}
	if command.SizeBytes <= 0 {
		return objectstorage.ObjectInfo{}, ErrInvalid
	}
	if command.SizeBytes > MaxUploadBytes {
		return objectstorage.ObjectInfo{}, ErrTooLarge
	}
	payload, err := io.ReadAll(io.LimitReader(command.Body, MaxUploadBytes+1))
	if err != nil {
		return objectstorage.ObjectInfo{}, fmt.Errorf("%w: read file", ErrInvalid)
	}
	if int64(len(payload)) > MaxUploadBytes {
		return objectstorage.ObjectInfo{}, ErrTooLarge
	}
	if len(payload) == 0 || int64(len(payload)) != command.SizeBytes {
		return objectstorage.ObjectInfo{}, ErrInvalid
	}
	contentType := detectContentType(payload)
	extension, ok := contentTypes[contentType]
	if !ok || (strings.HasPrefix(contentType, "video/") && command.Purpose != "material") {
		return objectstorage.ObjectInfo{}, ErrInvalid
	}
	digest := sha256.Sum256([]byte(strings.Join([]string{command.CallerID, command.IdempotencyKey}, "\x00")))
	payloadDigest := sha256.Sum256(payload)
	objectPath := fmt.Sprintf("materials/%s/%s/%s/%x%s", command.OrganizationID, command.Scope, command.Purpose, digest[:16], extension)
	result, err := w.storage.PutIfAbsent(ctx, objectstorage.PutInput{ObjectPath: objectPath, ContentType: contentType,
		Checksum: hex.EncodeToString(payloadDigest[:]), SizeBytes: int64(len(payload)), Body: bytes.NewReader(payload)})
	if errors.Is(err, objectstorage.ErrConflict) {
		return objectstorage.ObjectInfo{}, ErrConflict
	}
	if err != nil {
		return objectstorage.ObjectInfo{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return result, nil
}

func detectContentType(payload []byte) string {
	if len(payload) >= 12 && string(payload[4:8]) == "ftyp" {
		return "video/mp4"
	}
	if len(payload) >= 4 && payload[0] == 0x1a && payload[1] == 0x45 && payload[2] == 0xdf && payload[3] == 0xa3 {
		return "video/webm"
	}
	return http.DetectContentType(payload)
}

func validSegment(value string) bool { return segmentPattern.MatchString(value) }
