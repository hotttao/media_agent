package plugin_binding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/oklog/ulid/v2"
	"media_agent/hertz_infra/mcpupstream"
	d "media_agent/media_plugin/biz/domain/plugin_binding"
	"net/http"
	"slices"
	"strings"
	"time"
)

type TxRunner interface {
	RunInTx(context.Context, func(context.Context) error) error
}
type Service struct {
	repo d.Repository
	tx   TxRunner
}

func New(repo d.Repository, tx TxRunner) *Service { return &Service{repo, tx} }
func (s *Service) Get(ctx context.Context, id string) (*d.Binding, error) {
	return s.repo.Get(ctx, id, false)
}
func (s *Service) Version(ctx context.Context, id string, v int64) (*d.Binding, error) {
	if v < 1 {
		return nil, d.ErrInvalid
	}
	return s.repo.Version(ctx, id, v)
}
func (s *Service) List(ctx context.Context, ws string) ([]d.Binding, error) {
	return s.repo.List(ctx, ws)
}
func (s *Service) Versions(ctx context.Context, id string) ([]d.Binding, error) {
	return s.repo.Versions(ctx, id)
}
func (s *Service) Credential(ctx context.Context, id, binding string) ([]byte, error) {
	return s.repo.Credential(ctx, id, binding)
}

func ValidCredentialHeader(name string) bool { return mcpupstream.ValidCredentialHeader(name) }
func normalize(values []string) ([]string, error) {
	if values == nil {
		return []string{"*"}, nil
	}
	if len(values) > 256 {
		return nil, d.ErrInvalid
	}
	out := slices.Clone(values)
	for _, v := range out {
		if v == "" || len(v) > 2048 || strings.ContainsAny(v, "\r\n\x00") {
			return nil, d.ErrInvalid
		}
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}
func (s *Service) Put(ctx context.Context, v d.Binding, expected int64, encrypted *[]byte) (*d.Binding, error) {
	if v.WorkspaceID == "" || v.DefinitionID == "" || v.DefinitionID == "platform" || v.DefinitionVersion < 1 || expected < 0 || v.UpdatedBy == "" {
		return nil, d.ErrInvalid
	}
	var err error
	for _, values := range []*[]string{&v.AllowedMethods, &v.AllowedTools, &v.AllowedPrompts, &v.AllowedResources} {
		*values, err = normalize(*values)
		if err != nil {
			return nil, err
		}
	}
	for _, method := range v.AllowedMethods {
		if !slices.Contains([]string{"*", "tools/list", "tools/call", "prompts/list", "prompts/get", "resources/list", "resources/templates/list", "resources/read"}, method) {
			return nil, d.ErrInvalid
		}
	}
	if v.ID == "" {
		if expected != 0 {
			return nil, d.ErrInvalid
		}
		v.ID = ulid.Make().String()
	}
	err = s.tx.RunInTx(ctx, func(ctx context.Context) error {
		previous, e := s.repo.Get(ctx, v.ID, true)
		if e != nil && e != d.ErrNotFound {
			return e
		}
		now := time.Now().UTC()
		if previous == nil {
			if expected != 0 {
				return d.ErrConflict
			}
			v.CreatedAt = now
		} else {
			if previous.WorkspaceID != v.WorkspaceID || previous.DefinitionID != v.DefinitionID {
				return d.ErrForbidden
			}
			if previous.Version != expected {
				return d.ErrConflict
			}
			v.CreatedAt = previous.CreatedAt
			v.CredentialID = previous.CredentialID
		}
		var credential *d.Credential
		if encrypted != nil {
			v.CredentialID = ""
			if len(*encrypted) > 0 {
				v.CredentialID = ulid.Make().String()
				credential = &d.Credential{ID: v.CredentialID, BindingID: v.ID, Ciphertext: *encrypted}
			}
		}
		v.Version = expected + 1
		v.UpdatedAt = now
		v.Checksum = ""
		raw, e := json.Marshal(v)
		if e != nil {
			return e
		}
		hash := sha256.Sum256(raw)
		v.Checksum = hex.EncodeToString(hash[:])
		return s.repo.Save(ctx, &v, expected, credential)
	})
	if err != nil {
		return nil, err
	}
	return &v, nil
}
func CanonicalHeader(name string) string { return http.CanonicalHeaderKey(name) }
