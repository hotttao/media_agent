package media_management

import (
	"context"
	"fmt"
	"media_agent/hertz_infra/authn/mcpdelegation"
	contract "media_agent/media_worker/biz/shared/media_management"
	"sync/atomic"
	"testing"
)

type submissionAPI struct {
	submitted []string
	polled    atomic.Int32
}

func (a *submissionAPI) Execute(_ context.Context, name string, p contract.Params) (map[string]any, error) {
	switch name {
	case "workflows_get":
		return map[string]any{"version": 1, "steps": []any{map[string]any{"template_id": "t1", "template_version": 1}}}, nil
	case "templates_get":
		return map[string]any{"capability_key": "gpt.t2i.text_prompt", "capability_version": 1}, nil
	case "jobs_submit":
		a.submitted = append(a.submitted, p.IdempotencyKey)
		if p.IdempotencyKey == "bad" {
			return nil, &Failure{Code: "version_conflict", Message: "Changed"}
		}
		return map[string]any{"job_id": "job_" + p.IdempotencyKey, "status": "PENDING"}, nil
	case "jobs_get":
		a.polled.Add(1)
		return map[string]any{"job": map[string]any{"id": p.ID, "status": "RUNNING"}}, nil
	default:
		return nil, fmt.Errorf("unexpected operation %s", name)
	}
}
func TestBatchSubmitReturnsEachJobIDWithoutWaitingForGeneration(t *testing.T) {
	api := &submissionAPI{}
	management := Management{API: Operations{API: api}}
	ctx := mcpdelegation.WithPrincipal(context.Background(), mcpdelegation.Principal{Kind: "user", UserID: "u", ActorID: "User:u", CredentialID: "c", OrganizationID: "o", WorkspaceID: "w"})
	items := []Item[contract.Params]{}
	for _, key := range []string{"one", "bad", "three"} {
		items = append(items, Item[contract.Params]{ID: key, Params: contract.Params{WorkflowID: "wf1", WorkflowVersion: 1, IdempotencyKey: key}})
	}
	result, err := management.Execute(ctx, "jobs_submit", items)
	if err != nil || result.Summary.Succeeded != 2 || result.Summary.Failed != 1 {
		t.Fatalf("bad receipt: %+v %v", result, err)
	}
	if result.Results[0].Data["job_id"] != "job_one" || result.Results[2].Data["job_id"] != "job_three" {
		t.Fatal("successful Job IDs were lost")
	}
	if api.polled.Load() != 0 {
		t.Fatal("submit waited for execution")
	}
	query, err := management.Execute(ctx, "jobs_get", []Item[contract.Params]{{ID: "one", Params: contract.Params{ID: "job_one"}}, {ID: "three", Params: contract.Params{ID: "job_three"}}})
	if err != nil || query.Summary.Succeeded != 2 {
		t.Fatalf("cannot query submitted jobs: %+v %v", query, err)
	}
}

type receiptAPI struct {
	submissionAPI
	receipt map[string]any
	writes  int
}

func (a *receiptAPI) Execute(ctx context.Context, name string, p contract.Params) (map[string]any, error) {
	if name == "jobs_submit" {
		a.writes++
		return a.receipt, nil
	}
	return a.submissionAPI.Execute(ctx, name, p)
}
func TestIncompleteSubmissionReceiptIsUnknownAndNeverRetried(t *testing.T) {
	for _, receipt := range []map[string]any{{"status": "PENDING"}, {"job_id": "job-one"}} {
		api := &receiptAPI{receipt: receipt}
		ctx := mcpdelegation.WithPrincipal(context.Background(), mcpdelegation.Principal{Kind: "user", UserID: "u", ActorID: "User:u", CredentialID: "c", OrganizationID: "o", WorkspaceID: "w"})
		result, err := (Management{API: Operations{API: api}}).Execute(ctx, "jobs_submit", []Item[contract.Params]{{ID: "one", Params: contract.Params{WorkflowID: "wf1", WorkflowVersion: 1, IdempotencyKey: "one"}}})
		if err != nil || result.Summary.Unknown != 1 || api.writes != 1 || api.polled.Load() != 0 {
			t.Fatalf("unsafe submission handling: %+v, writes=%d, err=%v", result, api.writes, err)
		}
	}
}

func TestDuplicateSubmissionKeysFailBeforeAnyJobCreation(t *testing.T) {
	api := &submissionAPI{}
	ctx := mcpdelegation.WithPrincipal(context.Background(), mcpdelegation.Principal{Kind: "user", UserID: "u", ActorID: "User:u", CredentialID: "c", OrganizationID: "o", WorkspaceID: "w"})
	params := contract.Params{WorkflowID: "wf1", WorkflowVersion: 1, IdempotencyKey: "same-key"}
	_, err := (Management{API: Operations{API: api}}).Execute(ctx, "jobs_submit", []Item[contract.Params]{{ID: "one", Params: params}, {ID: "two", Params: params}})
	if err == nil || len(api.submitted) != 0 {
		t.Fatal("duplicate submission keys reached business API")
	}
}
