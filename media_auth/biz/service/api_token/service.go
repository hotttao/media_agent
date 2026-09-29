package apitoken

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	domain "media_agent/media_auth/biz/domain/api_token"
	"media_agent/media_auth/biz/shared/ids"
)

type SecretStore interface {
	Issue(context.Context, IssueSecretCommand) (IssuedSecret, error)
	Verify(context.Context, string) (VerifiedSecret, error)
	Rotate(context.Context, string, []string) (IssuedSecret, error)
	Revoke(context.Context, string) error
	Get(context.Context, string) (SecretMetadata, error)
}

type SecretMetadata struct {
	KeyID, Status string
}

type IssueSecretCommand struct {
	Name, ActorID, RequestID, TTL string
	Scopes                        []string
	Metadata                      map[string]any
}

type IssuedSecret struct {
	KeyID, ActorID, Secret string
	Scopes                 []string
	ExpiresAt              time.Time
}

type VerifiedSecret struct {
	KeyID, ActorID string
	Scopes         []string
	ExpiresAt      time.Time
}

type MembershipValidator func(context.Context, string, string) bool
type ActorValidator func(context.Context, domain.ActorType, string, string) bool
type SubjectValidator func(context.Context, string) bool

type Service struct {
	repo           domain.Repository
	secrets        SecretStore
	membership     MembershipValidator
	actorValidator ActorValidator
	subjectValid   SubjectValidator
	now            func() time.Time
	rotator        Rotator
	issuer         Issuer
}

type Issuer interface {
	Issue(context.Context, domain.Token) (CreateResult, error)
	Recover(context.Context, string, bool) error
}

func (s *Service) SetIssuer(issuer Issuer) { s.issuer = issuer }

type Rotator interface {
	Rotate(context.Context, domain.Token) (CreateResult, error)
}

func (s *Service) SetRotator(rotator Rotator) { s.rotator = rotator }

func NewService(repo domain.Repository, secrets SecretStore) *Service {
	return &Service{repo: repo, secrets: secrets, now: time.Now}
}

func (s *Service) SetMembershipValidator(validator MembershipValidator) { s.membership = validator }
func (s *Service) SetActorValidator(validator ActorValidator)           { s.actorValidator = validator }
func (s *Service) SetSubjectValidator(validator SubjectValidator)       { s.subjectValid = validator }

type CreateCommand struct {
	CreatorUserID, SubjectUserID string
	ActorType                    domain.ActorType
	ActorID                      string
	OrganizationID, Name         string
	Scopes                       []string
	ExpiresAt                    time.Time
	IdempotencyKey               string
	TrustedActorCreation         bool
}

type CreateResult struct {
	Token  domain.Token
	Secret string
}

type Principal struct {
	SubjectUserID, ActorID, CredentialID, OrganizationID string
	ActorType                                            domain.ActorType
	Scopes                                               []string
	ExpiresAt                                            time.Time
}

var (
	ErrInvalidRequest = errors.New("invalid api token request")
	ErrForbidden      = errors.New("api token operation is forbidden")
	ErrInactive       = errors.New("api token is inactive")
)

var allowedScopes = map[string]bool{
	"api.access":                true,
	"agent.gateway":             true,
	"workflow.worker.poll":      true,
	"workflow.worker.heartbeat": true,
	"workflow.worker.complete":  true,
	"workflow.worker.fail":      true,
	"workflow.worker.upload":    true,
}

func (s *Service) Create(ctx context.Context, cmd CreateCommand) (CreateResult, error) {
	cmd.Name = strings.TrimSpace(cmd.Name)
	cmd.ActorID = strings.TrimSpace(cmd.ActorID)
	cmd.SubjectUserID = strings.TrimSpace(cmd.SubjectUserID)
	cmd.IdempotencyKey = strings.TrimSpace(cmd.IdempotencyKey)
	if len(cmd.IdempotencyKey) > 200 {
		return CreateResult{}, ErrInvalidRequest
	}
	if cmd.Name == "" || !cmd.ActorType.Valid() || cmd.ActorID == "" || cmd.CreatorUserID == "" || s.secrets == nil {
		return CreateResult{}, ErrInvalidRequest
	}
	if cmd.ActorType != domain.ActorService && cmd.SubjectUserID == "" {
		return CreateResult{}, ErrInvalidRequest
	}
	if cmd.ActorType != domain.ActorService && (s.subjectValid == nil || !s.subjectValid(ctx, cmd.SubjectUserID)) {
		return CreateResult{}, ErrForbidden
	}
	if !cmd.TrustedActorCreation && (cmd.ActorType != domain.ActorUser || cmd.SubjectUserID != cmd.CreatorUserID || cmd.ActorID != "User:"+cmd.CreatorUserID) {
		return CreateResult{}, ErrForbidden
	}
	if cmd.ActorType == domain.ActorUser && cmd.ActorID != "User:"+cmd.SubjectUserID {
		return CreateResult{}, ErrInvalidRequest
	}
	if cmd.ActorType != domain.ActorUser && cmd.ActorType != domain.ActorService {
		if s.actorValidator == nil || !s.actorValidator(ctx, cmd.ActorType, cmd.ActorID, cmd.SubjectUserID) {
			return CreateResult{}, ErrForbidden
		}
	}
	scopes, err := normalizeScopes(cmd.Scopes)
	if err != nil {
		return CreateResult{}, err
	}
	if cmd.OrganizationID != "" && (s.membership == nil || !s.membership(ctx, cmd.OrganizationID, cmd.SubjectUserID)) {
		return CreateResult{}, ErrForbidden
	}
	now := s.now()
	if cmd.ExpiresAt.IsZero() || !cmd.ExpiresAt.After(now) {
		return CreateResult{}, fmt.Errorf("%w: future expires_at is required", ErrInvalidRequest)
	}
	if s.issuer == nil {
		return CreateResult{}, ErrInactive
	}
	return s.issuer.Issue(ctx, domain.Token{
		ID: ids.New(), SubjectUserID: cmd.SubjectUserID, ActorType: cmd.ActorType, ActorID: cmd.ActorID,
		OrganizationID: cmd.OrganizationID, Name: cmd.Name, Scopes: scopes,
		ExpiresAt: &cmd.ExpiresAt, CreatedBy: cmd.CreatorUserID, CreatedAt: now, ClientRequestKey: cmd.IdempotencyKey,
	})
}

