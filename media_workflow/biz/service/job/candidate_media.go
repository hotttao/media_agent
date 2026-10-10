package job

import (
	"context"
	"errors"
	"net/url"
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
			objectPath := candidate.ObjectPath
			// Legacy workers returned a storage URL rather than an ObjectPath.
			// Refresh only verified links from our configured storage origin;
			// derive the response view without rewriting the frozen candidate.
			if objectPath == "" && candidate.URL != "" && s.resumeStorageOrigin != "" {
				u, urlErr := url.Parse(candidate.URL)
				origin, originErr := url.Parse(s.resumeStorageOrigin)
				if urlErr == nil && originErr == nil && u.Scheme == origin.Scheme && u.Host == origin.Host {
					var err error
					objectPath, err = resumeObjectPath(candidate.URL, s.resumeStorageOrigin, j.OrganizationID)
					if err != nil {
						return nil, errors.New("candidate object outside execution namespace")
					}
				}
			}
			if objectPath == "" {
				continue
			}
			prefix := "materials/" + j.OrganizationID + "/" + j.Scope + "/"
			if j.OrganizationID == "" || j.Scope == "" || !strings.HasPrefix(objectPath, prefix) || path.Clean(objectPath) != objectPath || strings.Contains(objectPath, "\\") {
				return nil, errors.New("candidate object outside execution namespace")
			}
			if s.candidateMedia == nil {
				return nil, errors.New("candidate storage unavailable")
			}
			object, ok := cache[objectPath]
			if !ok {
				var err error
				object, err = s.candidateMedia.ResolveInternalObject(ctx, j.OrganizationID, objectPath)
				if err != nil || object.URL == "" || object.ExpiresAt <= time.Now().Unix() {
					return nil, errors.New("candidate storage unavailable")
				}
				cache[objectPath] = object
			}
			copy.Candidates[n].URL = object.URL
		}
	}
	return views, nil
}
