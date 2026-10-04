package platformtoken

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"time"

	domain "media_agent/media_auth/biz/domain/platform_token"
	"media_agent/media_auth/biz/shared/platformcipher"
)

var ErrUnavailable = errors.New("platform token service unavailable")
var ErrDenied = errors.New("platform token access denied")

type TransactionRunner interface {
	RunInTx(context.Context, func(context.Context) error) error
}
type UserReader interface {
	PlatformUserActive(context.Context, string, bool) (bool, error)
}

// CallerVerifier must derive identity from authenticated transport, not client
// headers or command fields. No implementation means no external access.
type CallerVerifier interface {
	VerifyPlatformCaller(context.Context) (string, error)
}
type Options struct {
	Now           func() time.Time
	NewCredential func() (string, string, error)
	NewAuditID    func() (string, error)
}
type Principal struct {
	Active               bool
	UserID, CredentialID string
}
type Service struct {
	repo    domain.Repository
	audit   domain.AuditWriter
	users   UserReader
	tx      TransactionRunner
	cipher  *platformcipher.Keyring
	caller  CallerVerifier
	options Options
}

func NewService(repo domain.Repository, audit domain.AuditWriter, users UserReader, tx TransactionRunner, cipher *platformcipher.Keyring, caller CallerVerifier, options Options) *Service {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.NewCredential == nil {
		options.NewCredential = platformcipher.GenerateCredential
	}
	if options.NewAuditID == nil {
		options.NewAuditID = platformcipher.GenerateID
	}
	return &Service{repo: repo, audit: audit, users: users, tx: tx, cipher: cipher, caller: caller, options: options}
}

func (s *Service) ready() bool {
	return s != nil && s.repo != nil && s.audit != nil && s.users != nil && s.tx != nil && s.cipher != nil
}
func (s *Service) authorize(ctx context.Context, purpose string) (string, error) {
	if s == nil || s.caller == nil {
		return "", ErrDenied
	}
	caller, err := s.caller.VerifyPlatformCaller(ctx)
	if err != nil || !domain.AllowedCaller(caller, purpose) {
		return "", ErrDenied
	}
	return caller, nil
}
func (s *Service) requireUser(ctx context.Context, user string) error {
	if !domain.ValidIdentity(user) {
		return ErrDenied
	}
	active, err := s.users.PlatformUserActive(ctx, user, true)
	if err != nil {
		return ErrUnavailable
	}
	if !active {
		return ErrDenied
	}
	return nil
}
func (s *Service) timestamp() time.Time { return s.options.Now().UTC().Truncate(time.Microsecond) }
func (s *Service) record(ctx context.Context, t domain.Token, op, caller, purpose string, now time.Time) error {
	id, err := s.options.NewAuditID()
	if err != nil {
		return ErrUnavailable
	}
	event := domain.AuditEvent{ID: id, UserID: t.UserID, CredentialID: t.ID, Operation: op, CallerService: caller, Purpose: purpose, OccurredAt: now}
	if !event.Valid() {
		return ErrUnavailable
	}
	return s.audit.AppendAudit(ctx, event)
}
func (s *Service) open(t domain.Token) (string, error) {
	return s.cipher.Open(t.ID, t.UserID, platformcipher.Envelope{KeyVersion: t.KeyVersion, TokenHash: t.TokenHash, Ciphertext: t.Ciphertext, Nonce: t.Nonce})
}
func (s *Service) issue(ctx context.Context, user, op, caller, purpose string, now time.Time, expiry *time.Time) (domain.Token, string, error) {
	id, secret, err := s.options.NewCredential()
	if err != nil {
		return domain.Token{}, "", ErrUnavailable
	}
	env, err := s.cipher.Seal(id, user, secret)
	if err != nil {
		return domain.Token{}, "", ErrUnavailable
	}
	t := domain.Token{ID: id, UserID: user, TokenHash: env.TokenHash, Ciphertext: env.Ciphertext, Nonce: env.Nonce, KeyVersion: env.KeyVersion, CreatedAt: now, ExpiresAt: expiry}
	if err := s.repo.Create(ctx, t); err != nil {
		return domain.Token{}, "", err
	}
	if err := s.record(ctx, t, op, caller, purpose, now); err != nil {
		return domain.Token{}, "", err
	}
	return t, secret, nil
}
func (s *Service) ensure(ctx context.Context, user, caller, purpose string) (domain.Token, string, error) {
	if err := s.requireUser(ctx, user); err != nil {
		return domain.Token{}, "", err
	}
	now := s.timestamp()
	t, err := s.repo.GetCurrent(ctx, user)
	if err == nil {
		if t.UserID != user {
			return domain.Token{}, "", ErrUnavailable
		}
		secret, err := s.open(t)
		if err != nil {
			return domain.Token{}, "", err
		}
		if t.ActiveAt(now) {
			return t, secret, nil
		}
	}
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return domain.Token{}, "", err
	}
	if err == nil {
		if err := s.repo.RevokeForUser(ctx, user, now); err != nil {
			return domain.Token{}, "", err
		}
	}
	return s.issue(ctx, user, "issued", caller, purpose, now, nil)
}

