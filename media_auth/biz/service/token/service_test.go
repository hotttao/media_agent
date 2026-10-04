package token

import (
	"context"
	"testing"
	"time"

	domainauth "media_agent/media_auth/biz/domain/auth"
	domainst "media_agent/media_auth/biz/domain/service_token"
	sharedtoken "media_agent/media_auth/biz/shared/token"
)

type tokenRepoStub struct {
	domainst.TokenRepo
	token domainst.Token
}

func (r *tokenRepoStub) GetByHash(context.Context, string) (domainst.Token, error) {
	return r.token, nil
}

type userRepoStub struct {
	domainauth.UserRepo
	user domainauth.User
}

func TestLegacyPlatformCannotBeVerifiedOrAcquired(t *testing.T) {
	repo := &tokenRepoStub{token: domainst.Token{ID: "old-platform", CreatedBy: "user-1", IsPlatform: true}}
	svc := NewService(repo, &userRepoStub{user: domainauth.User{ID: "user-1", Status: "active"}})
	if result, err := svc.VerifyToken(t.Context(), "fixture-platform-secret"); err != nil || result.Active {
		t.Fatal("legacy platform hash bypass accepted")
	}
	if secret, err := svc.GetPlatformToken(t.Context(), "user-1"); err == nil || secret != "" {
		t.Fatal("legacy platform acquisition accepted")
	}
}

func (r *userRepoStub) FindByID(context.Context, string) (domainauth.User, error) {
	return r.user, nil
}

func TestVerifyTokenReturnsWorkerPrincipal(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour)
	repo := &tokenRepoStub{token: domainst.Token{
		ID: "credential-1", Name: "desktop", TokenHash: sharedtoken.HashToken("tos_secret"), CreatedBy: "user-1",
		OrganizationID: "org-1", Scopes: []string{"workflow.worker.poll", "workflow.worker.complete"}, ExpiresAt: &expiresAt,
	}}
	svc := NewService(repo, &userRepoStub{user: domainauth.User{ID: "user-1", DisplayName: "Tester", Status: domainauth.UserStatusActive}})
	svc.SetMembershipValidator(func(_ context.Context, orgID, userID string) bool { return orgID == "org-1" && userID == "user-1" })

	result, err := svc.VerifyToken(context.Background(), "tos_secret")
	if err != nil {
		t.Fatalf("VerifyToken() error = %v", err)
	}
	if !result.Active || result.Sub != "user-1" || result.CredentialID != "credential-1" || result.OrganizationID != "org-1" {
		t.Fatalf("result = %#v", result)
	}
	if result.Scope != "workflow.worker.poll workflow.worker.complete" || result.AuthSource != "service_token" || result.ExpiresAt != expiresAt.Unix() {
		t.Fatalf("result = %#v", result)
	}
}

func TestVerifyTokenRejectsExpiredOrFormerMember(t *testing.T) {
	expiredAt := time.Now().Add(-time.Minute)
	repo := &tokenRepoStub{token: domainst.Token{
		ID: "credential-1", CreatedBy: "user-1", OrganizationID: "org-1", Scopes: []string{"workflow.worker.poll"}, ExpiresAt: &expiredAt,
	}}
	svc := NewService(repo, &userRepoStub{user: domainauth.User{ID: "user-1", Status: domainauth.UserStatusActive}})
	svc.SetMembershipValidator(func(context.Context, string, string) bool { return true })
	result, err := svc.VerifyToken(context.Background(), "tos_secret")
	if err != nil || result.Active {
		t.Fatalf("expired VerifyToken() = %#v, %v", result, err)
	}

	repo.token.ExpiresAt = nil
	svc.SetMembershipValidator(func(context.Context, string, string) bool { return false })
	result, err = svc.VerifyToken(context.Background(), "tos_secret")
	if err != nil || result.Active {
		t.Fatalf("former member VerifyToken() = %#v, %v", result, err)
	}
}
