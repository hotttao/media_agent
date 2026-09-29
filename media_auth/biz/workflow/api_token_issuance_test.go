package workflow

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	domain "media_agent/media_auth/biz/domain/api_token"
	service "media_agent/media_auth/biz/service/api_token"
)

type issuanceRepo struct {
	token                           domain.Token
	createErr, commitErr, finishErr error
}

type issuanceBatchSource struct {
	*issuanceRepo
	rows      []domain.Token
	attempted []string
	scanErr   error
}

func (s *issuanceBatchSource) ListPendingIssuances(_ context.Context, after string, _ time.Time, limit int) ([]domain.Token, error) {
	if s.scanErr != nil {
		return nil, s.scanErr
	}
	var rows []domain.Token
	for _, token := range s.rows {
		if token.ID > after {
			rows = append(rows, token)
			if len(rows) == limit {
				break
			}
		}
	}
	return rows, nil
}

func (s *issuanceBatchSource) ClaimIssuanceCleanup(ctx context.Context, id string, _ time.Time, _ bool) (domain.Token, bool, error) {
	s.attempted = append(s.attempted, id)
	if _, ok := ctx.Deadline(); !ok {
		return domain.Token{}, false, errors.New("missing attempt deadline")
	}
	if id == "token-000" {
		return domain.Token{}, false, errors.New("database unavailable")
	}
	// Other entries have been completed by another recovery instance.
	return domain.Token{}, false, nil
}

func TestIssuanceBatchFailureDoesNotStarveLaterPages(t *testing.T) {
	flow, repo, _, _ := issuanceFixture()
	source := &issuanceBatchSource{issuanceRepo: repo}
	for i := 0; i < 101; i++ {
		source.rows = append(source.rows, domain.Token{ID: fmt.Sprintf("token-%03d", i)})
	}
	flow.repo = source
	cursor, err := flow.ProcessBatch(t.Context(), "")
	if err != nil || cursor != "token-099" || len(source.attempted) != 100 {
		t.Fatal("failed entry stopped first page")
	}
	cursor, err = flow.ProcessBatch(t.Context(), cursor)
	if err != nil || cursor != "" || len(source.attempted) != 101 {
		t.Fatal("failure starved later page")
	}
	source.attempted = nil
	if _, err := flow.ProcessBatch(t.Context(), cursor); err != nil || source.attempted[0] != "token-000" {
		t.Fatal("next sweep missed failed entry")
	}
	source.scanErr = errors.New("scan unavailable")
	if next, err := flow.ProcessBatch(t.Context(), "saved-cursor"); err == nil || next != "saved-cursor" {
		t.Fatal("scan failure lost cursor")
	}
}

func TestIssuanceCancelledBatchDoesNotStartExternalWork(t *testing.T) {
	flow, repo, secrets, token := issuanceFixture()
	repo.token = token
	repo.token.Status = domain.StatusIssuanceCleanup
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := flow.ProcessBatch(ctx, ""); !errors.Is(err, context.Canceled) || len(secrets.calls) != 0 {
		t.Fatal("cancelled batch started external cleanup")
	}
	flow.Run(ctx)
}

func TestIssuanceLostRevokeResponseRequiresExactTerminalConfirmation(t *testing.T) {
	flow, repo, secrets, token := issuanceFixture()
	secrets.issueErr = errors.New("issue response lost")
	secrets.revokeErr = errors.New("revoke response lost")
	secrets.confirmation = service.SecretMetadata{KeyID: "key", Status: "KEY_STATUS_REVOKED"}
	result, err := flow.Issue(t.Context(), token)
	if err == nil || result.Secret != "" || repo.token.Status != domain.StatusRevoked {
		t.Fatal("confirmed revocation remained pending")
	}
}

func (r *issuanceRepo) Create(_ context.Context, token domain.Token) (domain.Token, error) {
	if r.createErr != nil {
		return domain.Token{}, r.createErr
	}
	r.token = token
	return token, nil
}
func (r *issuanceRepo) CommitIssuance(_ context.Context, id, key string, expiry *time.Time, at time.Time) (domain.Token, error) {
	if r.commitErr != nil {
		return domain.Token{}, r.commitErr
	}
	if r.token.ID != id || r.token.Status != domain.StatusIssuing || !at.Before(*r.token.IssuanceDeadline) {
		return domain.Token{}, domain.ErrConflict
	}
	r.token.Status, r.token.TalosKeyID, r.token.ExpiresAt = domain.StatusActive, key, expiry
	return r.token, nil
}
func (r *issuanceRepo) ClaimIssuanceCleanup(_ context.Context, id string, at time.Time, force bool) (domain.Token, bool, error) {
	if r.token.ID != id || (r.token.Status != domain.StatusIssuing && r.token.Status != domain.StatusIssuanceCleanup) {
		return domain.Token{}, false, nil
	}
	if r.token.Status == domain.StatusIssuing && !force && at.Before(*r.token.IssuanceDeadline) {
		return domain.Token{}, false, nil
	}
	r.token.Status = domain.StatusIssuanceCleanup
	return r.token, true, nil
}
func (r *issuanceRepo) FinishIssuanceCleanup(_ context.Context, id, key string, _ time.Time) error {
	if r.finishErr != nil {
		return r.finishErr
	}
	if r.token.ID != id || r.token.Status != domain.StatusIssuanceCleanup {
		return domain.ErrConflict
	}
	r.token.Status, r.token.TalosKeyID = domain.StatusRevoked, key
	return nil
}
func (r *issuanceRepo) ListPendingIssuances(_ context.Context, after string, at time.Time, _ int) ([]domain.Token, error) {
	if r.token.ID > after && (r.token.Status == domain.StatusIssuanceCleanup || (r.token.Status == domain.StatusIssuing && !at.Before(*r.token.IssuanceDeadline))) {
		return []domain.Token{r.token}, nil
	}
	return nil, nil
}

