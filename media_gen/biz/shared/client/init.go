package client

import (
	log "log"

	configpb "media_agent/hertz_gen/config"
	jobsvc "media_agent/hertz_gen/media_workflow/job_service"
	"media_agent/hertz_infra/clienthertz"
)

// Init 是 media_gen 客户端初始化唯一入口，由 main.go 调用。
//
// 装配 media_workflow.job_service client（供 generation.Repository 实现 shared/client/workflow.go 用）。
//
// cfg：当前配置快照（loader.Current()）。
// media_workflow 配置缺失时直接 log.Fatal - 无下游则 worker loop 无法运行。
func Init(cfg *configpb.Config) {
	wfCfg, ok := cfg.GetClient()["media_workflow"]
	if !ok || wfCfg.GetBaseDomain() == "" {
		log.Fatal("media_gen: client.media_workflow not configured")
	}

	base := clienthertz.NewClientSuiteFromConfig(cfg)
	wfSuite, err := base.NewAppClientSuite("media_workflow")
	if err != nil {
		log.Fatalf("media_workflow client suite: %v", err)
	}
	wfShared, err := wfSuite.NewClient()
	if err != nil {
		log.Fatalf("media_workflow shared client: %v", err)
	}
	wfHost := wfCfg.GetBaseDomain()
	MediaWorkflowJobClient, err = jobsvc.NewJobServiceClient(wfHost, jobsvc.WithHertzClient(wfShared))
	if err != nil {
		log.Fatalf("job client: %v", err)
	}
}
