package apitoken

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "media_agent/media_auth/biz/domain/api_token"
)

type fakeRepo struct {
	created        domain.Token
	byKey          map[string]domain.Token
	createErr      error
	revoking       domain.Token
	revoked        bool
	replaceErr     error
	markRevokedErr error
}

func (r *fakeRepo) Create(_ context.Context, token domain.Token) (domain.Token, error) {
	if r.createErr != nil {
		return domain.Token{}, r.createErr
	}
	r.created = token
	if r.byKey == nil {
		r.byKey = map[string]domain.Token{}
	}
	r.byKey[token.TalosKeyID] = token
	return token, nil
}
func (r *fakeRepo) Get(context.Context, string) (domain.Token, error) { return r.created, nil }
func (r *fakeRepo) GetByTalosKeyID(_ context.Context, key string) (domain.Token, error) {
	token, ok := r.byKey[key]
	if !ok {
		return domain.Token{}, domain.ErrNotFound
	}
	return token, nil
}
func (r *fakeRepo) ListBySubject(context.Context, string) ([]domain.Token, error) {
	return []domain.Token{r.created}, nil
}
func (r *fakeRepo) MarkRevoking(context.Context, string, string) (domain.Token, error) {
	return r.revoking, nil
}
func (r *fakeRepo) MarkRevoked(context.Context, string, time.Time) error {
	if r.markRevokedErr != nil {
		return r.markRevokedErr
	}
	r.revoked = true
	return nil
}
func (r *fakeRepo) ReplaceTalosKey(context.Context, string, string, string, *time.Time, time.Time) (domain.Token, error) {
	if r.replaceErr != nil {
		return domain.Token{}, r.replaceErr
	}
	return r.created, nil
}

type fakeSecrets struct {
	issued              IssuedSecret
	verified            VerifiedSecret
	revokedKey          string
	issueErr, revokeErr error
	rotated             IssuedSecret
	rotateErr           error
	metadata            SecretMetadata
	getErr              error
}

func (f *fakeSecrets) Get(context.Context, string) (SecretMetadata, error) {
	return f.metadata, f.getErr
}

func (f *fakeSecrets) Issue(context.Context, IssueSecretCommand) (IssuedSecret, error) {
	return f.issued, f.issueErr
}
func (f *fakeSecrets) Verify(context.Context, string) (VerifiedSecret, error) { return f.verified, nil }
func (f *fakeSecrets) Rotate(context.Context, string, []string) (IssuedSecret, error) {
	return f.rotated, f.rotateErr
}
func (f *fakeSecrets) Revoke(_ context.Context, key string) error {
	f.revokedKey = key
	return f.revokeErr
}

type fakeRotator struct {
	token  domain.Token
	result CreateResult
	err    error
}

type fakeIssuer struct {
	token       domain.Token
	err         error
	recoveredID string
}

func (f *fakeIssuer) Issue(_ context.Context, token domain.Token) (CreateResult, error) {
	f.token = token
	if f.err != nil {
		return CreateResult{}, f.err
	}
	token.Status, token.TalosKeyID = domain.StatusActive, "talos-1"
	return CreateResult{Token: token, Secret: "once"}, nil
}
func (f *fakeIssuer) Recover(_ context.Context, id string, force bool) error {
	if !force {
		return errors.New("revocation must claim cleanup immediately")
	}
	f.recoveredID = id
	return f.err
}

func (r *fakeRotator) Rotate(_ context.Context, token domain.Token) (CreateResult, error) {
	r.token = token
	return r.result, r.err
}

func TestCreateUserAPITokenAndVerify(t *testing.T) {
	now := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	expires := now.Add(24 * time.Hour)
	repo := &fakeRepo{}
	secrets := &fakeSecrets{issued: IssuedSecret{KeyID: "talos-1", ActorID: "User:u1", Secret: "once", Scopes: []string{"api.access"}, ExpiresAt: expires}}
	service := NewService(repo, secrets)
	service.SetSubjectValidator(func(context.Context, string) bool { return true })
	service.now = func() time.Time { return now }
	issuer := &fakeIssuer{}
	service.SetIssuer(issuer)

	result, err := service.Create(context.Background(), CreateCommand{
		CreatorUserID: "u1", SubjectUserID: "u1", ActorType: domain.ActorUser, ActorID: "User:u1",
		Name: "e2e", Scopes: []string{"api.access"}, ExpiresAt: expires, IdempotencyKey: "req-1",
	})
	if err != nil || result.Secret != "once" || result.Token.TalosKeyID != "talos-1" {
		t.Fatalf("Create() = %#v, %v", result, err)
	}
	if issuer.token.ClientRequestKey != "req-1" || issuer.token.Status != "" || issuer.token.TalosKeyID != "" {
		t.Fatal("validated creation was not delegated as an unsigned draft")
	}
	repo.byKey = map[string]domain.Token{"talos-1": result.Token}
	secrets.verified = VerifiedSecret{KeyID: "talos-1", ActorID: "User:u1", Scopes: []string{"api.access"}, ExpiresAt: expires}
	principal, err := service.Verify(context.Background(), "once")
	if err != nil || principal.SubjectUserID != "u1" || principal.ActorID != "User:u1" || principal.CredentialID == "" {
		t.Fatalf("Verify() = %#v, %v", principal, err)
	}
}

