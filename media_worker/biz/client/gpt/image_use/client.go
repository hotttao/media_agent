// Package image_use adapts a pinned local bridge to the web-only image-use backend.
package image_use

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"media_agent/media_worker/biz/domain/gpt"
	"media_agent/media_worker/biz/shared/worker_runtime"
)

var ErrBridge = errors.New("image-use web bridge unavailable or unconfirmed")
var validKey = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Config struct {
	Command                                                           []string
	Root, InputRoot, LockDirectory, Upstream, Size, WebModel, Project string
	Timeout                                                           time.Duration
}
type Client struct {
	cfg  Config
	lock *os.File
	slot chan struct{}
	mu   sync.Mutex
}
type requestDTO struct {
	Size     string   `json:"size,omitempty"`
	WebModel string   `json:"web_model,omitempty"`
	Key      string   `json:"key"`
	Prompt   string   `json:"prompt"`
	Images   []string `json:"images"`
}
type responseDTO struct {
	OK         bool   `json:"ok"`
	Ready      bool   `json:"ready"`
	State      string `json:"state"`
	Reference  string `json:"reference"`
	ArtifactID string `json:"artifact_id"`
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	Size       int64  `json:"size"`
	MediaType  string `json:"media_type"`
}

func privateDir(path string) error {
	if !filepath.IsAbs(path) {
		return ErrBridge
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return ErrBridge
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return ErrBridge
	}
	return nil
}
func Open(ctx context.Context, cfg Config) (*Client, error) {
	if len(cfg.Command) == 0 || !filepath.IsAbs(cfg.Command[0]) || !filepath.IsAbs(cfg.Upstream) || cfg.Timeout < 30*time.Second || cfg.Timeout > 30*time.Minute {
		return nil, ErrBridge
	}
	for _, p := range []string{cfg.Root, cfg.InputRoot, cfg.LockDirectory} {
		if privateDir(p) != nil {
			return nil, ErrBridge
		}
	}
	path := filepath.Join(cfg.LockDirectory, ".media-worker-image-use.lock")
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0) {
		return nil, ErrBridge
	}
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, ErrBridge
	}
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		lock.Close()
		return nil, ErrBridge
	}
	c := &Client{cfg: cfg, lock: lock, slot: make(chan struct{}, 1)}
	if _, err = c.call(ctx, "init", requestDTO{}); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}
