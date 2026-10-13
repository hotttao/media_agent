package workflow

import (
	"context"
	"errors"
	"fmt"
	"log"
	contract "media_agent/hertz_infra/agentworkload"
	instance "media_agent/media_agent_gateway/biz/domain/agent_instance"
	session "media_agent/media_agent_gateway/biz/domain/agent_session"
	instances "media_agent/media_agent_gateway/biz/service/agent_instance"
	"media_agent/media_agent_gateway/biz/shared/client"
	"strconv"
	"time"
)

// WorkloadTarget is an authorized Control projection, never a Gateway configuration cache.
type WorkloadTarget = client.WorkloadTarget
type NativeWorkload = client.NativeWorkload
type WorkloadControl interface {
	Ensure(context.Context, string, string) (WorkloadTarget, error)
	List(context.Context, string) ([]WorkloadTarget, string, error)
}
type WorkloadResources interface {
	Read(context.Context, contract.Spec) (*NativeWorkload, error)
	Save(context.Context, *NativeWorkload) error
}

var ErrWorkloadAbsent = client.ErrWorkloadAbsent
var ErrWorkloadConflict = client.ErrWorkloadConflict

type WorkloadProvisioning struct {
	Control       WorkloadControl
	Resources     WorkloadResources
	EnvironmentID string
	Instances     *instances.Service
	Bindings      session.BindingRepository
	Routing       *SessionRouting
	Factory       RuntimeFactory
	Native        instance.ResourceAuthority
}

