package gateway

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type uploadGrant struct {
	ObjectPath      string            `json:"object_path"`
	UploadURL       string            `json:"upload_url"`
	Headers         map[string]string `json:"headers"`
	ExpiresAt       int64             `json:"expires_at"`
	AlreadyUploaded bool              `json:"already_uploaded"`
}

type uploadUnconfirmed struct{}

func (uploadUnconfirmed) Error() string   { return "artifact upload not confirmed" }
func (uploadUnconfirmed) Permanent() bool { return false }

// UploadFile sends the bearer only to the gateway. Uncertain PUT responses
// are resolved by requesting a fresh grant (server-side HEAD), never by
// regenerating the Provider result or treating an unconfirmed write as done.
func (c *Client) UploadFile(ctx context.Context, workerID string, task Task, index int, filename string) (string, error) {
	ctx = TaskContext(ctx, task)
	f, err := os.Open(filename)
	if err != nil {
		return "", errors.New("artifact file unavailable")
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() <= 0 || stat.Size() > 256<<20 || index < 0 || index >= 8 {
		return "", errors.New("artifact file invalid")
	}
	head := make([]byte, 512)
	n, err := f.ReadAt(head, 0)
	if err != nil && err != io.EOF {
		return "", errors.New("artifact file unreadable")
	}
	mime := http.DetectContentType(head[:n])
	if n >= 12 && string(head[4:8]) == "ftyp" {
		mime = "video/mp4"
	}
	allowed := map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true, "image/gif": true, "video/mp4": true, "video/webm": true}
	if !allowed[mime] || strings.HasPrefix(mime, "image/") && stat.Size() > 16<<20 {
		return "", errors.New("artifact media type or size invalid")
	}
	sha, md := sha256.New(), md5.New()
	if copied, err := io.Copy(io.MultiWriter(sha, md), io.NewSectionReader(f, 0, stat.Size())); err != nil || copied != stat.Size() {
		return "", errors.New("artifact file unreadable")
	}
	body := map[string]any{"worker_instance_id": workerID, "attempt": task.Attempt, "candidate_index": index,
		"content_type": mime, "size_bytes": stat.Size(), "checksum_sha256": hex.EncodeToString(sha.Sum(nil)), "content_md5": base64.StdEncoding.EncodeToString(md.Sum(nil))}
	path := "/api/worker/v1/tasks/" + url.PathEscape(task.DispatchID) + "/uploads"
	storage := *c.http
	storage.Jar = nil
	storage.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	storage.Timeout = 5 * time.Minute
	var expectedPath string
	for attempt := 0; attempt < 4; attempt++ {
		var grant uploadGrant
		if err := c.doJSON(ctx, http.MethodPost, path, body, &grant); err != nil {
			return "", err
		}
		if grant.ObjectPath == "" || expectedPath != "" && grant.ObjectPath != expectedPath {
			return "", errors.New("artifact grant invalid")
		}
		expectedPath = grant.ObjectPath
		if grant.AlreadyUploaded {
			return grant.ObjectPath, nil
		}
		if attempt == 3 {
			break
		} // Final HEAD confirmation; no fourth PUT.
		u, err := url.Parse(grant.UploadURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Fragment != "" || grant.ExpiresAt <= time.Now().Unix() {
			return "", errors.New("artifact grant invalid")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, grant.UploadURL, io.NewSectionReader(f, 0, stat.Size()))
		if err != nil {
			return "", errors.New("artifact grant invalid")
		}
		req.ContentLength = stat.Size()
		for key, value := range grant.Headers {
			lower := strings.ToLower(key)
			switch {
			case lower == "host":
				if value != u.Host {
					return "", errors.New("artifact grant invalid")
				}
			case lower == "content-length":
				if value != strconv.FormatInt(stat.Size(), 10) {
					return "", errors.New("artifact grant invalid")
				}
			case lower == "if-none-match", lower == "content-md5", lower == "content-type", strings.HasPrefix(lower, "x-amz-"):
				req.Header.Set(key, value)
			default:
				return "", errors.New("artifact grant invalid")
			}
		}
		if req.Header.Get("If-None-Match") != "*" || req.Header.Get("Content-Type") != mime || req.Header.Get("Content-MD5") != body["content_md5"] {
			return "", errors.New("artifact grant missing signed constraints")
		}
		response, err := storage.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			continue // Never expose an error containing a signed URL.
		}
		io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		response.Body.Close()
		if response.StatusCode >= 300 && response.StatusCode != http.StatusPreconditionFailed && response.StatusCode < 500 {
			return "", errors.New("artifact storage rejected upload")
		}
		// Even a successful PUT is confirmed through the next preparation.
	}
	return "", uploadUnconfirmed{}
}
