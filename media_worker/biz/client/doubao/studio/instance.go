package studio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"media_agent/media_worker/biz/domain/doubao"
	"media_agent/media_worker/biz/domain/execution"
)

type InstanceConfig struct {
	UserData, AssetRoot, ArtifactRoot, AccountID, WorkingDirectory string
	Command                                                        []string
	StartupTimeout                                                 time.Duration
}
type Instance struct {
	cfg    InstanceConfig
	lock   *os.File
	cmd    *exec.Cmd
	done   chan struct{}
	client *Client
	pid    int
	slot   chan struct{}
	mu     sync.Mutex
}

// Open claims the dedicated profile, independently of credentials/Worker IDs.
func Open(cfg InstanceConfig) (*Instance, error) {
	if !filepath.IsAbs(cfg.UserData) || !filepath.IsAbs(cfg.AssetRoot) || cfg.AccountID == "" {
		return nil, ErrControl
	}
	if os.MkdirAll(cfg.UserData, 0700) != nil {
		return nil, ErrControl
	}
	info, err := os.Lstat(cfg.UserData)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, ErrControl
	}
	lockPath := filepath.Join(cfg.UserData, ".media-worker.lock")
	if info, err := os.Lstat(lockPath); err == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0) {
		return nil, ErrControl
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, ErrControl
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		f.Close()
		return nil, ErrControl
	}
	if cfg.StartupTimeout <= 0 {
		cfg.StartupTimeout = 90 * time.Second
	}
	return &Instance{cfg: cfg, lock: f, slot: make(chan struct{}, 1)}, nil
}

type discoveryDTO struct {
	ProfileHash   string `json:"profileHash"`
	AssetRootHash string `json:"assetRootHash"`
	Address       string `json:"address"`
	Port, PID     int
	TokenFile     string `json:"tokenFile"`
	ExpiresAt     string `json:"expiresAt"`
}

