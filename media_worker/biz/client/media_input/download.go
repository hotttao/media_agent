package media_input

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"media_agent/media_worker/biz/domain/execution"
)

var ErrDownload = errors.New("input image unavailable or invalid")

type Downloader struct {
	Root string
	HTTP *http.Client
}

func (d Downloader) Fetch(ctx context.Context, key, raw string) (execution.Artifact, error) {
	if os.MkdirAll(d.Root, 0700) != nil {
		return execution.Artifact{}, ErrDownload
	}
	rootInfo, rootErr := os.Lstat(d.Root)
	if rootErr != nil || !rootInfo.IsDir() || rootInfo.Mode().Perm()&0077 != 0 {
		return execution.Artifact{}, ErrDownload
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return execution.Artifact{}, ErrDownload
	}
	if len(key) != 64 {
		return execution.Artifact{}, ErrDownload
	}
	client := d.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return execution.Artifact{}, ErrDownload
	}
	response, err := client.Do(req)
	if err != nil {
		return execution.Artifact{}, ErrDownload
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || response.ContentLength > 16<<20 {
		return execution.Artifact{}, ErrDownload
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (16<<20)+1))
	if err != nil || len(data) == 0 || len(data) > 16<<20 {
		return execution.Artifact{}, ErrDownload
	}
	mime := http.DetectContentType(data)
	ext := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp"}[mime]
	if ext == "" {
		return execution.Artifact{}, ErrDownload
	}
	dir := filepath.Join(d.Root, key)
	if os.MkdirAll(dir, 0700) != nil {
		return execution.Artifact{}, ErrDownload
	}
	dirInfo, dirErr := os.Lstat(dir)
	if dirErr != nil || !dirInfo.IsDir() || dirInfo.Mode().Perm()&0077 != 0 {
		return execution.Artifact{}, ErrDownload
	}
	target := filepath.Join(dir, "input-0"+ext)
	if err := writePrivate(dir, target, data); err != nil {
		return execution.Artifact{}, ErrDownload
	}
	sum := sha256.Sum256(data)
	return execution.Artifact{ID: hex.EncodeToString(sum[:]), LocalPath: target, ChecksumSHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(data))}, nil
}
func writePrivate(dir, target string, data []byte) error {
	f, err := os.CreateTemp(dir, ".input-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	cerr := f.Close()
	if err != nil {
		return err
	}
	if cerr != nil {
		return cerr
	}
	if err = os.Rename(name, target); err != nil {
		return err
	}
	folder, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer folder.Close()
	return folder.Sync()
}

// Verify rejects changed, missing, symlinked or partially written manifest files.
func (d Downloader) Verify(ctx context.Context, a execution.Artifact) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	info, err := os.Lstat(a.LocalPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() != a.SizeBytes || a.SizeBytes <= 0 || a.SizeBytes > 256<<20 || len(a.ChecksumSHA256) != 64 {
		return ErrDownload
	}
	f, err := os.Open(a.LocalPath)
	if err != nil {
		return ErrDownload
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil || hex.EncodeToString(h.Sum(nil)) != a.ChecksumSHA256 {
		return ErrDownload
	}
	return nil
}
