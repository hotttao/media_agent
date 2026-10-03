package library_object_upload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	"media_agent/media_materials/biz/shared"
	"media_agent/media_materials/biz/shared/objectstorage"
)

const (
	MaxUploadBytes      int64 = 256 << 20
	MaxImageUploadBytes int64 = 16 << 20
)

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
		"generated_asset_import": true, "historical_publication_import": true,
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
	tmp, err := os.CreateTemp("", "media-object-upload-*")
	if err != nil {
		return objectstorage.ObjectInfo{}, fmt.Errorf("%w: create bounded upload spool", ErrUnavailable)
	}
	defer func() { _ = tmp.Close(); _ = os.Remove(tmp.Name()) }()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(command.Body, MaxUploadBytes+1))
	if err != nil {
		return objectstorage.ObjectInfo{}, fmt.Errorf("%w: read file", ErrInvalid)
	}
	if size > MaxUploadBytes {
		return objectstorage.ObjectInfo{}, ErrTooLarge
	}
	if size == 0 || size != command.SizeBytes {
		return objectstorage.ObjectInfo{}, ErrInvalid
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return objectstorage.ObjectInfo{}, fmt.Errorf("%w: rewind upload", ErrUnavailable)
	}
	header := make([]byte, 512)
	headerSize, err := io.ReadFull(tmp, header)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return objectstorage.ObjectInfo{}, fmt.Errorf("%w: inspect upload", ErrUnavailable)
	}
	contentType := detectContentType(header[:headerSize])
	extension, ok := contentTypes[contentType]
	if !ok || (strings.HasPrefix(contentType, "video/") && command.Purpose != "material" && command.Purpose != "generated_asset_import" && command.Purpose != "historical_publication_import") {
		return objectstorage.ObjectInfo{}, ErrInvalid
	}
	if !strings.HasPrefix(contentType, "video/") && size > MaxImageUploadBytes {
		return objectstorage.ObjectInfo{}, ErrTooLarge
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return objectstorage.ObjectInfo{}, fmt.Errorf("%w: rewind upload", ErrUnavailable)
	}
	digest := sha256.Sum256([]byte(strings.Join([]string{command.CallerID, command.IdempotencyKey}, "\x00")))
	objectPath := fmt.Sprintf("materials/%s/%s/%s/%x%s", command.OrganizationID, command.Scope, command.Purpose, digest[:16], extension)
	result, err := w.storage.PutIfAbsent(ctx, objectstorage.PutInput{ObjectPath: objectPath, ContentType: contentType,
		Checksum: hex.EncodeToString(hash.Sum(nil)), SizeBytes: size, Body: tmp})
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
