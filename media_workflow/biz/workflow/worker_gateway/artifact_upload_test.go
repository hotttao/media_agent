package worker_gateway

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	capability "media_agent/media_workflow/biz/domain/capability"
	job "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/client"
)

type artifactPreparer struct {
	calls int
	input client.ArtifactUploadRequest
	bad   string
}

func (p *artifactPreparer) PrepareArtifactUpload(_ context.Context, in client.ArtifactUploadRequest) (client.ArtifactUploadGrant, error) {
	p.calls++
	p.input = in
	grant := client.ArtifactUploadGrant{ObjectPath: "materials/" + in.OrganizationID + "/" + in.Scope + "/worker_generated/" + in.UploadKey + "/" + strings.Repeat("a", 64), UploadURL: "https://storage.test/objects/key", ExpiresAt: time.Now().Add(time.Minute).Unix(), Headers: map[string]string{"If-None-Match": "*"}}
	if p.bad == "path" {
		grant.ObjectPath = "materials/other/secret"
	}
	if p.bad == "url" {
		grant.UploadURL = "file:///private"
	}
	if p.bad == "header" {
		grant.Headers["Authorization"] = "must never forward a bearer"
	}
	return grant, nil
}

func artifactFixture(t *testing.T) (*Flow, *artifactPreparer, PrepareArtifactCommand, *taskEngineStub) {
	t.Helper()
	now := time.Now()
	f, _, engine := newPollFlow(now)
	contract := inputContract(false)
	f.Capabilities = &capabilityRepoStub{item: contract}
	f.ImportedAssets = importedAssetStub{object: client.ResolvedStorageObject{URL: "https://storage.test/fresh.png", ExpiresAt: now.Add(time.Hour).Unix(), ContentType: "image/png", SizeBytes: 100}}
	engine.task.Input["capability_key"], engine.task.Input["capability_version"] = contract.Key, 1
	engine.task.Input["prompt"] = "rendered prompt"
	engine.task.Input["images"] = []any{"object_path:materials/org-1/a.png"}
	f.Jobs.(*jobRepoStub).item.Scope = "ai_clothing"
	p := &artifactPreparer{}
	f.ArtifactUploads = p
	principal := Principal{UserID: "user-1", OrganizationID: "org-1", CredentialID: "credential-1", ActorID: "Worker:worker-1", AuthSource: "api_token"}
	d, err := f.Poll(context.Background(), PollCommand{Principal: principal, WorkerInstanceID: "desktop-1", CapabilityKey: contract.Key, CapabilityVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	cmd := PrepareArtifactCommand{Principal: principal, DispatchID: d.ID, WorkerInstanceID: d.WorkerInstanceID, Attempt: d.Attempt, ArtifactMetadata: ArtifactMetadata{ContentType: "image/png", SizeBytes: 256, ChecksumSHA256: strings.Repeat("a", 64), ContentMD5: base64.StdEncoding.EncodeToString(make([]byte, 16))}}
	return f, p, cmd, engine
}

func TestArtifactPreparationDerivesTenantAndStableOpaqueTaskKey(t *testing.T) {
	f, p, cmd, engine := artifactFixture(t)
	first, err := f.PrepareArtifactUpload(context.Background(), cmd)
	if err != nil || p.input.OrganizationID != "org-1" || p.input.Scope != "ai_clothing" || len(p.input.UploadKey) != 64 {
		t.Fatal("task-derived upload grant failed", err)
	}
	again, err := f.PrepareArtifactUpload(context.Background(), cmd)
	if err != nil || first.ObjectPath != again.ObjectPath || len(engine.updateCalls) != 0 {
		t.Fatal("upload preparation changed task or lost replay identity")
	}
	cmd.CandidateIndex = 1
	next, err := f.PrepareArtifactUpload(context.Background(), cmd)
	if err != nil || next.ObjectPath == first.ObjectPath {
		t.Fatal("two candidates shared object identity")
	}
}

func TestArtifactPreparationRejectsInvalidIdentityTaskAndManifest(t *testing.T) {
	for _, kind := range []string{"actor", "credential", "attempt", "cancelled", "replaced", "mime", "size", "checksum", "position", "target"} {
		t.Run(kind, func(t *testing.T) {
			f, p, cmd, engine := artifactFixture(t)
			switch kind {
			case "actor":
				cmd.ActorID = "Worker:other"
			case "credential":
				cmd.CredentialID = "other"
			case "attempt":
				cmd.Attempt++
			case "cancelled":
				f.Jobs.(*jobRepoStub).item.Status = job.JobCancelled
			case "replaced":
				engine.task.TaskID = "replacement"
			case "mime":
				cmd.ContentType = "video/mp4"
			case "size":
				cmd.SizeBytes = 17 << 20
			case "checksum":
				cmd.ContentMD5 = "invalid"
			case "position":
				cmd.CandidateIndex = 8
			case "target":
				f.Capabilities.(*capabilityRepoStub).item.ExecutionTarget = capability.ExecutionTargetManagedWorker
			}
			if _, err := f.PrepareArtifactUpload(context.Background(), cmd); err == nil || p.calls != 0 || len(engine.updateCalls) != 0 {
				t.Fatal("invalid upload signed an object or mutated task", err)
			}
		})
	}
	for _, bad := range []string{"path", "url", "header"} {
		f, p, cmd, _ := artifactFixture(t)
		p.bad = bad
		if _, err := f.PrepareArtifactUpload(context.Background(), cmd); err == nil {
			t.Fatal("unsafe downstream grant was exposed")
		}
	}
}