func (s *Service) Verify(ctx context.Context, secret string) (Principal, error) {
	verified, err := s.secrets.Verify(ctx, secret)
	if err != nil {
		return Principal{}, err
	}
	token, err := s.repo.GetByTalosKeyID(ctx, verified.KeyID)
	if err != nil {
		return Principal{}, ErrInactive
	}
	if !token.ActiveAt(s.now()) || token.ActorID != verified.ActorID || !sameScopes(token.Scopes, verified.Scopes) {
		return Principal{}, ErrInactive
	}
	if token.ActorType != domain.ActorService && (s.subjectValid == nil || !s.subjectValid(ctx, token.SubjectUserID)) {
		return Principal{}, ErrForbidden
	}
	if token.OrganizationID != "" && (s.membership == nil || !s.membership(ctx, token.OrganizationID, token.SubjectUserID)) {
		return Principal{}, ErrForbidden
	}
	if token.ActorType != domain.ActorUser && token.ActorType != domain.ActorService {
		if s.actorValidator == nil || !s.actorValidator(ctx, token.ActorType, token.ActorID, token.SubjectUserID) {
			return Principal{}, ErrForbidden
		}
	}
	var expiresAt time.Time
	if token.ExpiresAt != nil {
		expiresAt = *token.ExpiresAt
	}
	return Principal{
		SubjectUserID: token.SubjectUserID, ActorType: token.ActorType, ActorID: token.ActorID,
		CredentialID: token.ID, OrganizationID: token.OrganizationID, Scopes: append([]string(nil), token.Scopes...), ExpiresAt: expiresAt,
	}, nil
}

func (s *Service) List(ctx context.Context, subjectUserID string) ([]domain.Token, error) {
	return s.repo.ListBySubject(ctx, subjectUserID)
}

func (s *Service) Revoke(ctx context.Context, subjectUserID, id string) error {
	token, err := s.repo.MarkRevoking(ctx, id, subjectUserID)
	if err != nil {
		return err
	}
	if token.Status == domain.StatusRevoked {
		return nil
	}
	if token.Status == domain.StatusIssuanceCleanup {
		if s.issuer == nil {
			return ErrInactive
		}
		return s.issuer.Recover(ctx, token.ID, true)
	}
	if err := s.secrets.Revoke(ctx, token.TalosKeyID); err != nil {
		// Revoke may have succeeded while its response was lost. Only an
		// authoritative terminal status permits the local operation to finish.
		metadata, queryErr := s.secrets.Get(ctx, token.TalosKeyID)
		if queryErr != nil || metadata.KeyID != token.TalosKeyID || metadata.Status != "KEY_STATUS_REVOKED" {
			return err
		}
	}
	return s.repo.MarkRevoked(ctx, id, s.now())
}

// Rotate replaces the Talos credential while preserving the local API Token
// identity and authorization metadata. Talos invalidates the old secret as part
// of rotation; the replacement secret is returned once and is never persisted.
func (s *Service) Rotate(ctx context.Context, subjectUserID, id string, requestedExpiry time.Time) (CreateResult, error) {
	token, err := s.repo.Get(ctx, id)
	if err != nil {
		return CreateResult{}, err
	}
	if token.SubjectUserID != subjectUserID || !token.ActiveAt(s.now()) {
		return CreateResult{}, ErrForbidden
	}
	if !requestedExpiry.IsZero() {
		return CreateResult{}, fmt.Errorf("%w: changing expiry during rotation is not supported by Talos", ErrInvalidRequest)
	}
	if s.rotator == nil {
		return CreateResult{}, ErrInactive
	}
	return s.rotator.Rotate(ctx, token)
}

func normalizeScopes(input []string) ([]string, error) {
	seen := make(map[string]bool, len(input))
	out := make([]string, 0, len(input))
	for _, scope := range input {
		scope = strings.TrimSpace(scope)
		if scope == "" || !allowedScopes[scope] {
			return nil, fmt.Errorf("%w: unsupported scope %q", ErrInvalidRequest, scope)
		}
		if !seen[scope] {
			seen[scope] = true
			out = append(out, scope)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: at least one scope is required", ErrInvalidRequest)
	}
	sort.Strings(out)
	return out, nil
}

func sameScopes(a, b []string) bool {
	a = append([]string(nil), a...)
	b = append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	return strings.Join(a, "\x00") == strings.Join(b, "\x00")
}
