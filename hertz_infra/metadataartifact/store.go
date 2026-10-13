package metadataartifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
)

const MaxBytes = 8 << 20

var ErrInvalid = errors.New("invalid metadata artifact")

type Store struct{ root string }

func New(root string) (*Store, error) {
	absolute, err := filepath.Abs(root)
	if err != nil || root == "" {
		return nil, ErrInvalid
	}
	if err := os.MkdirAll(absolute, 0700); err != nil {
		return nil, err
	}
	return &Store{absolute}, nil
}
func valid(hash string) bool {
	if len(hash) != 64 {
		return false
	}
	bytes, err := hex.DecodeString(hash)
	return err == nil && len(bytes) == 32 && hex.EncodeToString(bytes) == hash
}
func (s *Store) Put(ctx context.Context, hash string, body []byte) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	sum := sha256.Sum256(body)
	if !valid(hash) || hex.EncodeToString(sum[:]) != hash || len(body) == 0 || len(body) > MaxBytes {
		return ErrInvalid
	}
	target := filepath.Join(s.root, hash)
	if existing, err := s.Get(ctx, hash); err == nil {
		if string(existing) != string(body) {
			return ErrInvalid
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temp, err := os.CreateTemp(s.root, ".staging-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(body); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	// Hard-link publishes without replacing an already-published object.
	if err := os.Link(temp.Name(), target); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		_, err = s.Get(ctx, hash)
		return err
	}
	return syncDir(s.root)
}
func (s *Store) Get(ctx context.Context, hash string) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if !valid(hash) {
		return nil, ErrInvalid
	}
	path := filepath.Join(s.root, hash)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxBytes {
		return nil, ErrInvalid
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != hash {
		return nil, ErrInvalid
	}
	return body, nil
}
func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
