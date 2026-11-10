package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	d "media_agent/media_auth/biz/domain/application"
	"strings"
	"time"
)

type Service struct {
	business d.BusinessRepository
	teams    d.TeamPermissions
	repo     d.Repository
	tx       d.TxRunner
}

func NewService(repo d.Repository, tx d.TxRunner) *Service { return &Service{repo: repo, tx: tx} }
func random(prefix string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b), nil
}
func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func (s *Service) Create(ctx context.Context, userID, role, id, name string) (d.Application, string, error) {
	if role != "admin" || userID == "" {
		return d.Application{}, "", d.ErrForbidden
	}
	if !d.ValidID(id) || strings.TrimSpace(name) == "" || len(name) > 256 {
		return d.Application{}, "", d.ErrInvalid
	}
	secret, err := random("mas_")
	if err != nil {
		return d.Application{}, "", err
	}
	a, err := s.repo.Create(ctx, d.Application{ID: id, Name: name, SecretHash: digest(secret), Status: "active", CredentialVersion: 1, CreatedBy: userID})
	if err != nil {
		return d.Application{}, "", err
	}
	return a, secret, nil
}
func (s *Service) List(ctx context.Context, role string) ([]d.Application, error) {
	if role != "admin" {
		return nil, d.ErrForbidden
	}
	return s.repo.List(ctx)
}
func (s *Service) SetStatus(ctx context.Context, role, id, status string) (d.Application, error) {
	if role != "admin" {
		return d.Application{}, d.ErrForbidden
	}
	if status != "active" && status != "disabled" {
		return d.Application{}, d.ErrInvalid
	}
	var a d.Application
	err := s.tx.RunInTx(ctx, func(ctx context.Context) error {
		current, e := s.repo.Get(ctx, id, true)
		if e != nil {
			return e
		}
		a = current
		if current.Status == status {
			return nil
		}
		a, e = s.repo.SetStatus(ctx, id, status)
		return e
	})
	return a, err
}
func (s *Service) Rotate(ctx context.Context, role, id string) (d.Application, string, error) {
	if role != "admin" {
		return d.Application{}, "", d.ErrForbidden
	}
	secret, err := random("mas_")
	if err != nil {
		return d.Application{}, "", err
	}
	var a d.Application
	err = s.tx.RunInTx(ctx, func(ctx context.Context) error {
		if _, e := s.repo.Get(ctx, id, true); e != nil {
			return e
		}
		var e error
		a, e = s.repo.Rotate(ctx, id, digest(secret))
		return e
	})
	if err != nil {
		return d.Application{}, "", err
	}
	return a, secret, nil
}

// AuthenticateLocked must run within the provisioning transaction; the lock also serializes rotations and disable.
func (s *Service) AuthenticateLocked(ctx context.Context, id, secret string) (d.Application, error) {
	if !d.ValidID(id) || len(secret) > 256 || secret == "" {
		return d.Application{}, d.ErrUnauthorized
	}
	a, err := s.repo.Get(ctx, id, true)
	if err == d.ErrNotFound {
		return d.Application{}, d.ErrUnauthorized
	}
	if err != nil {
		return d.Application{}, err
	}
	hash := digest(secret)
	if a.Status != "active" || subtle.ConstantTimeCompare([]byte(a.SecretHash), []byte(hash)) != 1 {
		return d.Application{}, d.ErrUnauthorized
	}
	return a, nil
}

// Issue receives versioned, locked facts from the cross-domain workflow.
func (s *Service) Issue(ctx context.Context, a d.Application, userID, externalID string, authVersion int64) (string, error) {
	if a.Status != "active" || userID == "" || authVersion < 1 {
		return "", d.ErrForbidden
	}
	raw, err := random("mac_")
	if err != nil {
		return "", err
	}
	err = s.repo.SaveCredential(ctx, d.Credential{Hash: digest(raw), ApplicationID: a.ID, UserID: userID, ExternalID: externalID, ApplicationVersion: a.CredentialVersion, AuthVersion: authVersion, ExpiresAt: time.Now().Add(d.AccessTTL)})
	if err != nil {
		return "", err
	}
	return raw, nil
}
func (s *Service) Profile(ctx context.Context, raw string) (d.Profile, error) {
	if !strings.HasPrefix(raw, "mac_") || len(raw) != 68 {
		return d.Profile{}, d.ErrUnauthorized
	}
	return s.repo.ResolveCredential(ctx, digest(raw))
}
