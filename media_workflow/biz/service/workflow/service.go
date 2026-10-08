package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"media_agent/media_workflow/biz/dal"
	domain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/policy/resource"
	"media_agent/media_workflow/biz/shared"
)

type Service struct {
	authority      shared.ResourceAuthority
	repo           domain.Repo
	grants         domain.GrantRepo
	tx             dal.TxRunner
	templateLookup domain.TemplateLookup
	publisher      domain.Publisher
	publications   domain.PublicationRepo
}

func (s *Service) ConfigureResourceAuthority(a shared.ResourceAuthority) { s.authority = a }

func (s *Service) ConfigurePublications(repo domain.PublicationRepo) { s.publications = repo }

func (s *Service) publicationView(ctx context.Context, w *domain.Workflow) (*domain.Workflow, error) {
	if s.publications == nil {
		return w, nil
	}
	p, err := s.publications.GetPublication(ctx, w.ID, w.Version)
	if err != nil {
		return nil, errors.New("publication receipt unavailable")
	}
	if p != nil {
		w.PublicationID, w.PublicationStatus = p.ID, p.Status
	}
	return w, nil
}

func NewService(repo domain.Repo, grants domain.GrantRepo, tx dal.TxRunner, tl domain.TemplateLookup, publisher domain.Publisher) *Service {
	return &Service{repo: repo, grants: grants, tx: tx, templateLookup: tl, publisher: publisher}
}

type CreateCommand struct {
	OrganizationID string
	WorkspaceID    string
	Scope          string
	Name           string
	Description    string
	OutputStepKey  string
	Steps          []domain.Step
	CreatedBy      string
}

type UpdateCommand struct {
	ID            string
	Name          string
	Description   string
	OutputStepKey string
	Steps         []domain.Step
	CallerID      string
	TeamIDs       []string
}

// ValidateWorkflow 闸1：不写库，返回 ValidationResult。
func (s *Service) ValidateWorkflow(ctx context.Context, scope string, steps []domain.Step, outputStepKey string) (*domain.ValidationResult, error) {
	v := newValidator(scope, steps, outputStepKey, s.templateLookup)
	return v.validate(ctx)
}

func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*domain.Workflow, error) {
	if s.authority != nil {
		w, err := s.authority.GetWorkspaceForUser(ctx, cmd.WorkspaceID, cmd.CreatedBy)
		if err != nil {
			return nil, err
		}
		if cmd.OrganizationID != "" && cmd.OrganizationID != w.OrganizationID {
			return nil, domain.ErrWorkflowForbidden
		}
		cmd.OrganizationID = w.OrganizationID
	} else if cmd.OrganizationID != "" || cmd.WorkspaceID != "" {
		return nil, shared.ErrResourceAuthorityUnavailable
	}
	// 闸1
	ctx = shared.WithResourceOrganization(ctx, cmd.OrganizationID)
	v := newValidator(cmd.Scope, cmd.Steps, cmd.OutputStepKey, s.templateLookup)
	res, err := v.validate(ctx)
	if err != nil || !res.Valid {
		return nil, fmt.Errorf("workflow validation failed: %v (reasons: %v)", err, res.RejectReasons)
	}
	w := &domain.Workflow{
		OrganizationID: cmd.OrganizationID, WorkspaceID: cmd.WorkspaceID,
		ID: "wf_" + uuid.NewString(), Scope: cmd.Scope, Name: strings.TrimSpace(cmd.Name),
		Description: cmd.Description, OutputStepKey: cmd.OutputStepKey, Steps: cmd.Steps,
		Status: domain.StatusDraft, Visibility: domain.VisibilityPrivate, Version: 1,
		InputSchema: res.InputSchema, CreatedBy: cmd.CreatedBy,
	}
	save := func(txCtx context.Context) error { return s.repo.Save(txCtx, w) }
	var saveErr error
	if s.tx != nil {
		saveErr = s.tx.RunInTx(ctx, save)
	} else if s.authority != nil {
		saveErr = shared.ErrResourceAuthorityUnavailable
	} else {
		saveErr = save(ctx)
	}
	if err := saveErr; err != nil {
		return nil, fmt.Errorf("save workflow: %w", err)
	}
	return s.repo.Get(ctx, w.ID)
}

