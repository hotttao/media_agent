package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"media_agent/media_workflow/biz/dal/repo/job"
	"media_agent/media_workflow/biz/shared/client"
	"media_agent/media_workflow/biz/workflow/conductor"
	"media_agent/media_workflow/biz/workflow/finalize_task"
	"media_agent/media_workflow/biz/workflow/render_task"
	"media_agent/media_workflow/biz/workflow/restore_task"
)

func main() {
	conductorURL := os.Getenv("CONDUCTOR_URL")
	workerID := os.Getenv("MEDIA_WORKFLOW_INTERNAL_WORKER_ID")
	if workerID == "" {
		hostname, _ := os.Hostname()
		workerID = "media-workflow-internal-" + hostname
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	renderHandler := render_task.Handler{}
	db, err := gorm.Open(postgres.Open(os.Getenv("POSTGRES_DSN")), &gorm.Config{})
	if err != nil {
		log.Fatal("internal worker database connection unavailable")
	}
	materialsURL := os.Getenv("MEDIA_MATERIALS_URL")
	if materialsURL == "" {
		log.Fatal("MEDIA_MATERIALS_URL is required for internal execution tasks")
	}
	restoreHandler := restore_task.Handler{
		Jobs: job.NewJobRepo(db), Steps: job.NewStepRunRepo(db), Assets: client.NewMaterialsClient(materialsURL),
	}
	api := conductor.NewClient(conductorURL)
	finalizeHandler := finalize_task.Handler{Jobs: job.NewJobRepo(db), Steps: job.NewStepRunRepo(db),
		Executions: &conductor.ExecutionEngine{Client: api}, Assets: client.NewMaterialsClient(materialsURL), StorageOrigin: os.Getenv("MEDIA_STORAGE_PUBLIC_ENDPOINT"), StorageBucket: os.Getenv("MEDIA_STORAGE_BUCKET")}
	runner := conductor.Runner{
		Client: api, WorkerID: workerID,
		Handlers: map[string]conductor.Handler{
			render_task.TaskDefinitionName:   conductor.JSONHandler(renderHandler.Execute),
			finalize_task.TaskDefinitionName: conductor.JSONHandlerContext(finalizeHandler.Execute),
			restore_task.TaskDefinitionName:  conductor.JSONHandlerContext(restoreHandler.Execute),
		},
	}
	log.Printf("media-workflow internal worker starting: worker_id=%s", workerID)
	if err := runner.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatalf("internal worker stopped: %v", err)
	}
}