func safeError(err error) error {
	if errors.Is(err, ErrDenied) {
		return ErrDenied
	}
	return ErrUnavailable
}

// EnsurePlatformToken is internal bootstrap only. It never returns Secret.
func (s *Service) EnsurePlatformToken(ctx context.Context, user string) error {
	if !s.ready() {
		return ErrUnavailable
	}
	err := s.tx.RunInTx(ctx, func(ctx context.Context) error {
		_, _, err := s.ensure(ctx, user, domain.CallerAuth, domain.PurposeLogin)
		return err
	})
	if err != nil {
		return safeError(err)
	}
	return nil
}

func (s *Service) AcquirePlatformToken(ctx context.Context, user, purpose string) (string, error) {
	caller, err := s.authorize(ctx, purpose)
	if err != nil {
		return "", err
	}
	if !s.ready() {
		return "", ErrUnavailable
	}
	var secret string
	err = s.tx.RunInTx(ctx, func(ctx context.Context) error {
		t, value, err := s.ensure(ctx, user, caller, purpose)
		if err != nil {
			return err
		}
		if err := s.record(ctx, t, "acquired", caller, purpose, s.timestamp()); err != nil {
			return err
		}
		secret = value
		return nil
	})
	if err != nil {
		return "", safeError(err)
	}
	return secret, nil
}

func (s *Service) RotatePlatformToken(ctx context.Context, user, purpose string, expiry *time.Time) (string, error) {
	caller, err := s.authorize(ctx, purpose)
	if err != nil {
		return "", err
	}
	if !s.ready() {
		return "", ErrUnavailable
	}
	var expiresAt *time.Time
	if expiry != nil {
		at := expiry.UTC().Truncate(time.Microsecond)
		expiresAt = &at
	}
	var secret string
	err = s.tx.RunInTx(ctx, func(ctx context.Context) error {
		if err := s.requireUser(ctx, user); err != nil {
			return err
		}
		now := s.timestamp()
		if expiresAt != nil && !expiresAt.After(now) {
			return ErrDenied
		}
		if err := s.repo.RevokeForUser(ctx, user, now); err != nil {
			return err
		}
		_, value, err := s.issue(ctx, user, "rotated", caller, purpose, now, expiresAt)
		secret = value
		return err
	})
	if err != nil {
		return "", safeError(err)
	}
	return secret, nil
}

// RevokeAllForUser participates in the caller's User Access Revocation
// transaction; it intentionally does not open a nested independent transaction.
func (s *Service) RevokeAllForUser(ctx context.Context, user string) error {
	if !s.ready() {
		return ErrUnavailable
	}
	if !domain.ValidIdentity(user) {
		return ErrDenied
	}
	if _, err := s.users.PlatformUserActive(ctx, user, true); err != nil {
		return ErrUnavailable
	}
	t, err := s.repo.GetCurrent(ctx, user)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return ErrUnavailable
	}
	now := s.timestamp()
	if err := s.repo.RevokeForUser(ctx, user, now); err != nil {
		return ErrUnavailable
	}
	if err := s.record(ctx, t, "revoked", domain.CallerAuth, domain.PurposeRevocation, now); err != nil {
		return ErrUnavailable
	}
	return nil
}

// RevokePlatformToken is the internal standalone revocation use case. HTTP
// exposure requires a separate authorized user-lifecycle command boundary.
func (s *Service) RevokePlatformToken(ctx context.Context, user string) error {
	if !s.ready() {
		return ErrUnavailable
	}
	err := s.tx.RunInTx(ctx, func(ctx context.Context) error { return s.RevokeAllForUser(ctx, user) })
	if err != nil {
		return safeError(err)
	}
	return nil
}

func (s *Service) IntrospectPlatformToken(ctx context.Context, secret, purpose string) (Principal, error) {
	caller, err := s.authorize(ctx, purpose)
	if err != nil {
		return Principal{}, err
	}
	if !s.ready() {
		return Principal{}, ErrUnavailable
	}
	if len(secret) != 68 {
		return Principal{}, nil
	}
	hash := sha256.Sum256([]byte(secret))
	var result Principal
	err = s.tx.RunInTx(ctx, func(ctx context.Context) error {
		t, err := s.repo.GetByHash(ctx, hash[:])
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := s.requireUser(ctx, t.UserID); err != nil {
			if errors.Is(err, ErrDenied) {
				return nil
			}
			return err
		}
		owner := t.UserID
		// Rotation may have committed while waiting for the User lock.
		t, err = s.repo.GetByHash(ctx, hash[:])
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if t.UserID != owner {
			return ErrUnavailable
		}
		if !t.ActiveAt(s.timestamp()) {
			return nil
		}
		value, err := s.open(t)
		if err != nil || subtle.ConstantTimeCompare([]byte(secret), []byte(value)) != 1 {
			return ErrUnavailable
		}
		if err := s.record(ctx, t, "introspected", caller, purpose, s.timestamp()); err != nil {
			return err
		}
		result = Principal{Active: true, UserID: t.UserID, CredentialID: t.ID}
		return nil
	})
	if err != nil {
		return Principal{}, safeError(err)
	}
	return result, nil
}
