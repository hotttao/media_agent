package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"media_agent/media_worker/internal/conductor"
	"media_agent/media_worker/internal/fixture"
)

var internalTasks = map[string]bool{
	"media_render_template_v1": true,
	"media_finalize_step_v1":   true,
}

func main() {
	conductorURL := flag.String("conductor-url", envOr("CONDUCTOR_URL", "http://127.0.0.1:8080"), "Conductor API base URL")
	workerID := flag.String("worker-id", envOr("MEDIA_WORKER_ID", defaultWorkerID()), "stable client worker id")
	profile := flag.String("profile", envOr("MEDIA_WORKER_PROFILE", ""), "execution profile; use media_ai_fixture only for integration testing")
	taskList := flag.String("tasks", "", "comma-separated capability task allowlist")
	flag.Parse()
	if *profile != "media_ai_fixture" {
		log.Fatalf("profile %q is not implemented; integration requires -profile media_ai_fixture", *profile)
	}
	tasks := fixture.Tasks()
	if strings.TrimSpace(*taskList) != "" {
		tasks = strings.Split(*taskList, ",")
	}
	for _, task := range tasks {
		if internalTasks[strings.TrimSpace(task)] {
			log.Fatalf("task %q is internal to media_workflow and cannot be executed by media_worker", task)
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	client := conductor.New(*conductorURL)
	var wg sync.WaitGroup
	for _, rawTask := range tasks {
		taskType := strings.TrimSpace(rawTask)
		if taskType == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			runTask(ctx, client, *workerID, taskType)
		}()
	}
	log.Printf("media_worker started: worker_id=%s profile=%s tasks=%s", *workerID, *profile, strings.Join(tasks, ","))
	<-ctx.Done()
	wg.Wait()
}

func runTask(ctx context.Context, client *conductor.Client, workerID, taskType string) {
	for ctx.Err() == nil {
		task, err := client.Poll(ctx, taskType, workerID)
		if err != nil {
			log.Printf("poll task=%s: %v", taskType, err)
			wait(ctx, time.Second)
			continue
		}
		if task == nil {
			wait(ctx, 300*time.Millisecond)
			continue
		}
		output, executeErr := fixture.Execute(*task)
		result := conductor.Result{WorkflowInstanceID: task.WorkflowInstanceID, TaskID: task.TaskID, WorkerID: workerID, Status: "COMPLETED", OutputData: output}
		if executeErr != nil {
			result.Status = "FAILED_WITH_TERMINAL_ERROR"
			result.ReasonForIncompletion = executeErr.Error()
		}
		if err := client.Update(ctx, result); err != nil {
			log.Printf("update task=%s id=%s: %v", taskType, task.TaskID, err)
			continue
		}
		log.Printf("completed task=%s ref=%s id=%s profile=media_ai_fixture", taskType, task.ReferenceTaskName, task.TaskID)
	}
}

func wait(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func defaultWorkerID() string {
	hostname, _ := os.Hostname()
	return "media-worker-" + hostname
}
