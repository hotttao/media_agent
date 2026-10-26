package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"media_agent/media_worker/biz/client/fixture"
	"media_agent/media_worker/biz/client/worker_gateway"
	"media_agent/media_worker/biz/dal/repo/results"
	policy "media_agent/media_worker/biz/policy/execution"
	"media_agent/media_worker/biz/shared/worker_runtime"
	"media_agent/media_worker/biz/workflow/execute"
)

func main() {
	gatewayURL := flag.String("gateway-url", envOr("MEDIA_GATEWAY_URL", ""), "public media gateway base URL")
	apiToken := flag.String("token", envOr("MEDIA_WORKER_API_TOKEN", ""), "opaque Personal Worker API Token")
	workerID := flag.String("worker-id", envOr("MEDIA_WORKER_ID", defaultWorkerID()), "stable client worker id")
	stateDir := flag.String("state-dir", envOr("MEDIA_WORKER_STATE_DIR", ".worker-state"), "private local directory for unconfirmed result replay")
	profile := flag.String("profile", envOr("MEDIA_WORKER_PROFILE", ""), "execution profile: doubao_studio, gpt_image_use, media_ai_fixture or provider_fixture")
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
	configPath := flag.String("config", envOr("CONFIG_FILE", "conf/local.yaml"), "shared config file (production profiles)")
	flag.Parse()
	if *gatewayURL == "" {
		log.Fatal("MEDIA_GATEWAY_URL or -gateway-url is required")
	}
	if *apiToken == "" {
		log.Fatal("MEDIA_WORKER_API_TOKEN or -token is required")
	}
	if *profile == "doubao_studio" || *profile == "gpt_image_use" {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		run := runDoubaoProduction
		if *profile == "gpt_image_use" {
			run = runGPTProduction
		}
		if err := run(ctx, *configPath, *gatewayURL, *apiToken, *workerID, *stateDir, *capabilityList); err != nil {
			log.Print(err)
			os.Exit(1)
		}
		return
	}
	if *profile != "media_ai_fixture" && *profile != fixture.ProviderProfile {
		log.Fatal("execution requires doubao_studio, gpt_image_use, media_ai_fixture or provider_fixture profile")
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
	capabilities, err = policy.SelectCapabilities(capabilities, *capabilityList)
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	client := worker_gateway.New(*gatewayURL, *apiToken)
	executor.SetFileUploader(func(ctx context.Context, task worker_runtime.Task, index int, file string) (string, error) {
		return client.UploadFile(ctx, *workerID, task, index, file)
	})
	store, err := results.Open(*stateDir, *gatewayURL, *workerID, *apiToken)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	log.Printf("media_worker starting: worker_id=%s profile=%s capabilities=%d", *workerID, *profile, len(capabilities))
	if err := (execute.Workflow{Gateway: client, Executor: executor, Results: store}).Run(ctx, execute.Command{WorkerID: *workerID, Capabilities: capabilities}); err != nil {
		log.Fatal(err)
	}
}

func parseFixtureResultURLs(raw string) ([]string, error) {
	var urls []string
	if json.Unmarshal([]byte(raw), &urls) != nil || len(urls) < 2 || len(urls) > 8 {
		return nil, errors.New("multiple fixture results require a JSON string array of two to eight URLs")
	}
	return urls, nil
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
