package skill_content

import (
	"context"
	"github.com/google/uuid"
	"media_agent/hertz_infra/skillarchive"
	d "media_agent/media_control/biz/domain/skill"
	service "media_agent/media_control/biz/service/skill"
	"net/url"
	"strings"
)

type Artifacts interface {
	Put(context.Context, string, []byte) error
	Get(context.Context, string) ([]byte, error)
}
type Git interface {
	Fetch(context.Context, d.GitSource, string) (skillarchive.Tree, string, error)
}
type Secrets interface {
	Encrypt(string) ([]byte, error)
	Decrypt([]byte) (string, error)
}
type Flow struct {
	Skills    *service.Service
	artifacts Artifacts
	git       Git
	secrets   Secrets
}

func New(s *service.Service, artifacts Artifacts, git Git, secrets Secrets) *Flow {
	return &Flow{s, artifacts, git, secrets}
}

type GitInput struct {
	URL, Branch, Subpath string
	AccessToken          *string
}
type Create struct {
	UserID, OrganizationID, Name, Description, Visibility, Slug string
	Grants                                                      []d.Grant
	Archive                                                     []byte
	Git                                                         *GitInput
}

func (f *Flow) store(ctx context.Context, tree skillarchive.Tree) (string, error) {
	body, err := skillarchive.Pack(tree)
	if err != nil {
		return "", d.ErrInvalid
	}
	checksum := skillarchive.Hash(body)
	if err := f.artifacts.Put(ctx, checksum, body); err != nil {
		return "", err
	}
	return checksum, nil
}
func (f *Flow) source(input GitInput, old d.GitSource) (d.GitSource, error) {
	source := d.GitSource{URL: strings.TrimSpace(input.URL), Branch: strings.TrimSpace(input.Branch), Subpath: strings.TrimSpace(input.Subpath)}
	if !strings.Contains(source.URL, "://") && len(strings.Split(source.URL, "/")) == 2 {
		source.URL = "https://github.com/" + source.URL
	}
	if parsed, err := url.Parse(source.URL); err == nil && parsed.Hostname() == "github.com" {
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			parsed.Path = "/" + parts[0] + "/" + strings.TrimSuffix(parts[1], ".git") + ".git"
			source.URL = parsed.String()
		}
	}
	if err := d.ValidateGit(source); err != nil {
		return source, err
	}
	if input.AccessToken != nil {
		if len(*input.AccessToken) > 8192 {
			return source, d.ErrInvalid
		}
		encrypted, err := f.secrets.Encrypt(*input.AccessToken)
		if err != nil {
			return source, err
		}
		source.EncryptedToken = encrypted
	} else if source.URL == old.URL {
		source.EncryptedToken = append([]byte{}, old.EncryptedToken...)
	}
	return source, nil
}
func (f *Flow) Create(ctx context.Context, c Create) (*service.View, error) {
	if err := f.Skills.AuthorizeOrganization(ctx, c.UserID, c.OrganizationID); err != nil {
		return nil, err
	}
	if c.Git != nil && len(c.Archive) != 0 {
		return nil, d.ErrInvalid
	}
	var tree skillarchive.Tree
	var err error
	sourceKind := "native"
	source := d.GitSource{}
	if c.Git != nil {
		if f.git == nil || f.secrets == nil {
			return nil, d.ErrUnavailable
		}
		source, err = f.source(*c.Git, d.GitSource{})
		if err != nil {
			return nil, err
		}
		token, err := f.secrets.Decrypt(source.EncryptedToken)
		if err != nil {
			return nil, err
		}
		tree, source.Commit, err = f.git.Fetch(ctx, source, token)
		if err != nil {
			return nil, err
		}
		sourceKind = "git"
	} else if len(c.Archive) != 0 {
		tree, err = skillarchive.Import(c.Archive)
		if err != nil {
			return nil, d.ErrInvalid
		}
		sourceKind = "upload"
	} else {
		slug := c.Slug
		if slug == "" {
			if d.IsSlug(c.Name) {
				slug = c.Name
			} else {
				slug = "skill-" + uuid.NewString()
			}
		}
		description := c.Description
		if strings.TrimSpace(description) == "" {
			description = c.Name
		}
		tree, err = skillarchive.Initial(slug, description)
		if err != nil {
			return nil, d.ErrInvalid
		}
	}
	if header, err := skillarchive.Validate(tree); err == nil {
		if strings.TrimSpace(c.Name) == "" {
			c.Name = header.Name
		}
		if c.Description == "" {
			c.Description = header.Description
		}
	}
	checksum, err := f.store(ctx, tree)
	if err != nil {
		return nil, err
	}
	return f.Skills.Create(ctx, service.CreateCommand{OwnerUserID: c.UserID, OrganizationID: c.OrganizationID, Name: c.Name, Description: c.Description, Visibility: c.Visibility, Grants: c.Grants, DraftChecksum: checksum, SourceKind: sourceKind, Source: source})
}
func (f *Flow) Tree(ctx context.Context, id, user string, version int64) (*service.View, skillarchive.Tree, error) {
	if version < 0 {
		return nil, nil, d.ErrInvalid
	}
	v, err := f.Skills.Get(ctx, id, user)
	if err != nil {
		return nil, nil, err
	}
	checksum := v.DraftChecksum
	if version > 0 {
		published, err := f.Skills.Version(ctx, id, user, version)
		if err != nil {
			return nil, nil, err
		}
		checksum = published.Checksum
	}
	if checksum == "" {
		return v, skillarchive.Tree{}, nil
	}
	body, err := f.artifacts.Get(ctx, checksum)
	if err != nil {
		return nil, nil, err
	}
	tree, err := skillarchive.Unpack(body)
	if err != nil {
		return nil, nil, d.ErrInvalid
	}
	return v, tree, nil
}
func (f *Flow) WriteFile(ctx context.Context, id, user, path string, expected int64, content []byte, executable bool) (*service.View, error) {
	if !skillarchive.Path(path) {
		return nil, d.ErrInvalid
	}
	if _, err := f.Skills.Writable(ctx, id, user, expected, false); err != nil {
		return nil, err
	}
	_, tree, err := f.Tree(ctx, id, user, 0)
	if err != nil {
		return nil, err
	}
	tree[path] = skillarchive.File{Content: content, Executable: executable}
	checksum, err := f.store(ctx, tree)
	if err != nil {
		return nil, err
	}
	return f.Skills.SaveDraft(ctx, id, user, expected, checksum)
}
func (f *Flow) DeleteFile(ctx context.Context, id, user, path string, expected int64) (*service.View, error) {
	if !skillarchive.Path(path) {
		return nil, d.ErrInvalid
	}
	if _, err := f.Skills.Writable(ctx, id, user, expected, false); err != nil {
		return nil, err
	}
	_, tree, err := f.Tree(ctx, id, user, 0)
	if err != nil {
		return nil, err
	}
	if _, ok := tree[path]; !ok {
		return nil, d.ErrNotFound
	}
	delete(tree, path)
	checksum, err := f.store(ctx, tree)
	if err != nil {
		return nil, err
	}
	return f.Skills.SaveDraft(ctx, id, user, expected, checksum)
}
func (f *Flow) UploadDraft(ctx context.Context, id, user string, expected int64, body []byte) (*service.View, error) {
	if _, err := f.Skills.Writable(ctx, id, user, expected, false); err != nil {
		return nil, err
	}
	tree, err := skillarchive.Import(body)
	if err != nil {
		return nil, d.ErrInvalid
	}
	checksum, err := f.store(ctx, tree)
	if err != nil {
		return nil, err
	}
	return f.Skills.SaveDraft(ctx, id, user, expected, checksum)
}
func (f *Flow) DiscardDraft(ctx context.Context, id, user string, expected int64) (*service.View, error) {
	v, err := f.Skills.Writable(ctx, id, user, expected, false)
	if err != nil {
		return nil, err
	}
	if v.PublishedVersion > 0 {
		return f.Skills.SaveDraft(ctx, id, user, expected, v.Checksum)
	}
	slug := v.Name
	if !d.IsSlug(slug) {
		slug = "skill-" + v.ID
	}
	description := v.Description
	if strings.TrimSpace(description) == "" {
		description = v.Name
	}
	tree, err := skillarchive.Initial(slug, description)
	if err != nil {
		return nil, d.ErrInvalid
	}
	checksum, err := f.store(ctx, tree)
	if err != nil {
		return nil, err
	}
	return f.Skills.SaveDraft(ctx, id, user, expected, checksum)
}
func (f *Flow) Publish(ctx context.Context, id, user string, expected int64) (*service.View, error) {
	v, err := f.Skills.Writable(ctx, id, user, expected, false)
	if err != nil {
		return nil, err
	}
	_, tree, err := f.Tree(ctx, id, user, 0)
	if err != nil {
		return nil, err
	}
	header, err := skillarchive.Validate(tree)
	if err != nil {
		return nil, d.ErrInvalid
	}
	checksum, err := f.store(ctx, tree)
	if err != nil {
		return nil, err
	}
	return f.Skills.Publish(ctx, id, user, expected, v.DraftChecksum, checksum, header.Name, header.Description)
}
func (f *Flow) UpdateSource(ctx context.Context, id, user string, expected int64, input GitInput) (*service.View, error) {
	v, err := f.Skills.Writable(ctx, id, user, expected, true)
	if err != nil {
		return nil, err
	}
	source, err := f.source(input, v.Source)
	if err != nil {
		return nil, err
	}
	return f.Skills.SaveSource(ctx, id, user, expected, source)
}
func (f *Flow) Sync(ctx context.Context, id, user string, expected int64) (*service.View, error) {
	v, err := f.Skills.Writable(ctx, id, user, expected, false)
	if err != nil {
		return nil, err
	}
	if v.SourceKind != "git" || f.git == nil || f.secrets == nil {
		return nil, d.ErrInvalid
	}
	token, err := f.secrets.Decrypt(v.Source.EncryptedToken)
	if err != nil {
		return nil, err
	}
	tree, commit, err := f.git.Fetch(ctx, v.Source, token)
	if err != nil {
		return nil, err
	}
	checksum, err := f.store(ctx, tree)
	if err != nil {
		return nil, err
	}
	return f.Skills.SaveSyncedDraft(ctx, id, user, expected, checksum, commit)
}
func (f *Flow) Package(ctx context.Context, id, user string, version int64) (*d.Version, []byte, error) {
	v, err := f.Skills.Version(ctx, id, user, version)
	if err != nil {
		return nil, nil, err
	}
	body, err := f.artifacts.Get(ctx, v.Checksum)
	return v, body, err
}

// SelectedVersion returns a published, currently authorized package to configuration confirmation.
func (f *Flow) SelectedVersion(ctx context.Context, id, user, org string, version int64) (*d.Version, error) {
	v, err := f.Skills.SelectedVersion(ctx, id, user, org, version)
	if err != nil {
		return nil, err
	}
	body, err := f.artifacts.Get(ctx, v.Checksum)
	if err != nil {
		return nil, err
	}
	tree, err := skillarchive.Unpack(body)
	if err != nil {
		return nil, d.ErrInvalid
	}
	header, err := skillarchive.Validate(tree)
	if err != nil {
		return nil, d.ErrInvalid
	}
	if v.Slug != "" && v.Slug != header.Name {
		return nil, d.ErrInvalid
	}
	v.Slug = header.Name
	v.Description = header.Description
	return v, nil
}
func (f *Flow) ReadableVersion(ctx context.Context, id, user, org string, version int64) (*d.Version, error) {
	return f.Skills.ReadableVersion(ctx, id, user, org, version)
}