func privateRead(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 64<<10 {
		return nil, ErrControl
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, ErrControl
	}
	return data, nil
}
func (i *Instance) discover(ctx context.Context) error {
	root := filepath.Join(i.cfg.UserData, "DoubaoStudioControl")
	data, err := privateRead(filepath.Join(root, "control-info.json"))
	if err != nil {
		return err
	}
	var info discoveryDTO
	if json.Unmarshal(data, &info) != nil || info.Address != "127.0.0.1" || info.Port < 1 || info.Port > 65535 || info.PID < 1 || info.TokenFile != "control-token" {
		return ErrControl
	}
	expires, err := time.Parse(time.RFC3339Nano, info.ExpiresAt)
	if err != nil || !expires.After(time.Now()) {
		return ErrControl
	}
	// Electron rewrites its argv/environ area; /proc/PID/environ cannot prove
	// ownership. Bind private discovery and authenticated health to canonical
	// profile/asset-root hashes plus the actual live main-process PID instead.
	profile, err := filepath.EvalSymlinks(i.cfg.UserData)
	if err != nil {
		return ErrControl
	}
	assets, err := filepath.EvalSymlinks(i.cfg.AssetRoot)
	if err != nil {
		return ErrControl
	}
	digest := func(value string) string { h := sha256.Sum256([]byte(value)); return hex.EncodeToString(h[:]) }
	if info.ProfileHash != digest(profile) || info.AssetRootHash != digest(assets) || syscall.Kill(info.PID, 0) != nil || i.cmd != nil && info.PID != i.cmd.Process.Pid {
		return ErrControl
	}
	token, err := privateRead(filepath.Join(root, info.TokenFile))
	if err != nil {
		return err
	}
	client, err := NewClient("http://127.0.0.1:"+strconv.Itoa(info.Port), string(token), i.cfg.AccountID, i.cfg.ArtifactRoot)
	if err != nil {
		return err
	}
	if client.Ready(ctx, info.PID, info.ProfileHash, info.AssetRootHash) != nil {
		return ErrControl
	}
	i.client = client
	i.pid = info.PID
	return nil
}
func (i *Instance) ensure(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, i.cfg.StartupTimeout)
	defer cancel()
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.discover(ctx) == nil {
		return nil
	}
	if i.cmd == nil {
		if len(i.cfg.Command) == 0 {
			return ErrControl
		}
		// Refuse to launch over a possibly live but unready dedicated instance.
		if data, err := privateRead(filepath.Join(i.cfg.UserData, "DoubaoStudioControl", "control-info.json")); err == nil {
			var old discoveryDTO
			if json.Unmarshal(data, &old) == nil && old.PID > 0 && syscall.Kill(old.PID, 0) == nil {
				return ErrControl
			}
		}
		cmd := exec.Command(i.cfg.Command[0], i.cfg.Command[1:]...)
		cmd.Dir = i.cfg.WorkingDirectory
		env := []string{}
		for _, v := range os.Environ() {
			if !strings.HasPrefix(v, "MEDIA_WORKER_STUDIO_USER_DATA=") && !strings.HasPrefix(v, "MEDIA_WORKER_STUDIO_ASSET_ROOT=") {
				env = append(env, v)
			}
		}
		cmd.Env = append(env, "MEDIA_WORKER_STUDIO_USER_DATA="+i.cfg.UserData, "MEDIA_WORKER_STUDIO_ASSET_ROOT="+i.cfg.AssetRoot)
		log, err := os.OpenFile(filepath.Join(i.cfg.UserData, "worker-studio.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return ErrControl
		}
		cmd.Stdout, cmd.Stderr = log, log
		if cmd.Start() != nil {
			log.Close()
			return ErrControl
		}
		i.cmd = cmd
		i.done = make(chan struct{})
		go func() { _ = cmd.Wait(); log.Close(); close(i.done) }()
	}
	deadline := time.NewTimer(i.cfg.StartupTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		if i.discover(ctx) == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-i.done:
			return ErrControl
		case <-deadline.C:
			return ErrControl
		case <-ticker.C:
		}
	}
}
func (i *Instance) Acquire(ctx context.Context) (func(), error) {
	select {
	case i.slot <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := i.ensure(ctx); err != nil {
		<-i.slot
		return nil, err
	}
	if err := i.client.AccountReady(ctx); err != nil {
		<-i.slot
		return nil, err
	}
	return func() { <-i.slot }, nil
}
func (i *Instance) Close() error {
	// Only a process launched by this object is signalled; adopted instances stay.
	if i.cmd != nil {
		select {
		case <-i.done:
		default:
			_ = i.cmd.Process.Signal(syscall.SIGTERM)
			select {
			case <-i.done:
			case <-time.After(5 * time.Second):
				_ = i.cmd.Process.Kill()
				<-i.done
			}
		}
	}
	return i.lock.Close()
}
func (i *Instance) Submit(ctx context.Context, r doubao.Request) (doubao.Reference, error) {
	if i.ensure(ctx) != nil {
		return "", ErrControl
	}
	return i.client.Submit(ctx, r)
}
func (i *Instance) Lookup(ctx context.Context, key string) (doubao.LookupResult, error) {
	if i.ensure(ctx) != nil {
		return doubao.LookupResult{State: doubao.Uncertain}, ErrControl
	}
	return i.client.Lookup(ctx, key)
}
func (i *Instance) Observe(ctx context.Context, ref doubao.Reference) (doubao.Observation, error) {
	if i.ensure(ctx) != nil {
		return doubao.Observation{}, ErrControl
	}
	return i.client.Observe(ctx, ref)
}
func (i *Instance) FetchArtifacts(ctx context.Context, ref doubao.Reference, as []doubao.Artifact) ([]doubao.Download, error) {
	if i.ensure(ctx) != nil {
		return nil, ErrControl
	}
	return i.client.FetchArtifacts(ctx, ref, as)
}

var _ doubao.Generator = (*Instance)(nil)
var _ execution.Admission = (*Instance)(nil)

func (i *Instance) AcquireRecovery(ctx context.Context) (func(), error) {
	select {
	case i.slot <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := i.ensure(ctx); err != nil {
		<-i.slot
		return nil, err
	}
	return func() { <-i.slot }, nil
}
