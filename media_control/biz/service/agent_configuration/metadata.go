package agent_configuration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	d "media_agent/media_control/biz/domain/agent_configuration"
	"regexp"
	"time"
)

type TxRunner interface {
	RunInTx(context.Context, func(context.Context) error) error
}
type Service struct {
	repo d.Repository
	tx   TxRunner
}

func New(repo d.Repository, tx TxRunner) *Service { return &Service{repo, tx} }
func Hash(value []byte) string                    { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }
func (s *Service) Resource(ctx context.Context, ref d.ResourceReference) (*d.ResourceVersion, error) {
	return s.repo.Resource(ctx, ref)
}
func (s *Service) Existing(ctx context.Context, id, key, hash string) (*d.Release, error) {
	v, err := s.repo.FindByKey(ctx, id, key)
	if err == nil && v.RequestHash != hash {
		return nil, d.ErrConflict
	}
	return v, err
}
func (s *Service) Confirm(ctx context.Context, snapshot d.Snapshot, user, key, hash string, expected int64) (*d.Release, error) {
	if expected <= 0 || key == "" || len(key) > 128 {
		return nil, d.ErrInvalid
	}
	snapshot.SelectionRevision = expected + 1
	snapshot.ContextVersion = expected + 1
	snapshot.TargetContextVersion = uuid.NewString()
	now := time.Now().UTC()
	manifest, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	release := &d.Release{Snapshot: snapshot, RequestHash: hash, IdempotencyKey: key, CreatorID: user, Manifest: manifest, CreatedAt: now}
	eventID := uuid.NewString()
	payload, err := json.Marshal(map[string]any{"schema_version": 1, "type": "agent.metadata.update_requested", "data_version": 1, "event_id": eventID, "source": "media_control", "occurred_at": now.Format(time.RFC3339Nano), "scope": map[string]string{"workspace_id": snapshot.WorkspaceID, "agent_config_id": snapshot.AgentConfigID}, "data": map[string]any{"selection_revision": snapshot.SelectionRevision, "target_context_version": snapshot.TargetContextVersion}})
	if err != nil {
		return nil, err
	}
	if err := s.tx.RunInTx(ctx, func(ctx context.Context) error {
		return s.repo.SaveSelection(ctx, release, expected, d.Notice{EventID: eventID, Payload: payload})
	}); err != nil {
		return nil, err
	}
	return s.repo.FindByKey(ctx, snapshot.AgentConfigID, key)
}
func (s *Service) Get(ctx context.Context, ws, id, target string, revision int64, forSession bool) (*d.Release, bool, error) {
	current, err := s.repo.Current(ctx, ws, id)
	if err != nil {
		return nil, false, err
	}
	if target == "" {
		if revision != 0 {
			return nil, false, d.ErrInvalid
		}
		return current, false, nil
	}
	selected, err := s.repo.Find(ctx, ws, id, target)
	if err != nil {
		return nil, false, err
	}
	if revision != 0 && revision != selected.SelectionRevision {
		return nil, false, d.ErrConflict
	}
	if !forSession && selected.TargetContextVersion != current.TargetContextVersion {
		return current, true, nil
	}
	return selected, false, nil
}

var stableErrorCode = regexp.MustCompile(`^[a-z][a-z0-9_]{0,127}$`)

func (s *Service) Report(ctx context.Context, v d.ApplicationResult) error {
	if v.ReportID == "" || len(v.ReportID) > 128 || v.ReplicaID == "" || v.IncarnationID == "" || len(v.ErrorCode) > 128 {
		return d.ErrInvalid
	}
	switch v.Status {
	case "applied":
		if v.AppliedContextVersion != v.TargetContextVersion || v.ErrorCode != "" {
			return d.ErrInvalid
		}
	case "failed", "incompatible":
		if !stableErrorCode.MatchString(v.ErrorCode) {
			return d.ErrInvalid
		}
	case "superseded":
		if v.ErrorCode != "" {
			return d.ErrInvalid
		}
	default:
		return d.ErrInvalid
	}
	release, err := s.repo.Find(ctx, v.WorkspaceID, v.AgentConfigID, v.TargetContextVersion)
	if err != nil {
		return err
	}
	if release.SelectionRevision != v.SelectionRevision {
		return d.ErrConflict
	}
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.tx.RunInTx(ctx, func(ctx context.Context) error { return s.repo.SaveReport(ctx, v, Hash(body)) })
}

type Progress struct {
	Release   *d.Release
	Results   []d.ApplicationResult
	Instances []d.Instance
	Status    string
	TimedOut  bool
}

func (s *Service) Progress(ctx context.Context, ws, id string, instances []d.Instance, complete bool) (Progress, error) {
	release, err := s.repo.Current(ctx, ws, id)
	if err != nil {
		return Progress{}, err
	}
	results, err := s.repo.Reports(ctx, id, release.SelectionRevision)
	if err != nil {
		return Progress{}, err
	}
	progress := Progress{Release: release, Results: results, Instances: instances, Status: "updating", TimedOut: time.Since(release.CreatedAt) > 300*time.Second}
	applied, failed, total := 0, 0, 0
	for _, instance := range instances {
		if instance.AgentConfigID != id || instance.WorkspaceID != ws || instance.Admission == "draining" {
			continue
		}
		total++
		status := ""
		for _, r := range results {
			if r.ReplicaID == instance.ReplicaID && r.IncarnationID == instance.IncarnationID {
				if status != "applied" {
					status = r.Status
				}
			}
		}
		switch status {
		case "applied":
			applied++
		case "failed", "incompatible":
			failed++
		}
	}
	if !complete {
		progress.Status = "waiting_instances"
	} else if total > 0 && applied == total {
		progress.Status = "applied"
		progress.TimedOut = false
	} else if total > 0 && failed == total {
		progress.Status = "failed"
	} else if applied > 0 && failed > 0 {
		progress.Status = "partial_failure"
	}
	return progress, nil
}
