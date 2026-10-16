package docker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"k8s.io/apimachinery/pkg/api/resource"
	contract "media_agent/hertz_infra/agentworkload"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

const digestLabel = contract.Group + "/spec-digest"
const proofLabel = contract.Group + "/bootstrap-hash"

type Policy struct {
	HTTPProxy, HTTPSProxy, NoProxy                                                     string
	EnvironmentID, Network, Root, PublicKeysFile, DatabaseURL, RabbitURL, SharedSecret string
	Images                                                                             map[string]string
	AgentTokens                                                                        map[string]string
	Command, HealthCommand                                                             []string
}
type State struct {
	Spec              contract.Spec
	Version, Revision int64
}
type Service struct {
	Engine *Engine
	Policy Policy
}
type Result struct {
	State     State
	Container *Inspection
}

func (s *Service) validate(spec contract.Spec) error {
	if spec.Validate() != nil || spec.Driver != "docker" || spec.EnvironmentID != s.Policy.EnvironmentID || spec.Replicas > 1 || spec.SharedSecret != s.Policy.SharedSecret || s.Policy.Images[spec.AgentType] != spec.Image || s.Policy.Network == "" || !filepath.IsAbs(s.Policy.Root) || !filepath.IsAbs(s.Policy.PublicKeysFile) {
		return ErrInvalid
	}
	for _, quantity := range []string{spec.CPU, spec.Memory} {
		v, e := resource.ParseQuantity(quantity)
		if e != nil || v.Sign() <= 0 {
			return ErrInvalid
		}
	}
	n := contract.Name(spec.EnvironmentID, spec.WorkspaceID)
	if spec.IdentitySecret != n+"-identity" || spec.ClaimName != n+"-workspace" {
		return ErrInvalid
	}
	return nil
}
func atomicFile(path string, data []byte, mode os.FileMode) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".pending-")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(data)
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	if e = os.Rename(tmp, path); e != nil {
		return e
	}
	dir, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer dir.Close()
	return dir.Sync()
}
func readRegular(path string) ([]byte, error) {
	info, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() {
		return nil, ErrConflict
	}
	return os.ReadFile(path)
}
func (s *Service) paths(spec contract.Spec) (string, string, string) {
	root := filepath.Join(s.Policy.Root, contract.Name(spec.EnvironmentID, spec.WorkspaceID))
	return root, filepath.Join(root, "workspace"), filepath.Join(root, "identity-token")
}
func digest(spec contract.Spec) string {
	spec.Replicas = 0
	body, _ := json.Marshal(spec)
	v := sha256.Sum256(body)
	return hex.EncodeToString(v[:])
}
func (s *Service) verify(v *Inspection, spec contract.Spec, proof []byte) error {
	if v.Config.User != fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()) || v.Config.Image != spec.Image || v.HostConfig.Privileged || v.HostConfig.NetworkMode != s.Policy.Network || v.HostConfig.RestartPolicy.Name != "unless-stopped" || v.Config.Labels[digestLabel] != digest(spec) {
		return ErrConflict
	}
	for k, val := range spec.Labels() {
		if v.Config.Labels[k] != val {
			return ErrConflict
		}
	}
	hash := sha256.Sum256(proof)
	if v.Config.Labels[proofLabel] != hex.EncodeToString(hash[:]) {
		return ErrConflict
	}
	_, workspace, token := s.paths(spec)
	expected := map[string]struct {
		source string
		rw     bool
	}{"/workspace": {workspace, true}, "/var/run/secrets/media-agent/identity-token": {token, false}, "/run/secrets/runtime-public-keys.json": {s.Policy.PublicKeysFile, false}}
	if len(v.Mounts) != len(expected) {
		return ErrConflict
	}
	for _, mount := range v.Mounts {
		want, ok := expected[mount.Destination]
		if !ok || mount.Type != "bind" || mount.Source != want.source || mount.RW != want.rw {
			return ErrConflict
		}
	}
	return nil
}
func (s *Service) Ensure(ctx context.Context, wanted State, expected string, apiToken string, observe bool) (*Result, error) {
	if !observe && wanted.Spec.Replicas != 1 {
		return nil, ErrNotImplemented
	}
	if e := s.validate(wanted.Spec); e != nil {
		return nil, e
	}
	root, workspace, proofPath := s.paths(wanted.Spec)
	if e := os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	info, e := os.Lstat(root)
	if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrConflict
	}
	lock, e := os.OpenFile(filepath.Join(root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	defer lock.Close()
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); e != nil {
		return nil, e
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	statePath := filepath.Join(root, "state.json")
	var previous State
	body, e := readRegular(statePath)
	exists := e == nil
	if e != nil && !errors.Is(e, os.ErrNotExist) {
		return nil, e
	}
	if exists && json.Unmarshal(body, &previous) != nil {
		return nil, ErrConflict
	}
	if observe {
		if !exists {
			return nil, ErrNotFound
		}
		proof, e := readRegular(proofPath)
		if e != nil {
			return nil, e
		}
		v, e := s.Engine.Inspect(ctx, contract.Name(wanted.Spec.EnvironmentID, wanted.Spec.WorkspaceID))
		if e != nil {
			return nil, e
		}
		if e = s.verify(v, previous.Spec, proof); e != nil {
			return nil, e
		}
		return &Result{previous, v}, nil
	}
	if wanted.Version < 1 || exists && (wanted.Version < previous.Version || wanted.Version != previous.Version && expected != strconv.FormatInt(previous.Revision, 10)) || !exists && expected != "" {
		return nil, ErrConflict
	}
	if exists && digest(previous.Spec) != digest(wanted.Spec) {
		return nil, ErrConflict
	}
	proof, e := readRegular(proofPath)
	if errors.Is(e, os.ErrNotExist) {
		proof = make([]byte, 32)
		if _, e = rand.Read(proof); e != nil {
			return nil, e
		}
		proof = []byte(hex.EncodeToString(proof))
		if e = atomicFile(proofPath, proof, 0400); e != nil {
			return nil, e
		}
	} else if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(workspace, 0755); e != nil {
		return nil, e
	}
	if info, e := os.Lstat(workspace); e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrConflict
	}
	name := contract.Name(wanted.Spec.EnvironmentID, wanted.Spec.WorkspaceID)
	v, e := s.Engine.Inspect(ctx, name)
	if errors.Is(e, ErrNotFound) {
		if apiToken == "" {
			apiToken = s.Policy.AgentTokens[wanted.Spec.AgentConfigID]
		}
		if apiToken == "" || s.Policy.DatabaseURL == "" || s.Policy.RabbitURL == "" {
			return nil, ErrInvalid
		}
		if e = s.Engine.Create(ctx, name, s.createBody(wanted.Spec, proof, workspace, proofPath, apiToken)); e != nil {
			var api EngineError
			if !errors.As(e, &api) || api.Status != 409 {
				return nil, e
			}
		}
		v, e = s.Engine.Inspect(ctx, name)
	}
	if e != nil {
		return nil, e
	}
	if e = s.verify(v, wanted.Spec, proof); e != nil {
		return nil, e
	}
	// A same-version retry retains the original revision and files. Persist the
	// create projection before start; a failed start is repaired by the next Ensure.
	wanted.Revision = previous.Revision
	if !exists || wanted.Version != previous.Version {
		wanted.Revision++
		encoded, _ := json.Marshal(wanted)
		if e = atomicFile(statePath, encoded, 0600); e != nil {
			return nil, e
		}
	}
	if !v.State.Running {
		e = s.Engine.Start(ctx, v.ID)
	}
	if e != nil {
		return nil, e
	}
	v, e = s.Engine.Inspect(ctx, name)
	if e != nil {
		return nil, e
	}
	return &Result{wanted, v}, nil
}
func (s *Service) createBody(spec contract.Spec, proof []byte, workspace, token, apiToken string) map[string]any {
	labels := spec.Labels()
	labels[digestLabel] = digest(spec)
	hash := sha256.Sum256(proof)
	labels[proofLabel] = hex.EncodeToString(hash[:])
	env := []string{"PORT=" + fmt.Sprint(spec.Port), "WORKSPACE_DIR=/workspace", "RUNTIME_WORKSPACE_ID=" + spec.WorkspaceID, "AGENT_INSTANCE_ENABLED=true", "AGENT_METADATA_ENABLED=true", "AGENT_CONFIG_ID=" + spec.AgentConfigID, "AGENT_GATEWAY_URL=" + spec.GatewayURL, "AGENT_ENVIRONMENT_ID=" + spec.EnvironmentID, "AGENT_REPLICA_ID=0", "POD_NAME=" + contract.Name(spec.EnvironmentID, spec.WorkspaceID), "AGENT_BOOTSTRAP_PROOF_FILE=/var/run/secrets/media-agent/identity-token", "RUNTIME_PUBLIC_KEYS_FILE=/run/secrets/runtime-public-keys.json", "AGENT_API_TOKEN=" + apiToken, "MEDIA_AGENTS_DB_URL=" + s.Policy.DatabaseURL, "MEDIA_AGENTS_RABBITMQ_URL=" + s.Policy.RabbitURL}
	for _, proxy := range []struct{ upper, lower, value string }{
		{"HTTP_PROXY", "http_proxy", s.Policy.HTTPProxy},
		{"HTTPS_PROXY", "https_proxy", s.Policy.HTTPSProxy},
		{"NO_PROXY", "no_proxy", s.Policy.NoProxy},
	} {
		if proxy.value != "" {
			env = append(env, proxy.upper+"="+proxy.value, proxy.lower+"="+proxy.value)
		}
	}
	if spec.SessionExchange != "" {
		env = append(env, "MEDIA_AGENTS_SESSION_EXCHANGE="+spec.SessionExchange)
	}
	if spec.MetadataExchange != "" {
		env = append(env, "AGENT_METADATA_EXCHANGE="+spec.MetadataExchange)
	}
	health := s.Policy.HealthCommand
	if len(health) == 0 {
		health = []string{"CMD", "python3", "-c", fmt.Sprintf("import urllib.request; urllib.request.urlopen('http://127.0.0.1:%d/health', timeout=3).read()", spec.Port)}
	}
	mount := func(source, target string, readOnly bool) map[string]any {
		return map[string]any{"Type": "bind", "Source": source, "Target": target, "ReadOnly": readOnly}
	}
	cpu, memory := resource.MustParse(spec.CPU), resource.MustParse(spec.Memory)
	body := map[string]any{"User": fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), "Image": spec.Image, "Labels": labels, "Env": env, "ExposedPorts": map[string]any{fmt.Sprintf("%d/tcp", spec.Port): map[string]any{}}, "Healthcheck": map[string]any{"Test": health, "Interval": int64(5e9), "Timeout": int64(3e9), "StartPeriod": int64(60e9), "Retries": 3}, "HostConfig": map[string]any{"NetworkMode": s.Policy.Network, "NanoCpus": cpu.MilliValue() * 1000000, "Memory": memory.Value(), "RestartPolicy": map[string]any{"Name": "unless-stopped"}, "Mounts": []any{mount(workspace, "/workspace", false), mount(token, "/var/run/secrets/media-agent/identity-token", true), mount(s.Policy.PublicKeysFile, "/run/secrets/runtime-public-keys.json", true)}, "CapDrop": []string{"ALL"}, "SecurityOpt": []string{"no-new-privileges:true"}}}
	if len(s.Policy.Command) > 0 {
		body["Cmd"] = s.Policy.Command
	}
	return body
}
func Generation(v *Inspection) string { return v.ID + "@" + v.State.StartedAt }
func Ready(v *Inspection) bool {
	return v.State.Running && v.State.Health != nil && v.State.Health.Status == "healthy"
}
func (s *Service) Identity(ctx context.Context, workspace, config, uid, generation, proof string, observation bool) (*Result, bool, error) {
	root := filepath.Join(s.Policy.Root, contract.Name(s.Policy.EnvironmentID, workspace))
	body, e := readRegular(filepath.Join(root, "state.json"))
	if e != nil {
		return nil, false, ErrNotFound
	}
	var state State
	if json.Unmarshal(body, &state) != nil || state.Spec.AgentConfigID != config || state.Spec.WorkspaceID != workspace {
		return nil, false, ErrConflict
	}
	result, e := s.Ensure(ctx, state, "", "", true)
	if e != nil {
		return nil, false, e
	}
	if uid != "" && result.Container.ID != uid {
		return nil, false, ErrConflict
	}
	if !observation {
		expected, e := readRegular(filepath.Join(root, "identity-token"))
		if e != nil || len(proof) == 0 || subtle.ConstantTimeCompare(expected, []byte(proof)) != 1 || !result.Container.State.Running || state.Spec.Replicas != 1 {
			return nil, false, ErrConflict
		}
	}
	started, se := time.Parse(time.RFC3339Nano, result.Container.State.StartedAt)
	finished, fe := time.Parse(time.RFC3339Nano, result.Container.State.FinishedAt)
	terminated := generation == Generation(result.Container) && !result.Container.State.Running && !result.Container.State.Restarting && se == nil && fe == nil && !finished.IsZero() && !finished.Before(started)
	return result, terminated, nil
}
