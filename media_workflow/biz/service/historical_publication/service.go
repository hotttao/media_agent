package historical_publication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	publication "media_agent/media_workflow/biz/domain/historical_publication"
	jobdomain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/client"
)

var ErrConflict = errors.New("historical VideoPush source already imported with different facts")
var ErrForbidden = errors.New("historical VideoPush Workspace is not accessible")

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type WorkspaceLookup interface {
	GetWorkspaceForUser(ctx context.Context, workspaceID, userID string) (*client.WorkspaceContext, error)
}

type ObjectLookup interface {
	ResolveInternalObject(ctx context.Context, organizationID, objectPath string) (client.ResolvedStorageObject, error)
}

type JobLookup interface {
	Get(ctx context.Context, id string) (*jobdomain.Job, error)
}

type Service struct {
	repo       publication.Repo
	jobs       JobLookup
	workspaces WorkspaceLookup
	objects    ObjectLookup
}

func New(repo publication.Repo, jobs JobLookup, workspaces WorkspaceLookup, objects ObjectLookup) *Service {
	return &Service{repo: repo, jobs: jobs, workspaces: workspaces, objects: objects}
}

type ImportCommand struct {
	OrganizationID  string                  `json:"organization_id"`
	WorkspaceID     string                  `json:"workspace_id"`
	OwnerUserID     string                  `json:"owner_user_id"`
	SourceRecordID  string                  `json:"source_record_id"`
	ProductID       string                  `json:"product_id"`
	VirtualIPID     string                  `json:"virtual_ip_id"`
	SourceSceneID   string                  `json:"source_scene_id"`
	TemplateName    string                  `json:"template_name"`
	Title           string                  `json:"title"`
	Content         string                  `json:"content"`
	SourceStatus    string                  `json:"source_status"`
	IsQualified     bool                    `json:"is_qualified"`
	IsPublished     bool                    `json:"is_published"`
	Video           publication.MediaObject `json:"video"`
	Thumbnail       publication.MediaObject `json:"thumbnail"`
	ManualClip      publication.MediaObject `json:"manual_clip"`
	SourceCreatedAt time.Time               `json:"source_created_at"`
	SourceUpdatedAt time.Time               `json:"source_updated_at"`
	Members         []publication.Member    `json:"members"`
}

