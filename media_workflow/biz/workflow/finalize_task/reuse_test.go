package finalize_task

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	capability "media_agent/media_workflow/biz/domain/capability"
	domain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/client"
	"media_agent/media_workflow/biz/shared/taskerror"
	"media_agent/media_workflow/biz/shared/taskref"
	"media_agent/media_workflow/biz/workflow/reuse_task"
)

type reuseEvidenceStore struct {
	entries []domain.ReuseEvidence
	saved   []domain.ReuseEvidence
	err     error
}

func (s *reuseEvidenceStore) Find(context.Context, domain.ReuseScope, string) ([]domain.ReuseEvidence, error) {
	return s.entries, s.err
}
func (s *reuseEvidenceStore) Save(_ context.Context, e domain.ReuseEvidence) error {
	s.saved = append(s.saved, e)
	return s.err
}

type reusableAssets struct {
	asset *client.ReusableGeneratedAsset
	err   error
	calls int
}

func (a *reusableAssets) VerifyReusableGeneratedAsset(context.Context, string, string, string, string) (*client.ReusableGeneratedAsset, error) {
	a.calls++
	return a.asset, a.err
}

func reuseFixture(hit bool) (Handler, context.Context, Input, *registrationExecution, *registrationAssets, *reuseEvidenceStore, *reusableAssets) {
	h, ctx, input, engine, registrar := registrationFixture()
	j := h.Jobs.(registrationJobs).item
	j.WorkspaceID, j.Origin = "workspace", "native"
	contract := reuse_task.Identity{TemplateID: "template", TemplateVersion: 1, CapabilityKey: "cap", CapabilityVersion: 1, CandidateCount: 1, OutputAssetKind: "image", Inputs: []capability.Input{{Name: "prompt", Type: capability.InputString, Required: true}}}
	input.FingerprintIdentity = &contract
	input.ReusePolicy = "reuse_if_available"
	entry := domain.ReuseEvidence{ID: domain.ReuseEvidenceID("source-step", "source-key"), Boundary: reuseBoundary(j), Fingerprint: strings.Repeat("a", 64), SourceJobID: "source-job", SourceStepRunID: "source-step", SourceResultKey: "source-key", AssetID: "gen_source", Content: "accepted", AssetKind: "image"}
	input.ReuseResult = &reuse_task.Output{Hit: hit, Fingerprint: entry.Fingerprint}
	if hit {
		input.Candidates = nil
		input.ReuseResult.EvidenceID, input.ReuseResult.SourceStepRunID = entry.ID, entry.SourceStepRunID
		input.ReuseResult.Resource = Resource{ID: entry.AssetID, Content: entry.Content, AssetKind: entry.AssetKind}
		engine.item.Tasks = engine.item.Tasks[:1] // No generation or HUMAN exists on hit.
	} else {
		engine.item.Tasks[1].Input = map[string]any{"prompt": "final prompt with feedback", "job_id": "job"}
	}
	var frozen, checkInput, checkOutput map[string]any
	_ = decode(input, &frozen)
	_ = decode(reuse_task.Input{JobID: input.JobID, StepKey: input.StepKey, Policy: input.ReusePolicy, Identity: contract, ProviderInput: map[string]any{"prompt": "initial prompt"}}, &checkInput)
	_ = decode(input.ReuseResult, &checkOutput)
	engine.item.Tasks[0].Input = frozen
	engine.item.Tasks = append(engine.item.Tasks, domain.EngineTask{TaskID: "check", TaskType: reuse_task.TaskDefinitionName, ReferenceTaskName: taskref.Reference(input.StepKey, "reuse"), Status: "COMPLETED", Input: checkInput, Output: checkOutput})
	store := &reuseEvidenceStore{entries: []domain.ReuseEvidence{entry}}
	assets := &reusableAssets{asset: &client.ReusableGeneratedAsset{ID: entry.AssetID, OrganizationID: entry.Boundary.OrganizationID, Scope: entry.Boundary.Scope, OwnerUserID: entry.Boundary.OwnerUserID, JobID: entry.SourceJobID, StepRunID: entry.SourceStepRunID, TemplateID: contract.TemplateID, TemplateVersion: contract.TemplateVersion, AssetKind: entry.AssetKind, Content: entry.Content}}
	h.Evidence, h.ReusableAssets = store, assets
	return h, ctx, input, engine, registrar, store, assets
}

func TestFinalizeReuseReturnsAcceptedResourceWithoutRegistration(t *testing.T) {
	h, ctx, input, _, registrar, store, assets := reuseFixture(true)
	for i := 0; i < 2; i++ {
		output, err := h.Execute(ctx, input)
		require.NoError(t, err)
		require.Equal(t, "gen_source", output.OutputResourceID)
		require.Equal(t, "reused", output.ResultSource)
		require.Equal(t, "source-step", output.ReusedFromStepRunID)
	}
	require.Empty(t, registrar.keys)
	require.Empty(t, store.saved)
	require.Equal(t, 2, assets.calls, "every technical retry rechecks source validity")
}

func TestFinalizeReuseRejectsForgedCheckAndSourceDeletion(t *testing.T) {
	t.Run("altered native output", func(t *testing.T) {
		h, ctx, input, engine, registrar, _, assets := reuseFixture(true)
		engine.item.Tasks[1].Output["evidence_id"] = "forged"
		_, err := h.Execute(ctx, input)
		require.True(t, taskerror.IsPermanent(err))
		require.Zero(t, assets.calls)
		require.Empty(t, registrar.keys)
	})
	for _, gone := range []bool{true, false} {
		h, ctx, input, _, registrar, _, assets := reuseFixture(true)
		assets.err = errors.New("storage temporarily down")
		if gone {
			assets.err = client.ErrReusableAssetNotFound
		}
		_, err := h.Execute(ctx, input)
		require.Error(t, err)
		require.Equal(t, gone, taskerror.IsPermanent(err))
		require.Empty(t, registrar.keys, "cannot register or locally replan on a stale hit")
	}
}

func TestFinalizeIndexesActualSuccessfulGenerationNotInitialProbe(t *testing.T) {
	h, ctx, input, engine, registrar, store, _ := reuseFixture(false)
	output, err := h.Execute(ctx, input)
	require.NoError(t, err)
	require.Equal(t, "generated", output.ResultSource)
	require.Len(t, registrar.keys, 1)
	require.Len(t, store.saved, 1)
	expected, err := reuse_task.BuildFingerprint(ctx, "org", *input.FingerprintIdentity, engine.item.Tasks[1].Input, nil)
	require.NoError(t, err)
	require.Equal(t, expected, store.saved[0].Fingerprint)
	require.NotEqual(t, input.ReuseResult.Fingerprint, store.saved[0].Fingerprint)
	require.NoError(t, store.saved[0].Validate())
	// Registration and evidence identities remain stable across transport retry.
	_, err = h.Execute(ctx, input)
	require.NoError(t, err)
	require.True(t, store.saved[0].Same(store.saved[1]))
}