func (w *WorkloadProvisioning) Ensure(ctx context.Context, user, workspace string) error {
	target, e := w.Control.Ensure(ctx, user, workspace)
	if e != nil {
		return e
	}
	if target.Spec.EnvironmentID != w.EnvironmentID || target.Version < 1 {
		return ErrWorkloadConflict
	}
	state, e := w.Resources.Read(ctx, target.Spec)
	if errors.Is(e, ErrWorkloadAbsent) {
		return w.Resources.Save(ctx, &NativeWorkload{Spec: target.Spec, Version: target.Version})
	}
	if e != nil {
		return e
	}
	approved := state.Spec
	approved.Replicas = target.Spec.Replicas
	if approved != target.Spec {
		return ErrWorkloadConflict
	}
	if state.Spec.AgentConfigID != target.Spec.AgentConfigID {
		return ErrWorkloadConflict
	}
	if state.Operation != "" {
		return w.advanceDrain(ctx, state)
	}
	if state.Version > target.Version {
		return ErrWorkloadConflict
	}
	if state.Version == target.Version && state.Spec.Replicas == target.Spec.Replicas {
		return nil
	}
	if target.Spec.Replicas >= state.Spec.Replicas {
		state.Spec.Replicas = target.Spec.Replicas
		state.Version = target.Version
		return w.Resources.Save(ctx, state)
	}
	// Persist the same operation across all Gateways and restarts before any Agent call.
	state.PendingReplicas = target.Spec.Replicas
	state.PendingVersion = target.Version
	state.Operation = fmt.Sprintf("scale:%s:%d", contract.Name(state.Spec.EnvironmentID, state.Spec.WorkspaceID), target.Version)
	if e = w.Resources.Save(ctx, state); e != nil {
		return e
	}
	return ErrRuntimeWaiting
}
func (w *WorkloadProvisioning) advanceDrain(ctx context.Context, state *NativeWorkload) error {
	rows, e := w.Instances.List(ctx, state.Spec.EnvironmentID, state.Spec.WorkspaceID, state.Spec.AgentConfigID)
	if e != nil {
		return e
	}
	if state.Spec.Replicas != state.PendingReplicas {

		// Recover bindings whose native incarnation changed while an operation
		// was pending, before preventing the replacement from accepting transfer.
		cursor := ""
		for {
			bound, e := w.Bindings.BoundSessions(ctx, cursor, 100)
			if e != nil {
				return e
			}
			if len(bound) == 0 {
				break
			}
			for i := range bound {
				s := &bound[i]
				cursor = s.ID
				if s.EnvironmentID != state.Spec.EnvironmentID || s.WorkspaceID != state.Spec.WorkspaceID || s.AgentConfigID != state.Spec.AgentConfigID {
					continue
				}
				current, e := w.Instances.Get(ctx, s.EnvironmentID, s.WorkspaceID, s.AgentConfigID, s.ReplicaID)
				if e == nil && current.IncarnationID != s.IncarnationID && current.Admission == "active" {
					if _, e = w.Routing.resolveBinding(ctx, s); e != nil {
						return e
					}
				}
			}
		}
		// Set all source admission barriers before migrating any Session.
		for i := range rows {
			v := &rows[i]
			ordinal, e := strconv.Atoi(v.ReplicaID)
			if e != nil {
				return ErrWorkloadConflict
			}
			if ordinal < int(state.PendingReplicas) || v.Admission == "retired" {
				continue
			}
			if e = w.Instances.Drain(ctx, v, state.Operation); e != nil {
				return e
			}
		}
		for _, v := range rows {
			ordinal, _ := strconv.Atoi(v.ReplicaID)
			if ordinal < int(state.PendingReplicas) || v.Admission == "retired" {
				continue
			}
			peer := w.Factory.ForInstance(v, 0)
			_ = peer.BeginDrain(ctx, state.Operation)
			drain, e := peer.GetDrain(ctx)
			if e != nil || drain.OperationID != state.Operation || drain.IncarnationID != v.IncarnationID || drain.RunningCount != 0 || !drain.Quiesced {
				stopped, err := w.Native.Terminated(ctx, v)
				if err != nil || !stopped {
					return ErrRuntimeWaiting
				}
				// Positive native termination permits transfer through a surviving Agent;
				// without a survivor at scale-to-zero we cannot inspect/flush business state.
				if state.PendingReplicas == 0 {
					return ErrRuntimeWaiting
				}
			}
		}
		cursor = ""
		for {
			bound, e := w.Bindings.BoundSessions(ctx, cursor, 100)
			if e != nil {
				return e
			}
			if len(bound) == 0 {
				break
			}
			for i := range bound {
				s := &bound[i]
				cursor = s.ID
				if s.EnvironmentID != state.Spec.EnvironmentID || s.WorkspaceID != state.Spec.WorkspaceID || s.AgentConfigID != state.Spec.AgentConfigID {
					continue
				}
				ordinal, e := strconv.Atoi(s.ReplicaID)
				if e != nil {
					return ErrWorkloadConflict
				}
				if ordinal < int(state.PendingReplicas) {
					continue
				}

				if state.PendingReplicas == 0 {
					var peer client.ControlledRuntime
					for _, v := range rows {
						observed, e := w.Native.Observe(ctx, v)
						if e == nil && observed.Running && observed.Generation == v.ResourceGeneration && observed.Locator.ResourceUID == v.ResourceUID {
							peer = w.Factory.ForInstance(v, 0)
							break
						}
					}
					if peer == nil {
						return ErrRuntimeWaiting
					}
					facts, e := peer.GetRuntimeSession(ctx, s.ID)
					if e != nil {
						var api *client.APIError
						if !errors.As(e, &api) || api.StatusCode != 404 {
							return ErrRuntimeWaiting
						}
					} else {
						if facts.ActiveTurnID != "" {
							return ErrRuntimeWaiting
						}
						if facts.OwnerIncarnationID != s.IncarnationID || facts.OwnerInstanceID != s.ReplicaID || facts.ExecutionEpoch != s.ExecutionEpoch {
							return ErrRuntimeWaiting
						}
						current, e := w.Instances.Get(ctx, s.EnvironmentID, s.WorkspaceID, s.AgentConfigID, s.ReplicaID)
						if e != nil || current.IncarnationID != s.IncarnationID {
							old, e := w.Instances.Historical(ctx, s.EnvironmentID, s.WorkspaceID, s.AgentConfigID, s.ReplicaID, s.IncarnationID)
							if e != nil {
								return ErrRuntimeWaiting
							}
							stopped, e := w.Native.Terminated(ctx, *old)
							if e != nil || !stopped {
								return ErrRuntimeWaiting
							}
						}
					}
				}
				if state.PendingReplicas > 0 {
					if _, e = w.Routing.resolveBinding(ctx, s); e != nil {
						return e
					}
					newOrdinal, _ := strconv.Atoi(s.ReplicaID)
					if s.BindingPhase != "active" || newOrdinal >= int(state.PendingReplicas) {
						return ErrRuntimeWaiting
					}
				}
			}
		}

		pending, e := w.Bindings.PendingTransfers(ctx, 256)
		if e != nil {
			return e
		}
		if len(pending) == 256 {
			return ErrRuntimeWaiting
		}
		for _, s := range pending {
			if s.EnvironmentID == state.Spec.EnvironmentID && s.WorkspaceID == state.Spec.WorkspaceID && s.AgentConfigID == state.Spec.AgentConfigID {
				return ErrRuntimeWaiting
			}
		}
		state.Spec.Replicas = state.PendingReplicas
		state.Version = state.PendingVersion
		if e = w.Resources.Save(ctx, state); e != nil {
			return e
		}
		// Read after CAS commit; retirement is recovered with the persisted operation.
		return ErrRuntimeWaiting
	}
	for i := range rows {
		ordinal, e := strconv.Atoi(rows[i].ReplicaID)
		if e != nil {
			return ErrWorkloadConflict
		}
		if ordinal >= int(state.PendingReplicas) {
			if e = w.Instances.Retire(ctx, &rows[i], state.Operation); e != nil {
				return e
			}
		}
	}
	if !state.Settled {
		return ErrRuntimeWaiting
	}
	state.Operation = ""
	state.PendingVersion = 0
	state.PendingReplicas = 0
	return w.Resources.Save(ctx, state)
}
func (w *WorkloadProvisioning) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		cursor := ""
		for {
			targets, next, e := w.Control.List(ctx, cursor)
			if e != nil {
				log.Print("workload Control reconciliation deferred")
				break
			}
			for _, v := range targets {
				if v.Spec.EnvironmentID != w.EnvironmentID {
					continue
				}
				call, cancel := context.WithTimeout(ctx, 20*time.Second)
				e = w.Ensure(call, v.OwnerUserID, v.Spec.WorkspaceID)
				cancel()
				if e != nil && !errors.Is(e, ErrRuntimeWaiting) {
					log.Printf("workload reconciliation deferred workspace=%s", v.Spec.WorkspaceID)
				}
			}
			if next == "" {
				break
			}
			cursor = next
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *WorkloadProvisioning) Membership(ctx context.Context, ws string) ([]client.MetadataInstance, bool, error) {
	// Read the authoritative desired projection each time. Missing/unready ordinals
	// are explicit pending members, so a partial rollout cannot report all applied.
	cursor := ""
	for {
		targets, next, e := w.Control.List(ctx, cursor)
		if e != nil {
			return nil, false, e
		}
		for _, target := range targets {
			if target.Spec.WorkspaceID != ws || target.Spec.EnvironmentID != w.EnvironmentID {
				continue
			}
			current, e := w.Control.Ensure(ctx, target.OwnerUserID, ws)
			if e != nil {
				return nil, false, e
			}
			rows, e := w.Instances.List(ctx, current.Spec.EnvironmentID, ws, current.Spec.AgentConfigID)
			if e != nil {
				return nil, false, e
			}
			result := []client.MetadataInstance{}
			for ordinal := int32(0); ordinal < current.Spec.Replicas; ordinal++ {
				v := client.MetadataInstance{WorkspaceID: ws, AgentConfigID: current.Spec.AgentConfigID, ReplicaID: strconv.Itoa(int(ordinal)), Admission: "active"}
				for _, row := range rows {
					if row.ReplicaID != v.ReplicaID || row.Admission != "active" {
						continue
					}
					f, e := w.Native.Observe(ctx, row)
					if e == nil && f.Running && f.Eligible && f.Generation == row.ResourceGeneration && f.Locator.ResourceUID == row.ResourceUID {
						v.IncarnationID = row.IncarnationID
					}
					break
				}
				result = append(result, v)
			}
			return result, true, nil
		}
		if next == "" {
			return nil, false, ErrWorkloadAbsent
		}
		cursor = next
	}
}

type WorkloadView struct {
	Target WorkloadTarget
	Native *NativeWorkload
}

func (w *WorkloadProvisioning) View(ctx context.Context, user, ws string) (WorkloadView, error) {
	target, e := w.Control.Ensure(ctx, user, ws)
	if e != nil {
		return WorkloadView{}, e
	}
	if target.Spec.EnvironmentID != w.EnvironmentID {
		return WorkloadView{}, ErrWorkloadConflict
	}
	native, e := w.Resources.Read(ctx, target.Spec)
	if errors.Is(e, ErrWorkloadAbsent) {
		return WorkloadView{Target: target}, nil
	}
	return WorkloadView{Target: target, Native: native}, e
}
func (w *WorkloadProvisioning) Scale(ctx context.Context, user, ws, key string, replicas int32, expected int64) error {
	if key == "" || len(key) > 128 || expected < 1 {
		return ErrWorkloadConflict
	}
	target, e := w.Control.Ensure(ctx, user, ws)
	if e != nil {
		return e
	}
	if target.Spec.Driver != "kubernetes" || target.Version != expected || target.Spec.Replicas != replicas {
		return ErrWorkloadConflict
	}
	return w.Ensure(ctx, user, ws)
}
