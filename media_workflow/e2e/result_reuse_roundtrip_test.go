//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/google/uuid"
	pgdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	configpb "media_agent/hertz_gen/config"
	"media_agent/hertz_infra/testpostgres"
	materialsDAL "media_agent/media_materials/biz/dal"
	kindrepo "media_agent/media_materials/biz/dal/repo/asset_kind"
	assetrepo "media_agent/media_materials/biz/dal/repo/generated_asset"
	productrepo "media_agent/media_materials/biz/dal/repo/product"
	kinddomain "media_agent/media_materials/biz/domain/asset_kind"
	productdomain "media_agent/media_materials/biz/domain/product"
	assethandler "media_agent/media_materials/biz/handler/generated_asset"
	objecthandler "media_agent/media_materials/biz/handler/library_object"
	assetrouter "media_agent/media_materials/biz/router/generated_asset"
	objectrouter "media_agent/media_materials/biz/router/library_object"
	kindservice "media_agent/media_materials/biz/service/asset_kind"
	assetservice "media_agent/media_materials/biz/service/generated_asset"
	"media_agent/media_materials/biz/shared/objectstorage"
	jobrepo "media_agent/media_workflow/biz/dal/repo/job"
	capability "media_agent/media_workflow/biz/domain/capability"
	domain "media_agent/media_workflow/biz/domain/job"
	template "media_agent/media_workflow/biz/domain/template"
	"media_agent/media_workflow/biz/shared/client"
	"media_agent/media_workflow/biz/workflow/compiler"
	"media_agent/media_workflow/biz/workflow/conductor"
	"media_agent/media_workflow/biz/workflow/executable"
	"media_agent/media_workflow/biz/workflow/finalize_task"
	"media_agent/media_workflow/biz/workflow/render_task"
	"media_agent/media_workflow/biz/workflow/reuse_task"
)

// Actual Check/Finalize/Runner, actual Materials routes/service/receipt storage,
// real Conductor, isolated PostgreSQL schemas. Only Capability generation is a
// mock. Text input/output deliberately needs no Storage mock or model billing.
func TestResultReuseInternalWorkerRoundTrip(t *testing.T) {
	reuseInternalRoundTrip(t, false)
}

func TestResultReuseBinaryStorageRoundTrip(t *testing.T) {
	if os.Getenv("MEDIA_E2E_REUSE_STORAGE_ENDPOINT") == "" {
		t.Skip("set explicit MEDIA_E2E_REUSE_STORAGE_* for real Storage gate")
	}
	reuseInternalRoundTrip(t, true)
}

