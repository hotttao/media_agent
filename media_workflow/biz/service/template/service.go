package template

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"media_agent/media_workflow/biz/dal"
	domain "media_agent/media_workflow/biz/domain/template"
	"media_agent/media_workflow/biz/shared"
)

type Service struct {
	repo       domain.Repo
	grants     domain.GrantRepo
	tx         dal.TxRunner
	kindLookup shared.AssetKindLookup
}

func NewService(repo domain.Repo, grants domain.GrantRepo, tx dal.TxRunner, lookup shared.AssetKindLookup) *Service {
	return &Service{repo: repo, grants: grants, tx: tx, kindLookup: lookup}
}

type CreateCommand struct {
	Scope           string
	Name            string
	Description     string
	PromptBody      string
	Inputs          []domain.Slot
	OutputAssetKind string
	Platform        string
	GenWay          string
	GenType         string
	NeedsConfirm    bool
	CandidateCount  int
	CreatedBy       string
}

type UpdateCommand struct {
	ID              string
	Name            string
	Description     string
	PromptBody      string
	Inputs          []domain.Slot
	OutputAssetKind string
	Platform        string
	GenWay          string
	GenType         string
	NeedsConfirm    bool
	CandidateCount  int
}

func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*domain.Template, error) {
	if err := s.validateSlots(ctx, cmd.Inputs, cmd.OutputAssetKind); err != nil {
		return nil, err
	}
	if err := validateBase(cmd.Name, cmd.PromptBody, cmd.Platform, cmd.GenWay); err != nil {
		return nil, err
	}
	t := &domain.Template{
		ID: "tpl_" + uuid.NewString(), Scope: cmd.Scope, Name: strings.TrimSpace(cmd.Name),
		Description: cmd.Description, PromptBody: cmd.PromptBody, Inputs: cmd.Inputs,
		OutputAssetKind: cmd.OutputAssetKind, Platform: cmd.Platform, GenWay: cmd.GenWay, GenType: cmd.GenType,
		NeedsConfirm: cmd.NeedsConfirm, CandidateCount: defaultCount(cmd.NeedsConfirm, cmd.CandidateCount),
		Status: domain.StatusDraft, Visibility: domain.VisibilityPrivate, Version: 1, CreatedBy: cmd.CreatedBy,
	}
	if err := s.repo.Save(ctx, t); err != nil {
		return nil, fmt.Errorf("save template: %w", err)
	}
	return s.repo.Get(ctx, t.ID)
}

func (s *Service) Get(ctx context.Context, id, callerID string) (*domain.Template, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("template %q: %w", id, err)
	}
	if !s.canRead(t, callerID) {
		return nil, fmt.Errorf("template %q not accessible", id)
	}
	return t, nil
}

func (s *Service) GetVersion(ctx context.Context, id string, version int) (*domain.TemplateVersion, error) {
	return s.repo.GetVersion(ctx, id, version)
}

func (s *Service) List(ctx context.Context, q domain.ListQuery, callerID string) ([]*domain.Template, int, error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	items, total, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	out := items[:0]
	for _, t := range items {
		if s.canRead(t, callerID) {
			out = append(out, t)
		}
	}
	return out, total, nil
}

func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*domain.Template, error) {
	if err := validateBase(cmd.Name, cmd.PromptBody, cmd.Platform, cmd.GenWay); err != nil {
		return nil, err
	}
	if err := s.validateSlots(ctx, cmd.Inputs, cmd.OutputAssetKind); err != nil {
		return nil, err
	}
	var result *domain.Template
	err := s.tx.RunInTx(ctx, func(txCtx context.Context) error {
		old, err := s.repo.Get(txCtx, cmd.ID)
		if err != nil {
			return err
		}
		// 快照旧版本
		snapshot := &domain.TemplateVersion{
			TemplateID: old.ID, Version: old.Version,
			PromptBody: old.PromptBody, OutputAssetKind: old.OutputAssetKind,
			Platform: old.Platform, GenWay: old.GenWay, GenType: old.GenType,
			NeedsConfirm: old.NeedsConfirm, CandidateCount: old.CandidateCount,
			Inputs: old.Inputs, CreatedBy: old.CreatedBy,
		}
		// 更新字段
		old.Name = cmd.Name
		old.Description = cmd.Description
		old.PromptBody = cmd.PromptBody
		old.Inputs = cmd.Inputs
		old.OutputAssetKind = cmd.OutputAssetKind
		old.Platform = cmd.Platform
		old.GenWay = cmd.GenWay
		old.GenType = cmd.GenType
		old.NeedsConfirm = cmd.NeedsConfirm
		old.CandidateCount = defaultCount(cmd.NeedsConfirm, cmd.CandidateCount)
		old.Version++
		if err := s.repo.Update(txCtx, old, snapshot); err != nil {
			return fmt.Errorf("update template: %w", err)
		}
		result = old
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, result.ID)
}

func (s *Service) Archive(ctx context.Context, id string) (*domain.Template, error) {
	if err := s.repo.UpdateStatus(ctx, id, domain.StatusArchived); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id string) (*domain.Template, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return nil, fmt.Errorf("delete template: %w", err)
	}
	return t, nil
}

