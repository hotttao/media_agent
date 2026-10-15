package agent_metadata_integration

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"gorm.io/gorm"
	"io"
	cfg "media_agent/hertz_gen/config"
	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/hertz_infra/authn/resourceaccess"
	"media_agent/hertz_infra/messagebus"
	"media_agent/hertz_infra/metadataartifact"
	gwDal "media_agent/media_agent_gateway/biz/dal"
	gwRepo "media_agent/media_agent_gateway/biz/dal/repo/agent_instance"
	gwHandler "media_agent/media_agent_gateway/biz/handler/agent_metadata"
	gwMiddleware "media_agent/media_agent_gateway/biz/middleware"
	gwRouter "media_agent/media_agent_gateway/biz/router/agent_metadata"
	client "media_agent/media_agent_gateway/biz/shared/client"
	gwFlow "media_agent/media_agent_gateway/biz/workflow"
	ctrlDal "media_agent/media_control/biz/dal"
	ctrlRepo "media_agent/media_control/biz/dal/repo/agent_configuration"
	promptRepo "media_agent/media_control/biz/dal/repo/prompt"
	skillRepo "media_agent/media_control/biz/dal/repo/skill"
	ctrlWorkspaceRepo "media_agent/media_control/biz/dal/repo/workspace"
	ctrlDomain "media_agent/media_control/biz/domain/agent_configuration"
	workspaceDomain "media_agent/media_control/biz/domain/workspace"
	ctrlHandler "media_agent/media_control/biz/handler/agent_metadata"
	ctrlMiddleware "media_agent/media_control/biz/middleware"
	ctrlRouter "media_agent/media_control/biz/router/agent_metadata"
	ctrlService "media_agent/media_control/biz/service/agent_configuration"
	promptService "media_agent/media_control/biz/service/prompt"
	skillService "media_agent/media_control/biz/service/skill"
	ctrlWorkspace "media_agent/media_control/biz/service/workspace"
	"media_agent/media_control/biz/shared/secret"
	ctrlFlow "media_agent/media_control/biz/workflow/metadata_update"
	skillFlow "media_agent/media_control/biz/workflow/skill_content"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

type teams struct{}

type catalogAccess struct{}

func (catalogAccess) Resolve(_ context.Context, user, org string) (resourceaccess.Facts, error) {
	return resourceaccess.Facts{Member: user == "user" && org == "org"}, nil
}
func (catalogAccess) Team(context.Context, string, string) (resourceaccess.Team, error) {
	return resourceaccess.Team{}, errors.New("no fixture grants")
}

func (teams) ListTeams(context.Context, string) ([]string, error) { return []string{"team"}, nil }
func (teams) Authorize(_ context.Context, user, team, permission string) (workspaceDomain.TeamAuthorization, error) {
	return workspaceDomain.TeamAuthorization{Allowed: user == "user" && team == "team", OrganizationID: "org", TeamID: "team"}, nil
}

type credentials struct{}

func (credentials) VerifyAgentCredential(_ context.Context, token string) (internaljwt.AgentPrincipal, error) {
	if token != "agent-test-token" {
		return internaljwt.AgentPrincipal{}, client.ErrAgentCredentialRejected
	}
	return internaljwt.AgentPrincipal{UserID: "user", ActorID: "Agent:config", OrganizationID: "org", CredentialID: "credential"}, nil
}

type authority struct{}

func (authority) ResolveAgentIdentity(context.Context, string) (client.AgentIdentity, error) {
	return client.AgentIdentity{ConfigurationID: "config", ActorID: "Agent:config", WorkspaceID: "workspace", OwnerUserID: "user", OrganizationID: "org", TeamID: "team", State: "running", OwnerAuthorized: true}, nil
}

type unavailablePublisher struct{}

func (unavailablePublisher) Publish(context.Context, string, string, []byte) error {
	return errors.New("test broker unavailable")
}

type roster struct{}

