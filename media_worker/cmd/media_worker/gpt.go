package main

import (
	"context"
	"errors"
	sharedconfig "media_agent/hertz_infra/config"
	"media_agent/media_worker/biz/client/gpt/image_use"
	"media_agent/media_worker/biz/client/media_input"
	"media_agent/media_worker/biz/client/worker_gateway"
	"media_agent/media_worker/biz/dal/repo/gpt_checkpoints"
	"media_agent/media_worker/biz/dal/repo/results"
	policy "media_agent/media_worker/biz/policy/execution"
	"media_agent/media_worker/biz/workflow/execute"
	"media_agent/media_worker/biz/workflow/gpt_generate"
	"path/filepath"
	"time"
)

func runGPTProduction(ctx context.Context, configPath, gatewayURL, token, workerID, stateDir, allowlist string) error {
	loader, err := sharedconfig.NewLoaderFromPath(configPath)
	if err != nil {
		return errors.New("worker shared configuration unavailable")
	}
	defer loader.Close()
	cfg := loader.Current().GetMediaWorker()
	if cfg == nil || !cfg.GetEnabled() || !cfg.GetGptImageUse().GetEnabled() {
		return errors.New("gpt_image_use configuration is disabled")
	}
	conf := cfg.GetGptImageUse()
	if err := policy.ValidateGPTParameters(conf.GetSize(), conf.GetWebModel(), conf.GetProjectUrl()); err != nil {
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
	client, err := image_use.Open(ctx, image_use.Config{Command: conf.GetBridgeCommand(), Root: filepath.Join(namespace, "image-use"), InputRoot: assets, LockDirectory: conf.GetBrowserLockDirectory(), Upstream: conf.GetUpstreamCli(), Size: conf.GetSize(), WebModel: conf.GetWebModel(), Project: conf.GetProjectUrl(), Timeout: time.Duration(conf.GetTimeoutSeconds()) * time.Second})
	if err != nil {
		return err
	}
	defer client.Close()
	repository, err := gpt_checkpoints.Open(namespace)
	if err != nil {
		return err
	}
	gateway := worker_gateway.New(gatewayURL, token)
	workflow := &gpt_generate.Workflow{Generator: client, Files: media_input.Downloader{}, Gateway: gateway, Uploader: gateway, Checkpoints: repository, Inputs: media_input.Downloader{Root: assets}, WorkerID: workerID, Namespace: filepath.Base(namespace), WebModel: conf.GetWebModel(), Size: conf.GetSize()}
	caps, err := policy.SelectCapabilities(workflow.AvailableCapabilities(), allowlist)
	if err != nil {
		return err
	}
	return (execute.Workflow{Gateway: gateway, Executor: workflow, Results: store, Recovery: workflow, Admission: client}).Run(ctx, execute.Command{WorkerID: workerID, Capabilities: caps})
}
