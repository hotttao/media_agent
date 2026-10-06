package publication

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"
	domain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/workflow/compiler"
	"media_agent/media_workflow/biz/workflow/conductor"
)

type publicationTx struct {
	active  bool
	commits int
}

func (t *publicationTx) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	t.active = true
	defer func() { t.active = false }()
	err := fn(ctx)
	if err == nil {
		t.commits++
	}
	return err
}

type publicationSource struct {
	domain.Repo
	source *domain.Workflow
	err    error
}

func (r *publicationSource) Get(context.Context, string) (*domain.Workflow, error) {
	return r.source, r.err
}

type publicationStore struct {
	domain.PublicationRepo
	command            *domain.Publication
	source             *publicationSource
	tx                 *publicationTx
	claimed            bool
	finishError        error
	released, finished string
	saves              int
}

func (r *publicationStore) GetPublication(context.Context, string, int) (*domain.Publication, error) {
	return r.command, nil
}
func (r *publicationStore) EnqueuePublication(ctx context.Context, c *domain.Compilation, p domain.Publication) (*domain.Publication, bool, error) {
	if !r.tx.active || p.Validate() != nil || c.ID != p.CompilationID {
		return nil, false, errors.New("invalid acceptance boundary")
	}
	r.saves++
	p.Status = "pending"
	r.command = &p
	r.source.source.Status = domain.Status("publishing")
	return &p, true, nil
}
func (r *publicationStore) ClaimPublication(context.Context) (*domain.Publication, error) {
	if r.command == nil || r.command.Status != "pending" || r.claimed {
		return nil, nil
	}
	r.claimed = true
	copy := *r.command
	copy.ClaimToken = "current-claim"
	return &copy, nil
}
func (r *publicationStore) ReleasePublication(_ context.Context, p domain.Publication, code string) error {
	if r.tx.active || !r.claimed || p.ClaimToken != "current-claim" {
		return domain.ErrPublicationClaimStale
	}
	r.released = code
	r.claimed = false
	return nil
}
func (r *publicationStore) FinishPublication(_ context.Context, p domain.Publication, status, code string) error {
	if !r.tx.active || !r.claimed {
		return errors.New("invalid completion boundary")
	}
	if r.finishError != nil {
		return r.finishError
	}
	source := r.source.source
	if r.source.err == gorm.ErrRecordNotFound || source == nil || source.CreatedBy != p.RequestedBy || source.Version != p.WorkflowVersion || string(source.Status) != "publishing" {
		status = "superseded"
	} else {
		if status == "registered" {
			source.Status = domain.StatusPublished
		} else {
			source.Status = domain.Status("publish_failed")
		}
	}
	r.finished = code
	r.command.Status = status
	r.claimed = false
	return nil
}

type publicationEngine struct {
	tx                       *publicationTx
	taskErr, workflowErr     error
	taskCalls, workflowCalls int
	tasks                    []conductor.TaskDefinition
	definition               compiler.Definition
	afterRegistration        func()
}

func (e *publicationEngine) RegisterTaskDefinitions(_ context.Context, tasks []conductor.TaskDefinition) error {
	if e.tx.active {
		return errors.New("engine request inside transaction")
	}
	e.taskCalls++
	e.tasks = tasks
	return e.taskErr
}
func (e *publicationEngine) EnsureWorkflow(_ context.Context, d compiler.Definition) error {
	if e.tx.active {
		return errors.New("engine request inside transaction")
	}
	e.workflowCalls++
	e.definition = d
	if e.afterRegistration != nil {
		e.afterRegistration()
	}
	return e.workflowErr
}

func deliveryFixture() (*Delivery, *publicationStore, *publicationEngine) {
	tx := &publicationTx{}
	source := &publicationSource{source: &domain.Workflow{ID: "wf", Version: 1, CreatedBy: "owner", Status: domain.Status("publishing")}}
	d := compiler.Definition{Name: "frozen_workflow", Version: 3, SchemaVersion: 2, Tasks: []compiler.Task{{Name: "frozen_task", TaskReferenceName: "step", Type: "SIMPLE"}}}
	encoded, _ := compiler.CanonicalJSON(d)
	hash, _ := compiler.DefinitionHash(d)
	p := &domain.Publication{ID: "publication", WorkflowID: "wf", WorkflowVersion: 1, CompilationID: "comp", RequestedBy: "owner", DefinitionJSON: encoded, DefinitionHash: hash, TaskDefinitionsJSON: []byte(`[{"name":"frozen_task","retryCount":2,"timeoutSeconds":60}]`), Status: "pending"}
	p.PayloadDigest, _ = p.Digest()
	store := &publicationStore{command: p, source: source, tx: tx}
	engine := &publicationEngine{tx: tx}
	return &Delivery{Intents: store, Workflows: source, Engine: engine, Tx: tx}, store, engine
}

