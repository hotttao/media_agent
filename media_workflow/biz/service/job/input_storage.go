package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	domain "media_agent/media_workflow/biz/domain/job"
	wfdomain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/workflow/executable"
)

// freezeInputStorage validates transport URLs before replacing them with
// durable object references. It never modifies the caller's bundle.
func (s *Service) freezeInputStorage(ctx context.Context, organization string, input domain.JobInput) (domain.JobInput, error) {
	return s.normalizeInputStorage(ctx, organization, input, true)
}

// Read-free normalization allows replay after a transport signature expires.
// A new submission still verifies readability before accepting its aggregate.
func (s *Service) normalizeInputStorage(ctx context.Context, organization string, input domain.JobInput, verify bool) (domain.JobInput, error) {
	frozen := input
	frozen.Materials = append([]domain.MaterialData(nil), input.Materials...)
	frozen.Assets = append([]domain.AssetData(nil), input.Assets...)
	if input.Product != nil {
		copied := *input.Product
		frozen.Product = &copied
	}
	if input.VirtualIP != nil {
		copied := *input.VirtualIP
		frozen.VirtualIP = &copied
	}
	cache := map[string]string{}
	freeze := func(raw string) (string, error) {
		if raw == "" {
			return "", nil
		}
		if cached, ok := cache[raw]; ok {
			return cached, nil
		}
		if s.resumeStorageOrigin == "" || (verify && s.resumeURLVerifier == nil) {
			return "", errors.New("execution storage verification is not configured")
		}
		objectPath, err := resumeObjectPath(raw, s.resumeStorageOrigin, organization)
		if err != nil {
			return "", errors.New("input media is outside the authorized storage namespace")
		}
		if verify {
			if err := s.resumeURLVerifier(ctx, raw); err != nil {
				return "", errors.New("input media is not currently readable")
			}
		}
		cache[raw] = "object_path:" + objectPath
		return cache[raw], nil
	}
	var fields []*string
	if frozen.Product != nil {
		fields = append(fields, &frozen.Product.ImageURL)
	}
	if frozen.VirtualIP != nil {
		fields = append(fields, &frozen.VirtualIP.AvatarURL, &frozen.VirtualIP.FullBodyURL, &frozen.VirtualIP.ThreeViewURL, &frozen.VirtualIP.NineViewURL)
	}
	for i := range frozen.Materials {
		fields = append(fields, &frozen.Materials[i].URL)
	}
	for i := range frozen.Assets {
		fields = append(fields, &frozen.Assets[i].URL)
	}
	for _, field := range fields {
		value, err := freeze(*field)
		if err != nil {
			return domain.JobInput{}, fmt.Errorf("freeze execution media: %w", err)
		}
		*field = value
	}
	return frozen, nil
}

// Retry must validate the publication used by this Job, not a newer editor
// version. Compilation and Template versions are immutable execution inputs.
func (s *Service) freezeRetryInput(ctx context.Context, job *domain.Job, input domain.JobInput) (domain.JobInput, error) {
	var schema []wfdomain.InputSchemaEntry
	if s.compilations != nil {
		compilation, err := s.compilations.GetRegistered(ctx, job.WorkflowID, job.WorkflowVersion)
		if err != nil {
			return domain.JobInput{}, err
		}
		var ir executable.ExecutableWorkflowIR
		if err := json.Unmarshal(compilation.IRJSON, &ir); err != nil {
			return domain.JobInput{}, err
		}
		if ir.WorkflowID != job.WorkflowID || ir.WorkflowVersion != job.WorkflowVersion || compilation.DefinitionHash != job.CompilationHash {
			return domain.JobInput{}, errors.New("retry compilation does not match the frozen Job")
		}
		entries := map[string]wfdomain.InputSchemaEntry{}
		for _, step := range ir.Steps {
			tpl, err := s.tplLookup.GetVersion(ctx, step.Template.ID, step.Template.Version)
			if err != nil {
				return domain.JobInput{}, err
			}
			for slotName, ref := range step.Context {
				if ref.Source != executable.ValueWorkflowInput {
					continue
				}
				found := false
				for _, slot := range tpl.Inputs {
					if slot.Name != slotName {
						continue
					}
					found = true
					entry := wfdomain.InputSchemaEntry{Name: ref.Ref, ResourceType: slot.ResourceType, AssetKind: slot.AssetKind, Required: slot.Required}
					if previous, exists := entries[ref.Ref]; exists {
						if previous.ResourceType != entry.ResourceType || previous.AssetKind != entry.AssetKind {
							return domain.JobInput{}, errors.New("conflicting frozen input types")
						}
						entry.Required = entry.Required || previous.Required
					}
					entries[ref.Ref] = entry
					break
				}
				if !found || ref.Ref == "" {
					return domain.JobInput{}, errors.New("frozen input slot is missing")
				}
			}
		}
		for _, entry := range entries {
			schema = append(schema, entry)
		}
	} else {
		workflow, err := s.wfRepo.Get(ctx, job.WorkflowID)
		if err != nil {
			return domain.JobInput{}, err
		}
		schema = workflow.InputSchema
	}
	if err := s.validateBundle(schema, input); err != nil {
		return domain.JobInput{}, err
	}
	return s.freezeInputStorage(ctx, job.OrganizationID, input)
}
