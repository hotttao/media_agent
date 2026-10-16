package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	domain "media_agent/media_agent_gateway/biz/domain/agent_session"
	service "media_agent/media_agent_gateway/biz/service/agent_session"
	"media_agent/media_agent_gateway/biz/shared/client"
)

type workflowRepo struct{ session *domain.Session }

func (r *workflowRepo) FindLatestUsable(context.Context, string) (*domain.Session, error) {
	if r.session == nil {
		return nil, domain.ErrNotFound
	}
	return r.session, nil
}
func (r *workflowRepo) ListOwned(context.Context, string, string) ([]domain.Session, error) {
	if r.session == nil {
		return nil, nil
	}
	return []domain.Session{*r.session}, nil
}
func (r *workflowRepo) CreateSession(_ context.Context, s *domain.Session) error {
	r.session = s
	return nil
}
func (r *workflowRepo) GetOwned(_ context.Context, id, user string) (*domain.Session, error) {
	if r.session == nil || r.session.ID != id {
		return nil, domain.ErrNotFound
	}
	if r.session.UserID != user {
		return nil, domain.ErrForbidden
	}
	return r.session, nil
}
func (r *workflowRepo) TransitionStatus(_ context.Context, id, from, to string, at time.Time) error {
	if r.session == nil || r.session.ID != id || r.session.Status != from {
		return domain.ErrNotFound
	}
	r.session.Status, r.session.UpdatedAt = to, at
	return nil
}

type workflowContexts struct {
	value service.AgentContext
	err   error
}

func (c workflowContexts) Default(context.Context, string) (service.AgentContext, error) {
	return c.value, nil
}
func (c workflowContexts) Resolve(context.Context, string, string, string) (service.AgentContext, error) {
	return c.value, c.err
}

type fakeRuntime struct {
	registerInput                   client.RegisterRuntimeSessionInput
	registerState                   *client.RuntimeSession
	registerErr                     error
	submitInput                     client.SubmitTurnInput
	state                           client.RuntimeSession
	interruptSession, interruptTurn string
	messages                        []client.Message
	eventPages                      map[int64][]client.UniversalEvent
	replay                          func(int64) []client.UniversalEvent
	requests                        []client.AgentRequest
	resolvedRequest                 *client.AgentRequest
}

func (r *fakeRuntime) RegisterRuntimeSession(_ context.Context, in client.RegisterRuntimeSessionInput) (*client.RuntimeSession, error) {
	r.registerInput = in
	return r.registerState, r.registerErr
}
func (r *fakeRuntime) GetRuntimeSession(context.Context, string) (*client.RuntimeSession, error) {
	return &r.state, nil
}
func (r *fakeRuntime) SubmitTurn(_ context.Context, in client.SubmitTurnInput) (*client.Turn, error) {
	r.submitInput = in
	return &client.Turn{TurnID: "turn", SessionID: in.SessionID, Status: "queued"}, nil
}
func (r *fakeRuntime) GetTurn(_ context.Context, sessionID, turnID string) (*client.Turn, error) {
	return &client.Turn{TurnID: turnID, SessionID: sessionID, Status: "completed"}, nil
}
func (r *fakeRuntime) InterruptTurn(_ context.Context, session, turn string) (bool, error) {
	r.interruptSession, r.interruptTurn = session, turn
	return true, nil
}
func (r *fakeRuntime) ListMessages(context.Context, string, int) ([]client.Message, error) {
	return r.messages, nil
}
func (r *fakeRuntime) ReplayEvents(_ context.Context, _ string, after int64, _ int) ([]client.UniversalEvent, error) {
	if r.replay != nil {
		return r.replay(after), nil
	}
	return r.eventPages[after], nil
}
func (r *fakeRuntime) ListAgentRequests(context.Context, string, string) ([]client.AgentRequest, error) {
	return r.requests, nil
}
func (r *fakeRuntime) ResolveAgentRequest(context.Context, string, string, string, map[string]any) (*client.AgentRequest, error) {
	return r.resolvedRequest, nil
}

