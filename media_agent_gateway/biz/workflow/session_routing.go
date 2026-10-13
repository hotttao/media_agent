package workflow

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"log"
	instance "media_agent/media_agent_gateway/biz/domain/agent_instance"
	session "media_agent/media_agent_gateway/biz/domain/agent_session"
	instances "media_agent/media_agent_gateway/biz/service/agent_instance"
	sessions "media_agent/media_agent_gateway/biz/service/agent_session"
	"media_agent/media_agent_gateway/biz/shared/client"
	"time"
)

var ErrRuntimeWaiting = errors.New("runtime ownership reconciliation pending")

type RuntimeFactory interface {
	ForInstance(instance.Instance, int64) client.ControlledRuntime
}
type SessionRouting struct {
	repo        session.BindingRepository
	sessions    *sessions.Service
	instances   *instances.Service
	placement   EnvironmentAuthority
	native      instance.ResourceAuthority
	factory     RuntimeFactory
	provisioner interface {
		Ensure(context.Context, string, string) error
	}
}

func NewSessionRouting(repo session.BindingRepository, sessions *sessions.Service, instances *instances.Service, placement EnvironmentAuthority, native instance.ResourceAuthority, factory RuntimeFactory) *SessionRouting {
	return &SessionRouting{repo: repo, sessions: sessions, instances: instances, placement: placement, native: native, factory: factory}
}
func (w *SessionRouting) canonical(ctx context.Context, requested *session.Session) (*session.Session, error) {
	current, err := w.repo.GetSession(ctx, requested.ID)
	if err != nil {
		return nil, err
	}
	if current.AgentConfigID == "" {
		fixed, err := w.sessions.RuntimeContext(ctx, current, "")
		if err != nil {
			return nil, err
		}
		if fixed.AgentConfigID == "" {
			return nil, ErrRuntimeWaiting
		}
		current, err = w.repo.EnsureIdentity(ctx, current.ID, fixed.AgentConfigID)
		if err != nil {
			return nil, err
		}
	}
	*requested = *current
	return current, nil
}
func (w *SessionRouting) candidates(ctx context.Context, s *session.Session) ([]instance.Instance, error) {
	environment, err := w.placement.Environment(ctx, s.UserID, s.WorkspaceID)
	if err != nil {
		return nil, err
	}
	rows, err := w.instances.List(ctx, environment, s.WorkspaceID, s.AgentConfigID)
	if err != nil {
		return nil, err
	}
	ready := []instance.Instance{}
	for _, v := range rows {
		if v.Admission != "active" || v.AgentType != s.AgentType {
			continue
		}
		f, err := w.native.Observe(ctx, v)
		if err == nil && f.Ready && f.Running && f.Eligible && f.Locator.ResourceUID == v.ResourceUID && f.Generation == v.ResourceGeneration && f.AgentType == s.AgentType {
			ready = append(ready, v)
		}
	}
	if len(ready) == 0 {
		return nil, ErrRuntimeWaiting
	}
	return ready, nil
}
func choose(id string, candidates []instance.Instance) instance.Instance {
	selected := candidates[0]
	score := sha256.Sum256([]byte(id + ":" + selected.ReplicaID))
	for _, v := range candidates[1:] {
		next := sha256.Sum256([]byte(id + ":" + v.ReplicaID))
		if bytes.Compare(next[:], score[:]) > 0 {
			score = next
			selected = v
		}
	}
	return selected
}
func (w *SessionRouting) WithProvisioner(p interface {
	Ensure(context.Context, string, string) error
}) *SessionRouting {
	w.provisioner = p
	return w
}
func (w *SessionRouting) Resolve(ctx context.Context, requested *session.Session) (client.Runtime, error) {
	if w.provisioner != nil {
		if e := w.provisioner.Ensure(ctx, requested.UserID, requested.WorkspaceID); e != nil && !errors.Is(e, ErrRuntimeWaiting) {
			return nil, e
		}
	}
	return w.resolveBinding(ctx, requested)
}
func (w *SessionRouting) resolveBinding(ctx context.Context, requested *session.Session) (client.Runtime, error) {
	for attempt := 0; attempt < 6; attempt++ {
		current, err := w.canonical(ctx, requested)
		if err != nil {
			return nil, err
		}
		if current.BindingPhase == "transferring" {
			if err := w.reconcile(ctx, current); err != nil {
				return nil, err
			}
			continue
		}
		ready, err := w.candidates(ctx, current)
		if err != nil {
			return nil, err
		}
		if current.BindingPhase == "active" {
			for _, v := range ready {
				if v.EnvironmentID == current.EnvironmentID && v.ReplicaID == current.ReplicaID && v.IncarnationID == current.IncarnationID {
					peer := w.factory.ForInstance(v, current.ExecutionEpoch)
					if current.Status == session.StatusActive {
						facts, e := peer.GetRuntimeSession(ctx, current.ID)
						if e != nil && current.ExecutionEpoch == 1 {
							var apiErr *client.APIError
							if errors.As(e, &apiErr) && (apiErr.StatusCode == 404 || apiErr.StatusCode == 409) {
								// Register serializes absent/legacy idle aggregates with
								// their first managed owner. Running legacy work is refused.
								facts, e = peer.RegisterRuntimeSession(ctx, client.RegisterRuntimeSessionInput{SessionID: current.ID, WorkspaceID: current.WorkspaceID, AgentConfigID: current.AgentConfigID, ContextVersion: current.ContextVersion, ContextRef: client.SessionContextReference(current)})
							}
						}
						if e != nil || facts.OwnerInstanceID != current.ReplicaID || facts.OwnerIncarnationID != current.IncarnationID || facts.ExecutionEpoch != current.ExecutionEpoch {
							return nil, ErrRuntimeWaiting
						}
						if facts.ActiveTurnID == "" && facts.Status == "active" {
							if _, e = peer.Adopt(ctx, current.ID); e != nil {
								return nil, ErrRuntimeWaiting
							}
						}
					}
					return peer, nil
				}
			}
			target := choose(current.ID, ready)
			if target.EnvironmentID != current.EnvironmentID {
				return nil, ErrRuntimeWaiting
			}
			_, err := w.repo.BeginTransfer(ctx, current.ID, current.BindingVersion, session.Binding{EnvironmentID: current.EnvironmentID, ReplicaID: target.ReplicaID, IncarnationID: target.IncarnationID, ExecutionEpoch: current.ExecutionEpoch}, uuid.NewString())
			if errors.Is(err, session.ErrBindingConflict) {
				continue
			}
			if err != nil {
				return nil, err
			}
			continue
		}
		target := choose(current.ID, ready)
		bound, err := w.repo.AssignBinding(ctx, current.ID, current.BindingVersion, session.Binding{EnvironmentID: target.EnvironmentID, ReplicaID: target.ReplicaID, IncarnationID: target.IncarnationID, ExecutionEpoch: 1})
		if errors.Is(err, session.ErrBindingConflict) {
			continue
		}
		if err != nil {
			return nil, err
		}
		*requested = *bound
		return w.factory.ForInstance(target, 1), nil
	}
	return nil, ErrRuntimeWaiting
}
func (w *SessionRouting) Facts(ctx context.Context, s *session.Session) (client.Runtime, error) {
	current, err := w.canonical(ctx, s)
	if err != nil {
		return nil, err
	}
	env, err := w.placement.Environment(ctx, s.UserID, s.WorkspaceID)
	if err != nil {
		return nil, err
	}
	rows, err := w.instances.List(ctx, env, s.WorkspaceID, s.AgentConfigID)
	if err != nil {
		return nil, err
	}
	usable, ready := []instance.Instance{}, []instance.Instance{}
	for _, v := range rows {
		if v.AgentType != current.AgentType {
			continue
		}
		f, e := w.native.Observe(ctx, v)
		if e == nil && f.Running && f.Generation == v.ResourceGeneration && f.Locator.ResourceUID == v.ResourceUID {
			usable = append(usable, v)
			if f.Ready {
				ready = append(ready, v)
			}
		}
	}
	if len(ready) > 0 {
		usable = ready
	}
	if len(usable) == 0 {
		return nil, ErrRuntimeWaiting
	}
	for _, v := range usable {
		if v.ReplicaID == current.ReplicaID && v.IncarnationID == current.IncarnationID {
			return w.factory.ForInstance(v, 0), nil
		}
	}
	return w.factory.ForInstance(choose(current.ID, usable), 0), nil
}
func (w *SessionRouting) Owner(ctx context.Context, s *session.Session) (client.Runtime, error) {
	runtime, err := w.Facts(ctx, s)
	if err != nil {
		return nil, err
	}
	facts, err := runtime.GetRuntimeSession(ctx, s.ID)
	if err != nil {
		return nil, err
	}
	owner, err := w.instances.Get(ctx, s.EnvironmentID, s.WorkspaceID, s.AgentConfigID, facts.OwnerInstanceID)
	if err != nil || owner.IncarnationID != facts.OwnerIncarnationID {
		return nil, ErrRuntimeWaiting
	}
	f, err := w.native.Observe(ctx, *owner)
	if err != nil || f.Generation != owner.ResourceGeneration || !f.Running {
		return nil, ErrRuntimeWaiting
	}
	return w.factory.ForInstance(*owner, facts.ExecutionEpoch), nil
}
func (w *SessionRouting) reconcile(ctx context.Context, s *session.Session) error {
	source, err := w.instances.Historical(ctx, s.EnvironmentID, s.WorkspaceID, s.AgentConfigID, s.ReplicaID, s.IncarnationID)
	if err != nil {
		return ErrRuntimeWaiting
	}
	// Quiesce only the Session being moved; other Sessions on the replica
	// keep running. Lost responses repeat the durable transfer ID.
	_ = w.factory.ForInstance(*source, s.ExpectedExecutionEpoch).Quiesce(ctx, s.ID, s.TransferID)
	factsRuntime, err := w.Facts(ctx, s)
	if err != nil {
		return ErrRuntimeWaiting
	}
	facts, err := factsRuntime.GetRuntimeSession(ctx, s.ID)
	if err != nil {
		var apiErr *client.APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != 404 {
			return ErrRuntimeWaiting
		}
		// An absent execution aggregate has no running Turn to replay. The
		// primary key serializes a late source registration with this creation.
		target, e := w.transferTarget(ctx, s)
		if e != nil {
			return e
		}
		moved, e := w.factory.ForInstance(*target, s.ExpectedExecutionEpoch+1).RegisterRuntimeSession(ctx, client.RegisterRuntimeSessionInput{SessionID: s.ID, WorkspaceID: s.WorkspaceID, AgentConfigID: s.AgentConfigID, ContextVersion: s.ContextVersion, ContextRef: client.SessionContextReference(s), InitialEpoch: s.ExpectedExecutionEpoch + 1, TransferID: s.TransferID})
		if e != nil {
			return ErrRuntimeWaiting
		}
		return w.complete(ctx, s, moved)
	}
	if facts.LastTransferID == s.TransferID && facts.ExecutionEpoch == s.ExpectedExecutionEpoch+1 {
		return w.complete(ctx, s, facts)
	}
	if facts.OwnerInstanceID != s.ReplicaID || facts.OwnerIncarnationID != s.IncarnationID || facts.ExecutionEpoch != s.ExpectedExecutionEpoch {
		return ErrRuntimeWaiting
	}
	target, err := w.transferTarget(ctx, s)
	if err != nil {
		return err
	}
	terminated := false
	if facts.ActiveTurnID != "" {
		terminated, err = w.native.Terminated(ctx, *source)
		if err != nil || !terminated {
			return ErrRuntimeWaiting
		}
	}
	moved, err := w.factory.ForInstance(*target, 0).Transfer(ctx, s.ID, client.TransferSessionInput{TransferID: s.TransferID, SourceReplicaID: s.ReplicaID, SourceIncarnationID: s.IncarnationID, ExpectedEpoch: s.ExpectedExecutionEpoch, TerminationConfirmed: terminated})
	if err != nil {
		return ErrRuntimeWaiting
	}
	return w.complete(ctx, s, moved)
}
func (w *SessionRouting) complete(ctx context.Context, s *session.Session, facts *client.RuntimeSession) error {
	if facts.SessionID != s.ID || facts.WorkspaceID != s.WorkspaceID || facts.AgentConfigID != s.AgentConfigID || facts.ContextVersion != s.ContextVersion || facts.ExecutionEpoch != s.ExpectedExecutionEpoch+1 || facts.LastTransferID != s.TransferID {
		return ErrRuntimeWaiting
	}
	actual, err := w.instances.Historical(ctx, s.EnvironmentID, s.WorkspaceID, s.AgentConfigID, facts.OwnerInstanceID, facts.OwnerIncarnationID)
	if err != nil {
		return ErrRuntimeWaiting
	}
	bound, err := w.repo.CompleteTransfer(ctx, s.ID, s.BindingVersion, s.TransferID, session.Binding{EnvironmentID: s.EnvironmentID, ReplicaID: actual.ReplicaID, IncarnationID: actual.IncarnationID, ExecutionEpoch: facts.ExecutionEpoch})
	if errors.Is(err, session.ErrBindingConflict) {
		return nil
	}
	if err != nil {
		return err
	}
	*s = *bound
	// Adoption never resumes paused state. Failure after binding commit is recovered
	// by subsequent Resolve/periodic reconciliation, using this same owner/epoch.
	_, err = w.factory.ForInstance(*actual, bound.ExecutionEpoch).Adopt(ctx, s.ID)
	if err != nil {
		return ErrRuntimeWaiting
	}
	return nil
}
func (w *SessionRouting) AuthorizeMetadataInstance(ctx context.Context, user string, q client.MetadataQuery) error {
	env, err := w.placement.Environment(ctx, user, q.WorkspaceID)
	if err != nil {
		return err
	}
	v, err := w.instances.Get(ctx, env, q.WorkspaceID, q.AgentConfigID, q.ReplicaID)
	if err != nil {
		return err
	}
	if v.IncarnationID != q.IncarnationID {
		return ErrMetadataForbidden
	}
	f, err := w.native.Observe(ctx, *v)
	if err != nil {
		return err
	}
	if f.Generation != v.ResourceGeneration || f.Locator.ResourceUID != v.ResourceUID || !f.Running {
		return ErrMetadataForbidden
	}
	return nil
}
func (w *SessionRouting) AllowLive(ctx context.Context, id string, payload []byte) bool {
	var frame struct {
		Epoch       int64  `json:"execution_epoch"`
		Owner       string `json:"owner_instance_id"`
		Incarnation string `json:"owner_incarnation_id"`
	}
	if json.Unmarshal(payload, &frame) != nil {
		return false
	}
	s, err := w.repo.GetSession(ctx, id)
	return err == nil && s.BindingPhase == "active" && frame.Epoch == s.ExecutionEpoch && frame.Owner == s.ReplicaID && frame.Incarnation == s.IncarnationID
}
func (w *SessionRouting) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	cursor := ""
	for {
		pending, err := w.repo.PendingTransfers(ctx, 32)
		if err == nil {
			for i := range pending {
				call, cancel := context.WithTimeout(ctx, 15*time.Second)
				_ = w.reconcile(call, &pending[i])
				cancel()
			}
		}
		active, err := w.repo.BoundSessions(ctx, cursor, 32)
		if err == nil {
			if len(active) == 0 {
				cursor = ""
			}
			for i := range active {
				cursor = active[i].ID
				call, cancel := context.WithTimeout(ctx, 15*time.Second)
				_, _ = w.Resolve(call, &active[i])
				cancel()
			}
		} else {
			log.Print("Session ownership reconciliation deferred")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *SessionRouting) transferTarget(ctx context.Context, s *session.Session) (*instance.Instance, error) {
	target, err := w.instances.Get(ctx, s.EnvironmentID, s.WorkspaceID, s.AgentConfigID, s.TargetReplicaID)
	if err == nil && target.IncarnationID == s.TargetIncarnationID && target.Admission == "active" {
		observed, e := w.native.Observe(ctx, *target)
		if e == nil && observed.Ready && observed.Running && observed.Eligible && observed.Locator.ResourceUID == target.ResourceUID && observed.Generation == target.ResourceGeneration {
			return target, nil
		}
	}
	ready, err := w.candidates(ctx, s)
	if err != nil {
		return nil, ErrRuntimeWaiting
	}
	candidates := ready[:0]
	for _, v := range ready {
		if v.EnvironmentID == s.EnvironmentID && (v.ReplicaID != s.ReplicaID || v.IncarnationID != s.IncarnationID) {
			candidates = append(candidates, v)
		}
	}
	if len(candidates) == 0 {
		return nil, ErrRuntimeWaiting
	}
	chosen := choose(s.ID, candidates)
	updated, err := w.repo.RetargetTransfer(ctx, s.ID, s.BindingVersion, s.TransferID, session.Binding{EnvironmentID: s.EnvironmentID, ReplicaID: chosen.ReplicaID, IncarnationID: chosen.IncarnationID})
	if err != nil {
		return nil, ErrRuntimeWaiting
	}
	*s = *updated
	return &chosen, nil
}