func (s *Service) permission(ctx context.Context, w *domain.Workflow, caller string, teams []string) (resource.Level, error) {
	if s.authority == nil && (w.OrganizationID != "" || w.WorkspaceID != "") {
		return resource.None, shared.ErrResourceAuthorityUnavailable
	}
	if s.authority != nil {
		_, facts, err := shared.PrepareResourceFacts(ctx, caller, s.authority)
		if err != nil {
			return resource.None, err
		}
		if w.OrganizationID == "" {
			if caller == w.CreatedBy && caller != "" {
				return resource.Owner, nil
			}
			return resource.None, nil
		}
		if !shared.HasResourceID(facts.OrganizationIDs, w.OrganizationID) || !shared.HasResourceID(facts.WorkspaceIDs, w.WorkspaceID) {
			return resource.None, nil
		}
		teams = facts.TeamIDs
	}
	var grants []resource.Grant
	if caller != "" && caller != w.CreatedBy && len(teams) > 0 && w.Visibility != domain.VisibilityPrivate {
		if s.grants == nil {
			return resource.None, errors.New("workflow grants unavailable")
		}
		rows, err := s.grants.ListGrants(ctx, w.ID)
		if err != nil {
			return resource.None, errors.New("workflow grants unavailable")
		}
		for _, grant := range rows {
			grants = append(grants, resource.Grant{TeamID: grant.TeamID, Permission: string(grant.Permission)})
		}
	}
	return resource.Effective(w.CreatedBy, string(w.Visibility), caller, teams, grants), nil
}

