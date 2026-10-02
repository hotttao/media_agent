package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	domain "media_agent/media_workflow/biz/domain/job"
	wfdomain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/workflow/compiler"
	"media_agent/media_workflow/biz/workflow/executable"
)

var (
	ErrResumeForbidden = errors.New("historical Job does not belong to this user or Workspace")
	ErrResumeConflict  = domain.ErrResumeConflict
)

type ResumeWorkspaceAccess interface {
	CanUseWorkflow(ctx context.Context, workspaceID, userID string) (bool, error)
}

type ResumeLauncher interface {
	StartResume(ctx context.Context, start domain.ResumeStart) (executionID, definitionHash string, err error)
}

type ResumeCommand struct {
	JobID          string
	UserID         string
	IdempotencyKey string
	InputPatch     domain.JobInput
}

func (s *Service) ConfigureResume(repo domain.ResumeRepo, access ResumeWorkspaceAccess, launcher ResumeLauncher, storageOrigin string) {
	s.resumeRepo = repo
	s.resumeAccess = access
	s.resumeLauncher = launcher
	s.resumeStorageOrigin = strings.TrimRight(storageOrigin, "/")
	s.resumeURLVerifier = verifyResumeURLReachable
}

// Resume validates the frozen historical prefix and creates a durable Start
// command. The API never directly starts Conductor inside the Job transaction.
func (s *Service) Resume(ctx context.Context, cmd ResumeCommand) (*domain.Job, error) {
	if s.resumeRepo == nil || s.resumeAccess == nil || s.resumeLauncher == nil || s.historyAssets == nil || s.compilations == nil || s.tplLookup == nil || s.resumeStorageOrigin == "" {
		return nil, errors.New("historical resume is not configured")
	}
	if cmd.JobID == "" || cmd.UserID == "" || strings.TrimSpace(cmd.IdempotencyKey) == "" || len(cmd.IdempotencyKey) > 160 {
		return nil, errors.New("job ID, user ID and bounded idempotency key are required")
	}
	job, steps, err := s.jobRepo.GetWithStepRuns(ctx, cmd.JobID)
	if err != nil {
		return nil, err
	}
	if job.Origin != "legacy_import" || job.SubmittedBy != cmd.UserID || job.WorkspaceID == "" {
		return nil, ErrResumeForbidden
	}
	allowed, err := s.resumeAccess.CanUseWorkflow(ctx, job.WorkspaceID, cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("verify Workspace workflow access: %w", err)
	}
	if !allowed {
		return nil, ErrResumeForbidden
	}
	// A retried command identifies the same resume generation even after the
	// asynchronous execution has completed. Returning the current Job avoids
	// turning a successful retry into a conflict or launching a second run.
	if job.ResumeGeneration > 0 && job.ResumeIdempotencyKey == cmd.IdempotencyKey {
		return job, nil
	}
	if job.Status != domain.JobAwaitingResume || job.ResumeGeneration != 0 {
		return nil, ErrResumeConflict
	}
	compilation, err := s.compilations.GetRegistered(ctx, job.WorkflowID, job.WorkflowVersion)
	if err != nil {
		return nil, fmt.Errorf("frozen workflow compilation required: %w", err)
	}
	var ir executable.ExecutableWorkflowIR
	if err := json.Unmarshal(compilation.IRJSON, &ir); err != nil || ir.SchemaVersion != executable.SchemaVersion ||
		ir.WorkflowID != job.WorkflowID || ir.WorkflowVersion != job.WorkflowVersion {
		return nil, errors.New("frozen workflow compilation does not match historical Job")
	}
	restored := make(map[string]bool)
	for _, step := range steps {
		if step.Origin != "legacy_import" {
			continue
		}
		if step.Status != domain.SRSucceeded || step.OutputAssetID == "" {
			return nil, errors.New("historical StepRun is not a frozen success")
		}
		asset, err := s.historyAssets.GetImportedGeneratedAsset(ctx, step.OutputAssetID)
		if err != nil {
			return nil, fmt.Errorf("reverify historical asset %s: %w", step.OutputAssetID, err)
		}
		if step.SourceCreatedAt == nil || asset.ID != step.OutputAssetID || asset.OrganizationID != job.OrganizationID ||
			asset.OwnerUserID != job.SubmittedBy || asset.Scope != job.Scope || asset.AssetKind != step.AssetKind ||
			asset.SourceSystem != step.SourceSystem || asset.SourceRecordID != step.SourceRecordID ||
			asset.SourceSHA256 != step.SourceSHA256 || asset.SourceCreatedAt != step.SourceCreatedAt.UnixNano() ||
			asset.ObjectPath == "" || asset.AccessURL == "" || asset.AccessExpiresAt <= time.Now().Add(15*time.Second).Unix() {
			return nil, fmt.Errorf("historical asset %s no longer matches its frozen StepRun", step.OutputAssetID)
		}
		restored[step.StepKey] = true
	}
	requirements, err := s.pendingInputRequirements(ctx, ir, restored)
	if err != nil {
		return nil, err
	}
	merged, err := mergeResumeInputs(job.Input, cmd.InputPatch, requirements)
	if err != nil {
		return nil, err
	}
	for _, material := range cmd.InputPatch.Materials {
		if !validTrustedResumeAssetURL(material.URL, s.resumeStorageOrigin, job.OrganizationID) {
			return nil, fmt.Errorf("material %q is not a storage object in the Job organization", material.Name)
		}
		if err := s.resumeURLVerifier(ctx, material.URL); err != nil {
			return nil, fmt.Errorf("material %q is not currently readable: %w", material.Name, err)
		}
	}
	for _, asset := range cmd.InputPatch.Assets {
		if !validTrustedResumeAssetURL(asset.URL, s.resumeStorageOrigin, job.OrganizationID) {
			return nil, fmt.Errorf("asset %q is not a storage object in the Job organization", asset.Name)
		}
		if err := s.resumeURLVerifier(ctx, asset.URL); err != nil {
			return nil, fmt.Errorf("asset %q is not currently readable: %w", asset.Name, err)
		}
	}
	// Signed URLs are transport credentials, not durable execution input. Keep
	// only the verified organization object path in Job and StartResume Outbox.
	for i := range merged.Materials {
		if merged.Materials[i].URL == "" {
			continue
		}
		for _, patch := range cmd.InputPatch.Materials {
			if merged.Materials[i].Name == patch.Name {
				objectPath, err := resumeObjectPath(patch.URL, s.resumeStorageOrigin, job.OrganizationID)
				if err != nil {
					return nil, err
				}
				merged.Materials[i].URL = "object_path:" + objectPath
			}
		}
	}
	for i := range merged.Assets {
		for _, patch := range cmd.InputPatch.Assets {
			if merged.Assets[i].Name == patch.Name {
				objectPath, err := resumeObjectPath(patch.URL, s.resumeStorageOrigin, job.OrganizationID)
				if err != nil {
					return nil, err
				}
				merged.Assets[i].URL = "object_path:" + objectPath
			}
		}
	}
	definition, err := compiler.CompileResumeExecutableIR(ir, restored)
	if err != nil {
		return nil, err
	}
	domains, err := taskDomainsForIR(ir, job.SubmittedBy)
	if err != nil {
		return nil, err
	}
	definitionJSON, _ := json.Marshal(definition)
	domainsJSON, _ := json.Marshal(domains)
	inputJSON, _ := json.Marshal(map[string]any{"job_id": job.ID, "inputs": conductorInputs(merged)})
	start := domain.ResumeStart{
		ID: "resume_" + uuid.NewString(), JobID: job.ID, Generation: job.ResumeGeneration + 1,
		IdempotencyKey: cmd.IdempotencyKey, CorrelationID: fmt.Sprintf("resume:%s:%d", job.ID, job.ResumeGeneration+1),
		DefinitionJSON: definitionJSON, TaskDomainsJSON: domainsJSON, WorkflowInputJSON: inputJSON,
	}
	if _, err := s.resumeRepo.BeginResume(ctx, start, merged); err != nil {
		return nil, err
	}
	return s.jobRepo.Get(ctx, job.ID)
}

