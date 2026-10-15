package agent_product_integration

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"github.com/cloudwego/hertz/pkg/app/server"
	metadataPB "media_agent/hertz_gen/model/media_control/agent_metadata"
	pb "media_agent/hertz_gen/model/media_control/skill"
	"media_agent/hertz_infra/authn/resourceaccess"
	"media_agent/hertz_infra/metadataartifact"
	"media_agent/hertz_infra/skillarchive"
	"media_agent/media_control/biz/dal"
	metadataRepo "media_agent/media_control/biz/dal/repo/agent_configuration"
	skillRepo "media_agent/media_control/biz/dal/repo/skill"
	workspaceRepo "media_agent/media_control/biz/dal/repo/workspace"
	metadataHandler "media_agent/media_control/biz/handler/agent_metadata"
	skillHandler "media_agent/media_control/biz/handler/skill"
	"media_agent/media_control/biz/middleware"
	metadataRouter "media_agent/media_control/biz/router/agent_metadata"
	skillRouter "media_agent/media_control/biz/router/skill"
	metadataService "media_agent/media_control/biz/service/agent_configuration"
	skillService "media_agent/media_control/biz/service/skill"
	workspaceService "media_agent/media_control/biz/service/workspace"
	authClient "media_agent/media_control/biz/shared/client/mediaauth"
	"media_agent/media_control/biz/shared/client/skillgit"
	"media_agent/media_control/biz/shared/secret"
	metadataFlow "media_agent/media_control/biz/workflow/metadata_update"
	skillFlow "media_agent/media_control/biz/workflow/skill_content"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func entry(text string) []byte {
	return []byte("---\nname: demo\ndescription: Example skill\n---\n" + text)
}
func archive(t *testing.T, tarball bool, text string) []byte {
	t.Helper()
	var out bytes.Buffer
	if tarball {
		gz := gzip.NewWriter(&out)
		writer := tar.NewWriter(gz)
		body := entry(text)
		must(t, writer.WriteHeader(&tar.Header{Name: "./demo/SKILL.md", Size: int64(len(body)), Mode: 0644, Typeflag: tar.TypeReg}))
		_, err := writer.Write(body)
		must(t, err)
		must(t, writer.Close())
		must(t, gz.Close())
	} else {
		writer := zip.NewWriter(&out)
		file, err := writer.Create("demo/SKILL.md")
		must(t, err)
		_, err = file.Write(entry(text))
		must(t, err)
		must(t, writer.Close())
	}
	return out.Bytes()
}
func gitRepository(t *testing.T) (string, func()) {
	t.Helper()
	git, err := exec.LookPath("git")
	must(t, err)
	work := filepath.Join(t.TempDir(), "work")
	must(t, os.MkdirAll(filepath.Join(work, "skills/demo"), 0700))
	run := func(args ...string) {
		t.Helper()
		command := exec.Command(git, args...)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("fixture git error: %v: %s", err, output)
		}
	}
	run("init", "--quiet", "--initial-branch=main", work)
	must(t, os.WriteFile(filepath.Join(work, "skills/demo/SKILL.md"), entry("initial Git content"), 0600))
	must(t, os.WriteFile(filepath.Join(work, "README.md"), []byte("outside selected directory"), 0600))
	run("-C", work, "add", ".")
	run("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "-C", work, "commit", "--quiet", "-m", "initial")
	root := t.TempDir()
	bare := filepath.Join(root, "demo.git")
	run("clone", "--quiet", "--bare", work, bare)
	backend := &cgi.Handler{Path: git, Args: []string{"http-backend"}, Root: root, Env: []string{"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, password, ok := r.BasicAuth()
		if !ok || password != "git-pat-fixture" {
			w.Header().Set("WWW-Authenticate", `Basic realm="skills"`)
			w.WriteHeader(401)
			return
		}
		backend.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	update := func() {
		must(t, os.WriteFile(filepath.Join(work, "skills/demo/SKILL.md"), entry("updated Git content"), 0600))
		run("-C", work, "add", ".")
		run("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "-C", work, "commit", "--quiet", "-m", "updated")
		run("-C", work, "push", "--quiet", bare, "main")
	}
	return server.URL + "/demo.git", update
}
func TestSkillContentPublicationAndGitImport(t *testing.T) {
	db, _ := database(t)
	auth := identityFacts(t)
	defer auth.Close()
	artifacts, err := metadataartifact.New(t.TempDir())
	must(t, err)
	skills := skillService.New(skillRepo.New(db), dal.NewTxRunner(db), resourceaccess.New(auth.URL, "auth-internal", nil))
	content := skillFlow.New(skills, artifacts, skillgit.Client{}, secret.New("skill-test-key"))
	workspace := workspaceService.New(workspaceRepo.New(db), dal.NewTxRunner(db), secret.New("workspace-test-key"), authClient.New(auth.URL, "auth-internal", nil))
	flow := metadataFlow.New(metadataService.New(metadataRepo.New(db), dal.NewTxRunner(db)), workspace, artifacts, nil).WithSkills(content)
	skillHandler.Configure(content)
	metadataHandler.Configure(flow)
	middleware.Configure("", "product-internal")
	h := server.New()
	skillRouter.Register(h)
	metadataRouter.Register(h)
	httpServer := mount(h)
	defer httpServer.Close()
	base := httpServer.URL
	must(t, db.Exec("INSERT INTO workspaces(id,owner_user_id,created_by,organization_id,team_id,name,desired_state,version,created_at,updated_at) VALUES('workspace','owner','owner','org','workspace-team','Workspace','running',1,now(),now())").Error)
	must(t, db.Exec("INSERT INTO agent_configurations(id,workspace_id,agent_id,agent_type,version,created_at,updated_at) VALUES('config','workspace','codex','codex',1,now(),now())").Error)
	var value pb.SkillResponse
	call(t, base, "owner", "POST", "/api/v1/skills", map[string]any{"organization_id": "org", "name": "Example", "content": archive(t, false, "uploaded"), "visibility": "team", "grants": []map[string]string{{"team_id": "editor-team", "permission": "editor"}, {"team_id": "viewer-team", "permission": "viewer"}}}, 201, &value)
	id := value.Id
	path := "/api/v1/skills/" + id
	if value.PublishedVersion != 0 || !value.HasDraft || value.Description != "Example skill" {
		t.Fatal(value.String())
	}
	var files pb.SkillFilesResponse
	call(t, base, "viewer", "GET", path+"/files", nil, 200, &files)
	if len(files.Files) != 1 || files.Files[0].Path != "SKILL.md" {
		t.Fatal(files.String())
	}
	var selected metadataPB.MetadataContext
	confirm := func(version int64, key string, resources []map[string]any, status int) {
		call(t, base, "editor", "POST", "/api/v1/workspaces/workspace/agent-metadata:confirm", map[string]any{"agent_config_id": "config", "expected_version": version, "idempotency_key": key, "prompt": map[string]string{"mode": "none"}, "resources": resources}, status, &selected)
	}
	reference := func(version int64) []map[string]any {
		return []map[string]any{{"kind": "media_control.skill", "id": id, "version": version}}
	}
	confirm(1, "unpublished", reference(1), 403)
	call(t, base, "viewer", "PUT", path+"/file", map[string]any{"path": "scripts/run.sh", "content": []byte("bad"), "expected_version": 1}, 403, nil)
	call(t, base, "editor", "PUT", path+"/file", map[string]any{"path": "scripts/run.sh", "content": []byte("#!/bin/sh\nprintf 'skill works\\n'\n"), "executable": true, "expected_version": 1}, 200, &value)
	call(t, base, "editor", "POST", path+"/publish", map[string]any{"expected_version": value.Version}, 200, &value)
	if value.PublishedVersion != 1 || value.HasDraft {
		t.Fatal(value.String())
	}
	firstChecksum := value.Checksum
	var outboxCount int64
	must(t, db.Table("metadata_notification_outbox").Count(&outboxCount).Error)
	if outboxCount != 0 {
		t.Fatal("catalog publication updated Agent automatically")
	}
	var pkg pb.SkillPackageResponse
	call(t, base, "viewer", "GET", path+"/versions/1/package", nil, 200, &pkg)
	unpacked, err := skillarchive.Unpack(pkg.Content)
	must(t, err)
	if !unpacked["scripts/run.sh"].Executable || pkg.Checksum != firstChecksum {
		t.Fatal("published scripts not preserved")
	}
	confirm(1, "first", reference(1), 200)
	if selected.Resources[0].Checksum != firstChecksum {
		t.Fatal(selected.String())
	}
	call(t, base, "editor", "PUT", path+"/file", map[string]any{"path": "SKILL.md", "content": entry("unpublished draft"), "expected_version": value.Version}, 200, &value)
	call(t, base, "viewer", "GET", path+"/versions/1/package", nil, 200, &pkg)
	if pkg.Checksum != firstChecksum {
		t.Fatal("draft changed published artifact")
	}
	call(t, base, "editor", "DELETE", path+"/file?path=SKILL.md&expected_version="+strconv.FormatInt(value.Version, 10), nil, 200, &value)
	call(t, base, "editor", "POST", path+"/publish", map[string]any{"expected_version": value.Version}, 400, nil)
	call(t, base, "editor", "PUT", path+"/draft", map[string]any{"content": archive(t, true, "tar imported"), "expected_version": value.Version}, 200, &value)
	call(t, base, "editor", "POST", path+"/publish", map[string]any{"expected_version": value.Version}, 200, &value)
	if value.PublishedVersion != 2 || value.Checksum == firstChecksum {
		t.Fatal(value.String())
	}
	var current metadataPB.MetadataSelection
	call(t, base, "viewer", "GET", "/api/v1/workspaces/workspace/agent-metadata", nil, 200, &current)
	if current.Resources[0].Version != 1 {
		t.Fatal("new publication advanced existing selection")
	}
	confirm(2, "second", reference(2), 200)
	var versions pb.SkillVersionsResponse
	call(t, base, "viewer", "GET", path+"/versions", nil, 200, &versions)
	if len(versions.Versions) != 2 {
		t.Fatal(versions.String())
	}
	call(t, base, "owner", "POST", path+"/status", map[string]any{"status": "disabled", "expected_version": value.Version}, 200, &value)
	confirm(3, "disabled", reference(2), 422)
	call(t, base, "viewer", "GET", path+"/versions/1/package", nil, 200, nil)
	call(t, base, "outsider", "GET", path+"/files", nil, 404, nil)
	call(t, base, "editor", "PUT", path+"/file", map[string]any{"path": "../outside", "content": []byte("x"), "expected_version": value.Version}, 400, nil)
	call(t, base, "owner", "POST", path+"/status", map[string]any{"status": "active", "expected_version": value.Version}, 200, &value)
	call(t, base, "editor", "PUT", path+"/file", map[string]any{"path": "extra.txt", "content": []byte("changed"), "expected_version": value.Version}, 200, &value)
	call(t, base, "editor", "POST", path+"/draft:discard", map[string]any{"expected_version": value.Version}, 200, &value)
	if value.HasDraft {
		t.Fatal("discard did not restore published content")
	}
	// Real local smart Git HTTP exercises private credentials, branch and subdirectory.
	gitURL, updateGit := gitRepository(t)
	var response map[string]any
	call(t, base, "owner", "POST", "/api/v1/skills/import-git", map[string]any{"organization_id": "org", "name": "Git example", "git": map[string]string{"url": gitURL, "branch": "main", "subpath": "skills/demo", "access_token": "git-pat-fixture"}}, 201, &response)
	encoded, err := json.Marshal(response)
	must(t, err)
	if bytes.Contains(encoded, []byte("git-pat-fixture")) {
		t.Fatal("Git token leaked in response")
	}
	must(t, json.Unmarshal(encoded, &value))
	gitPath := "/api/v1/skills/" + value.Id
	if value.Source == nil || !value.Source.CredentialConfigured || value.Source.Commit == "" {
		t.Fatal(value.String())
	}
	oldCommit := value.Source.Commit
	call(t, base, "owner", "GET", gitPath+"/files", nil, 200, &files)
	if len(files.Files) != 1 || files.Files[0].Path != "SKILL.md" {
		t.Fatal("Git subdirectory not respected", files.String())
	}
	var cipherRow struct{ EncryptedGitToken []byte }
	must(t, db.Table("skills").Where("id = ?", value.Id).Take(&cipherRow).Error)
	if len(cipherRow.EncryptedGitToken) == 0 || bytes.Contains(cipherRow.EncryptedGitToken, []byte("git-pat-fixture")) {
		t.Fatal("Git credential not encrypted")
	}
	var leaked int64
	must(t, db.Table("catalog_resource_versions").Where("resource_id = ? AND payload ? 'encrypted_git_token'", value.Id).Count(&leaked).Error)
	if leaked != 0 {
		t.Fatal("credential copied into history")
	}
	updateGit()
	call(t, base, "owner", "POST", gitPath+"/git:sync", map[string]any{"expected_version": value.Version}, 200, &value)
	if value.Source.Commit == oldCommit || value.PublishedVersion != 0 {
		t.Fatal("Git sync did not update draft only")
	}
	var file pb.SkillFileResponse
	call(t, base, "owner", "GET", gitPath+"/file?path=SKILL.md", nil, 200, &file)
	if !bytes.Contains(file.Content, []byte("updated Git content")) {
		t.Fatal("sync content missing")
	}
	call(t, base, "owner", "POST", gitPath+"/publish", map[string]any{"expected_version": value.Version}, 200, &value)
	duplicate := append(reference(2), map[string]any{"kind": "media_control.skill", "id": value.Id, "version": 1})
	// Shared access is required before duplicate Runtime names can be validated.
	call(t, base, "owner", "PUT", gitPath+"/access", map[string]any{"visibility": "team", "grants": []map[string]string{{"team_id": "editor-team", "permission": "viewer"}}, "expected_version": value.Version}, 200, &value)
	confirm(3, "duplicate-names", duplicate, 422)
	call(t, base, "owner", "POST", "/api/v1/skills/import-git", map[string]any{"organization_id": "org", "name": "bad", "git": map[string]string{"url": "file:///etc"}}, 400, nil)
	if _, err := content.SelectedVersion(context.Background(), id, "viewer", "org", 1); err != nil {
		t.Fatal("old published version unavailable", err)
	}
	must(t, db.Table("metadata_notification_outbox").Count(&outboxCount).Error)
	if outboxCount != 2 {
		t.Fatal("unexpected automatic update", outboxCount)
	}
}