func newWorkflowFixture(status string) (*RunTurn, *workflowRepo, *fakeRuntime) {
	session := &domain.Session{ID: "session", UserID: "user", WorkspaceID: "workspace", AgentID: "agent", AgentType: "codex", ContextVersion: 7, Status: status}
	repo := &workflowRepo{session: session}
	runtime := &fakeRuntime{registerState: &client.RuntimeSession{SessionID: "session", ContextVersion: 7, Status: "active", ChatStatus: "human"}}
	contexts := workflowContexts{value: service.AgentContext{WorkspaceID: "workspace", AgentID: "agent", AgentType: "codex", ContextVersion: 7, APIKey: "secret-api-key"}}
	return NewRunTurn(service.New(repo, contexts), client.Registry{"codex": runtime}), repo, runtime
}

func TestRegisterActivatesOnlyAfterCompatibleAgentRegistration(t *testing.T) {
	flow, repo, runtime := newWorkflowFixture(domain.StatusStarting)
	if _, err := flow.Register(context.Background(), repo.session, "session-token"); err != nil {
		t.Fatal(err)
	}
	if repo.session.Status != domain.StatusActive {
		t.Fatalf("status=%q", repo.session.Status)
	}
	if runtime.registerInput.SessionID != "session" || runtime.registerInput.ContextVersion != 7 || runtime.registerInput.ContextRef != "workspace:workspace/agent:agent/context:7" {
		t.Fatalf("input=%+v", runtime.registerInput)
	}
}

func TestRegisterKeepsStartingForOutcomeUnknownAndFailsDefinitiveConflict(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"transport", errors.New("connection reset"), domain.StatusStarting},
		{"server", &client.APIError{StatusCode: http.StatusServiceUnavailable}, domain.StatusStarting},
		{"conflict", &client.APIError{StatusCode: http.StatusConflict}, domain.StatusFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flow, repo, runtime := newWorkflowFixture(domain.StatusStarting)
			runtime.registerErr = tc.err
			runtime.registerState = nil
			if _, err := flow.Register(context.Background(), repo.session, ""); err == nil {
				t.Fatal("expected error")
			}
			if repo.session.Status != tc.want {
				t.Fatalf("status=%q, want %q", repo.session.Status, tc.want)
			}
		})
	}
}

