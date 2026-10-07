package job

import (
	"context"
	"testing"

	"gorm.io/gorm"
	domain "media_agent/media_workflow/biz/domain/job"
	wfdomain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/shared/client"
)

type registrationProbe struct {
	wfdomain.CompilationRepo
	reads int
}

func TestSubmitNeverFallsBackToLocalScheduling(t *testing.T) {
	for _, missing := range []string{"engine", "compilation"} {
		t.Run(missing, func(t *testing.T) {
			svc, jobs, starts, _, _, cmd := submissionFixture()
			cmd.IdempotencyKey = ""
			if missing == "engine" {
				svc.engine = nil
			} else {
				svc.compilations = nil
			}
			if _, err := svc.Submit(context.Background(), cmd); err == nil || len(jobs.jobs) != 0 || starts.command != nil {
				t.Fatal("missing native dependency silently created a local execution", err)
			}
		})
	}
}

func (r *registrationProbe) GetRegistered(context.Context, string, int) (*wfdomain.Compilation, error) {
	r.reads++
	return nil, gorm.ErrRecordNotFound
}

func TestSubmitRequiresPublishedAndRegisteredVersion(t *testing.T) {
	for _, status := range []wfdomain.Status{wfdomain.StatusDraft, wfdomain.StatusPublishing, wfdomain.StatusPublishFailed, wfdomain.StatusArchived, wfdomain.StatusPublished} {
		t.Run(string(status), func(t *testing.T) {
			svc, jobs, steps := newTestService(false)
			w := svc.wfRepo.(*mockWorkflowLookup).wf
			w.Status, w.CreatedBy = status, "owner"
			authority := &submitAuthority{workspace: &client.WorkspaceContext{ID: "workspace", OrganizationID: "org"}, allowed: true}
			svc.historyWorkspaces, svc.resumeAccess = authority, authority
			compilations, engine := &registrationProbe{}, &mockWorkflowEngine{}
			svc.compilations, svc.engine = compilations, engine
			_, err := svc.Submit(context.Background(), SubmitCommand{WorkflowID: "wf1", SubmittedBy: "owner", WorkspaceID: "workspace", OrganizationID: "org", Input: domain.JobInput{}})
			if err == nil || len(jobs.jobs) != 0 || len(steps.items) != 0 || engine.correlationID != "" {
				t.Fatal("unregistered workflow created execution facts")
			}
			wantReads := 0
			if status == wfdomain.StatusPublished {
				wantReads = 1
			}
			if compilations.reads != wantReads {
				t.Fatal("source-state or registered-version guard bypassed")
			}
		})
	}
}