func (s *Service) pendingInputRequirements(ctx context.Context, ir executable.ExecutableWorkflowIR, restored map[string]bool) (map[string]wfdomain.SlotSnapshot, error) {
	requirements := make(map[string]wfdomain.SlotSnapshot)
	for _, step := range ir.Steps {
		if restored[step.Key] {
			continue
		}
		tpl, err := s.tplLookup.GetVersion(ctx, step.Template.ID, step.Template.Version)
		if err != nil {
			return nil, fmt.Errorf("load frozen template for %s: %w", step.Key, err)
		}
		slots := make(map[string]wfdomain.SlotSnapshot, len(tpl.Inputs))
		for _, slot := range tpl.Inputs {
			slots[slot.Name] = slot
		}
		for slotName, ref := range step.Context {
			if ref.Source != executable.ValueWorkflowInput {
				continue
			}
			slot, ok := slots[slotName]
			if !ok || ref.Ref == "" {
				return nil, fmt.Errorf("frozen step %s input %s has no template slot", step.Key, slotName)
			}
			if existing, exists := requirements[ref.Ref]; exists && (existing.ResourceType != slot.ResourceType || existing.AssetKind != slot.AssetKind) {
				return nil, fmt.Errorf("workflow input %s has conflicting frozen slot types", ref.Ref)
			}
			requirements[ref.Ref] = slot
		}
	}
	return requirements, nil
}