type issuanceSecrets struct {
	repo                            *issuanceRepo
	issued                          service.IssuedSecret
	resolved                        service.IssuanceKey
	issueErr, cleanupErr, revokeErr error
	pages                           map[string]service.RotationKeyPage
	command                         service.IssueSecretCommand
	calls                           []string
	onIssue                         func(context.Context)
	confirmation                    service.SecretMetadata
}

func (s *issuanceSecrets) Issue(ctx context.Context, cmd service.IssueSecretCommand) (service.IssuedSecret, error) {
	s.calls = append(s.calls, "issue")
	s.command = cmd
	if s.repo.token.Status != domain.StatusIssuing || s.repo.token.IssuanceDeadline == nil {
		return service.IssuedSecret{}, errors.New("no durable intent")
	}
	if s.onIssue != nil {
		s.onIssue(ctx)
	}
	return s.issued, s.issueErr
}
func (s *issuanceSecrets) RecoverIssue(ctx context.Context, cmd service.IssueSecretCommand) (service.IssuanceKey, error) {
	if err := ctx.Err(); err != nil {
		return service.IssuanceKey{}, err
	}
	s.calls = append(s.calls, "resolve")
	if cmd.RequestID != s.repo.token.ID || cmd.Metadata["issuance_id"] != cmd.RequestID {
		return service.IssuanceKey{}, errors.New("request identity changed")
	}
	return s.resolved, s.cleanupErr
}
func (s *issuanceSecrets) List(ctx context.Context, actor, cursor string) (service.RotationKeyPage, error) {
	if err := ctx.Err(); err != nil {
		return service.RotationKeyPage{}, err
	}
	s.calls = append(s.calls, "list:"+cursor)
	return s.pages[cursor], s.cleanupErr
}
func (s *issuanceSecrets) Revoke(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.calls = append(s.calls, "revoke:"+key)
	return s.revokeErr
}
func (s *issuanceSecrets) Get(context.Context, string) (service.SecretMetadata, error) {
	return s.confirmation, nil
}

func issuanceFixture() (*APITokenIssuance, *issuanceRepo, *issuanceSecrets, domain.Token) {
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	expiry := now.Add(time.Hour)
	token := domain.Token{ID: "attempt", ActorID: "User:owner", SubjectUserID: "owner", ActorType: domain.ActorUser,
		Name: "test", CreatedBy: "owner", CreatedAt: now, ExpiresAt: &expiry, Scopes: []string{"api.access"}, ClientRequestKey: "client-key"}
	repo := &issuanceRepo{}
	secrets := &issuanceSecrets{repo: repo, issued: service.IssuedSecret{KeyID: "key", ActorID: token.ActorID, Scopes: token.Scopes, Secret: "once", ExpiresAt: expiry},
		resolved: service.IssuanceKey{KeyID: "key", ActorID: token.ActorID, Status: "KEY_STATUS_ACTIVE", Metadata: issuanceMetadata(token)}}
	flow := NewAPITokenIssuance(repo, secrets, rotationTx{})
	flow.now = func() time.Time { return now }
	return flow, repo, secrets, token
}

func TestIssuanceCommitsBeforeTalosAndDeliversAfterActivation(t *testing.T) {
	flow, repo, secrets, token := issuanceFixture()
	result, err := flow.Issue(t.Context(), token)
	if err != nil || result.Secret != "once" || !repo.token.ActiveAt(flow.now()) || result.Token.TalosKeyID != "key" {
		t.Fatal("successful issuance not durably activated")
	}
	if secrets.command.RequestID != token.ID || secrets.command.RequestID == token.ClientRequestKey || secrets.command.Metadata["issuance_id"] != token.ID {
		t.Fatal("upstream received untrusted request identity")
	}
	if err := flow.Recover(t.Context(), token.ID, true); err != nil || !slices.Equal(secrets.calls, []string{"issue"}) {
		t.Fatal("completed issuance compensated")
	}
}

