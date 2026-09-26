package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"media_agent/media_workflow/biz/workflow/conductor"
	"media_agent/media_workflow/biz/workflow/finalize_task"
	"media_agent/media_workflow/biz/workflow/render_task"
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
	finalizeHandler := finalize_task.Handler{}
	runner := conductor.Runner{
		Client: conductor.NewClient(conductorURL), WorkerID: workerID,
		Handlers: map[string]conductor.Handler{
			render_task.TaskDefinitionName:   conductor.JSONHandler(renderHandler.Execute),
			finalize_task.TaskDefinitionName: conductor.JSONHandler(finalizeHandler.Execute),
		},
	}
	log.Printf("media-workflow internal worker starting: worker_id=%s", workerID)
	if err := runner.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatalf("internal worker stopped: %v", err)
	}
}