func (s *Service) Import(ctx context.Context, cmd ImportCommand) (*publication.HistoricalVideoPush, error) {
	if s == nil || s.repo == nil || s.jobs == nil || s.workspaces == nil || s.objects == nil {
		return nil, errors.New("historical publication import is not configured")
	}
	if cmd.OrganizationID == "" || cmd.WorkspaceID == "" || cmd.OwnerUserID == "" || cmd.SourceRecordID == "" ||
		cmd.ProductID == "" || cmd.VirtualIPID == "" || cmd.SourceCreatedAt.IsZero() || cmd.SourceUpdatedAt.IsZero() ||
		len(cmd.Members) == 0 || len(cmd.Members) > 100 || len(cmd.SourceRecordID) > 160 {
		return nil, errors.New("invalid historical VideoPush identity or members")
	}
	workspace, err := s.workspaces.GetWorkspaceForUser(ctx, cmd.WorkspaceID, cmd.OwnerUserID)
	if err != nil || workspace == nil || workspace.OrganizationID != cmd.OrganizationID {
		return nil, ErrForbidden
	}
	encoded, err := json.Marshal(cmd)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(encoded)
	fingerprint := hex.EncodeToString(digest[:])
	existing, err := s.repo.GetBySource(ctx, cmd.WorkspaceID, "media_ai", cmd.SourceRecordID)
	if err == nil {
		if existing.SourceDigest != fingerprint {
			return nil, ErrConflict
		}
		return existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	seenVideo, seenJob := map[string]bool{}, map[string]bool{}
	for i, member := range cmd.Members {
		if member.Ordinal != i || member.SourceVideoID == "" || member.JobID == "" || seenVideo[member.SourceVideoID] || seenJob[member.JobID] {
			return nil, fmt.Errorf("invalid or duplicate VideoPush member at ordinal %d", i)
		}
		seenVideo[member.SourceVideoID], seenJob[member.JobID] = true, true
		job, err := s.jobs.Get(ctx, member.JobID)
		if err != nil || job == nil || job.Origin != "legacy_import" || job.Status != jobdomain.JobSucceeded ||
			job.WorkspaceID != cmd.WorkspaceID || job.OrganizationID != cmd.OrganizationID || job.SubmittedBy != cmd.OwnerUserID ||
			job.SourceSystem != "media_ai" || job.SourceRecordID != "video:"+member.SourceVideoID ||
			job.Input.Product == nil || job.Input.Product.ID != cmd.ProductID ||
			job.Input.VirtualIP == nil || job.Input.VirtualIP.ID != cmd.VirtualIPID {
			return nil, fmt.Errorf("VideoPush member %q is not a matching completed historical Job", member.SourceVideoID)
		}
	}
	for label, media := range map[string]publication.MediaObject{"video": cmd.Video, "thumbnail": cmd.Thumbnail, "manual_clip": cmd.ManualClip} {
		if media.ObjectPath == "" && media.SHA256 == "" {
			continue
		}
		if media.ObjectPath == "" || !digestPattern.MatchString(media.SHA256) ||
			!strings.HasPrefix(media.ObjectPath, "materials/"+cmd.OrganizationID+"/ai_clothing/historical_publication_import/") {
			return nil, fmt.Errorf("invalid %s media object", label)
		}
		if _, err := s.objects.ResolveInternalObject(ctx, cmd.OrganizationID, media.ObjectPath); err != nil {
			return nil, fmt.Errorf("unreadable %s media object: %w", label, err)
		}
	}
	push := &publication.HistoricalVideoPush{
		ID: "hvp_" + uuid.NewString(), OrganizationID: cmd.OrganizationID, WorkspaceID: cmd.WorkspaceID,
		OwnerUserID: cmd.OwnerUserID, SourceSystem: "media_ai", SourceRecordID: cmd.SourceRecordID, SourceDigest: fingerprint,
		ProductID: cmd.ProductID, VirtualIPID: cmd.VirtualIPID, SourceSceneID: cmd.SourceSceneID,
		TemplateName: cmd.TemplateName, Title: cmd.Title, Content: cmd.Content, SourceStatus: cmd.SourceStatus,
		IsQualified: cmd.IsQualified, IsPublished: cmd.IsPublished, Video: cmd.Video, Thumbnail: cmd.Thumbnail,
		ManualClip: cmd.ManualClip, SourceCreatedAt: cmd.SourceCreatedAt, SourceUpdatedAt: cmd.SourceUpdatedAt,
		ImportedAt: time.Now().UTC(), Members: append([]publication.Member(nil), cmd.Members...),
	}
	if err := s.repo.Save(ctx, push); err != nil {
		if existing, lookupErr := s.repo.GetBySource(ctx, cmd.WorkspaceID, "media_ai", cmd.SourceRecordID); lookupErr == nil {
			if existing.SourceDigest == fingerprint {
				return existing, nil
			}
			return nil, ErrConflict
		}
		return nil, err
	}
	return push, nil
}

func (s *Service) Get(ctx context.Context, id, userID string) (*publication.HistoricalVideoPush, error) {
	push, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	workspace, err := s.workspaces.GetWorkspaceForUser(ctx, push.WorkspaceID, userID)
	if err != nil || workspace == nil || workspace.OrganizationID != push.OrganizationID {
		return nil, ErrForbidden
	}
	if err := s.resolveMedia(ctx, push); err != nil {
		return nil, err
	}
	return push, nil
}

func (s *Service) List(ctx context.Context, workspaceID, userID string, page, pageSize int) ([]*publication.HistoricalVideoPush, int64, error) {
	if page < 1 || pageSize < 1 || pageSize > 100 {
		return nil, 0, errors.New("invalid pagination")
	}
	workspace, err := s.workspaces.GetWorkspaceForUser(ctx, workspaceID, userID)
	if err != nil || workspace == nil {
		return nil, 0, ErrForbidden
	}
	items, total, err := s.repo.List(ctx, workspaceID, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	for _, item := range items {
		if item == nil || item.OrganizationID != workspace.OrganizationID {
			return nil, 0, ErrForbidden
		}
		if err := s.resolveMedia(ctx, item); err != nil {
			return nil, 0, err
		}
	}
	return items, total, nil
}

func (s *Service) resolveMedia(ctx context.Context, push *publication.HistoricalVideoPush) error {
	for label, media := range map[string]*publication.MediaObject{"video": &push.Video, "thumbnail": &push.Thumbnail, "manual_clip": &push.ManualClip} {
		if media.ObjectPath == "" {
			continue
		}
		resolved, err := s.objects.ResolveInternalObject(ctx, push.OrganizationID, media.ObjectPath)
		if err != nil {
			return fmt.Errorf("resolve historical %s media: %w", label, err)
		}
		media.AccessURL = resolved.URL
	}
	return nil
}
