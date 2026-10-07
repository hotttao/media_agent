package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"media_agent/media_worker/internal/fixture"
	"media_agent/media_worker/internal/gateway"
)

func main() {
	gatewayURL := flag.String("gateway-url", envOr("MEDIA_GATEWAY_URL", ""), "public media gateway base URL")
	apiToken := flag.String("token", envOr("MEDIA_WORKER_API_TOKEN", ""), "opaque Personal Worker API Token")
	workerID := flag.String("worker-id", envOr("MEDIA_WORKER_ID", defaultWorkerID()), "stable client worker id")
	profile := flag.String("profile", envOr("MEDIA_WORKER_PROFILE", ""), "execution profile; use media_ai_fixture only for integration testing")
	fixtureConfig := flag.String("fixture-config", envOr("MEDIA_WORKER_FIXTURE_CONFIG", ""), "private Provider fixture JSON file with explicit Job/Step results")
	capabilityList := flag.String("capabilities", envOr("MEDIA_WORKER_CAPABILITIES", ""), "comma-separated capability key@version allowlist")
	fixtureAssetsURL := flag.String("fixture-assets-url", envOr("MEDIA_WORKER_FIXTURE_ASSETS_URL", "http://192.168.2.38"), "media_ai fixture asset service base URL (test profile only)")
	fixtureVideoResultURL := flag.String("fixture-video-result-url", envOr("MEDIA_WORKER_FIXTURE_VIDEO_RESULT_URL", ""), "pre-uploaded target-storage video URL for the historical fixture")
	fixtureModelResultURL := flag.String("fixture-model-result-url", envOr("MEDIA_WORKER_FIXTURE_MODEL_RESULT_URL", ""), "pre-uploaded target-storage model image URL (test profile only)")
	fixtureModelResultURLs := flag.String("fixture-model-result-urls", envOr("MEDIA_WORKER_FIXTURE_MODEL_RESULT_URLS", ""), "JSON array of distinct pre-uploaded model image URLs (test profile only)")
	fixtureStyleResultURL := flag.String("fixture-style-result-url", envOr("MEDIA_WORKER_FIXTURE_STYLE_RESULT_URL", ""), "pre-uploaded target-storage style image URL (test profile only)")
	fixtureFirstFrameResultURL := flag.String("fixture-first-frame-result-url", envOr("MEDIA_WORKER_FIXTURE_FIRST_FRAME_RESULT_URL", ""), "pre-uploaded target-storage first-frame image URL (test profile only)")
	fixtureFailStep := flag.String("fixture-fail-step", envOr("MEDIA_WORKER_FIXTURE_FAIL_STEP", ""), "step to fail once per Job (test profile only)")
	fixtureFailCall := flag.String("fixture-fail-call", envOr("MEDIA_WORKER_FIXTURE_FAIL_CALL", "1"), "distinct dispatch number to fail, one to eight (test profile only)")
	flag.Parse()
	if *gatewayURL == "" {
		log.Fatal("MEDIA_GATEWAY_URL or -gateway-url is required")
	}
	if *apiToken == "" {
		log.Fatal("MEDIA_WORKER_API_TOKEN or -token is required")
	}
	if *profile != "media_ai_fixture" && *profile != fixture.ProviderProfile {
		log.Fatal("integration requires media_ai_fixture or provider_fixture profile")
	}
	var executor *fixture.Executor
	var err error
	if *profile == fixture.ProviderProfile {
		if *fixtureConfig == "" {
			log.Fatal("provider_fixture requires MEDIA_WORKER_FIXTURE_CONFIG")
		}
		if *fixtureVideoResultURL != "" || *fixtureModelResultURL != "" || *fixtureModelResultURLs != "" || *fixtureStyleResultURL != "" || *fixtureFirstFrameResultURL != "" {
			log.Fatal("provider_fixture cannot use legacy business Capability result overrides")
		}
		file, openErr := os.Open(*fixtureConfig)
		if openErr != nil {
			log.Fatal("cannot open provider fixture configuration")
		}
		executor, err = fixture.ReadProviderConfig(file)
		file.Close()
	} else {
		if *fixtureConfig != "" {
			log.Fatal("fixture-config requires provider_fixture profile")
		}
		executor, err = fixture.NewExecutor(*fixtureAssetsURL)
	}
	if err != nil {
		log.Fatal(err)
	}
	if *fixtureFailStep != "" {
		call, err := strconv.Atoi(*fixtureFailCall)
		if err != nil || executor.SetFailureOnce(*fixtureFailStep, call) != nil {
			log.Fatal("fixture failure requires a step and call between one and eight")
		}
	}
	if *profile == "media_ai_fixture" {
		if err := executor.SetVideoResultURL(*fixtureVideoResultURL); err != nil {
			log.Fatal(err)
		}
		for key, resultURL := range map[string]string{
			"media.image.model":       *fixtureModelResultURL,
			"media.image.style":       *fixtureStyleResultURL,
			"media.image.first_frame": *fixtureFirstFrameResultURL,
		} {
			if err := executor.SetResultURL(key, resultURL); err != nil {
				log.Fatal(err)
			}
		}
		if *fixtureModelResultURLs != "" {
			if *fixtureModelResultURL != "" {
				log.Fatal("configure either single or multiple model results, not both")
			}
			urls, err := parseFixtureResultURLs(*fixtureModelResultURLs)
			if err != nil {
				log.Fatal(err)
			}
			if err := executor.SetResultURLs("media.image.model", urls); err != nil {
				log.Fatal(err)
			}
		}
	}
	capabilities := executor.AvailableCapabilities()
	capabilities, err = selectCapabilities(capabilities, *capabilityList)
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	client := gateway.New(*gatewayURL, *apiToken)
	if err := client.Register(ctx, *workerID, capabilities); err != nil {
		log.Fatalf("register worker: %v", err)
	}
	var wg sync.WaitGroup
	for _, capability := range capabilities {
		for range capability.Concurrency {
			wg.Add(1)
			go func(capability gateway.Capability) {
				defer wg.Done()
				runCapability(ctx, client, *workerID, capability, executor)
			}(capability)
		}
	}
	log.Printf("media_worker started: worker_id=%s profile=%s capabilities=%d", *workerID, *profile, len(capabilities))
	<-ctx.Done()
	wg.Wait()
}

