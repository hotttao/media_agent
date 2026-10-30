package studio

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"media_agent/media_worker/biz/domain/doubao"
)

var ErrControl = errors.New("studio control unavailable or operation unconfirmed")

type Client struct {
	baseURL, token, account, artifacts string
	http                               *http.Client
}

func NewClient(baseURL, token, account, artifacts string) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil || u.RawQuery != "" || u.Path != "" || token == "" || account == "" {
		return nil, ErrControl
	}
	return &Client{baseURL: baseURL, token: token, account: account, artifacts: artifacts, http: &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

type generationDTO struct {
	OK            bool   `json:"ok"`
	Reference     string `json:"reference"`
	Status        string `json:"status"`
	Started       bool   `json:"started"`
	Code          string `json:"code"`
	Authoritative bool   `json:"authoritative"`
	Artifacts     []struct {
		ID        string `json:"id"`
		Kind      string `json:"kind"`
		MediaType string `json:"mediaType"`
	} `json:"artifacts"`
}

func validRef(ref string) bool {
	if len(ref) != 64 {
		return false
	}
	_, err := hex.DecodeString(ref)
	return err == nil && strings.ToLower(ref) == ref
}
func path(ref string) string { return "/v1/worker/generations/" + ref }
func (c *Client) request(ctx context.Context, method, endpoint string, body any) (*http.Response, error) {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return nil, ErrControl
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, ErrControl
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(req)
	if err != nil {
		return nil, ErrControl
	}
	return response, nil
}
func (c *Client) json(ctx context.Context, method, endpoint string, body, out any) (int, error) {
	response, err := c.request(ctx, method, endpoint, body)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if out != nil {
		if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(out) != nil {
			return response.StatusCode, ErrControl
		}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.StatusCode, ErrControl
	}
	return response.StatusCode, nil
}
func (c *Client) Ready(ctx context.Context, pid int, identity ...string) error {
	var health struct {
		ProfileHash    string `json:"profileHash"`
		AssetRootHash  string `json:"assetRootHash"`
		Ready          bool   `json:"ready"`
		RendererReady  bool   `json:"rendererReady"`
		PID            int    `json:"pid"`
		WorkerProtocol string `json:"workerProtocol"`
	}
	if _, err := c.json(ctx, "GET", "/v1/health", nil, &health); err != nil || !health.Ready || !health.RendererReady || health.PID != pid || health.WorkerProtocol != "worker-v1" {
		return ErrControl
	}
	if len(identity) == 2 && (health.ProfileHash != identity[0] || health.AssetRootHash != identity[1]) {
		return ErrControl
	}
	return nil
}
func (c *Client) AccountReady(ctx context.Context) error {
	var health struct {
		WorkerBusy bool `json:"workerBusy"`
	}
	if _, err := c.json(ctx, "GET", "/v1/health", nil, &health); err != nil || health.WorkerBusy {
		return ErrControl
	}
	var out struct {
		Accounts []struct {
			ID             string `json:"id"`
			Health         string `json:"health"`
			ActionRequired bool   `json:"actionRequired"`
			Busy           bool   `json:"busy"`
			PredictedQuota *struct {
				RemainingUnits int `json:"remainingUnits"`
			} `json:"predictedQuota"`
		} `json:"accounts"`
	}
	if _, err := c.json(ctx, "GET", "/v1/accounts", nil, &out); err != nil {
		return err
	}
	for _, a := range out.Accounts {
		if a.Busy {
			return ErrControl
		}
	}
	for _, a := range out.Accounts {
		if a.ID == c.account && a.Health == "ok" && !a.ActionRequired && a.PredictedQuota != nil && a.PredictedQuota.RemainingUnits > 0 {
			return nil
		}
	}
	return ErrControl
}
func (c *Client) Submit(ctx context.Context, r doubao.Request) (doubao.Reference, error) {
	if !validRef(r.Key) || r.Kind != doubao.Video || len(r.Images) == 0 || r.Prompt == "" || r.Model == "" || r.DurationSeconds < 4 || r.DurationSeconds > 15 || r.AspectRatio == "" {
		return "", ErrControl
	}
	images := make([]string, 0, len(r.Images))
	for _, image := range r.Images {
		if strings.TrimSpace(image.Path) == "" {
			return "", ErrControl
		}
		images = append(images, image.Path)
	}
	body := map[string]any{"prompt": r.Prompt, "images": images, "model": r.Model, "duration": strconv.Itoa(r.DurationSeconds) + "s", "aspectRatio": r.AspectRatio, "accountId": c.account}
	var out generationDTO
	if _, err := c.json(ctx, "PUT", path(r.Key), body, &out); err != nil || !out.OK || out.Reference != r.Key {
		return "", ErrControl
	}
	return doubao.Reference(out.Reference), nil
}
func (c *Client) Lookup(ctx context.Context, key string) (doubao.LookupResult, error) {
	if !validRef(key) {
		return doubao.LookupResult{State: doubao.Uncertain}, ErrControl
	}
	var out generationDTO
	status, err := c.json(ctx, "GET", path(key), nil, &out)
	if status == 404 && out.Code == "WORKER_NOT_FOUND" && out.Authoritative {
		return doubao.LookupResult{State: doubao.Absent}, nil
	}
	if err != nil || !out.OK || out.Reference != key {
		return doubao.LookupResult{State: doubao.Uncertain}, ErrControl
	}
	return doubao.LookupResult{State: doubao.Found, Reference: doubao.Reference(key)}, nil
}
func (c *Client) Observe(ctx context.Context, reference doubao.Reference) (doubao.Observation, error) {
	ref := string(reference)
	if !validRef(ref) {
		return doubao.Observation{}, ErrControl
	}
	var out generationDTO
	if _, err := c.json(ctx, "POST", path(ref)+"/advance", map[string]any{}, &out); err != nil {
		return doubao.Observation{}, err
	}
	if !out.OK || out.Reference != ref {
		return doubao.Observation{}, ErrControl
	}
	state := doubao.NeedsHuman
	switch out.Status {
	case "queued":
		if !out.Started {
			state = doubao.Accepted
		}
	case "executing", "generating", "manual_submission_observing":
		state = doubao.Generating
	case "done":
		state = doubao.Succeeded
	case "fail", "cancelled":
		state = doubao.Failed
	}
	result := doubao.Observation{State: state}
	for _, a := range out.Artifacts {
		result.Artifacts = append(result.Artifacts, doubao.Artifact{ID: a.ID, Kind: doubao.MediaKind(a.Kind), MediaType: a.MediaType})
	}
	return result, nil
}
func (c *Client) FetchArtifacts(ctx context.Context, reference doubao.Reference, artifacts []doubao.Artifact) ([]doubao.Download, error) {
	ref := string(reference)
	if !validRef(ref) || len(artifacts) != 1 || artifacts[0].Kind != doubao.Video || artifacts[0].ID == "" {
		return nil, ErrControl
	}
	artifact := artifacts[0]
	sum := sha256.Sum256([]byte(ref + "\x00" + artifact.ID))
	key := hex.EncodeToString(sum[:])
	dir := filepath.Join(c.artifacts, ref)
	if os.MkdirAll(dir, 0700) != nil {
		return nil, ErrControl
	}
	target := filepath.Join(dir, key+".mp4")
	response, err := c.request(ctx, "GET", path(ref)+"/artifacts/"+url.PathEscape(artifact.ID), nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || response.ContentLength > 256<<20 {
		return nil, ErrControl
	}
	file, err := os.CreateTemp(dir, ".download-")
	if err != nil {
		return nil, ErrControl
	}
	name := file.Name()
	defer os.Remove(name)
	digest := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, digest), io.LimitReader(response.Body, (256<<20)+1))
	if err == nil && size > 0 && size <= 256<<20 {
		err = file.Sync()
	} else {
		err = ErrControl
	}
	head := make([]byte, 12)
	_, readErr := file.ReadAt(head, 0)
	closeErr := file.Close()
	if err != nil || readErr != nil || closeErr != nil || string(head[4:8]) != "ftyp" {
		return nil, ErrControl
	}
	if os.Rename(name, target) != nil {
		return nil, ErrControl
	}
	folder, err := os.Open(dir)
	if err != nil {
		return nil, ErrControl
	}
	defer folder.Close()
	if folder.Sync() != nil {
		return nil, ErrControl
	}
	return []doubao.Download{{ArtifactID: artifact.ID, LocalPath: target, ChecksumSHA256: hex.EncodeToString(digest.Sum(nil)), SizeBytes: size}}, nil
}

var _ doubao.Generator = (*Client)(nil)
