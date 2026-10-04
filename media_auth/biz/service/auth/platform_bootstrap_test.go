package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "media_agent/media_auth/biz/domain/auth"
	ldapclient "media_agent/media_auth/biz/shared/client/ldap"
)

type bootstrapObservation struct {
	user    string
	bounded bool
	value   any
}
type bootstrapKey struct{}
type recordingBootstrap struct {
	calls chan bootstrapObservation
	err   error
}

func (b *recordingBootstrap) EnsurePlatformToken(ctx context.Context, user string) error {
	deadline, ok := ctx.Deadline()
	b.calls <- bootstrapObservation{user, ok && deadline.After(time.Now()) && time.Until(deadline) <= 5*time.Second, ctx.Value(bootstrapKey{})}
	return b.err
}

type kratosBootstrapRepo struct {
	*fakeRepo
	user domain.User
}

func (r *kratosBootstrapRepo) ResolveKratosIdentity(context.Context, string, string, string, bool) (domain.User, error) {
	return r.user, nil
}

func TestPlatformBootstrapLoginRegistrationAndKratosAreBoundedAndOptional(t *testing.T) {
	ctx := context.WithValue(context.Background(), bootstrapKey{}, "request-context")
	for _, mode := range []string{"login", "register", "kratos"} {
		t.Run(mode, func(t *testing.T) {
			bootstrap := &recordingBootstrap{calls: make(chan bootstrapObservation, 1), err: errors.New("private custody diagnostic")}
			repo := &kratosBootstrapRepo{fakeRepo: newFakeRepo(), user: domain.User{ID: "kratos-user", Username: "kratos-user", Role: "user", Status: "active"}}
			svc := newSvc(repo, fakeLdapClient{user: &ldapclient.LdapUser{Username: "tester"}})
			svc.SetPlatformTokenEnsurer(bootstrap)
			var err error
			switch mode {
			case "login":
				_, _, err = svc.Login(ctx, LoginCommand{Username: "tester", Password: "fixture-password"})
			case "register":
				_, _, err = svc.Register(ctx, RegisterCommand{Username: "tester", Password: "fixture-password"})
			case "kratos":
				_, err = svc.ResolveKratosIdentity(ctx, "identity", "tester@example.test", "Tester", false)
			}
			if err != nil {
				t.Fatal("optional custody failure changed authentication outcome")
			}
			select {
			case observation := <-bootstrap.calls:
				if observation.user == "" || !observation.bounded || observation.value != "request-context" {
					t.Fatal("bootstrap identity or context boundary lost")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("bootstrap was not attempted")
			}
		})
	}
}

func TestDisabledKratosUserCannotBootstrapPlatform(t *testing.T) {
	repo := &kratosBootstrapRepo{fakeRepo: newFakeRepo(), user: domain.User{ID: "disabled", Status: "disabled", Role: "user"}}
	svc := newSvc(repo, nil)
	bootstrap := &recordingBootstrap{calls: make(chan bootstrapObservation, 1)}
	svc.SetPlatformTokenEnsurer(bootstrap)
	if _, err := svc.ResolveKratosIdentity(context.Background(), "identity", "", "", false); err == nil {
		t.Fatal("disabled identity accepted")
	}
	select {
	case <-bootstrap.calls:
		t.Fatal("disabled identity bootstrapped")
	default:
	}
}
