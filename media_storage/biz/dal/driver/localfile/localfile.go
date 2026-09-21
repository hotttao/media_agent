// Package localfile implements the Stored Object driver on a local filesystem.
package localfile

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	domain "media_agent/media_storage/biz/domain/stored_object"
)

// Driver stores objects below a single resolved root. It rejects symbolic
// links in object paths so a writable local backend cannot escape its root.
type Driver struct {
	root string
}

func New(root string) (*Driver, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("local storage root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve local storage root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("create local storage root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("resolve local storage symlinks: %w", err)
	}
	return &Driver{root: resolved}, nil
}

func (d *Driver) Put(ctx context.Context, objectPath string, body io.Reader) (domain.BlobInfo, error) {
	target, err := d.resolve(objectPath, true)
	if err != nil {
		return domain.BlobInfo{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.BlobInfo{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".upload-*")
	if err != nil {
		return domain.BlobInfo{}, fmt.Errorf("create upload temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(tmp, hash), &contextReader{ctx: ctx, reader: body})
	if copyErr == nil {
		copyErr = tmp.Sync()
	}
	closeErr := tmp.Close()
	if copyErr != nil {
		return domain.BlobInfo{}, fmt.Errorf("write object: %w", copyErr)
	}
	if closeErr != nil {
		return domain.BlobInfo{}, fmt.Errorf("close object: %w", closeErr)
	}

	checksum := fmt.Sprintf("sha256:%x", hash.Sum(nil))
	if err := os.Link(tmpName, target); err == nil {
		return domain.BlobInfo{ObjectPath: objectPath, SizeBytes: size, Checksum: checksum}, nil
	} else if !errors.Is(err, os.ErrExist) {
		return domain.BlobInfo{}, fmt.Errorf("commit object: %w", err)
	}

	existing, err := d.Stat(ctx, objectPath)
	if err != nil {
		return domain.BlobInfo{}, err
	}
	if existing.SizeBytes == size && existing.Checksum == checksum {
		return existing, nil
	}
	return domain.BlobInfo{}, domain.ErrObjectConflict
}

func (d *Driver) Open(ctx context.Context, objectPath string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	target, err := d.resolve(objectPath, false)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil, domain.ErrObjectNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("open object: %w", err)
	}
	return file, nil
}

func (d *Driver) Stat(ctx context.Context, objectPath string) (domain.BlobInfo, error) {
	reader, err := d.Open(ctx, objectPath)
	if err != nil {
		return domain.BlobInfo{}, err
	}
	defer reader.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, &contextReader{ctx: ctx, reader: reader})
	if err != nil {
		return domain.BlobInfo{}, fmt.Errorf("checksum object: %w", err)
	}
	return domain.BlobInfo{ObjectPath: objectPath, SizeBytes: size, Checksum: fmt.Sprintf("sha256:%x", hash.Sum(nil))}, nil
}

func (d *Driver) Delete(ctx context.Context, objectPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target, err := d.resolve(objectPath, false)
	if errors.Is(err, domain.ErrObjectNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.Remove(target); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("delete object: %w", err)
	}
	return nil
}

func (d *Driver) resolve(objectPath string, createParents bool) (string, error) {
	if err := validateObjectPath(objectPath); err != nil {
		return "", err
	}
	parts := strings.Split(objectPath, "/")
	current := d.root
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) && createParents {
			if err := os.Mkdir(current, 0o750); err != nil && !errors.Is(err, os.ErrExist) {
				return "", fmt.Errorf("create object directory: %w", err)
			}
			info, err = os.Lstat(current)
		}
		if errors.Is(err, os.ErrNotExist) {
			return "", domain.ErrObjectNotFound
		}
		if err != nil {
			return "", fmt.Errorf("inspect object directory: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", domain.ErrInvalidObjectPath
		}
	}
	target := filepath.Join(current, parts[len(parts)-1])
	if info, err := os.Lstat(target); err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
		return "", domain.ErrInvalidObjectPath
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect object: %w", err)
	}
	return target, nil
}

func validateObjectPath(value string) error {
	if value == "" || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") || path.Clean(value) != value {
		return domain.ErrInvalidObjectPath
	}
	parts := strings.Split(value, "/")
	if len(parts) < 2 || parts[0] == "" || parts[len(parts)-1] == "" {
		return domain.ErrInvalidObjectPath
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