func TestIssuanceInitialPersistenceFailureNeverCallsTalos(t *testing.T) {
	flow, repo, secrets, token := issuanceFixture()
	repo.createErr = domain.ErrConflict
	result, err := flow.Issue(t.Context(), token)
	if !errors.Is(err, domain.ErrConflict) || result.Secret != "" || len(secrets.calls) != 0 {
		t.Fatal("failed insert created external credential")
	}
}

func TestIssuanceFailureRetainsIntentThenRecoversAfterRestart(t *testing.T) {
	for _, failure := range []string{"response lost", "local commit failed", "incomplete secret", "actor mismatch", "scope mismatch"} {
		t.Run(failure, func(t *testing.T) {
			flow, repo, secrets, token := issuanceFixture()
			switch failure {
			case "response lost":
				secrets.issueErr = errors.New("response lost")
			case "local commit failed":
				repo.commitErr = errors.New("database unavailable")
			case "incomplete secret":
				secrets.issued.Secret = ""
			case "actor mismatch":
				secrets.issued.ActorID = "User:other"
			case "scope mismatch":
				secrets.issued.Scopes = []string{"agent.gateway"}
			}
			secrets.cleanupErr = errors.New("cleanup unavailable")
			result, err := flow.Issue(t.Context(), token)
			if err == nil || result.Secret != "" || repo.token.Status != domain.StatusIssuanceCleanup {
				t.Fatal("failure lost durable cleanup intent")
			}
			secrets.cleanupErr = nil
			restarted := NewAPITokenIssuance(repo, secrets, rotationTx{})
			restarted.now = flow.now
			if _, err := restarted.ProcessBatch(t.Context(), ""); err != nil || repo.token.Status != domain.StatusRevoked || !slices.Contains(secrets.calls, "revoke:key") {
				t.Fatal("restart could not finish cleanup")
			}
			if len(slices.DeleteFunc(slices.Clone(secrets.calls), func(call string) bool { return call != "issue" })) != 1 {
				t.Fatal("restart repeated normal issuance")
			}
		})
	}
}

func TestIssuanceCancellationCleanupHasIndependentContext(t *testing.T) {
	flow, repo, secrets, token := issuanceFixture()
	ctx, cancel := context.WithCancel(t.Context())
	secrets.onIssue = func(context.Context) { cancel() }
	secrets.issueErr = context.Canceled
	result, err := flow.Issue(ctx, token)
	if !errors.Is(err, context.Canceled) || result.Secret != "" || repo.token.Status != domain.StatusRevoked {
		t.Fatal("request cancellation cancelled credential cleanup")
	}
}

func TestIssuanceCleanupFencesLateSuccessAndAvoidsQuotaReplayForExistingKey(t *testing.T) {
	flow, repo, secrets, token := issuanceFixture()
	secrets.onIssue = func(ctx context.Context) { _, _, _ = repo.ClaimIssuanceCleanup(ctx, token.ID, flow.now(), true) }
	secrets.pages = map[string]service.RotationKeyPage{"": {NextPageToken: "second"}, "second": {Keys: []service.RotationKey{
		{KeyID: "unrelated", ActorID: token.ActorID, Metadata: map[string]any{"api_token_id": "other"}},
		{KeyID: "key", ActorID: token.ActorID, Status: "KEY_STATUS_ACTIVE", Metadata: issuanceMetadata(token)},
	}}}
	result, err := flow.Issue(t.Context(), token)
	if !errors.Is(err, domain.ErrConflict) || result.Secret != "" || repo.token.Status != domain.StatusRevoked || slices.Contains(secrets.calls, "resolve") || slices.Contains(secrets.calls, "revoke:unrelated") {
		t.Fatal("late success escaped fence or cleanup touched another key")
	}
}

func TestIssuanceIncompleteCleanupNeverCompletesLocally(t *testing.T) {
	for _, failure := range []string{"cursor cycle", "cross actor", "wrong attempt", "revoke failure", "wrong confirmation", "local finish failed"} {
		t.Run(failure, func(t *testing.T) {
			flow, repo, secrets, token := issuanceFixture()
			secrets.issueErr = errors.New("issue uncertain")
			switch failure {
			case "cursor cycle":
				secrets.pages = map[string]service.RotationKeyPage{"": {NextPageToken: "same"}, "same": {NextPageToken: "same"}}
			case "cross actor":
				secrets.resolved.ActorID = "User:other"
			case "wrong attempt":
				secrets.resolved.Metadata["issuance_id"] = "other"
			case "revoke failure":
				secrets.revokeErr = errors.New("revoke failed")
			case "wrong confirmation":
				secrets.revokeErr = errors.New("response lost")
				secrets.confirmation = service.SecretMetadata{KeyID: "other", Status: "KEY_STATUS_REVOKED"}
			case "local finish failed":
				repo.finishErr = errors.New("database unavailable")
			}
			result, err := flow.Issue(t.Context(), token)
			if err == nil || result.Secret != "" || repo.token.Status != domain.StatusIssuanceCleanup {
				t.Fatal("unconfirmed cleanup lost intent")
			}
		})
	}
}
