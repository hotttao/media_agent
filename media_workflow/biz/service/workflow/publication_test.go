package workflow

import (
	"context"
	"errors"
	"testing"

	domain "media_agent/media_workflow/biz/domain/workflow"
)

type publicationRepo struct {
	domain.Repo
	w domain.Workflow
}

func (r *publicationRepo) Get(context.Context, string) (*domain.Workflow, error) {
	w := r.w
	return &w, nil
}

type acceptingPublisher struct {
	repo   *publicationRepo
	calls  int
	status string
}

func (p *acceptingPublisher) AcceptPublication(_ context.Context, w *domain.Workflow, caller string) (*domain.Publication, error) {
	p.calls++
	if caller != w.CreatedBy {
		return nil, errors.New("bad caller")
	}
	if p.status == "pending" {
		p.repo.w.Status = domain.StatusPublishing
	}
	return &domain.Publication{ID: "receipt", WorkflowVersion: w.Version, Status: p.status}, nil
}

type receiptRepo struct {
	domain.PublicationRepo
	calls int
}

func (r *receiptRepo) GetPublication(_ context.Context, id string, version int) (*domain.Publication, error) {
	r.calls++
	if id != "wf" || version != 1 {
		return nil, errors.New("bad identity")
	}
	return &domain.Publication{ID: "receipt", Status: "pending", DefinitionJSON: []byte(`{"private":"contract"}`)}, nil
}

func TestPublicationAuthorizationAndDurableReceipt(t *testing.T) {
	ctx := context.Background()
	r := &publicationRepo{w: domain.Workflow{ID: "wf", Version: 1, CreatedBy: "owner", Status: domain.StatusDraft, Visibility: domain.VisibilityPrivate}}
	p := &acceptingPublisher{repo: r, status: "pending"}
	s := NewService(r, nil, nil, nil, p)
	if _, err := s.Publish(ctx, "wf", "other"); !errors.Is(err, domain.ErrWorkflowForbidden) || p.calls != 0 {
		t.Fatal("unauthorized publish reached compiler")
	}
	for range 2 {
		w, err := s.Publish(ctx, "wf", "owner")
		if err != nil || w.Status != domain.StatusPublishing || w.PublicationID != "receipt" || w.PublicationStatus != "pending" {
			t.Fatal("acceptance or pending replay lost receipt")
		}
	}
	p.status, r.w.Status = "registered", domain.StatusPublished
	w, err := s.Publish(ctx, "wf", "owner")
	if err != nil || w.PublicationStatus != "registered" || w.Status != domain.StatusPublished {
		t.Fatal("registered replay rejected")
	}
	p.status, r.w.Status = "failed", domain.StatusPublishFailed
	w, err = s.Publish(ctx, "wf", "owner")
	if err != nil || w.PublicationStatus != "failed" {
		t.Fatal("failed replay discarded audit")
	}
	r.w.Status = domain.StatusArchived
	calls := p.calls
	if _, err := s.Publish(ctx, "wf", "owner"); !errors.Is(err, domain.ErrPublicationConflict) || p.calls != calls {
		t.Fatal("archived workflow republished")
	}

	receipts := &receiptRepo{}
	s.ConfigurePublications(receipts)
	if _, err := s.Get(ctx, "wf", "other"); err == nil || receipts.calls != 0 {
		t.Fatal("receipt queried before authorization")
	}
	w, err = s.Get(ctx, "wf", "owner")
	if err != nil || w.PublicationID != "receipt" || receipts.calls != 1 {
		t.Fatal("authorized query lost current receipt")
	}
}
