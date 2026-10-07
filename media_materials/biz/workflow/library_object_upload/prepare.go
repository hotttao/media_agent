package library_object_upload

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"media_agent/media_materials/biz/shared/objectstorage"
)

type PrepareCommand struct {
	OrganizationID, Scope, UploadKey  string
	ContentType, Checksum, ContentMD5 string
	SizeBytes                         int64
}

// Prepare is an internal write-grant port. It never accepts an arbitrary
// Object Path and never creates a Material or Generated Asset.
func (w *Workflow) Prepare(ctx context.Context, command PrepareCommand) (objectstorage.UploadGrant, error) {
	sha, shaErr := hex.DecodeString(command.Checksum)
	key, keyErr := hex.DecodeString(command.UploadKey)
	md5, md5Err := base64.StdEncoding.DecodeString(command.ContentMD5)
	if !validSegment(command.OrganizationID) || !validSegment(command.Scope) || keyErr != nil || len(key) != 32 || strings.ToLower(command.UploadKey) != command.UploadKey ||
		shaErr != nil || len(sha) != 32 || strings.ToLower(command.Checksum) != command.Checksum || md5Err != nil || len(md5) != 16 || base64.StdEncoding.EncodeToString(md5) != command.ContentMD5 || command.SizeBytes <= 0 {
		return objectstorage.UploadGrant{}, ErrInvalid
	}
	if _, ok := contentTypes[command.ContentType]; !ok {
		return objectstorage.UploadGrant{}, ErrInvalid
	}
	if command.SizeBytes > MaxUploadBytes || !strings.HasPrefix(command.ContentType, "video/") && command.SizeBytes > MaxImageUploadBytes {
		return objectstorage.UploadGrant{}, ErrTooLarge
	}
	storage, ok := w.storage.(interface {
		PresignPut(context.Context, objectstorage.UploadInput) (objectstorage.UploadGrant, error)
	})
	if !ok {
		return objectstorage.UploadGrant{}, ErrUnavailable
	}
	// Include the content manifest so a new file cannot share an already
	// referenced path, even across concurrent preparation requests.
	manifest := sha256.Sum256([]byte(strings.Join([]string{command.Checksum, command.ContentMD5, command.ContentType, strconv.FormatInt(command.SizeBytes, 10)}, "\x00")))
	objectPath := fmt.Sprintf("materials/%s/%s/worker_generated/%s/%x", command.OrganizationID, command.Scope, command.UploadKey, manifest[:])
	grant, err := storage.PresignPut(ctx, objectstorage.UploadInput{ObjectPath: objectPath, ContentType: command.ContentType, Checksum: command.Checksum, ContentMD5: command.ContentMD5, SizeBytes: command.SizeBytes})
	if errors.Is(err, objectstorage.ErrConflict) {
		return objectstorage.UploadGrant{}, ErrConflict
	}
	if err != nil {
		return objectstorage.UploadGrant{}, ErrUnavailable
	}
	return grant, nil
}
