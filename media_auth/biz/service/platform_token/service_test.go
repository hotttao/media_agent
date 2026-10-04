package platformtoken

import (
	"context"
	"errors"
	"testing"

	domain "media_agent/media_auth/biz/domain/platform_token"
)

type testCaller struct {
	name string
	err  error
}

func (c testCaller) VerifyPlatformCaller(context.Context) (string, error) { return c.name, c.err }

type forbiddenTx struct{}

func (forbiddenTx) RunInTx(context.Context, func(context.Context) error) error {
	panic("unauthorized request reached storage")
}

func TestExternalOperationsRejectMissingOrUntrustedCallerBeforeStorage(t *testing.T) {
	for _, c := range []CallerVerifier{nil, testCaller{name: domain.CallerGateway, err: errors.New("sensitive transport diagnostic")}, testCaller{name: "personal_worker"}, testCaller{name: domain.CallerAuth}} {
		s := &Service{caller: c, tx: forbiddenTx{}}
		if secret, err := s.AcquirePlatformToken(t.Context(), "owner", domain.PurposeOffline); secret != "" || !errors.Is(err, ErrDenied) {
			t.Fatal("untrusted acquire did not fail closed")
		}
		if secret, err := s.RotatePlatformToken(t.Context(), "owner", domain.PurposeOffline, nil); secret != "" || !errors.Is(err, ErrDenied) {
			t.Fatal("untrusted rotation did not fail closed")
		}
		if p, err := s.IntrospectPlatformToken(t.Context(), "sensitive-secret", domain.PurposeOffline); p.Active || !errors.Is(err, ErrDenied) {
			t.Fatal("untrusted introspection did not fail closed")
		}
	}
	for _, purpose := range []string{"", domain.PurposeLogin, domain.PurposeRevocation, "OFFLINE-TASK", "offline-task\n"} {
		s := &Service{caller: testCaller{name: domain.CallerGateway}, tx: forbiddenTx{}}
		if secret, err := s.AcquirePlatformToken(t.Context(), "owner", purpose); secret != "" || !errors.Is(err, ErrDenied) {
			t.Fatal("caller expanded allowed purpose")
		}
	}
}

func TestUnavailableDependenciesDoNotAllowBootstrapOrRevocation(t *testing.T) {
	for _, s := range []*Service{nil, {}, {caller: testCaller{name: domain.CallerGateway}}} {
		if err := s.EnsurePlatformToken(t.Context(), "owner"); !errors.Is(err, ErrUnavailable) {
			t.Fatal("unconfigured bootstrap succeeded")
		}
		if err := s.RevokePlatformToken(t.Context(), "owner"); !errors.Is(err, ErrUnavailable) {
			t.Fatal("unconfigured revocation succeeded")
		}
	}
	if err := safeError(errors.New("secret database diagnostic")); err != ErrUnavailable {
		t.Fatal("infrastructure error leaked")
	}
}
