package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	sharedconfig "media_agent/hertz_infra/config"
	"media_agent/media_worker/biz/client/doubao/studio"
	"media_agent/media_worker/biz/client/media_input"
	"media_agent/media_worker/biz/client/worker_gateway"
	"media_agent/media_worker/biz/dal/repo/checkpoints"
	"media_agent/media_worker/biz/dal/repo/results"
	policy "media_agent/media_worker/biz/policy/execution"
	"media_agent/media_worker/biz/workflow/execute"
	"media_agent/media_worker/biz/workflow/generate"
)

func runProduction(ctx context.Context, configPath, gatewayURL, token, workerID, stateDir, allowlist string) error {
	loader, err := sharedconfig.NewLoaderFromPath(configPath)
	if err != nil {
		return errors.New("worker shared configuration unavailable")
	}
	defer loader.Close()
	cfg := loader.Current().GetMediaWorker()
	if cfg == nil || !cfg.GetEnabled() {
		return errors.New("media_worker configuration is disabled")
	}
	if cfg.GetModel() == "" || cfg.GetDurationSeconds() < 4 || cfg.GetDurationSeconds() > 15 || cfg.GetAspectRatio() == "" || cfg.GetStudioAccountId() == "" {
		return errors.New("explicit studio account/model/duration/aspect ratio required")
	}
	if err := policy.ValidateProductionParameters(cfg.GetModel(), int(cfg.GetDurationSeconds()), cfg.GetAspectRatio()); err != nil {
		return err
	}
	store, err := results.Open(stateDir, gatewayURL, workerID, token)
	if err != nil {
		return err
	}
	defer store.Close()
	namespace, err := filepath.Abs(store.Namespace())
	if err != nil {
		return err
	}
	assets := filepath.Join(namespace, "generation-inputs")
	if os.MkdirAll(assets, 0700) != nil {
		return errors.New("private input directory unavailable")
	}
	instance, err := studio.Open(studio.InstanceConfig{UserData: cfg.GetStudioUserData(), AssetRoot: assets, ArtifactRoot: filepath.Join(namespace, "generation-artifacts"), AccountID: cfg.GetStudioAccountId(), WorkingDirectory: cfg.GetStudioWorkingDirectory(), Command: cfg.GetStudioCommand(), StartupTimeout: time.Duration(cfg.GetStartupTimeoutSeconds()) * time.Second})
	if err != nil {
		return err
	}
	defer instance.Close()
	repository, err := checkpoints.Open(namespace)
	if err != nil {
		return err
	}
	gateway := worker_gateway.New(gatewayURL, token)
	generator := &generate.Workflow{Generator: instance, Files: media_input.Downloader{}, Gateway: gateway, Uploader: gateway, Checkpoints: repository, Inputs: media_input.Downloader{Root: assets}, WorkerID: workerID, Namespace: filepath.Base(namespace), Model: cfg.GetModel(), DurationSeconds: int(cfg.GetDurationSeconds()), AspectRatio: cfg.GetAspectRatio()}
	capabilities, err := policy.SelectCapabilities(generator.AvailableCapabilities(), allowlist)
	if err != nil {
		return err
	}
	return (execute.Workflow{Gateway: gateway, Executor: generator, Results: store, Recovery: generator, Admission: instance}).Run(ctx, execute.Command{WorkerID: workerID, Capabilities: capabilities})
}
