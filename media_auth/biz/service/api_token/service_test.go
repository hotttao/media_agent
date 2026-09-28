package apitoken

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "media_agent/media_auth/biz/domain/api_token"
)

type fakeRepo struct {
	created    domain.Token
	byKey      map[string]domain.Token
	createErr  error
	revoking   domain.Token
	revoked    bool
	replaceErr error
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
	r.revoked = true
	return nil
}
func (r *fakeRepo) ReplaceTalosKey(context.Context, string, string, *time.Time, time.Time) (domain.Token, error) {
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

func TestCreateUserAPITokenAndVerify(t *testing.T) {
	now := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	expires := now.Add(24 * time.Hour)
	repo := &fakeRepo{}
	secrets := &fakeSecrets{issued: IssuedSecret{KeyID: "talos-1", ActorID: "User:u1", Secret: "once", Scopes: []string{"api.access"}, ExpiresAt: expires}}
	service := NewService(repo, secrets)
	service.SetSubjectValidator(func(context.Context, string) bool { return true })
	service.now = func() time.Time { return now }

	result, err := service.Create(context.Background(), CreateCommand{
		CreatorUserID: "u1", SubjectUserID: "u1", ActorType: domain.ActorUser, ActorID: "User:u1",
		Name: "e2e", Scopes: []string{"api.access"}, ExpiresAt: expires, IdempotencyKey: "req-1",
	})
	if err != nil || result.Secret != "once" || result.Token.TalosKeyID != "talos-1" {
		t.Fatalf("Create() = %#v, %v", result, err)
	}
	secrets.verified = VerifiedSecret{KeyID: "talos-1", ActorID: "User:u1", Scopes: []string{"api.access"}, ExpiresAt: expires}
	principal, err := service.Verify(context.Background(), "once")
	if err != nil || principal.SubjectUserID != "u1" || principal.ActorID != "User:u1" || principal.CredentialID == "" {
		t.Fatalf("Verify() = %#v, %v", principal, err)
	}
}

func TestCreateCompensatesTalosWhenMetadataPersistenceFails(t *testing.T) {
	expires := time.Now().Add(time.Hour)
	repo := &fakeRepo{createErr: errors.New("database unavailable")}
	secrets := &fakeSecrets{issued: IssuedSecret{KeyID: "orphan", ActorID: "User:u1", Secret: "never-return", Scopes: []string{"api.access"}, ExpiresAt: expires}}
	service := NewService(repo, secrets)
	service.SetSubjectValidator(func(context.Context, string) bool { return true })
	_, err := service.Create(context.Background(), CreateCommand{
		CreatorUserID: "u1", SubjectUserID: "u1", ActorType: domain.ActorUser, ActorID: "User:u1",
		Name: "test", Scopes: []string{"api.access"}, ExpiresAt: expires,
	})
	if err == nil || secrets.revokedKey != "orphan" {
		t.Fatalf("Create() error=%v revoked=%q", err, secrets.revokedKey)
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

func TestRotateReturnsOneTimeSecretAndReplacesKey(t *testing.T) {
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

	result, err := service.Rotate(context.Background(), "u1", token.ID, time.Time{})
	if err != nil || result.Secret != "show-once" {
		t.Fatalf("Rotate() = %#v, %v", result, err)
	}
}

func TestRotateRevokesReplacementWhenMetadataUpdateFails(t *testing.T) {
	now := time.Now()
	expires := now.Add(time.Hour)
	token := domain.Token{ID: "api-1", TalosKeyID: "old-key", SubjectUserID: "u1", ActorType: domain.ActorUser, ActorID: "User:u1", Scopes: []string{"api.access"}, Status: domain.StatusActive, ExpiresAt: &expires}
	repo := &fakeRepo{created: token, replaceErr: errors.New("database unavailable")}
	secrets := &fakeSecrets{rotated: IssuedSecret{KeyID: "new-key", Secret: "never-return", ExpiresAt: expires}}
	service := NewService(repo, secrets)
	service.SetSubjectValidator(func(context.Context, string) bool { return true })
	service.now = func() time.Time { return now }

	_, err := service.Rotate(context.Background(), "u1", token.ID, time.Time{})
	if err == nil || secrets.revokedKey != "new-key" {
		t.Fatalf("Rotate() error=%v revoked=%q", err, secrets.revokedKey)
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