func (s *Service) Grant(ctx context.Context, id, teamID, permission, grantedBy string) (*domain.Template, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.CreatedBy != grantedBy {
		return nil, errors.New("only owner can grant template")
	}
	perm := domain.Permission(permission)
	if perm != domain.PermissionViewer && perm != domain.PermissionEditor {
		return nil, fmt.Errorf("invalid permission %q", permission)
	}
	if err := s.grants.AddGrant(ctx, &domain.Grant{
		TemplateID: id, TeamID: teamID, Permission: perm, GrantedBy: grantedBy,
	}); err != nil {
		return nil, fmt.Errorf("add grant: %w", err)
	}
	return t, nil
}

func (s *Service) Revoke(ctx context.Context, id, teamID, callerID string) (*domain.Template, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.CreatedBy != callerID {
		return nil, errors.New("only owner can revoke template grant")
	}
	if err := s.grants.RemoveGrant(ctx, id, teamID); err != nil {
		return nil, fmt.Errorf("remove grant: %w", err)
	}
	return t, nil
}

func (s *Service) ChangeVisibility(ctx context.Context, id, visibility, callerID string) (*domain.Template, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.CreatedBy != callerID {
		return nil, errors.New("only owner can change visibility")
	}
	vis := domain.Visibility(visibility)
	switch vis {
	case domain.VisibilityPrivate, domain.VisibilityTeam, domain.VisibilityPublic:
	default:
		return nil, fmt.Errorf("invalid visibility %q", visibility)
	}
	// 直接更新 visibility（repo.Update 会触发 version+1，不合适；用专门方法）
	// TODO: repo 加 UpdateVisibility 方法；暂复用 UpdateStatus 的模式
	if err := s.repo.UpdateVisibility(ctx, id, vis); err != nil {
		return nil, fmt.Errorf("update visibility: %w", err)
	}
	return s.repo.Get(ctx, id)
}

// canRead 可见性：owner ∨ public ∨ team（team 待接入）。
func (s *Service) canRead(t *domain.Template, callerID string) bool {
	if t.CreatedBy == callerID {
		return true
	}
	if t.Visibility == domain.VisibilityPublic {
		return true
	}
	// team：待 team 成员查询接入后补
	return false
}

// —— 校验 ——

// validateSlots 校验 output_asset_kind（已注册、ASSET）+ 每个 MATERIAL/ASSET slot.name（已注册、resource_type 匹配）。
func (s *Service) validateSlots(ctx context.Context, inputs []domain.Slot, outputAssetKind string) error {
	// 收集所有需校验的 kind 名
	names := make(map[string]struct{})
	if outputAssetKind != "" {
		names[outputAssetKind] = struct{}{}
	}
	for _, in := range inputs {
		if in.ResourceType == "MATERIAL" || in.ResourceType == "ASSET" {
			if in.Name == "" {
				return fmt.Errorf("slot resource_type=%s name is required (asset_kind)", in.ResourceType)
			}
			names[in.Name] = struct{}{}
		}
	}
	if len(names) == 0 {
		return nil
	}
	nameList := make([]string, 0, len(names))
	for n := range names {
		nameList = append(nameList, n)
	}
	kindMap, notFound, err := s.kindLookup.BatchGet(ctx, nameList)
	if err != nil {
		return fmt.Errorf("lookup asset_kinds: %w", err)
	}
	if len(notFound) > 0 {
		return fmt.Errorf("asset_kind not registered: %v", notFound)
	}
	// 校验 output_asset_kind resource_type=ASSET
	if ak, ok := kindMap[outputAssetKind]; ok {
		if ak.ResourceType != "ASSET" {
			return fmt.Errorf("output_asset_kind %q resource_type=%s, want ASSET", outputAssetKind, ak.ResourceType)
		}
		if ak.Status != "SYSTEM" && ak.Status != "ENABLED" {
			return fmt.Errorf("output_asset_kind %q is %s, cannot reference", outputAssetKind, ak.Status)
		}
	}
	// 校验 slot.name resource_type 匹配
	for _, in := range inputs {
		if in.ResourceType != "MATERIAL" && in.ResourceType != "ASSET" {
			continue
		}
		ak, ok := kindMap[in.Name]
		if !ok {
			continue
		}
		if ak.ResourceType != in.ResourceType {
			return fmt.Errorf("slot %q kind %q resource_type=%s, want %s", in.Name, in.Name, ak.ResourceType, in.ResourceType)
		}
	}
	return nil
}

func validateBase(name, promptBody, platform, genWay string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("template name is required")
	}
	if strings.TrimSpace(promptBody) == "" {
		return errors.New("prompt_body is required")
	}
	if strings.TrimSpace(platform) == "" {
		return errors.New("platform is required")
	}
	if strings.TrimSpace(genWay) == "" {
		return errors.New("gen_way is required")
	}
	return nil
}

func defaultCount(needsConfirm bool, count int) int {
	if count > 0 {
		return count
	}
	if needsConfirm {
		return 4 // 默认 4 候选
	}
	return 1
}
