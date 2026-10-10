package job

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/client"
)

type candidateResolverStub struct {
	calls  int
	object client.ResolvedStorageObject
	err    error
}

func (r *candidateResolverStub) ResolveInternalObject(_ context.Context, org, path string) (client.ResolvedStorageObject, error) {
	r.calls++
	if org != "org" || path != "materials/org/scope/frame.png" {
		return client.ResolvedStorageObject{}, errors.New("wrong namespace")
	}
	return r.object, r.err
}

func TestJobAndStepCandidateViewsDoNotPersistSignedURLs(t *testing.T) {
	j := &domain.Job{ID: "job", OrganizationID: "org", Scope: "scope"}
	step := &domain.StepRun{ID: "step", JobID: j.ID, Candidates: []domain.Candidate{{Index: 0, ObjectPath: "materials/org/scope/frame.png"}}}
	jr := &mockJobRepo{jobs: map[string]*domain.Job{j.ID: j}, stepRuns: map[string][]*domain.StepRun{j.ID: {step}}}
	s := &Service{jobRepo: jr, stepRunRepo: &mockStepRunRepo{items: map[string]*domain.StepRun{step.ID: step}}}
	r := &candidateResolverStub{object: client.ResolvedStorageObject{URL: "https://storage.test/fresh?private=test-signature", ExpiresAt: time.Now().Add(time.Minute).Unix()}}
	s.ConfigureCandidateMedia(r)
	_, views, err := s.Get(context.Background(), j.ID)
	if err != nil || views[0].Candidates[0].URL != r.object.URL || views[0] == step || step.Candidates[0].URL != "" {
		t.Fatal("Job response failed or mutated stored candidates", err)
	}
	r.object.URL = "https://storage.test/fresher?private=second"
	view, err := s.GetStepRun(context.Background(), step.ID)
	if err != nil || view.Candidates[0].URL != r.object.URL || step.Candidates[0].URL != "" || r.calls != 2 {
		t.Fatal("Step read returned raw or stale candidates", err)
	}
	// No saver is configured: querying must not register Assets.
}

func TestCandidateViewsRejectForeignAndUnreadableObjects(t *testing.T) {
	for _, kind := range []string{"org", "scope", "traversal", "expired", "unavailable"} {
		t.Run(kind, func(t *testing.T) {
			r := &candidateResolverStub{object: client.ResolvedStorageObject{URL: "https://storage.test/fresh", ExpiresAt: time.Now().Add(time.Minute).Unix()}}
			s := &Service{candidateMedia: r}
			candidate := domain.Candidate{ObjectPath: "materials/org/scope/frame.png"}
			switch kind {
			case "org":
				candidate.ObjectPath = "materials/other/scope/frame.png"
			case "scope":
				candidate.ObjectPath = "materials/org/other/frame.png"
			case "traversal":
				candidate.ObjectPath = "materials/org/scope/../frame.png"
			case "expired":
				r.object.ExpiresAt = time.Now().Add(-time.Minute).Unix()
			case "unavailable":
				r.err = errors.New("private provider error")
			}
			_, err := s.candidateViews(context.Background(), &domain.Job{OrganizationID: "org", Scope: "scope"}, []*domain.StepRun{{Candidates: []domain.Candidate{candidate}}})
			if err == nil || strings.Contains(err.Error(), "private provider error") {
				t.Fatal("unsafe candidate exposed")
			}
			if (kind == "org" || kind == "scope" || kind == "traversal") && r.calls != 0 {
				t.Fatal("foreign object reached storage")
			}
		})
	}
}

func TestLegacyStorageCandidateViewRefreshesWithoutChangingSnapshot(t *testing.T) {
	r := &candidateResolverStub{object: client.ResolvedStorageObject{URL: "https://storage.test/fresh", ExpiresAt: time.Now().Add(time.Minute).Unix()}}
	s := &Service{candidateMedia: r, resumeStorageOrigin: "https://storage.test"}
	j := &domain.Job{OrganizationID: "org", Scope: "scope"}
	stored := &domain.StepRun{Candidates: []domain.Candidate{{URL: "https://storage.test/media/materials/org/scope/frame.png?old=expired"}}}
	views, err := s.candidateViews(context.Background(), j, []*domain.StepRun{stored})
	if err != nil || views[0].Candidates[0].URL != r.object.URL || stored.Candidates[0].URL != "https://storage.test/media/materials/org/scope/frame.png?old=expired" || stored.Candidates[0].ObjectPath != "" {
		t.Fatal("legacy storage candidate was not refreshed as a response-only view", err)
	}
	stored.Candidates[0].URL = "https://external.test/media/materials/org/scope/frame.png"
	views, err = s.candidateViews(context.Background(), j, []*domain.StepRun{stored})
	if err != nil || views[0].Candidates[0].URL != stored.Candidates[0].URL || r.calls != 1 {
		t.Fatal("external origin was interpreted as our storage")
	}
	stored.Candidates[0].URL = "https://storage.test/media/materials/org/other/frame.png"
	if _, err := s.candidateViews(context.Background(), j, []*domain.StepRun{stored}); err == nil || r.calls != 1 {
		t.Fatal("foreign Scope reached storage through a legacy URL")
	}
	stored.Candidates[0].URL = "https://storage.test/media/materials/other/scope/frame.png"
	if _, err := s.candidateViews(context.Background(), j, []*domain.StepRun{stored}); err == nil || r.calls != 1 {
		t.Fatal("foreign Organization reached storage through a legacy URL")
	}
}
