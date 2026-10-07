package job

import (
	"context"
	"errors"
	"path"
	"strings"
	"time"

	domain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/client"
)

type CandidateObjectResolver interface {
	ResolveInternalObject(context.Context, string, string) (client.ResolvedStorageObject, error)
}

func (s *Service) ConfigureCandidateMedia(resolver CandidateObjectResolver) {
	s.candidateMedia = resolver
}

// Response-only copies: neither a signed URL nor an Asset registration is
// written while reading a Job, including candidates awaiting approval.
func (s *Service) candidateViews(ctx context.Context, j *domain.Job, steps []*domain.StepRun) ([]*domain.StepRun, error) {
	views := make([]*domain.StepRun, len(steps))
	cache := make(map[string]client.ResolvedStorageObject)
	for i, step := range steps {
		copy := *step
		copy.Candidates = append([]domain.Candidate(nil), step.Candidates...)
		views[i] = &copy
		for n, candidate := range copy.Candidates {
			if candidate.ObjectPath == "" {
				continue
			}
			prefix := "materials/" + j.OrganizationID + "/" + j.Scope + "/"
			if j.OrganizationID == "" || j.Scope == "" || !strings.HasPrefix(candidate.ObjectPath, prefix) || path.Clean(candidate.ObjectPath) != candidate.ObjectPath || strings.Contains(candidate.ObjectPath, "\\") {
				return nil, errors.New("candidate object outside execution namespace")
			}
			if s.candidateMedia == nil {
				return nil, errors.New("candidate storage unavailable")
			}
			object, ok := cache[candidate.ObjectPath]
			if !ok {
				var err error
				object, err = s.candidateMedia.ResolveInternalObject(ctx, j.OrganizationID, candidate.ObjectPath)
				if err != nil || object.URL == "" || object.ExpiresAt <= time.Now().Unix() {
					return nil, errors.New("candidate storage unavailable")
				}
				cache[candidate.ObjectPath] = object
			}
			copy.Candidates[n].URL = object.URL
		}
	}
	return views, nil
}