func parseFixtureResultURLs(raw string) ([]string, error) {
	var urls []string
	if json.Unmarshal([]byte(raw), &urls) != nil || len(urls) < 2 || len(urls) > 8 {
		return nil, errors.New("multiple fixture results require a JSON string array of two to eight URLs")
	}
	return urls, nil
}

func runCapability(ctx context.Context, client *gateway.Client, workerID string, capability gateway.Capability, executor *fixture.Executor) {
	for ctx.Err() == nil {
		task, err := client.Poll(ctx, workerID, capability)
		if err != nil {
			log.Printf("poll capability=%s@%d: %v", capability.Key, capability.Version, err)
			wait(ctx, time.Second)
			continue
		}
		if task == nil {
			wait(ctx, 300*time.Millisecond)
			continue
		}
		taskCtx, cancelTask := context.WithCancel(ctx)
		go heartbeat(taskCtx, client, workerID, *task)
		output, executeErr := executor.Execute(*task)
		idempotencyKey := task.DispatchID + ":" + fmt.Sprint(task.Attempt)
		submitted := submitResult(ctx, client, workerID, *task, idempotencyKey, output, executeErr)
		cancelTask()
		if !submitted {
			continue
		}
		if executeErr != nil {
			log.Printf("failed dispatch=%s capability=%s@%d", task.DispatchID, capability.Key, capability.Version)
			continue
		}
		log.Printf("completed dispatch=%s capability=%s@%d", task.DispatchID, capability.Key, capability.Version)
	}
}

func submitResult(ctx context.Context, client *gateway.Client, workerID string, task gateway.Task, idempotencyKey string, output map[string]any, executeErr error) bool {
	for ctx.Err() == nil {
		var err error
		if executeErr != nil {
			_, err = client.Fail(ctx, workerID, task, idempotencyKey+":fail", executeErr.Error())
		} else {
			_, err = client.Complete(ctx, workerID, task, idempotencyKey+":complete", output)
		}
		if err == nil {
			return true
		}
		var httpErr *gateway.HTTPError
		if errors.As(err, &httpErr) && (httpErr.StatusCode == 409 || httpErr.StatusCode == 410) {
			log.Printf("result rejected dispatch=%s: %v", task.DispatchID, err)
			return false
		}
		log.Printf("retry result dispatch=%s: %v", task.DispatchID, err)
		wait(ctx, time.Second)
	}
	return false
}

func selectCapabilities(available []gateway.Capability, raw string) ([]gateway.Capability, error) {
	if strings.TrimSpace(raw) == "" {
		return available, nil
	}
	byRef := make(map[string]gateway.Capability, len(available))
	for _, capability := range available {
		byRef[fmt.Sprintf("%s@%d", capability.Key, capability.Version)] = capability
	}
	selected := make([]gateway.Capability, 0)
	seen := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		ref := strings.TrimSpace(item)
		capability, ok := byRef[ref]
		if !ok {
			return nil, fmt.Errorf("unsupported capability %q", ref)
		}
		if !seen[ref] {
			selected = append(selected, capability)
			seen[ref] = true
		}
	}
	if len(selected) == 0 {
		return nil, errors.New("at least one capability is required")
	}
	return selected, nil
}

func heartbeat(ctx context.Context, client *gateway.Client, workerID string, task gateway.Task) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := client.Heartbeat(ctx, workerID, task); err != nil && ctx.Err() == nil {
				log.Printf("heartbeat dispatch=%s: %v", task.DispatchID, err)
			}
		}
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
