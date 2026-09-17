package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"media_agent/media_workflow/biz/dal"
	domain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/workflow/compiler"
)

type Service struct {
	repo           domain.Repo
	grants         domain.GrantRepo
	tx             dal.TxRunner
	templateLookup domain.TemplateLookup
}

func NewService(repo domain.Repo, grants domain.GrantRepo, tx dal.TxRunner, tl domain.TemplateLookup) *Service {
	return &Service{repo: repo, grants: grants, tx: tx, templateLookup: tl}
}

type CreateCommand struct {
	Scope         string
	Name          string
	Description   string
	OutputStepKey string
	Steps         []domain.Step
	CreatedBy     string
}

type UpdateCommand struct {
	ID            string
	Name          string
	Description   string
	OutputStepKey string
	Steps         []domain.Step
	CallerID      string
}

// ValidateWorkflow 闸1：不写库，返回 ValidationResult。
func (s *Service) ValidateWorkflow(ctx context.Context, scope string, steps []domain.Step, outputStepKey string) (*domain.ValidationResult, error) {
	v := newValidator(scope, steps, outputStepKey, s.templateLookup)
	return v.validate(ctx)
}

func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*domain.Workflow, error) {
	// 闸1
	v := newValidator(cmd.Scope, cmd.Steps, cmd.OutputStepKey, s.templateLookup)
	res, err := v.validate(ctx)
	if err != nil || !res.Valid {
		return nil, fmt.Errorf("workflow validation failed: %v (reasons: %v)", err, res.RejectReasons)
	}
	w := &domain.Workflow{
		ID: "wf_" + uuid.NewString(), Scope: cmd.Scope, Name: strings.TrimSpace(cmd.Name),
		Description: cmd.Description, OutputStepKey: cmd.OutputStepKey, Steps: cmd.Steps,
		Status: domain.StatusDraft, Visibility: domain.VisibilityPrivate, Version: 1,
		InputSchema: res.InputSchema, CreatedBy: cmd.CreatedBy,
	}
	if err := s.repo.Save(ctx, w); err != nil {
		return nil, fmt.Errorf("save workflow: %w", err)
	}
	return s.repo.Get(ctx, w.ID)
}

// canRead 可见性：owner ∨ public ∨ team（team 待接入）。
func (s *Service) canRead(w *domain.Workflow, callerID string) bool {
	if w.CreatedBy == callerID {
		return true
	}
	if w.Visibility == domain.VisibilityPublic {
		return true
	}
	return false
}

func (s *Service) canReadSummary(w *domain.WorkflowSummary, callerID string) bool {
	if w.CreatedBy == callerID {
		return true
	}
	if w.Visibility == domain.VisibilityPublic {
		return true
	}
	return false
}

// ChangeVisibility 改可见性。仅 owner。
func (s *Service) ChangeVisibility(ctx context.Context, id, visibility, callerID string) (*domain.Workflow, error) {
	w, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if w.CreatedBy != callerID {
		return nil, errors.New("only owner can change visibility")
	}
	vis := domain.Visibility(visibility)
	switch vis {
	case domain.VisibilityPrivate, domain.VisibilityTeam, domain.VisibilityPublic:
	default:
		return nil, fmt.Errorf("invalid visibility %q", visibility)
	}
	if err := s.repo.UpdateVisibility(ctx, id, vis); err != nil {
		return nil, fmt.Errorf("update visibility: %w", err)
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Get(ctx context.Context, id, callerID string) (*domain.Workflow, error) {
	w, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("workflow %q: %w", id, err)
	}
	if !s.canRead(w, callerID) {
		return nil, fmt.Errorf("workflow %q not accessible", id)
	}
	return w, nil
}

func (s *Service) List(ctx context.Context, q domain.ListQuery, callerID string) ([]*domain.WorkflowSummary, int, error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	items, total, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	out := items[:0]
	for _, w := range items {
		if s.canReadSummary(w, callerID) {
			out = append(out, w)
		}
	}
	return out, total, nil
}

func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*domain.Workflow, error) {
	v := newValidator("", cmd.Steps, cmd.OutputStepKey, s.templateLookup)
	// scope 从已有 workflow 取
	old, err := s.repo.Get(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if old.CreatedBy != cmd.CallerID {
		return nil, errors.New("only owner can update workflow")
	}
	v.scope = old.Scope
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
	var result *domain.Workflow
	err = s.tx.RunInTx(ctx, func(txCtx context.Context) error {
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

func (s *Service) Archive(ctx context.Context, id, callerID string) (*domain.Workflow, error) {
	w, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if w.CreatedBy != callerID {
		return nil, errors.New("only owner can archive workflow")
	}
	if err := s.repo.UpdateStatus(ctx, id, domain.StatusArchived); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, id)
}

// Publish draft -> published（状态机守卫：仅 draft 可 publish；archived 不可 publish）。
func (s *Service) Publish(ctx context.Context, id, callerID string) (*domain.Workflow, error) {
	w, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if w.Status != domain.StatusDraft {
		return nil, fmt.Errorf("workflow %q status %s, expected draft", id, w.Status)
	}
	if w.CreatedBy != callerID {
		return nil, errors.New("only owner can publish workflow")
	}
	// Publishing is the business boundary at which the immutable DAG is
	// compiled to the Conductor definition shape. Registration is performed by
	// the execution adapter; rejecting an invalid definition here prevents a
	// published workflow that cannot be scheduled.
	if _, err := compiler.Compile(*w); err != nil {
		return nil, fmt.Errorf("compile workflow for publish: %w", err)
	}
	if err := s.repo.UpdateStatus(ctx, id, domain.StatusPublished); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id string) (*domain.Workflow, error) {
	w, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return nil, fmt.Errorf("delete workflow: %w", err)
	}
	return w, nil
}

func (s *Service) Grant(ctx context.Context, id, teamID, permission, grantedBy string) (*domain.Workflow, error) {
	w, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if w.CreatedBy != grantedBy {
		return nil, errors.New("only owner can grant workflow")
	}
	perm := domain.Permission(permission)
	if perm != domain.PermissionViewer && perm != domain.PermissionEditor {
		return nil, fmt.Errorf("invalid permission %q", permission)
	}
	if err := s.grants.AddGrant(ctx, &domain.Grant{
		WorkflowID: id, TeamID: teamID, Permission: perm, GrantedBy: grantedBy,
	}); err != nil {
		return nil, fmt.Errorf("add grant: %w", err)
	}
	return w, nil
}

func (s *Service) Revoke(ctx context.Context, id, teamID, callerID string) (*domain.Workflow, error) {
	w, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if w.CreatedBy != callerID {
		return nil, errors.New("only owner can revoke workflow grant")
	}
	if err := s.grants.RemoveGrant(ctx, id, teamID); err != nil {
		return nil, fmt.Errorf("remove grant: %w", err)
	}
	return w, nil
}
