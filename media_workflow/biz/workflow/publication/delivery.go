package publication

import (
	"context"
	"encoding/json"
	"errors"

	"gorm.io/gorm"
	domain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/workflow/compiler"
	"media_agent/media_workflow/biz/workflow/conductor"
)

var ErrTemporarilyUnavailable = errors.New("workflow publication registration temporarily unavailable")

type RegistrationEngine interface {
	RegisterTaskDefinitions(context.Context, []conductor.TaskDefinition) error
	EnsureWorkflow(context.Context, compiler.Definition) error
}

type Delivery struct {
	Intents   domain.PublicationRepo
	Workflows domain.Repo
	Engine    RegistrationEngine
	Tx        interface {
		RunInTx(context.Context, func(context.Context) error) error
	}
}

func (f *Delivery) DispatchOne(ctx context.Context) error {
	if f.Intents == nil || f.Workflows == nil || f.Engine == nil || f.Tx == nil {
		return errors.New("publication delivery unavailable")
	}
	p, err := f.Intents.ClaimPublication(ctx)
	if err != nil || p == nil {
		return err
	}
	release := func(code string) error {
		if err := f.Intents.ReleasePublication(ctx, *p, code); err != nil {
			return err
		}
		return ErrTemporarilyUnavailable
	}
	finish := func(status, code string) error {
		return f.Tx.RunInTx(ctx, func(txCtx context.Context) error { return f.Intents.FinishPublication(txCtx, *p, status, code) })
	}
	if p.Validate() != nil {
		return release("invalid_frozen_publication")
	}
	source, err := f.Workflows.Get(ctx, p.WorkflowID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return finish("superseded", "source_unavailable")
	}
	if err != nil || source == nil {
		return release("source_unavailable")
	}
	if source.CreatedBy != p.RequestedBy || source.Version != p.WorkflowVersion || string(source.Status) != "publishing" {
		return finish("superseded", "source_version_changed")
	}
	var definition compiler.Definition
	var tasks []conductor.TaskDefinition
	if json.Unmarshal(p.DefinitionJSON, &definition) != nil || json.Unmarshal(p.TaskDefinitionsJSON, &tasks) != nil {
		return release("invalid_frozen_contract")
	}
	hash, err := compiler.DefinitionHash(definition)
	if err != nil || hash != p.DefinitionHash {
		return finish("failed", "definition_hash_conflict")
	}
	if err := f.Engine.RegisterTaskDefinitions(ctx, tasks); err != nil {
		if errors.Is(err, conductor.ErrTaskDefinitionConflict) {
			return finish("failed", "task_definition_conflict")
		}
		return release("task_registration_unavailable")
	}
	if err := f.Engine.EnsureWorkflow(ctx, definition); err != nil {
		if errors.Is(err, conductor.ErrWorkflowDefinitionConflict) {
			return finish("failed", "workflow_definition_conflict")
		}
		return release("workflow_registration_unverified")
	}
	return finish("registered", "")
}
