package job

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	domain "media_agent/media_workflow/biz/domain/job"
	wfdomain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/shared/client"
	"media_agent/media_workflow/biz/workflow/executable"
)

var ErrHistoryImportConflict = errors.New("historical execution source already imported with different facts")

type ImportedAssetLookup interface {
	GetImportedGeneratedAsset(ctx context.Context, id string) (*client.ImportedGeneratedAsset, error)
}

type HistoricalWorkspaceLookup interface {
	GetWorkspaceForUser(ctx context.Context, workspaceID, userID string) (*client.WorkspaceContext, error)
}

type HistoricalStep struct {
	StepKey        string `json:"step_key"`
	SourceRecordID string `json:"source_record_id"`
	AssetID        string `json:"asset_id"`
}

type ImportIncompleteCommand struct {
	SourceGroupID   string           `json:"source_group_id"`
	OrganizationID  string           `json:"organization_id"`
	WorkspaceID     string           `json:"workspace_id"`
	OwnerUserID     string           `json:"owner_user_id"`
	WorkflowID      string           `json:"workflow_id"`
	WorkflowVersion int              `json:"workflow_version"`
	Input           domain.JobInput  `json:"input"`
	Completed       []HistoricalStep `json:"completed"`
}

func (s *Service) ConfigureHistoryAssets(lookup ImportedAssetLookup) { s.historyAssets = lookup }
func (s *Service) ConfigureHistoryWorkspaces(lookup HistoricalWorkspaceLookup) {
	s.historyWorkspaces = lookup
}

// ImportIncomplete freezes a verified historical image prefix. It never
// starts Conductor; only an explicit Resume may continue this Job.
func (s *Service) ImportIncomplete(ctx context.Context, cmd ImportIncompleteCommand) (*domain.Job, error) {
	return s.importHistorical(ctx, cmd, false)
}

// ImportCompleted records one existing video as a terminal historical Job.
// Every Video has its own Job; a VideoPush is not collapsed into one Step output.
// Like ImportIncomplete, this never starts or invents a Conductor execution.
func (s *Service) ImportCompleted(ctx context.Context, cmd ImportIncompleteCommand) (*domain.Job, error) {
	return s.importHistorical(ctx, cmd, true)
}