func (c *Client) Close() error {
	if c.lock == nil {
		return nil
	}
	_ = syscall.Flock(int(c.lock.Fd()), syscall.LOCK_UN)
	err := c.lock.Close()
	c.lock = nil
	return err
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 64<<10 {
		return 0, ErrBridge
	}
	return b.Buffer.Write(p)
}
func (c *Client) call(ctx context.Context, action string, in requestDTO) (responseDTO, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, err := json.Marshal(in)
	if err != nil {
		return responseDTO{}, ErrBridge
	}
	args := append([]string{}, c.cfg.Command[1:]...)
	args = append(args, action, "--backend", "web", "--root", c.cfg.Root, "--input-root", c.cfg.InputRoot, "--upstream", c.cfg.Upstream, "--size", c.cfg.Size, "--web-model", c.cfg.WebModel, "--project", c.cfg.Project, "--timeout", fmtSeconds(c.cfg.Timeout))
	budget := c.cfg.Timeout + 30*time.Second
	if action == "preflight" {
		budget = 45 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	cmd := exec.CommandContext(runCtx, c.cfg.Command[0], args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 3 * time.Second
	cmd.Stdin = bytes.NewReader(data)
	var out limitedBuffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	// The gateway credential, trace and provider overrides never cross this boundary.
	for _, e := range os.Environ() {
		key := e
		for n, ch := range e {
			if ch == '=' {
				key = e[:n]
				break
			}
		}
		if strings.HasPrefix(key, "MEDIA_") || strings.HasPrefix(key, "IMAGE_USE_") || strings.HasPrefix(key, "CHATGPT_IMAGEGEN_") || key == "OPENAI_API_KEY" || key == "CODEX_HOME" {
			continue
		}
		cmd.Env = append(cmd.Env, e)
	}
	cmd.Env = append(cmd.Env, "IMAGE_USE_BACKEND=web", "IMAGE_USE_NO_UPDATE_CHECK=1", "USE_NO_UPDATE_CHECK=1")
	if cmd.Run() != nil {
		return responseDTO{}, ErrBridge
	}
	var response responseDTO
	if json.Unmarshal(out.Bytes(), &response) != nil || !response.OK {
		return responseDTO{}, ErrBridge
	}
	return response, nil
}
func fmtSeconds(d time.Duration) string                               { return strconv.FormatInt(int64(d/time.Second), 10) }
func (c *Client) Acquire(ctx context.Context) (func(), error)         { return c.acquire(ctx, true) }
func (c *Client) AcquireRecovery(ctx context.Context) (func(), error) { return c.acquire(ctx, false) }
func (c *Client) acquire(ctx context.Context, check bool) (func(), error) {
	select {
	case c.slot <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	release := func() { <-c.slot }
	if check {
		r, err := c.call(ctx, "preflight", requestDTO{})
		if err != nil || !r.Ready {
			release()
			return nil, ErrBridge
		}
	}
	return release, nil
}
func (c *Client) Lookup(ctx context.Context, key string) (gpt.LookupResult, error) {
	if !validKey.MatchString(key) {
		return gpt.LookupResult{State: gpt.Uncertain}, ErrBridge
	}
	r, err := c.call(ctx, "lookup", requestDTO{Key: key})
	if err != nil {
		return gpt.LookupResult{State: gpt.Uncertain}, err
	}
	switch r.State {
	case "absent":
		return gpt.LookupResult{State: gpt.Absent}, nil
	case "found":
		if r.Reference == key {
			return gpt.LookupResult{State: gpt.Found, Reference: gpt.Reference(key)}, nil
		}
	}
	return gpt.LookupResult{State: gpt.Uncertain}, ErrBridge
}
func (c *Client) Submit(ctx context.Context, r gpt.Request) (gpt.Reference, error) {
	if !validKey.MatchString(r.Key) || r.Prompt == "" || len(r.Images) > 4 {
		return "", ErrBridge
	}
	in := requestDTO{Key: r.Key, Prompt: r.Prompt, Size: r.Size, WebModel: r.WebModel}
	for _, image := range r.Images {
		in.Images = append(in.Images, image.Path)
	}
	out, err := c.call(ctx, "submit", in)
	if err != nil || out.Reference != r.Key {
		return "", ErrBridge
	}
	return gpt.Reference(r.Key), nil
}
func (c *Client) Observe(ctx context.Context, ref gpt.Reference) (gpt.Observation, error) {
	if !validKey.MatchString(string(ref)) {
		return gpt.Observation{}, ErrBridge
	}
	r, err := c.call(ctx, "observe", requestDTO{Key: string(ref)})
	if err != nil {
		return gpt.Observation{}, err
	}
	switch r.State {
	case "succeeded":
		if _, err = c.verify(string(ref), r); err != nil {
			return gpt.Observation{}, err
		}
		return gpt.Observation{State: gpt.Succeeded, Artifacts: []gpt.Artifact{{ID: r.ArtifactID, MediaType: r.MediaType}}}, nil
	case "generating":
		return gpt.Observation{State: gpt.Generating}, nil
	default:
		return gpt.Observation{State: gpt.NeedsHuman, Reason: "image_use_web_requires_attention"}, nil
	}
}
func (c *Client) FetchArtifacts(ctx context.Context, ref gpt.Reference, as []gpt.Artifact) ([]gpt.Download, error) {
	if !validKey.MatchString(string(ref)) || len(as) != 1 {
		return nil, ErrBridge
	}
	r, err := c.call(ctx, "artifact", requestDTO{Key: string(ref)})
	if err != nil || as[0].ID != r.ArtifactID {
		return nil, ErrBridge
	}
	a, err := c.verify(string(ref), r)
	if err != nil {
		return nil, err
	}
	return []gpt.Download{a}, nil
}
func (c *Client) verify(key string, r responseDTO) (gpt.Download, error) {
	expected := filepath.Join(c.cfg.Root, key, "output.png")
	real, err := filepath.EvalSymlinks(expected)
	if err != nil || real != expected || r.Path != expected || r.Size <= 0 || r.Size > 64<<20 || !validKey.MatchString(r.SHA256) || r.ArtifactID != "image-"+r.SHA256 {
		return gpt.Download{}, ErrBridge
	}
	info, err := os.Lstat(expected)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() != r.Size {
		return gpt.Download{}, ErrBridge
	}
	f, err := os.Open(expected)
	if err != nil {
		return gpt.Download{}, ErrBridge
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	mime := http.DetectContentType(head[:n])
	if mime != r.MediaType || (mime != "image/png" && mime != "image/jpeg" && mime != "image/webp") {
		return gpt.Download{}, ErrBridge
	}
	if mime != "image/webp" {
		_, _ = f.Seek(0, 0)
		cfg, _, err := image.DecodeConfig(io.LimitReader(f, 64<<20))
		if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
			return gpt.Download{}, ErrBridge
		}
	}
	_, _ = f.Seek(0, 0)
	h := sha256.New()
	if _, err = io.Copy(h, io.LimitReader(f, (64<<20)+1)); err != nil || hex.EncodeToString(h.Sum(nil)) != r.SHA256 {
		return gpt.Download{}, ErrBridge
	}
	return gpt.Download{ArtifactID: r.ArtifactID, LocalPath: expected, ChecksumSHA256: r.SHA256, SizeBytes: r.Size}, nil
}

var _ gpt.Generator = (*Client)(nil)
var _ worker_runtime.Admission = (*Client)(nil)
var _ worker_runtime.RecoveryAdmission = (*Client)(nil)
