package runtime_environment

import (
	"context"
	"errors"
	"testing"

	domain "media_agent/media_control/biz/domain/runtime_environment"
)

type tokenRepository struct {
	environment *domain.Environment
	tokens      map[string]*domain.Token
}

func (r *tokenRepository) CreateEnvironment(context.Context, *domain.Environment) error { return nil }
func (r *tokenRepository) ListEnvironments(context.Context) ([]domain.Environment, error) {
	return nil, nil
}
func (r *tokenRepository) GetEnvironment(_ context.Context, id string) (*domain.Environment, error) {
	if r.environment == nil || r.environment.ID != id {
		return nil, domain.ErrNotFound
	}
	return r.environment, nil
}
func (r *tokenRepository) UpdateEnvironment(context.Context, *domain.Environment, int64) error {
	return nil
}
func (r *tokenRepository) DeleteEnvironment(context.Context, *domain.Environment, int64) error {
	return nil
}
func (r *tokenRepository) FindPlacement(context.Context, string) (*domain.Placement, error) {
	return nil, domain.ErrPlacementNotFound
}
func (r *tokenRepository) UpsertPlacement(context.Context, *domain.Placement, int64) error {
	return nil
}
func (r *tokenRepository) CreateToken(_ context.Context, value *domain.Token) error {
	copy := *value
	r.tokens[value.ID] = &copy
	return nil
}
func (r *tokenRepository) ListActiveTokens(_ context.Context, environmentID string) ([]domain.Token, error) {
	result := make([]domain.Token, 0, len(r.tokens))
	for _, value := range r.tokens {
		if value.EnvironmentID == environmentID && value.RevokedAt == nil {
			result = append(result, *value)
		}
	}
	return result, nil
}
func (r *tokenRepository) GetToken(_ context.Context, id, environmentID string) (*domain.Token, error) {
	value := r.tokens[id]
	if value == nil || value.EnvironmentID != environmentID {
		return nil, domain.ErrTokenNotFound
	}
	copy := *value
	return &copy, nil
}
func (r *tokenRepository) RevokeToken(_ context.Context, value *domain.Token) error {
	stored := r.tokens[value.ID]
	if stored == nil || stored.EnvironmentID != value.EnvironmentID {
		return domain.ErrTokenNotFound
	}
	stored.RevokedAt = value.RevokedAt
	return nil
}
func (r *tokenRepository) FindActiveTokenByHash(_ context.Context, hash string) (*domain.EnvironmentPrincipal, error) {
	for _, value := range r.tokens {
		if value.TokenHash == hash && value.RevokedAt == nil {
			return &domain.EnvironmentPrincipal{EnvironmentID: value.EnvironmentID}, nil
		}
	}
	return nil, domain.ErrTokenNotFound
}

type directTx struct{}

func (directTx) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

func TestEnvironmentTokenShownOnceHashedVerifiedAndRevoked(t *testing.T) {
	repository := &tokenRepository{environment: &domain.Environment{ID: "env-1"}, tokens: map[string]*domain.Token{}}
	service := New(repository, directTx{}, nil)
	raw := "env_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	service.issueToken = func() (string, error) { return raw, nil }

	issued, err := service.IssueToken(context.Background(), "env-1", " primary ", " user-1 ")
	if err != nil {
		t.Fatal(err)
	}
	if issued.Token != raw || issued.Metadata.Name != "primary" || issued.Metadata.CreatedBy != "user-1" {
		t.Fatalf("unexpected issue result: %+v", issued)
	}
	if issued.Metadata.TokenHash == raw || issued.Metadata.TokenHash != hashEnvironmentToken(raw) {
		t.Fatalf("token was not hashed at rest: %q", issued.Metadata.TokenHash)
	}
	principal, err := service.VerifyToken(context.Background(), raw)
	if err != nil || principal.EnvironmentID != "env-1" {
		t.Fatalf("principal=%+v err=%v", principal, err)
	}
	listed, err := service.ListTokens(context.Background(), "env-1")
	if err != nil || len(listed) != 1 {
		t.Fatalf("listed=%+v err=%v", listed, err)
	}
	if _, err := service.RevokeToken(context.Background(), "env-1", issued.Metadata.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RevokeToken(context.Background(), "env-1", issued.Metadata.ID); err != nil {
		t.Fatalf("repeat revoke must be idempotent: %v", err)
	}
	if _, err := service.VerifyToken(context.Background(), raw); !errors.Is(err, domain.ErrTokenNotFound) {
		t.Fatalf("revoked token verified: %v", err)
	}
	listed, err = service.ListTokens(context.Background(), "env-1")
	if err != nil || len(listed) != 0 {
		t.Fatalf("revoked token remained active: %+v err=%v", listed, err)
	}
}

func TestEnvironmentTokenSupportsOverlapForRotation(t *testing.T) {
	repository := &tokenRepository{environment: &domain.Environment{ID: "env-1"}, tokens: map[string]*domain.Token{}}
	service := New(repository, directTx{}, nil)
	values := []string{
		"env_1111111111111111111111111111111111111111111111111111111111111111",
		"env_2222222222222222222222222222222222222222222222222222222222222222",
	}
	service.issueToken = func() (string, error) {
		value := values[0]
		values = values[1:]
		return value, nil
	}
	for _, name := range []string{"old", "new"} {
		if _, err := service.IssueToken(context.Background(), "env-1", name, "user-1"); err != nil {
			t.Fatal(err)
		}
	}
	listed, err := service.ListTokens(context.Background(), "env-1")
	if err != nil || len(listed) != 2 {
		t.Fatalf("rotation overlap unavailable: %+v err=%v", listed, err)
	}
}