func TestCreateDelegatesFailureWithoutIssuingOutsideWorkflow(t *testing.T) {
	expires := time.Now().Add(time.Hour)
	repo := &fakeRepo{createErr: errors.New("database unavailable")}
	secrets := &fakeSecrets{issued: IssuedSecret{KeyID: "orphan", ActorID: "User:u1", Secret: "never-return", Scopes: []string{"api.access"}, ExpiresAt: expires}}
	service := NewService(repo, secrets)
	service.SetSubjectValidator(func(context.Context, string) bool { return true })
	issuer := &fakeIssuer{err: domain.ErrConflict}
	service.SetIssuer(issuer)
	_, err := service.Create(context.Background(), CreateCommand{
		CreatorUserID: "u1", SubjectUserID: "u1", ActorType: domain.ActorUser, ActorID: "User:u1",
		Name: "test", Scopes: []string{"api.access"}, ExpiresAt: expires,
	})
	if !errors.Is(err, domain.ErrConflict) || secrets.revokedKey != "" || issuer.token.ID == "" {
		t.Fatalf("Create() error=%v revoked=%q", err, secrets.revokedKey)
	}
}

func TestRevokeUnfinishedIssuanceUsesDurableCleanupWorkflow(t *testing.T) {
	repo := &fakeRepo{revoking: domain.Token{ID: "pending", Status: domain.StatusIssuanceCleanup}}
	secrets := &fakeSecrets{}
	svc := NewService(repo, secrets)
	issuer := &fakeIssuer{err: errors.New("upstream unavailable")}
	svc.SetIssuer(issuer)
	if err := svc.Revoke(t.Context(), "owner", "pending"); err == nil || issuer.recoveredID != "pending" || repo.revoked || secrets.revokedKey != "" {
		t.Fatal("issuance cleanup bypassed workflow or lost failure")
	}
}