func (roster) ListMetadataInstances(context.Context, string, string) ([]ctrlDomain.Instance, bool, error) {
	return []ctrlDomain.Instance{{WorkspaceID: "workspace", AgentConfigID: "config", ReplicaID: "0", IncarnationID: "boot0", Admission: "active"}, {WorkspaceID: "workspace", AgentConfigID: "config", ReplicaID: "1", IncarnationID: "boot1", Admission: "active"}}, true, nil
}
func mount(h *server.Hertz) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := []ut.Header{}
		for key, values := range r.Header {
			for _, value := range values {
				headers = append(headers, ut.Header{Key: key, Value: value})
			}
		}
		recorder := ut.PerformRequest(h.Engine, r.Method, r.URL.RequestURI(), &ut.Body{Body: r.Body, Len: int(r.ContentLength)}, headers...)
		response := recorder.Result()
		response.Header.VisitAll(func(key, value []byte) { w.Header().Add(string(key), string(value)) })
		w.WriteHeader(response.StatusCode())
		w.Write(response.Body())
	}))
}
func call(t *testing.T, base, method, path string, body any, expected int, out any) {
	t.Helper()
	encoded, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, base+path, bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Control-Service-Token", "internal-test-token")
	req.Header.Set("X-Control-Acting-User-ID", "user")
	req.Header.Set("Authorization", "Bearer agent-test-token")
	instanceCredential := "instance-0-test-token"
	if value, ok := body.(client.MetadataResult); ok {
		instanceCredential = "instance-" + value.ReplicaID + "-test-token"
	}
	req.Header.Set("X-Agent-Instance-Credential", instanceCredential)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	content, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != expected {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, path, resp.StatusCode, expected, content)
	}
	if out != nil && json.Unmarshal(content, out) != nil {
		t.Fatal("invalid response JSON")
	}
}
func TestConfirmedMetadataPipeline(t *testing.T) {
	dsn := os.Getenv("AGENT_METADATA_TEST_DSN")
	broker := os.Getenv("MEDIA_AGENTS_TEST_RABBITMQ_URL")
	if dsn == "" || broker == "" {
		t.Skip("real PostgreSQL/RabbitMQ environment required")
	}
	admin, err := ctrlDal.NewDB(&cfg.DatabaseConfig{Enabled: true, Driver: "postgres", Dsn: dsn})
	must(t, err)
	makeDB := func(prefix string) (*gorm.DB, *cfg.DatabaseConfig) {
		name := prefix + fmt.Sprint(time.Now().UnixNano())
		must(t, admin.Exec("CREATE DATABASE "+name).Error)
		parsed, err := url.Parse(dsn)
		must(t, err)
		parsed.Path = "/" + name
		config := &cfg.DatabaseConfig{Enabled: true, Driver: "postgres", Dsn: parsed.String(), MigrateOnStart: true}
		db, err := ctrlDal.NewDB(config)
		must(t, err)
		t.Cleanup(func() {
			sqlDB, err := db.DB()
			if err == nil {
				_ = sqlDB.Close()
			}
			_ = admin.Exec("DROP DATABASE " + name + " WITH (FORCE)").Error
		})
		return db, config
	}
	db, config := makeDB("metadata_control_")
	gatewayDB, gatewayConfig := makeDB("metadata_gateway_")
	must(t, ctrlDal.Migrate("../../../media_control/migrations", config))
	must(t, gwDal.Migrate("../../../media_agent_gateway/migrations", gatewayConfig))
	ctx := context.Background()
	seed(t, db, gatewayDB)
	artifacts, err := metadataartifact.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository := ctrlRepo.New(db)
	service := ctrlService.New(repository, ctrlDal.NewTxRunner(db))
	workspace := ctrlWorkspace.New(ctrlWorkspaceRepo.New(db), ctrlDal.NewTxRunner(db), secret.New("integration-secret"), teams{})
	prompts := promptService.New(promptRepo.New(db), ctrlDal.NewTxRunner(db), catalogAccess{})
	skills := skillFlow.New(skillService.New(skillRepo.New(db), ctrlDal.NewTxRunner(db), catalogAccess{}), artifacts, nil, nil)
	ctrlHandler.Configure(ctrlFlow.New(service, workspace, artifacts, roster{}).WithPrompts(prompts).WithSkills(skills))
	ctrlMiddleware.Configure("integration-secret", "internal-test-token")
	ctrlMiddleware.ConfigureMetadataGateway("metadata-gateway-test")
	controlHertz := server.New()
	ctrlRouter.Register(controlHertz)
	control := mount(controlHertz)
	defer control.Close()
	metadataGateway := gwFlow.NewAgentMetadata(credentials{}, authority{}, gwRepo.New(gatewayDB), client.NewMetadataControlClient(control.URL, "internal-test-token", "metadata-gateway-test"))
	gwHandler.Configure(metadataGateway)
	gwMiddleware.Configure("integration-secret", "internal-test-token")
	gatewayHertz := server.New()
	gwRouter.Register(gatewayHertz)
	gateway := mount(gatewayHertz)
	defer gateway.Close()
	// A mutable Catalog edit creates history, never an update notification.
	must(t, db.Exec("INSERT INTO prompts(id,organization_id,owner_user_id,name,content,status,version,created_at,updated_at) VALUES('prompt','org','user','Prompt','old','active',1,now(),now())").Error)
	must(t, db.Exec("UPDATE prompts SET content='new',version=2 WHERE id='prompt'").Error)
	var count int64
	must(t, db.Table("metadata_notification_outbox").Count(&count).Error)
	if count != 0 {
		t.Fatal("catalog edit notified Agents")
	}

	oldSkill, oldHash := zippedSkill(t, "old skill")
	newSkill, newHash := zippedSkill(t, "new skill")
	for hash, content := range map[string][]byte{oldHash: oldSkill, newHash: newSkill} {
		call(t, control.URL, "PUT", "/api/v1/workspaces/workspace/agent-metadata/artifacts/"+hash, map[string]any{"content": content}, 201, nil)
	}
	must(t, db.Exec("INSERT INTO skills(id,organization_id,owner_user_id,name,content_path,checksum,status,version,published_version,draft_checksum,created_at,updated_at) VALUES('skill','org','user','Skill',?,?,'active',1,1,?,now(),now())", "sha256/"+oldHash, oldHash, oldHash).Error)
	must(t, db.Exec("UPDATE skills SET content_path=?,checksum=?,draft_checksum=?,published_version=2,version=2 WHERE id='skill'", "sha256/"+newHash, newHash, newHash).Error)
	for version, hash := range map[int]string{1: oldHash, 2: newHash} {
		must(t, db.Exec("INSERT INTO skill_versions(skill_id,version,checksum,slug,description,published_by) VALUES('skill',?,?,'skill','Metadata integration skill','user')", version, hash).Error)
	}
	must(t, db.Exec("INSERT INTO providers(id,name,provider_type,endpoint,capabilities,status,version,created_at,updated_at) VALUES('provider','Provider','vector','https://example.invalid','{}','active',1,now(),now())").Error)
	must(t, db.Exec("INSERT INTO credential_references(id,workspace_id,owner_user_id,name,provider_id,secret_ref,status,version,created_at,updated_at) VALUES('credential-ref','workspace','user','Credential','provider','secret-reference','active',1,now(),now())").Error)
	must(t, db.Exec("INSERT INTO memory_stores(id,workspace_id,owner_user_id,name,store_type,configuration,credential_reference_id,status,version,created_at,updated_at) VALUES('memory','workspace','user','Memory','vector','{\"api_key\":\"must-stay-private\"}','credential-ref','active',1,now(),now())").Error)
	request := map[string]any{"agent_config_id": "config", "expected_version": 1, "idempotency_key": "choice-one", "resources": []map[string]any{{"kind": "media_control.configured_prompt", "id": "prompt", "version": 1}, {"kind": "media_control.skill", "id": "skill", "version": 1}, {"kind": "media_control.memory_store", "id": "memory", "version": 1}}}
	var first client.MetadataContext
	call(t, control.URL, "POST", "/api/v1/workspaces/workspace/agent-metadata:confirm", request, 200, &first)
	if first.ConfiguredPrompt != "old" || first.SelectionRevision != 2 {
		t.Fatalf("wrong frozen selection: %+v", first)
	}
	if len(first.Resources) != 3 {
		t.Fatal("missing selected metadata")
	}
	for _, resource := range first.Resources {
		if resource.Kind == "media_control.memory_store" {
			if !bytes.Contains([]byte(resource.Content), []byte("credential-ref")) || bytes.Contains([]byte(resource.Content), []byte("must-stay-private")) {
				t.Fatal("memory descriptor lost reference or leaked secret")
			}
		}
	}
	var duplicate client.MetadataContext
	call(t, control.URL, "POST", "/api/v1/workspaces/workspace/agent-metadata:confirm", request, 200, &duplicate)
	if duplicate.TargetContextVersion != first.TargetContextVersion {
		t.Fatal("idempotency generated another release")
	}
	request["resources"] = []map[string]any{{"kind": "media_control.configured_prompt", "id": "prompt", "version": 2}, {"kind": "media_control.skill", "id": "skill", "version": 2}}
	call(t, control.URL, "POST", "/api/v1/workspaces/workspace/agent-metadata:confirm", request, 409, nil)
	request["idempotency_key"] = "choice-two"
	request["expected_version"] = 2
	var second client.MetadataContext
	call(t, control.URL, "POST", "/api/v1/workspaces/workspace/agent-metadata:confirm", request, 200, &second)
	var historical client.MetadataContext
	call(t, gateway.URL, "GET", "/api/agent/v1/runtime-context?workspace_id=workspace&agent_config_id=config&replica_id=0&incarnation_id=boot0&for_session=true&target_context_version="+first.TargetContextVersion, nil, 200, &historical)
	if historical.ConfiguredPrompt != "old" || historical.PlatformPrompt == "" || bytes.Contains([]byte(historical.PlatformPrompt), []byte("old")) {
		t.Fatal("old Session lost its version or Gateway merged user prompt")
	}
	var superseded client.MetadataContext
	call(t, gateway.URL, "GET", "/api/agent/v1/runtime-context?workspace_id=workspace&agent_config_id=config&replica_id=0&incarnation_id=boot0&target_context_version="+first.TargetContextVersion, nil, 200, &superseded)
	if superseded.Status != "superseded" || superseded.TargetContextVersion != second.TargetContextVersion {
		t.Fatal("old notice did not resolve current confirmed selection")
	}
	// Execute the actual Python consumer/filesystem adapter against both Go APIs.
	publisher, err := messagebus.NewTopicPublisher(broker, "test.metadata.pipeline")
	must(t, err)
	defer publisher.Close()
	// A publication failure preserves committed selections and the original event IDs.
	failedRelay := ctrlFlow.NewRelay(repository, unavailablePublisher{}, ctrlDal.NewTxRunner(db))
	must(t, failedRelay.Once(ctx))
	must(t, db.Table("metadata_notification_outbox").Where("delivered_at IS NULL AND attempts=1").Count(&count).Error)
	if count != 2 {
		t.Fatal("failed publication lost a durable notice")
	}
	must(t, db.Exec("UPDATE metadata_notification_outbox SET available_at=now() WHERE delivered_at IS NULL").Error)
	relay := ctrlFlow.NewRelay(repository, publisher, ctrlDal.NewTxRunner(db))
	script := filepath.Join("..", "..", "..", "media_agents", "scripts", "metadata_pipeline_check.py")
	python := filepath.Join("..", "..", "..", "media_agents", ".venv", "bin", "python")
	command := exec.Command(python, script)
	readyFile := filepath.Join(t.TempDir(), "ready")
	command.Env = append(os.Environ(), "TEST_METADATA_READY="+readyFile, "TEST_GATEWAY_URL="+gateway.URL, "TEST_METADATA_EXCHANGE=test.metadata.pipeline", "TEST_METADATA_CONTEXT="+second.TargetContextVersion, "TEST_PREVIOUS_CONTEXT="+first.TargetContextVersion, "TEST_METADATA_ROOT="+t.TempDir())
	command.Dir = "."
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	must(t, command.Start())
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := os.Stat(readyFile); err == nil {
			break
		}
		if time.Now().After(deadline) {
			command.Process.Kill()
			t.Fatalf("Python subscribers not ready: %s", output.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	must(t, relay.Once(ctx))
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Python pipeline: %v %s", err, output.String())
		}
		t.Log(output.String())
	case <-time.After(30 * time.Second):
		command.Process.Kill()
		t.Fatal("Python pipeline timed out")
	}
	// A late failure cannot override success for the same instance/selection.
	for _, replica := range []string{"0", "1"} {
		result := client.MetadataResult{ReportID: "late-failure-" + replica, WorkspaceID: "workspace", AgentConfigID: "config", SelectionRevision: second.SelectionRevision, TargetContextVersion: second.TargetContextVersion, ReplicaID: replica, IncarnationID: "boot" + replica, Status: "failed", ErrorCode: "io_error"}
		call(t, gateway.URL, "POST", "/api/agent/v1/metadata-results", result, 200, nil)
	}
	// Stable Agent credentials cannot impersonate another replica or a previous incarnation.
	call(t, gateway.URL, "GET", "/api/agent/v1/runtime-context?workspace_id=workspace&agent_config_id=config&replica_id=1&incarnation_id=boot1", nil, 403, nil)
	stale := client.MetadataResult{ReportID: "old-incarnation", WorkspaceID: "workspace", AgentConfigID: "config", SelectionRevision: second.SelectionRevision, TargetContextVersion: second.TargetContextVersion, ReplicaID: "0", IncarnationID: "old-boot", Status: "failed", ErrorCode: "io_error"}
	call(t, gateway.URL, "POST", "/api/agent/v1/metadata-results", stale, 403, nil)
	conflict := client.MetadataResult{ReportID: "late-failure-0", WorkspaceID: "workspace", AgentConfigID: "config", SelectionRevision: second.SelectionRevision, TargetContextVersion: second.TargetContextVersion, ReplicaID: "0", IncarnationID: "boot0", Status: "failed", ErrorCode: "other_error"}
	call(t, gateway.URL, "POST", "/api/agent/v1/metadata-results", conflict, 409, nil)
	var progress struct{ Status string }
	call(t, control.URL, "GET", "/api/v1/workspaces/workspace/agent-metadata/progress", nil, 200, &progress)
	if progress.Status != "applied" {
		t.Fatalf("late failure revoked success: %s", progress.Status)
	}
	request["expected_version"] = 3
	request["idempotency_key"] = "explicit-rollback"
	request["resources"] = []map[string]any{{"kind": "media_control.configured_prompt", "id": "prompt", "version": 1}, {"kind": "media_control.skill", "id": "skill", "version": 1}}
	var rollback client.MetadataContext
	call(t, control.URL, "POST", "/api/v1/workspaces/workspace/agent-metadata:confirm", request, 200, &rollback)
	if rollback.SelectionRevision != 4 || rollback.ConfiguredPrompt != "old" || rollback.TargetContextVersion == first.TargetContextVersion {
		t.Fatal("explicit rollback did not create a new selection")
	}
	call(t, control.URL, "GET", "/api/v1/workspaces/workspace/agent-metadata/progress", nil, 200, &progress)
	if progress.Status != "updating" {
		t.Fatal("historical applied results completed a new rollback selection")
	}
	pending, err := service.Progress(ctx, "workspace", "config", nil, true)
	must(t, err)
	if pending.Status == "applied" {
		t.Fatal("empty roster declared all applied")
	}
	pending, err = service.Progress(ctx, "workspace", "config", nil, false)
	must(t, err)
	if pending.Status != "waiting_instances" {
		t.Fatal("incomplete roster declared success")
	}

	// Two confirmations of the same expected version must commit exactly one release.
	competing := make(chan error, 2)
	flow := ctrlFlow.New(service, workspace, artifacts, roster{}).WithPrompts(prompts).WithSkills(skills)
	for _, key := range []string{"competing-a", "competing-b"} {
		go func(key string) {
			_, err := flow.Confirm(ctx, ctrlFlow.Confirm{WorkspaceID: "workspace", AgentConfigID: "config", UserID: "user", IdempotencyKey: key, ExpectedVersion: 4, Resources: []ctrlDomain.ResourceReference{{Kind: "media_control.configured_prompt", ID: "prompt", Version: 2}}})
			competing <- err
		}(key)
	}
	successes, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		err := <-competing
		if err == nil {
			successes++
		} else if errors.Is(err, ctrlDomain.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal("concurrent confirmations committed inconsistent selections")
	}
	// Desired credentials may change, but old and new Sessions use confirmed historical versions.
	desired, err := workspace.Update(ctx, ctrlWorkspace.UpdateCommand{WorkspaceID: "workspace", UserID: "user", Name: "Metadata", AgentType: "codex", APIKey: "test-new-provider-key"})
	must(t, err)
	if desired.AgentConfiguration.Version != 6 {
		t.Fatal("desired configuration did not advance")
	}
	fixed, key, target, err := workspace.RuntimeContextVersion(ctx, "workspace", "user", second.ContextVersion)
	must(t, err)
	if fixed.AgentConfiguration.Version != second.ContextVersion || key != "" || target != second.TargetContextVersion {
		t.Fatal("old Session lost historical configuration")
	}
	current, key, _, err := workspace.RuntimeContextVersion(ctx, "workspace", "user", 0)
	must(t, err)
	if current.AgentConfiguration.Version != 5 || key != "" {
		t.Fatal("unconfirmed desired edit changed new Session context")
	}
}
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func seed(t *testing.T, db *gorm.DB, gatewayDB *gorm.DB) {
	t.Helper()
	must(t, db.Exec("INSERT INTO workspaces(id,organization_id,team_id,created_by,owner_user_id,name,desired_state,version,created_at,updated_at) VALUES('workspace','org','team','user','user','Metadata','running',1,now(),now()) ON CONFLICT DO NOTHING").Error)
	must(t, db.Exec("INSERT INTO agent_configurations(id,workspace_id,agent_id,agent_type,version,created_at,updated_at) VALUES('config','workspace','codex','codex',1,now(),now()) ON CONFLICT DO NOTHING").Error)
	for _, replica := range []string{"0", "1"} {
		must(t, gatewayDB.Exec("INSERT INTO agent_instances(environment_id,workspace_id,replica_id,agent_config_id,incarnation_id,resource_uid,endpoint,admission,credential_hash) VALUES('environment','workspace',?,'config',?,'verified-resource','http://runtime','active',?) ON CONFLICT DO NOTHING", replica, "boot"+replica, hashInstanceCredential(replica)).Error)
	}
}

func zippedSkill(t *testing.T, text string) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	archive := zip.NewWriter(&body)
	entry, err := archive.Create("SKILL.md")
	must(t, err)
	_, err = entry.Write([]byte("---\nname: skill\ndescription: Metadata integration skill\n---\n" + text))
	must(t, err)
	must(t, archive.Close())
	sum := sha256.Sum256(body.Bytes())
	return body.Bytes(), hex.EncodeToString(sum[:])
}

func hashInstanceCredential(replica string) string {
	sum := sha256.Sum256([]byte("instance-" + replica + "-test-token"))
	return hex.EncodeToString(sum[:])
}
