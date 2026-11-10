package main

import (
	"context"
	"errors"
	sharedconfig "media_agent/hertz_infra/config"
	"media_agent/media_worker/biz/client/worker_gateway"
	"media_agent/media_worker/biz/client/wx_channel"
	"media_agent/media_worker/biz/dal/repo/results"
	policy "media_agent/media_worker/biz/policy/execution"
	"media_agent/media_worker/biz/workflow/execute"
	"media_agent/media_worker/biz/workflow/reference_import"
	"path/filepath"
)

func runWXChannelProduction(ctx context.Context, configPath, gatewayURL, token, workerID, stateDir, allowlist string) error {
	loader, err := sharedconfig.NewLoaderFromPath(configPath)
	if err != nil {
		return errors.New("worker shared configuration unavailable")
	}
	defer loader.Close()
	cfg := loader.Current().GetMediaWorker()
	if cfg == nil || !cfg.GetEnabled() || !cfg.GetWxChannel().GetEnabled() {
		return errors.New("wx_channel configuration is disabled")
	}
	conf := cfg.GetWxChannel()
	source, err := wx_channel.Open(ctx, conf.GetEndpoint(), conf.GetTokenFile(), conf.GetDownloadsDirectory())
	if err != nil {
		return err
	}
	defer source.Close()
	store, err := results.Open(stateDir, gatewayURL, workerID, token)
	if err != nil {
		return err
	}
	defer store.Close()
	gateway := worker_gateway.New(gatewayURL, token)
	flow := &reference_import.Workflow{Source: source, Gateway: gateway, Uploader: gateway, WorkerID: workerID, Root: filepath.Join(store.Namespace(), "references")}
	caps, err := policy.SelectCapabilities(flow.AvailableCapabilities(), allowlist)
	if err != nil {
		return err
	}
	return (execute.Workflow{Gateway: gateway, Executor: flow, Results: store}).Run(ctx, execute.Command{WorkerID: workerID, Capabilities: caps})
}
