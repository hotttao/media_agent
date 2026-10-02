//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/datatypes"

	"media_agent/media_workflow/biz/dal/model"
	jobRepo "media_agent/media_workflow/biz/dal/repo/job"
	jobDomain "media_agent/media_workflow/biz/domain/job"
)

func TestResumeOutboxTransactionAndReplay(t *testing.T) {
	db := newDatabase(t)
	ctx := context.Background()
	now := time.Now().UTC()
	name, sourceID, digest := "media_ai", "frame:test-resume", strings.Repeat("a", 64)
	job := &model.Job{ID: "job_resume_e2e", Origin: "legacy_import", OrganizationID: "org_e2e", WorkspaceID: "ws_e2e",
		SourceSystem: &name, SourceRecordID: &sourceID, SourceDigest: &digest, ImportedAt: &now,
		Scope: "ai_clothing", WorkflowID: "wf_e2e", WorkflowVersion: 1, Input: datatypes.JSON(`{"product":{"id":"prd"}}`),
		Status: "awaiting_resume", RetryHistory: datatypes.JSON(`[]`), SubmittedBy: "owner_e2e", SubmittedAt: now}
	if err := db.Create(job).Error; err != nil {
		t.Fatal(err)
	}
	repo := jobRepo.NewJobRepo(db)
	start := jobDomain.ResumeStart{ID: "resume_e2e", JobID: job.ID, Generation: 1,
		IdempotencyKey: "once", CorrelationID: "resume:" + job.ID + ":1",
		DefinitionJSON:  json.RawMessage(`{"name":"wf_resume_e2e","version":1,"tasks":[],"schemaVersion":2}`),
		TaskDomainsJSON: json.RawMessage(`{}`), WorkflowInputJSON: json.RawMessage(`{"job_id":"job_resume_e2e","inputs":{}}`)}
	var wg sync.WaitGroup
	created := make(chan bool, 4)
	errorsSeen := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fresh, err := repo.BeginResume(ctx, start, jobDomain.JobInput{Product: &jobDomain.ProductData{ID: "prd"}})
			created <- fresh
			errorsSeen <- err
		}()
	}
	wg.Wait()
	close(created)
	close(errorsSeen)
	newCount := 0
	for fresh := range created {
		if fresh {
			newCount++
		}
	}
	for err := range errorsSeen {
		if err != nil {
			t.Fatal(err)
		}
	}
	if newCount != 1 {
		t.Fatalf("concurrent replay created %d starts, want 1", newCount)
	}
	if _, err := repo.BeginResume(ctx, jobDomain.ResumeStart{ID: "resume_other", JobID: job.ID, Generation: 1,
		IdempotencyKey: "different", CorrelationID: "resume:" + job.ID + ":1",
		DefinitionJSON: start.DefinitionJSON, TaskDomainsJSON: start.TaskDomainsJSON, WorkflowInputJSON: start.WorkflowInputJSON}, jobDomain.JobInput{}); !errors.Is(err, jobDomain.ErrResumeConflict) {
		t.Fatalf("different key was not rejected: %v", err)
	}
	claimed, err := repo.ClaimPendingResume(ctx)
	if err != nil || claimed == nil || claimed.ID != start.ID || !json.Valid(claimed.DefinitionJSON) {
		t.Fatalf("frozen start was not claimed: start=%+v err=%v", claimed, err)
	}
	if err := repo.ReleaseResumeClaim(ctx, start.ID, "temporary Conductor outage"); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE media_workflow.resume_outbox SET next_attempt_at=now()-interval '1 second' WHERE id=?", start.ID).Error; err != nil {
		t.Fatal(err)
	}
	claimed, err = repo.ClaimPendingResume(ctx)
	if err != nil || claimed == nil || claimed.ID != start.ID {
		t.Fatalf("released command was not reclaimed after outage: start=%+v err=%v", claimed, err)
	}
	if err := repo.MarkResumeStarted(ctx, *claimed, "conductor_e2e", "hash_e2e"); err != nil {
		t.Fatal(err)
	}
	var actual model.Job
	if err := db.First(&actual, "id = ?", job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if actual.Status != "running" || actual.ResumeGeneration != 1 || actual.ConductorWorkflowID != "conductor_e2e" {
		t.Fatalf("resume Job not updated atomically: %+v", actual)
	}
	if err := db.Model(&model.Job{}).Where("id = ?", job.ID).Update("status", "succeeded").Error; err != nil {
		t.Fatal(err)
	}
	if fresh, err := repo.BeginResume(ctx, start, jobDomain.JobInput{}); err != nil || fresh {
		t.Fatalf("completed resume replay created another start: fresh=%v err=%v", fresh, err)
	}
	var count int64
	if err := db.Table("media_workflow.resume_outbox").Where("job_id = ?", job.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("resume outbox rows=%d, want 1", count)
	}
	claimed, err = repo.ClaimPendingResume(ctx)
	if err != nil || claimed != nil {
		t.Fatalf("started command reclaimed: %+v err=%v", claimed, err)
	}
}