func TestCreateRejectsUntrustedDelegatedActor(t *testing.T) {
	service := NewService(&fakeRepo{}, &fakeSecrets{})
	service.SetSubjectValidator(func(context.Context, string) bool { return true })
	_, err := service.Create(context.Background(), CreateCommand{
		CreatorUserID: "u1", SubjectUserID: "u1", ActorType: domain.ActorPersonalWorker, ActorID: "Worker:w1",
		Name: "worker", Scopes: []string{"workflow.worker.poll"}, ExpiresAt: time.Now().Add(time.Hour),
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestVerifyRejectsTalosMetadataMismatch(t *testing.T) {
	expires := time.Now().Add(time.Hour)
	token := domain.Token{ID: "api-1", TalosKeyID: "key-1", SubjectUserID: "u1", ActorType: domain.ActorUser, ActorID: "User:u1", Scopes: []string{"api.access"}, Status: domain.StatusActive, ExpiresAt: &expires}
	repo := &fakeRepo{byKey: map[string]domain.Token{"key-1": token}}
	secrets := &fakeSecrets{verified: VerifiedSecret{KeyID: "key-1", ActorID: "User:attacker", Scopes: []string{"api.access"}, ExpiresAt: expires}}
	service := NewService(repo, secrets)
	service.SetSubjectValidator(func(context.Context, string) bool { return true })
	_, err := service.Verify(context.Background(), "redacted")
	if !errors.Is(err, ErrInactive) {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestRotateDelegatesAuthorizedTokenMetadata(t *testing.T) {
	now := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	expires := now.Add(24 * time.Hour)
	token := domain.Token{
		ID: "api-1", TalosKeyID: "old-key", SubjectUserID: "u1", ActorType: domain.ActorUser,
		ActorID: "User:u1", Scopes: []string{"api.access"}, Status: domain.StatusActive, ExpiresAt: &expires,
	}
	repo := &fakeRepo{created: token}
	secrets := &fakeSecrets{rotated: IssuedSecret{KeyID: "new-key", ActorID: "User:u1", Secret: "show-once", Scopes: token.Scopes, ExpiresAt: expires}}
	service := NewService(repo, secrets)
	service.SetSubjectValidator(func(context.Context, string) bool { return true })
	service.now = func() time.Time { return now }
	rotator := &fakeRotator{result: CreateResult{Token: token, Secret: "show-once"}}
	service.SetRotator(rotator)

	result, err := service.Rotate(context.Background(), "u1", token.ID, time.Time{})
	if err != nil || result.Secret != "show-once" {
		t.Fatalf("Rotate() = %#v, %v", result, err)
	}
	if rotator.token.ID != token.ID || rotator.token.TalosKeyID != token.TalosKeyID {
		t.Fatal("rotation did not use authoritative token metadata")
	}
}

func TestRotateRejectsWrongOwnerWithoutCallingWorkflow(t *testing.T) {
	now := time.Now()
	expires := now.Add(time.Hour)
	token := domain.Token{ID: "api-1", TalosKeyID: "old-key", SubjectUserID: "u1", ActorType: domain.ActorUser, ActorID: "User:u1", Scopes: []string{"api.access"}, Status: domain.StatusActive, ExpiresAt: &expires}
	repo := &fakeRepo{created: token}
	service := NewService(repo, &fakeSecrets{})
	service.SetSubjectValidator(func(context.Context, string) bool { return true })
	service.now = func() time.Time { return now }

	rotator := &fakeRotator{}
	service.SetRotator(rotator)
	_, err := service.Rotate(context.Background(), "other", token.ID, time.Time{})
	if !errors.Is(err, ErrForbidden) || rotator.token.ID != "" {
		t.Fatal("wrong owner invoked rotation workflow")
	}
}

func TestRevokeCanRetryAfterTalosFailure(t *testing.T) {
	repo := &fakeRepo{revoking: domain.Token{ID: "api-1", TalosKeyID: "key-1", Status: domain.StatusRevoking}}
	secrets := &fakeSecrets{revokeErr: errors.New("Talos unavailable")}
	service := NewService(repo, secrets)
	if err := service.Revoke(context.Background(), "u1", "api-1"); err == nil || repo.revoked {
		t.Fatal("failed upstream revocation must remain pending")
	}
	secrets.revokeErr = nil
	if err := service.Revoke(context.Background(), "u1", "api-1"); err != nil || !repo.revoked {
		t.Fatalf("retry must finish revocation: %v", err)
	}
}

func TestRevokeAlreadyRevokedIsIdempotent(t *testing.T) {
	repo := &fakeRepo{revoking: domain.Token{Status: domain.StatusRevoked}}
	secrets := &fakeSecrets{revokeErr: errors.New("Talos unavailable")}
	if err := NewService(repo, secrets).Revoke(context.Background(), "u1", "api-1"); err != nil {
		t.Fatalf("already revoked token must remain successful: %v", err)
	}
}

func TestRevokeResolvesAmbiguousUpstreamResult(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metadata SecretMetadata
		queryErr error
		complete bool
	}{
		{"confirmed revoked", SecretMetadata{"key-1", "KEY_STATUS_REVOKED"}, nil, true},
		{"still active", SecretMetadata{"key-1", "KEY_STATUS_ACTIVE"}, nil, false},
		{"wrong key", SecretMetadata{"other-key", "KEY_STATUS_REVOKED"}, nil, false},
		{"unknown status", SecretMetadata{"key-1", ""}, nil, false},
		{"query unavailable", SecretMetadata{}, errors.New("unavailable"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{revoking: domain.Token{ID: "api-1", TalosKeyID: "key-1", Status: domain.StatusRevoking}}
			secrets := &fakeSecrets{revokeErr: errors.New("response lost"), metadata: tc.metadata, getErr: tc.queryErr}
			err := NewService(repo, secrets).Revoke(t.Context(), "u1", "api-1")
			if (err == nil) != tc.complete || repo.revoked != tc.complete {
				t.Fatalf("completed=%v err=%v", repo.revoked, err)
			}
		})
	}
}

func TestRevokeRetriesLocalCommitAfterUpstreamSuccess(t *testing.T) {
	repo := &fakeRepo{revoking: domain.Token{ID: "api-1", TalosKeyID: "key-1", Status: domain.StatusRevoking}, markRevokedErr: errors.New("database unavailable")}
	secrets := &fakeSecrets{}
	service := NewService(repo, secrets)
	if err := service.Revoke(t.Context(), "u1", "api-1"); err == nil || repo.revoked {
		t.Fatal("local commit failure must remain pending")
	}
	repo.markRevokedErr = nil
	secrets.revokeErr = errors.New("already revoked")
	secrets.metadata = SecretMetadata{"key-1", "KEY_STATUS_REVOKED"}
	if err := service.Revoke(t.Context(), "u1", "api-1"); err != nil || !repo.revoked {
		t.Fatal("confirmed upstream revocation must finish local commit")
	}
}