func TestPublicationDeliveryRetriesFrozenContractWithoutRecompilation(t *testing.T) {
	flow, store, engine := deliveryFixture()
	engine.workflowErr = errors.New("private upstream body")
	if err := flow.DispatchOne(context.Background()); !errors.Is(err, ErrTemporarilyUnavailable) || strings.Contains(err.Error(), "private") || store.command.Status != "pending" || store.released != "workflow_registration_unverified" || store.tx.commits != 0 {
		t.Fatal("temporary failure did not preserve intent")
	}
	engine.workflowErr = nil
	// A new dispatcher has no Template/Capability lookup port. It must restore
	// exactly the old target version and registration parameters from the intent.
	restarted := &Delivery{Intents: store, Workflows: store.source, Engine: engine, Tx: store.tx}
	if err := restarted.DispatchOne(context.Background()); err != nil || store.command.Status != "registered" || store.source.source.Status != domain.StatusPublished || store.tx.commits != 1 {
		t.Fatal("publication recovery failed", err)
	}
	if engine.definition.Name != "frozen_workflow" || engine.definition.Version != 3 || len(engine.tasks) != 1 || engine.tasks[0].RetryCount != 2 || engine.tasks[0].TimeoutSeconds != 60 {
		t.Fatal("recovery changed frozen target or policy")
	}
	if restarted.DispatchOne(context.Background()) != nil || engine.workflowCalls != 2 {
		t.Fatal("completed intent resent registration")
	}
}

func TestPublicationDeliveryConflictAndSourceChange(t *testing.T) {
	for _, mode := range []string{"task conflict", "workflow conflict", "hash conflict", "unknown contract field", "archived", "new version", "deleted", "changed during registration", "stale claim"} {
		t.Run(mode, func(t *testing.T) {
			flow, store, engine := deliveryFixture()
			switch mode {
			case "task conflict":
				engine.taskErr = conductor.ErrTaskDefinitionConflict
			case "workflow conflict":
				engine.workflowErr = conductor.ErrWorkflowDefinitionConflict
			case "hash conflict":
				store.command.DefinitionHash = strings.Repeat("b", 64)
				store.command.PayloadDigest, _ = store.command.Digest()
			case "unknown contract field":
				store.command.TaskDefinitionsJSON = json.RawMessage(`[{"name":"frozen_task","retryCount":2,"privateExtension":true}]`)
				store.command.PayloadDigest, _ = store.command.Digest()
			case "archived":
				store.source.source.Status = domain.StatusArchived
			case "new version":
				store.source.source.Version++
			case "deleted":
				store.source.err = gorm.ErrRecordNotFound
			case "changed during registration":
				engine.afterRegistration = func() { store.source.source.Version++ }
			case "stale claim":
				store.finishError = domain.ErrPublicationClaimStale
			}
			err := flow.DispatchOne(context.Background())
			if mode == "stale claim" {
				if !errors.Is(err, domain.ErrPublicationClaimStale) || store.command.Status != "pending" {
					t.Fatal("stale writer committed")
				}
				return
			}
			if err != nil {
				t.Fatal("terminal reconciliation failed", err)
			}
			obsolete := mode == "archived" || mode == "new version" || mode == "deleted" || mode == "changed during registration"
			if obsolete {
				if store.command.Status != "superseded" || store.source.source.Status == domain.StatusPublished {
					t.Fatal("changed source published")
				}
			} else if store.command.Status != "failed" {
				t.Fatal("permanent conflict was retried")
			}
			if (mode == "archived" || mode == "new version" || mode == "deleted" || mode == "hash conflict" || mode == "unknown contract field") && engine.taskCalls != 0 {
				t.Fatal("invalid or superseded command contacted engine")
			}
		})
	}
}
