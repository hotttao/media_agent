// Package stored_object defines the Stored Object domain ports and errors.
package stored_object

import (
	"context"
	"errors"
	"io"
)

var (
	ErrInvalidObjectPath = errors.New("invalid object path")
	ErrObjectConflict    = errors.New("object path already contains different content")
	ErrObjectNotFound    = errors.New("stored object not found")
)

// BlobInfo describes physical content. Business ownership and access policy
// deliberately remain outside the storage driver.
type BlobInfo struct {
	ObjectPath string
	SizeBytes  int64
	Checksum   string
}

// Driver is the replaceable physical storage port used by Stored Object.
type Driver interface {
	Put(ctx context.Context, objectPath string, body io.Reader) (BlobInfo, error)
	Open(ctx context.Context, objectPath string) (io.ReadCloser, error)
	Stat(ctx context.Context, objectPath string) (BlobInfo, error)
	Delete(ctx context.Context, objectPath string) error
}