func TestStartDelegatesSecretsAndFactsToAgent(t *testing.T) {
	flow, repo, runtime := newWorkflowFixture(domain.StatusActive)
	turn, err := flow.Start(context.Background(), "session", "user", "session-token", "idem", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if turn.TurnID != "turn" || runtime.submitInput.IdempotencyKey != "idem" || runtime.submitInput.APIKey != "secret-api-key" || runtime.submitInput.SessionToken != "" {
		t.Fatalf("submit=%+v turn=%+v", runtime.submitInput, turn)
	}
	encoded, _ := json.Marshal(repo.session)
	if string(encoded) == "" || contains(string(encoded), "secret") {
		t.Fatalf("Registration persisted a secret: %s", encoded)
	}
}

func TestFailedRegistrationCannotSubmitTurn(t *testing.T) {
	flow, _, runtime := newWorkflowFixture(domain.StatusFailed)
	if _, err := flow.Start(context.Background(), "session", "user", "token", "key", "hello"); err != domain.ErrInactive {
		t.Fatalf("error=%v", err)
	}
	if runtime.submitInput.SessionID != "" {
		t.Fatal("failed Registration reached Agent SubmitTurn")
	}
}

func TestStartRejectsChangedWorkspaceConfigurationWithoutCallingAgent(t *testing.T) {
	flow, repo, runtime := newWorkflowFixture(domain.StatusActive)
	flow.sessions = service.New(repo, workflowContexts{value: service.AgentContext{
		WorkspaceID: "workspace", AgentID: "claude-code", AgentType: "claude-code", ContextVersion: 8,
	}})
	if _, err := flow.Start(context.Background(), "session", "user", "token", "key", "hello"); !errors.Is(err, domain.ErrContextChanged) {
		t.Fatalf("error=%v", err)
	}
	if runtime.registerInput.SessionID != "" || runtime.submitInput.SessionID != "" {
		t.Fatalf("stale Session reached Agent: register=%+v submit=%+v", runtime.registerInput, runtime.submitInput)
	}
}

func TestInterruptUsesAgentActiveTurn(t *testing.T) {
	flow, _, runtime := newWorkflowFixture(domain.StatusActive)
	runtime.state.ActiveTurnID = "turn-active"
	turn, err := flow.Interrupt(context.Background(), "session", "user", "token")
	if err != nil {
		t.Fatal(err)
	}
	if turn.TurnID != "turn-active" || runtime.interruptSession != "session" || runtime.interruptTurn != "turn-active" {
		t.Fatalf("turn=%+v interrupt=%s/%s", turn, runtime.interruptSession, runtime.interruptTurn)
	}
}

func TestInterruptRemainsAvailableAfterWorkspaceConfigurationChanges(t *testing.T) {
	session := &domain.Session{ID: "session", UserID: "user", WorkspaceID: "workspace", AgentID: "codex", AgentType: "codex", ContextVersion: 7, Status: domain.StatusActive}
	repo := &workflowRepo{session: session}
	runtime := &fakeRuntime{state: client.RuntimeSession{SessionID: "session", ActiveTurnID: "turn-active"}}
	contexts := workflowContexts{err: errors.New("current Workspace configuration is unavailable")}
	flow := NewRunTurn(service.New(repo, contexts), client.Registry{"codex": runtime})

	turn, err := flow.Interrupt(context.Background(), "session", "user", "token")
	if err != nil {
		t.Fatal(err)
	}
	if turn.TurnID != "turn-active" || runtime.interruptTurn != "turn-active" {
		t.Fatalf("turn=%+v interrupt=%s/%s", turn, runtime.interruptSession, runtime.interruptTurn)
	}
}

func TestAgentRequestsAreAuthorizedAndDelegated(t *testing.T) {
	flow, _, runtime := newWorkflowFixture(domain.StatusActive)
	runtime.requests = []client.AgentRequest{{RequestID: "request"}}
	runtime.resolvedRequest = &client.AgentRequest{RequestID: "request", Status: "resolved"}

	requests, err := flow.AgentRequests(context.Background(), "session", "user", "turn")
	if err != nil || len(requests) != 1 || requests[0].RequestID != "request" {
		t.Fatalf("requests=%+v error=%v", requests, err)
	}
	resolved, err := flow.ResolveAgentRequest(context.Background(), "session", "user", "turn", "request", map[string]any{"option_id": "allow"})
	if err != nil || resolved.Status != "resolved" {
		t.Fatalf("resolved=%+v error=%v", resolved, err)
	}
	if _, err := flow.AgentRequests(context.Background(), "session", "other-user", "turn"); err != domain.ErrForbidden {
		t.Fatalf("unauthorized error=%v", err)
	}
}

func contains(value, needle string) bool {
	for i := 0; i+len(needle) <= len(value); i++ {
		if value[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

type startingRouter struct {
	peer          client.Runtime
	calls         int
	alwaysWaiting bool
}

func (r *startingRouter) Resolve(context.Context, *domain.Session) (client.Runtime, error) {
	r.calls++
	if r.alwaysWaiting || r.calls == 1 {
		return nil, ErrRuntimeWaiting
	}
	return r.peer, nil
}
func (r *startingRouter) Facts(context.Context, *domain.Session) (client.Runtime, error) {
	return r.peer, nil
}
func (r *startingRouter) Owner(context.Context, *domain.Session) (client.Runtime, error) {
	return r.peer, nil
}
func TestRegisterWaitsForNativeStartupWithoutCreatingAnotherSession(t *testing.T) {
	flow, repo, peer := newWorkflowFixture(domain.StatusStarting)
	router := &startingRouter{peer: peer}
	flow.WithRouting(router)
	if _, err := flow.Register(context.Background(), repo.session, ""); err != nil {
		t.Fatal(err)
	}
	if router.calls != 2 || repo.session.ID != "session" || repo.session.Status != domain.StatusActive {
		t.Fatal("startup retry changed session identity")
	}
	flow, repo, peer = newWorkflowFixture(domain.StatusStarting)
	router = &startingRouter{peer: peer, alwaysWaiting: true}
	flow.WithRouting(router)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := flow.Register(ctx, repo.session, ""); !errors.Is(err, ErrRuntimeWaiting) || repo.session.Status != domain.StatusStarting {
		t.Fatal("cancelled startup lost recoverable session")
	}
}

func TestStartRejectsPausedAgentWithoutSubmittingAnotherTurn(t *testing.T) {
	flow, _, runtime := newWorkflowFixture(domain.StatusActive)
	runtime.registerState.Status = "paused"
	_, err := flow.Start(context.Background(), "session", "user", "", "key", "hello")
	if !errors.Is(err, ErrSessionPaused) || runtime.submitInput.SessionID != "" {
		t.Fatalf("err=%v; input=%+v", err, runtime.submitInput)
	}
}
