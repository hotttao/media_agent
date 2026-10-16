package model_connection

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	d "media_agent/media_control/biz/domain/model_connection"
	"media_agent/media_control/biz/shared/secret"
	"strings"
	"time"
)

type Service struct {
	repo   d.Repository
	cipher *secret.Cipher
}

func New(r d.Repository, c *secret.Cipher) *Service { return &Service{r, c} }

type envelope struct {
	ID, Owner, Auth   string
	CredentialVersion int64
}

func (s *Service) seal(v *d.Connection, auth string, revision int64) error {
	b, _ := json.Marshal(envelope{ID: v.ID, Owner: v.OwnerUserID, Auth: auth, CredentialVersion: revision})
	var e error
	v.EncryptedAuth, e = s.cipher.Encrypt(string(b))
	return e
}
func (s *Service) Create(ctx context.Context, owner, name, auth string) (*d.Connection, error) {
	name = strings.TrimSpace(name)
	if owner == "" || name == "" || len(name) > 160 {
		return nil, d.ErrInvalid
	}
	if e := d.ValidateAuth(auth); e != nil {
		return nil, e
	}
	now := time.Now().UTC()
	v := &d.Connection{ID: uuid.NewString(), OwnerUserID: owner, Name: name, Kind: "codex_auth_json", Status: "active", Version: 1, CreatedAt: now, UpdatedAt: now}
	if e := s.seal(v, auth, 1); e != nil {
		return nil, e
	}
	return v, s.repo.Save(ctx, v, 0)
}
func (s *Service) List(ctx context.Context, owner string) ([]d.Connection, error) {
	if owner == "" {
		return nil, d.ErrNotFound
	}
	return s.repo.List(ctx, owner)
}
func (s *Service) Get(ctx context.Context, id, owner string) (*d.Connection, error) {
	v, e := s.repo.Get(ctx, id)
	if e != nil {
		return nil, e
	}
	if owner == "" || v.OwnerUserID != owner {
		return nil, d.ErrNotFound
	}
	return v, nil
}
func (s *Service) Update(ctx context.Context, id, owner, name, auth string, expected int64, revoke bool) (*d.Connection, error) {
	v, e := s.Get(ctx, id, owner)
	if e != nil {
		return nil, e
	}
	if expected < 1 || expected != v.Version {
		return nil, d.ErrConflict
	}
	if v.Status != "active" {
		return nil, d.ErrRevoked
	}
	if revoke {
		v.Status = "revoked"
		v.EncryptedAuth = nil
	} else {
		name = strings.TrimSpace(name)
		if name == "" || len(name) > 160 {
			return nil, d.ErrInvalid
		}
		v.Name = name
		if auth != "" {
			if e := d.ValidateAuth(auth); e != nil {
				return nil, e
			}
			if e := s.seal(v, auth, v.Version+1); e != nil {
				return nil, e
			}
		}
	}
	v.Version++
	v.UpdatedAt = time.Now().UTC()
	return v, s.repo.Save(ctx, v, expected)
}
func (s *Service) ValidateBinding(ctx context.Context, id, owner, agent string) error {
	if agent != "codex" {
		return d.ErrInvalid
	}
	v, e := s.Get(ctx, id, owner)
	if e != nil {
		return e
	}
	if v.Status != "active" {
		return d.ErrRevoked
	}
	return nil
}

// Resolve is internal-only after the caller authorizes a fixed Workspace configuration.
func (s *Service) Resolve(ctx context.Context, id string) (d.Secret, error) {
	v, e := s.repo.Get(ctx, id)
	if e != nil {
		return d.Secret{}, e
	}
	if v.Status != "active" {
		return d.Secret{}, d.ErrRevoked
	}
	plain, e := s.cipher.Decrypt(v.EncryptedAuth)
	if e != nil {
		return d.Secret{}, d.ErrInvalid
	}
	var b envelope
	if json.Unmarshal([]byte(plain), &b) != nil || b.ID != v.ID || b.Owner != v.OwnerUserID || d.ValidateAuth(b.Auth) != nil {
		return d.Secret{}, d.ErrInvalid
	}
	if b.CredentialVersion == 0 {
		b.CredentialVersion = 1
	} // Original imported records used the initial credential revision.
	return d.Secret{ID: v.ID, Version: b.CredentialVersion, AuthJSON: b.Auth}, nil
}