func (s *Service) importHistorical(ctx context.Context, cmd ImportIncompleteCommand, completedVideo bool) (*domain.Job, error) {
	if s.historyAssets == nil || s.historyWorkspaces == nil || s.compilations == nil || s.jobRepo == nil {
		return nil, errors.New("historical import is not configured")
	}
	minSteps, maxSteps := 1, 3
	if completedVideo {
		minSteps, maxSteps = 4, 4
	}
	if cmd.SourceGroupID == "" || cmd.OrganizationID == "" || cmd.WorkspaceID == "" || cmd.OwnerUserID == "" ||
		cmd.WorkflowID == "" || cmd.WorkflowVersion <= 0 || cmd.Input.Product == nil || cmd.Input.Product.ID == "" ||
		cmd.Input.VirtualIP == nil || cmd.Input.VirtualIP.ID == "" || len(cmd.Completed) < minSteps || len(cmd.Completed) > maxSteps {
		return nil, errors.New("invalid historical execution import request")
	}
	if len(cmd.SourceGroupID) > 160 || len(cmd.WorkspaceID) > 160 {
		return nil, errors.New("historical source or workspace ID is too long")
	}
	workspace, err := s.historyWorkspaces.GetWorkspaceForUser(ctx, cmd.WorkspaceID, cmd.OwnerUserID)
	if err != nil {
		return nil, fmt.Errorf("verify historical execution workspace: %w", err)
	}
	if workspace.OrganizationID != cmd.OrganizationID {
		return nil, errors.New("historical execution organization does not match workspace")
	}
	stagePrefix := []string{"model:", "style:", "frame:", "video:"}[len(cmd.Completed)-1]
	if !strings.HasPrefix(cmd.SourceGroupID, stagePrefix) || strings.TrimPrefix(cmd.SourceGroupID, stagePrefix) != cmd.Completed[len(cmd.Completed)-1].SourceRecordID {
		return nil, errors.New("source group ID does not identify the last completed output")
	}
	fingerprint, err := historyImportDigest(cmd)
	if err != nil {
		return nil, err
	}
	existing, err := s.jobRepo.GetByLegacySource(ctx, cmd.WorkspaceID, "media_ai", cmd.SourceGroupID)
	if err == nil {
		if existing.SourceDigest != fingerprint {
			return nil, ErrHistoryImportConflict
		}
		return existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("lookup historical execution: %w", err)
	}
	wf, err := s.wfRepo.Get(ctx, cmd.WorkflowID)
	if err != nil || wf.Version != cmd.WorkflowVersion || wf.Status != wfdomain.StatusPublished || wf.Scope != "ai_clothing" {
		return nil, errors.New("target workflow is not the requested published ai_clothing version")
	}
	compilation, err := s.compilations.GetRegistered(ctx, wf.ID, wf.Version)
	if err != nil {
		return nil, fmt.Errorf("registered workflow compilation required: %w", err)
	}
	var ir executable.ExecutableWorkflowIR
	if err := json.Unmarshal(compilation.IRJSON, &ir); err != nil || ir.SchemaVersion != executable.SchemaVersion || ir.WorkflowID != wf.ID || ir.WorkflowVersion != wf.Version {
		return nil, errors.New("invalid frozen executable workflow IR")
	}
	steps := append([]executable.ExecutableStepIR(nil), ir.Steps...)
	sort.Slice(steps, func(i, j int) bool { return steps[i].Order < steps[j].Order })
	keys := []string{"model_image", "style_image", "first_frame", "video"}
	if len(steps) != len(keys) || ir.OutputStepKey != "video" {
		return nil, errors.New("historical image import requires the four-step media_ai workflow")
	}
	for i, step := range steps {
		if step.Key != keys[i] || i > 0 && (len(step.DependsOn) != 1 || step.DependsOn[0] != keys[i-1]) {
			return nil, errors.New("target workflow does not preserve the historical image chain")
		}
	}
	now := time.Now().UTC()
	jobID := "job_" + uuid.NewString()
	job := &domain.Job{
		ID: jobID, Origin: "legacy_import", OrganizationID: cmd.OrganizationID, WorkspaceID: cmd.WorkspaceID,
		SourceSystem: "media_ai", SourceRecordID: cmd.SourceGroupID, SourceDigest: fingerprint, ImportedAt: &now,
		Scope: wf.Scope, WorkflowID: wf.ID, WorkflowVersion: wf.Version, Input: cmd.Input,
		Status: domain.JobAwaitingResume, SubmittedBy: cmd.OwnerUserID, SubmittedAt: now,
	}
	if completedVideo {
		job.Status = domain.JobSucceeded
	}
	stepRuns := make([]*domain.StepRun, 0, len(steps))
	for i, step := range steps {
		sr := &domain.StepRun{
			ID: "sr_" + uuid.NewString(), JobID: jobID, StepKey: step.Key, StepOrder: step.Order,
			Origin: "native", TemplateID: step.Template.ID, TemplateVersion: step.Template.Version,
			ApprovalPolicy: domain.ApprovalPolicy(step.ApprovalPolicy), Status: domain.SRBlocked,
			ResolvedInputs: map[string]any{}, Candidates: []domain.Candidate{}, AssetKind: step.OutputAssetKind,
			CandidateCount: step.CandidateCount,
		}
		if i < len(cmd.Completed) {
			item := cmd.Completed[i]
			if item.StepKey != step.Key || item.SourceRecordID == "" || item.AssetID == "" {
				return nil, fmt.Errorf("completed step %d does not match frozen workflow", i)
			}
			asset, err := s.historyAssets.GetImportedGeneratedAsset(ctx, item.AssetID)
			if err != nil {
				return nil, fmt.Errorf("verify historical asset %s: %w", item.AssetID, err)
			}
			if asset.OrganizationID != cmd.OrganizationID || asset.Scope != wf.Scope || asset.OwnerUserID != cmd.OwnerUserID ||
				asset.AssetKind != step.OutputAssetKind || asset.SourceSystem != "media_ai" || asset.SourceRecordID != item.SourceRecordID ||
				asset.SourceProductID != cmd.Input.Product.ID || asset.SourceVirtualIPID != cmd.Input.VirtualIP.ID ||
				asset.ObjectPath == "" || len(asset.SourceSHA256) != 64 || asset.SourceCreatedAt <= 0 {
				return nil, fmt.Errorf("historical asset %s does not belong to the requested execution", item.AssetID)
			}
			created := time.Unix(0, asset.SourceCreatedAt).UTC()
			if i == 0 {
				job.SubmittedAt = created
			}
			if completedVideo && i == len(cmd.Completed)-1 {
				job.FinishedAt = &created
			}
			selected := 0
			sr.Origin, sr.SourceSystem, sr.SourceRecordID = "legacy_import", "media_ai", item.SourceRecordID
			sr.SourceSHA256, sr.SourceCreatedAt, sr.FinishedAt = asset.SourceSHA256, &created, &created
			sr.Status, sr.OutputAssetID, sr.SelectedIndex = domain.SRSucceeded, asset.ID, &selected
		}
		stepRuns = append(stepRuns, sr)
	}
	if err := s.jobRepo.SaveImported(ctx, job, stepRuns); err != nil {
		if existing, lookupErr := s.jobRepo.GetByLegacySource(ctx, cmd.WorkspaceID, "media_ai", cmd.SourceGroupID); lookupErr == nil {
			if existing.SourceDigest != fingerprint {
				return nil, ErrHistoryImportConflict
			}
			return existing, nil
		}
		return nil, fmt.Errorf("save imported execution: %w", err)
	}
	return s.jobRepo.Get(ctx, jobID)
}

func historyImportDigest(cmd ImportIncompleteCommand) (string, error) {
	encoded, err := json.Marshal(cmd)
	if err != nil {
		return "", fmt.Errorf("encode historical import request: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