// ChangeVisibility 改可见性。仅 owner。
func (s *Service) ChangeVisibility(ctx context.Context, id, visibility, callerID string) (*domain.Workflow, error) {
	w, err := s.Get(ctx, id, callerID)
	if err != nil {
		return nil, err
	}
	if w.CreatedBy != callerID {
		return nil, domain.ErrWorkflowForbidden
	}
	vis := domain.Visibility(visibility)
	if s.authority != nil && w.OrganizationID == "" && vis != domain.VisibilityPrivate {
		return nil, domain.ErrWorkflowForbidden
	}
	switch vis {
	case domain.VisibilityPrivate, domain.VisibilityTeam, domain.VisibilityPublic:
	default:
		return nil, fmt.Errorf("invalid visibility %q", visibility)
	}
	if err := s.writeResource(ctx, id, callerID, resource.Owner, nil, func(txCtx context.Context, w *domain.Workflow) error { return s.repo.UpdateVisibility(txCtx, id, vis) }); err != nil {
		return nil, fmt.Errorf("update visibility: %w", err)
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Get(ctx context.Context, id, callerID string, teamIDs ...string) (*domain.Workflow, error) {
	w, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("workflow %q: %w", id, err)
	}
	permission, err := s.permission(ctx, w, callerID, teamIDs)
	if err != nil {
		return nil, err
	}
	if permission < resource.Viewer {
		return nil, domain.ErrWorkflowForbidden
	}
	return s.publicationView(ctx, w)
}

func (s *Service) List(ctx context.Context, q domain.ListQuery, callerID string, teamIDs ...string) ([]*domain.WorkflowSummary, int, error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	if callerID == "" {
		return nil, 0, domain.ErrWorkflowForbidden
	}
	q.EnforceAccess, q.CallerID, q.TeamIDs = true, callerID, append([]string(nil), teamIDs...)
	if s.authority != nil {
		_, facts, err := shared.PrepareResourceFacts(ctx, callerID, s.authority)
		if err != nil {
			return nil, 0, err
		}
		q.EnforceBoundary, q.OrganizationIDs, q.WorkspaceIDs, q.TeamIDs = true, facts.OrganizationIDs, facts.WorkspaceIDs, facts.TeamIDs
	}
	return s.repo.List(ctx, q)
}

func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*domain.Workflow, error) {
	if s.authority != nil {
		var err error
		ctx, _, err = shared.PrepareResourceFacts(ctx, cmd.CallerID, s.authority)
		if err != nil {
			return nil, err
		}
	}
	v := newValidator("", cmd.Steps, cmd.OutputStepKey, s.templateLookup)
	// scope 从已有 workflow 取
	old, err := s.repo.Get(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	permission, err := s.permission(ctx, old, cmd.CallerID, cmd.TeamIDs)
	if err != nil {
		return nil, err
	}
	if permission < resource.Editor {
		return nil, domain.ErrWorkflowForbidden
	}
	if old.Status == domain.StatusArchived {
		return nil, domain.ErrPublicationConflict
	}
	if err := s.requireAuthoring(ctx, old, cmd.CallerID); err != nil {
		return nil, err
	}
	v.scope = old.Scope
	ctx = shared.WithResourceOrganization(ctx, old.OrganizationID)
	res, err := v.validate(ctx)
	if err != nil || !res.Valid {
		return nil, fmt.Errorf("workflow validation failed: reasons: %v", res.RejectReasons)
	}
	old.Name = cmd.Name
	old.Description = cmd.Description
	old.OutputStepKey = cmd.OutputStepKey
	old.Steps = cmd.Steps
	old.InputSchema = res.InputSchema
	old.Version++
	old.Status = domain.StatusDraft
	var result *domain.Workflow
	err = s.tx.RunInTx(ctx, func(txCtx context.Context) error {
		current, err := s.lockResource(txCtx, cmd.ID)
		if err != nil {
			return err
		}
		if current.Version != old.Version-1 {
			return domain.ErrPublicationConflict
		}
		if current.OrganizationID != old.OrganizationID || current.WorkspaceID != old.WorkspaceID {
			return domain.ErrPublicationConflict
		}
		permission, err := s.permission(txCtx, current, cmd.CallerID, cmd.TeamIDs)
		if err != nil {
			return err
		}
		if permission < resource.Editor {
			return domain.ErrWorkflowForbidden
		}
		if err := s.repo.Update(txCtx, old); err != nil {
			return fmt.Errorf("update workflow: %w", err)
		}
		result = old
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, result.ID)
}

func (s *Service) Archive(ctx context.Context, id, callerID string, teamIDs ...string) (*domain.Workflow, error) {
	w, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	permission, err := s.permission(ctx, w, callerID, teamIDs)
	if err != nil {
		return nil, err
	}
	if permission < resource.Editor {
		return nil, domain.ErrWorkflowForbidden
	}
	if err := s.writeResource(ctx, id, callerID, resource.Editor, teamIDs, func(txCtx context.Context, w *domain.Workflow) error {
		return s.repo.UpdateStatus(txCtx, id, domain.StatusArchived)
	}); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, id)
}

// Publish accepts a durable command; registration is owned by background delivery.
func (s *Service) Publish(ctx context.Context, id, callerID string) (*domain.Workflow, error) {
	w, err := s.Get(ctx, id, callerID)
	if err != nil {
		return nil, err
	}
	if w.CreatedBy != callerID {
		return nil, domain.ErrWorkflowForbidden
	}
	if err := s.requireAuthoring(ctx, w, callerID); err != nil {
		return nil, err
	}
	if w.Status == domain.StatusArchived {
		return nil, domain.ErrPublicationConflict
	}
	if s.publisher == nil {
		return nil, errors.New("workflow publisher is not configured")
	}
	p, err := s.publisher.AcceptPublication(ctx, w, callerID)
	if err != nil {
		return nil, err
	}
	current, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil || current.Version != p.WorkflowVersion {
		return nil, domain.ErrPublicationConflict
	}
	current.PublicationID, current.PublicationStatus = p.ID, p.Status
	return current, nil
}

func (s *Service) Delete(ctx context.Context, id, callerID string) (*domain.Workflow, error) {
	w, err := s.Get(ctx, id, callerID)
	if err != nil {
		return nil, err
	}
	if callerID == "" || w.CreatedBy != callerID {
		return nil, domain.ErrWorkflowForbidden
	}
	if err := s.writeResource(ctx, id, callerID, resource.Owner, nil, func(txCtx context.Context, w *domain.Workflow) error { return s.repo.Delete(txCtx, id) }); err != nil {
		return nil, fmt.Errorf("delete workflow: %w", err)
	}
	return w, nil
}

func (s *Service) Grant(ctx context.Context, id, teamID, permission, grantedBy string) (*domain.Workflow, error) {
	w, err := s.Get(ctx, id, grantedBy)
	if err != nil {
		return nil, err
	}
	if grantedBy == "" || w.CreatedBy != grantedBy {
		return nil, domain.ErrWorkflowForbidden
	}
	perm := domain.Permission(permission)
	if s.authority != nil {
		org, err := s.authority.TeamOrganization(ctx, grantedBy, teamID)
		if err != nil {
			return nil, err
		}
		if w.OrganizationID == "" || org != w.OrganizationID {
			return nil, domain.ErrWorkflowForbidden
		}
	}
	if perm != domain.PermissionViewer && perm != domain.PermissionEditor {
		return nil, fmt.Errorf("invalid permission %q", permission)
	}
	if err := s.writeResource(ctx, id, grantedBy, resource.Owner, nil, func(txCtx context.Context, current *domain.Workflow) error {
		if current.OrganizationID != w.OrganizationID {
			return domain.ErrWorkflowForbidden
		}
		return s.grants.AddGrant(txCtx, &domain.Grant{WorkflowID: id, TeamID: teamID, Permission: perm, GrantedBy: grantedBy})
	}); err != nil {
		return nil, fmt.Errorf("add grant: %w", err)
	}
	return w, nil
}

func (s *Service) Revoke(ctx context.Context, id, teamID, callerID string) (*domain.Workflow, error) {
	w, err := s.Get(ctx, id, callerID)
	if err != nil {
		return nil, err
	}
	if w.CreatedBy != callerID {
		return nil, domain.ErrWorkflowForbidden
	}
	if err := s.writeResource(ctx, id, callerID, resource.Owner, nil, func(txCtx context.Context, w *domain.Workflow) error { return s.grants.RemoveGrant(txCtx, id, teamID) }); err != nil {
		return nil, fmt.Errorf("remove grant: %w", err)
	}
	return w, nil
}