func reuseInternalRoundTrip(t *testing.T, binary bool) {
	address := os.Getenv("MEDIA_E2E_CONDUCTOR_URL")
	if address == "" {
		t.Skip("set MEDIA_E2E_CONDUCTOR_URL")
	}
	db := newDatabase(t)
	dsn := db.Dialector.(*pgdriver.Dialector).Config.DSN
	admin, err := db.DB()
	if err != nil {
		t.Fatal("isolated PostgreSQL pool unavailable")
	}
	materialsDSN, cleanup, err := testpostgres.Create(context.Background(), admin, dsn, "reuse_materials")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cleanup(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if err := materialsDAL.Migrate("../../media_materials/migrations", &configpb.DatabaseConfig{Dsn: materialsDSN, MigrateOnStart: true}); err != nil {
		t.Fatal("isolated Materials migration failed")
	}
	materialsDB, err := gorm.Open(pgdriver.Open(materialsDSN), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("isolated Materials pool unavailable")
	}
	t.Cleanup(func() { pool, _ := materialsDB.DB(); _ = pool.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var storage *objectstorage.Client
	var objectPath string
	origin, bucket := os.Getenv("MEDIA_E2E_REUSE_STORAGE_PUBLIC_ENDPOINT"), os.Getenv("MEDIA_E2E_REUSE_STORAGE_BUCKET")
	if binary {
		storage, err = objectstorage.New(ctx, objectstorage.Config{Provider: "s3", Endpoint: os.Getenv("MEDIA_E2E_REUSE_STORAGE_ENDPOINT"), PublicEndpoint: origin, Bucket: bucket, Region: os.Getenv("MEDIA_E2E_REUSE_STORAGE_REGION"), AccessKey: os.Getenv("MEDIA_E2E_REUSE_STORAGE_ACCESS_KEY"), SecretKey: os.Getenv("MEDIA_E2E_REUSE_STORAGE_SECRET_KEY"), ForcePathStyle: true})
		if err != nil {
			t.Fatal("explicit test Storage configuration unavailable")
		}
		image, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(image)
		objectPath = "materials/org_reuse/reuse/e2e/" + uuid.NewString() + ".png"
		if _, err := storage.PutIfAbsent(ctx, objectstorage.PutInput{ObjectPath: objectPath, ContentType: "image/png", Checksum: hex.EncodeToString(digest[:]), SizeBytes: int64(len(image)), Body: bytes.NewReader(image)}); err != nil {
			t.Fatal("isolated real Storage fixture upload failed")
		}
		t.Cleanup(func() {
			if err := storage.Delete(context.Background(), objectPath); err != nil {
				t.Error("isolated real Storage fixture cleanup failed")
			}
		})
	}
	products := productrepo.NewRepo(materialsDB)
	product := &productdomain.Product{ID: "product_reuse", OrganizationID: "org_reuse", Scope: "reuse", Name: "reuse fixture", Status: productdomain.StatusActive, CreatedBy: "owner", UpdatedBy: "owner", DisplayActions: "{}", Tags: "[]", ExternalCommerce: "{}", Metadata: "{}"}
	if err := products.Save(ctx, product); err != nil {
		t.Fatal("isolated source Product seed failed", err)
	}
	kinds := kindrepo.NewRepo(materialsDB)
	mediaType := kinddomain.MediaTypeText
	if binary {
		mediaType = kinddomain.MediaTypeImage
	}
	if err := kinds.Save(ctx, &kinddomain.AssetKind{Name: "reuse_output", ResourceType: kinddomain.ResourceTypeAsset, MediaType: mediaType, SourceTable: "generated_assets", Status: kinddomain.StatusEnabled}); err != nil {
		t.Fatal("isolated output kind seed failed")
	}
	assets := assetrepo.NewRepo(materialsDB)
	service := assetservice.NewService(assets, products, nil)
	service.ConfigureResultReceipts(assets, materialsDAL.NewTxRunner(materialsDB))
	service.ConfigureHistoryImport(nil, kindservice.NewService(kinds))
	if binary {
		service.ConfigureHistoryImport(storage, kindservice.NewService(kinds))
		service.ConfigureNativeOutputStorage(origin, bucket)
		previousResolver := objecthandler.Resolver
		objecthandler.Resolver = storage
		t.Cleanup(func() { objecthandler.Resolver = previousResolver })
	}
	prior := assethandler.Svc
	assethandler.Svc = service
	t.Cleanup(func() { assethandler.Svc = prior })
	t.Setenv("MEDIA_MATERIALS_INTERNAL_TOKEN", "isolated-reuse-token")
	h := server.Default()
	assetrouter.Register(h)
	if binary {
		objectrouter.Register(h)
	}
	transport := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(io.LimitReader(req.Body, 65537))
		if err != nil || len(body) > 65536 {
			w.WriteHeader(400)
			return
		}
		response := ut.PerformRequest(h.Engine, req.Method, req.URL.RequestURI(), &ut.Body{Body: bytes.NewReader(body), Len: len(body)}, ut.Header{Key: "Content-Type", Value: "application/json"}, ut.Header{Key: "X-Media-Materials-Internal-Token", Value: req.Header.Get("X-Media-Materials-Internal-Token")}).Result()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.StatusCode())
		_, _ = w.Write(response.Body())
	}))
	defer transport.Close()
	api := conductor.NewClient(address)
	// Check is a new production task definition. Register only if absent;
	// preexisting shared task retry policies are never overwritten by this gate.
	for _, taskName := range []string{render_task.TaskDefinitionName, reuse_task.TaskDefinitionName, finalize_task.TaskDefinitionName} {
		response, err := http.Get(address + "/api/metadata/taskdefs/" + taskName)
		if err != nil {
			t.Fatal("internal metadata preflight unavailable")
		}
		status := response.StatusCode
		response.Body.Close()
		if status == 404 && taskName == reuse_task.TaskDefinitionName {
			if err := api.RegisterTaskDefinitions(ctx, []conductor.TaskDefinition{{Name: taskName, RetryCount: 2, RetryLogic: "FIXED", RetryDelaySeconds: 1, TimeoutSeconds: 60, ResponseTimeoutSeconds: 30, TimeoutPolicy: "TIME_OUT_WF"}}); err != nil {
				t.Fatal(err)
			}
		} else if status != 200 {
			t.Fatal("required internal task definition unavailable")
		}
	}
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	provider, queue := "reuse_provider_"+suffix, "reuse_domain_"+suffix
	if err := api.RegisterTaskDefinitions(ctx, []conductor.TaskDefinition{{Name: provider, RetryCount: 0, TimeoutSeconds: 120, ResponseTimeoutSeconds: 90, TimeoutPolicy: "TIME_OUT_WF"}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		req, _ := http.NewRequest(http.MethodDelete, address+"/api/metadata/taskdefs/"+provider, nil)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
	})
	jobs, steps, evidence := jobrepo.NewJobRepo(db), jobrepo.NewStepRunRepo(db), jobrepo.NewResultReuseRepo(db)
	materials := client.NewMaterialsClient(transport.URL)
	engine := &conductor.ExecutionEngine{Client: api}
	digests := reuse_task.StorageDigests{Objects: materials, StorageOrigin: origin, StorageBucket: bucket}
	check := reuse_task.Handler{Jobs: jobs, Steps: steps, Executions: engine, Evidence: evidence, Assets: materials, Digests: digests, StorageOrigin: origin, StorageBucket: bucket}
	finalize := finalize_task.Handler{Jobs: jobs, Steps: steps, Executions: engine, Evidence: evidence, Assets: materials, ReusableAssets: materials, Digests: digests, StorageOrigin: origin, StorageBucket: bucket}
	var generated atomic.Int32
	domains := map[string]string{render_task.TaskDefinitionName: queue, reuse_task.TaskDefinitionName: queue, finalize_task.TaskDefinitionName: queue, provider: queue}
	runner := conductor.Runner{Client: api, WorkerID: queue, TaskDomains: domains, Handlers: map[string]conductor.Handler{
		render_task.TaskDefinitionName:   conductor.JSONHandler((render_task.Handler{}).Execute),
		reuse_task.TaskDefinitionName:    conductor.JSONHandlerContext(check.Execute),
		finalize_task.TaskDefinitionName: conductor.JSONHandlerContext(finalize.Execute),
		provider: func(context.Context, map[string]any) (map[string]any, error) {
			generated.Add(1)
			candidate := domain.Candidate{Content: "accepted fixture"}
			if binary {
				candidate = domain.Candidate{ObjectPath: objectPath}
			}
			return map[string]any{"candidates": []domain.Candidate{candidate}}, nil
		},
	}}
	workerCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- runner.Run(workerCtx) }()
	defer func() { stop(); <-done }()
	var firstAsset string
	for index, policy := range []string{"reuse_if_available", "reuse_if_available", "force_new"} {
		workflow := "reuse_roundtrip_" + suffix + "_" + policy
		stepIR := executable.ExecutableStepIR{Key: "text", Template: executable.TemplateIR{ID: "tpl", Version: 1, Body: "same actual prompt"}, Capability: executable.CapabilityIR{Key: provider, Version: 1, ConductorTaskDef: provider, Inputs: []capability.Input{{Name: "prompt", Type: capability.InputString, Required: true}}}, Invocation: template.InvocationMapping{"prompt": {Op: template.InvocationRenderTemplate}}, CandidateCount: 1, OutputAssetKind: "reuse_output", ApprovalPolicy: "when_multiple_candidates", ReusePolicy: policy}
		if binary {
			stepIR.Context = map[string]executable.ValueRef{"reference": {Source: executable.ValueConst, ConstValue: "object_path:" + objectPath}}
			stepIR.Capability.Inputs = append(stepIR.Capability.Inputs, capability.Input{Name: "images", Type: capability.InputResourceRefList, MediaType: capability.MediaImage, Required: true, MinItems: 1})
			stepIR.Invocation["images"] = template.InvocationExpression{Op: template.InvocationList, Items: []template.InvocationExpression{{Op: template.InvocationAssetRef, Slot: "reference"}}}
		}
		definition, err := compiler.CompileExecutableIR(executable.ExecutableWorkflowIR{SchemaVersion: executable.SchemaVersion, WorkflowID: workflow, WorkflowVersion: 1, OutputStepKey: "text", Steps: []executable.ExecutableStepIR{stepIR}})
		if err != nil {
			t.Fatal(err)
		}
		if index != 1 {
			if err := api.RegisterWorkflow(ctx, definition); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				req, _ := http.NewRequest(http.MethodDelete, address+"/api/metadata/workflow/"+definition.Name+"/1", nil)
				if resp, err := http.DefaultClient.Do(req); err == nil {
					resp.Body.Close()
				}
			})
		}
		jobID := "job_" + uuid.NewString()
		step := &domain.StepRun{ID: "sr_" + uuid.NewString(), JobID: jobID, StepKey: "text", StepOrder: 1, TemplateID: "tpl", TemplateVersion: 1, AssetKind: "reuse_output", ApprovalPolicy: domain.ApprovalWhenMultipleCandidates, Status: domain.SRPending}
		job := &domain.Job{ID: jobID, OrganizationID: product.OrganizationID, WorkspaceID: "workspace", Scope: product.Scope, SubmittedBy: "owner", WorkflowID: workflow, WorkflowVersion: 1, Status: domain.JobPending, Input: domain.JobInput{Product: &domain.ProductData{ID: product.ID}}}
		if err := jobs.Save(ctx, job, []*domain.StepRun{step}); err != nil {
			t.Fatal("isolated Job seed failed")
		}
		id, err := api.StartWorkflow(ctx, conductor.StartWorkflowRequest{Name: definition.Name, Version: 1, Input: map[string]any{"job_id": jobID, "inputs": map[string]any{"product": map[string]any{"value": map[string]any{"id": product.ID}}}}, TaskToDomain: domains})
		if err != nil {
			t.Fatal("isolated start outcome uncertain; do not restart", err)
		}
		t.Cleanup(func() { _ = api.TerminateWorkflow(context.Background(), id, "isolated reuse roundtrip cleanup") })
		if err := jobs.UpdateConductorExecution(ctx, jobID, id, "fixture"); err != nil {
			t.Fatal(err)
		}
		for {
			snapshot, err := api.GetWorkflow(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.Status == "COMPLETED" {
				asset, _ := snapshot.Output["output_resource_id"].(string)
				if !strings.HasPrefix(asset, "gen_") {
					t.Fatal("real registered output missing")
				}
				if index == 0 {
					firstAsset = asset
				}
				if index == 1 && (asset != firstAsset || generated.Load() != 1) {
					t.Fatal("reuse regenerated or replaced accepted source")
				}
				if index == 2 && (asset == firstAsset || generated.Load() != 2) {
					t.Fatal("force_new did not create independent result")
				}
				var count int64
				if materialsDB.Table("media_materials.generated_asset_results").Count(&count).Error != nil || count != int64(generated.Load()) {
					t.Fatal("reuse registered duplicate receipt")
				}
				for _, task := range snapshot.Tasks {
					if index == 1 && (task.TaskType == provider || task.TaskType == "HUMAN") {
						t.Fatal("reuse scheduled Capability or HUMAN")
					}
					if task.TaskType == finalize_task.TaskDefinitionName {
						want := "generated"
						if index == 1 {
							want = "reused"
						}
						if task.OutputData["result_source"] != want {
							t.Fatal("native provenance not returned")
						}
					}
				}
				break
			}
			if snapshot.Status != "RUNNING" {
				t.Fatalf("real reuse roundtrip failed: %s", snapshot.Status)
			}
			select {
			case <-ctx.Done():
				t.Fatal("real reuse roundtrip timed out")
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
}