func mergeResumeInputs(original, patch domain.JobInput, needed map[string]wfdomain.SlotSnapshot) (domain.JobInput, error) {
	if patch.Product != nil || patch.VirtualIP != nil {
		return domain.JobInput{}, errors.New("historical Product and Virtual IP cannot be changed by Resume")
	}
	merged := original
	merged.Materials = append([]domain.MaterialData(nil), original.Materials...)
	merged.Assets = append([]domain.AssetData(nil), original.Assets...)
	seen := make(map[string]bool)
	for _, item := range original.Materials {
		seen[item.Name] = true
	}
	for _, item := range original.Assets {
		seen[item.Name] = true
	}
	for _, item := range patch.Materials {
		slot, allowed := needed[item.Name]
		if !allowed || slot.ResourceType != "MATERIAL" || slot.AssetKind != item.AssetKind || seen[item.Name] || item.Name == "" || !validResumeAssetURL(item.URL) {
			return domain.JobInput{}, fmt.Errorf("invalid or duplicate resumed material %q", item.Name)
		}
		seen[item.Name] = true
		merged.Materials = append(merged.Materials, item)
	}
	for _, item := range patch.Assets {
		slot, allowed := needed[item.Name]
		if !allowed || slot.ResourceType != "ASSET" || slot.AssetKind != item.AssetKind || seen[item.Name] || item.Name == "" || !validResumeAssetURL(item.URL) {
			return domain.JobInput{}, fmt.Errorf("invalid or duplicate resumed asset %q", item.Name)
		}
		seen[item.Name] = true
		merged.Assets = append(merged.Assets, item)
	}
	missing := make([]string, 0)
	for name := range needed {
		if name == "product" && merged.Product != nil || name == "virtual_ip" && merged.VirtualIP != nil || seen[name] {
			continue
		}
		missing = append(missing, name)
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return domain.JobInput{}, fmt.Errorf("missing inputs for pending steps: %s", strings.Join(missing, ", "))
	}
	return merged, nil
}

func validResumeAssetURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil
}

func validTrustedResumeAssetURL(raw, storageOrigin, organizationID string) bool {
	_, err := resumeObjectPath(raw, storageOrigin, organizationID)
	return err == nil
}

func resumeObjectPath(raw, storageOrigin, organizationID string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || !validResumeAssetURL(raw) || organizationID == "" || strings.ContainsAny(organizationID, "/\\") {
		return "", errors.New("invalid storage object URL")
	}
	base, err := url.Parse(storageOrigin)
	if err != nil || u.Scheme != base.Scheme || u.Host != base.Host || u.Fragment != "" {
		return "", errors.New("storage origin does not match")
	}
	const bucketPrefix = "/media/"
	if !strings.HasPrefix(u.Path, bucketPrefix+"materials/"+organizationID+"/") || path.Clean(u.Path) != u.Path || strings.Contains(u.Path, "\\") {
		return "", errors.New("storage object is outside the Job organization")
	}
	return strings.TrimPrefix(u.Path, bucketPrefix), nil
}

func verifyResumeURLReachable(ctx context.Context, raw string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return errors.New("invalid storage URL")
	}
	request.Header.Set("Range", "bytes=0-0")
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("storage object could not be reached")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusPartialContent && response.StatusCode != http.StatusOK {
		return fmt.Errorf("storage returned status %d", response.StatusCode)
	}
	return nil
}

func (s *Service) DispatchPendingResume(ctx context.Context) error {
	if s.resumeRepo == nil || s.resumeLauncher == nil {
		return errors.New("historical resume is not configured")
	}
	start, err := s.resumeRepo.ClaimPendingResume(ctx)
	if err != nil || start == nil {
		return err
	}
	executionID, hash, err := s.resumeLauncher.StartResume(ctx, *start)
	if err == nil {
		err = s.resumeRepo.MarkResumeStarted(ctx, *start, executionID, hash)
	}
	if err != nil {
		_ = s.resumeRepo.ReleaseResumeClaim(ctx, start.ID, err.Error())
		return err
	}
	return nil
}
