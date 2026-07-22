package media_workflow

import (
	log "log"

	configpb "media_agent/hertz_gen/config"
	prodsvc "media_agent/hertz_gen/media_materials/product_service"
	reachsvc "media_agent/hertz_gen/media_materials/reachability_service"
	vipsvc "media_agent/hertz_gen/media_materials/virtual_ip_service"
	jobsvc "media_agent/hertz_gen/media_workflow/job_service"
	wfsvc "media_agent/hertz_gen/media_workflow/workflow_service"
	"media_agent/hertz_infra/clienthertz"
)

// caller 侧 typed client（reachability + product + vip + workflow job）。
// 装配参照 cmd/media_auth/user/client.go 的 clienthertz 套件模式。
var (
	ReachCli reachsvc.Client
	ProdCli  prodsvc.Client
	VipCli   vipsvc.Client
	JobCli   jobsvc.Client
	WfCli    wfsvc.Client
)

// Init 装配 media_materials + media_workflow 的 typed client。
// gate：cfg.GetClient()["media_materials"] 和 ["media_workflow"] 均配置时才 Init。
func Init(cfg *configpb.Config) {
	matCfg, ok := cfg.GetClient()["media_materials"]
	if !ok || matCfg.GetBaseDomain() == "" {
		log.Fatal("media_workflow cmd: client.media_materials not configured")
	}
	wfCfg, ok := cfg.GetClient()["media_workflow"]
	if !ok || wfCfg.GetBaseDomain() == "" {
		log.Fatal("media_workflow cmd: client.media_workflow not configured")
	}

	base := clienthertz.NewClientSuiteFromConfig(cfg)

	// media_materials（reachability + product + vip 共享 app suite）
	matSuite, err := base.NewAppClientSuite("media_materials")
	if err != nil {
		log.Fatalf("media_materials client suite: %v", err)
	}
	matShared, err := matSuite.NewClient()
	if err != nil {
		log.Fatalf("media_materials shared client: %v", err)
	}
	matHost := matCfg.GetBaseDomain()
	ReachCli, err = reachsvc.NewReachabilityServiceClient(matHost, reachsvc.WithHertzClient(matShared))
	if err != nil {
		log.Fatalf("reachability client: %v", err)
	}
	ProdCli, err = prodsvc.NewProductServiceClient(matHost, prodsvc.WithHertzClient(matShared))
	if err != nil {
		log.Fatalf("product client: %v", err)
	}
	VipCli, err = vipsvc.NewVirtualIpServiceClient(matHost, vipsvc.WithHertzClient(matShared))
	if err != nil {
		log.Fatalf("virtual_ip client: %v", err)
	}

	// media_workflow（独立 app suite）
	wfSuite, err := base.NewAppClientSuite("media_workflow")
	if err != nil {
		log.Fatalf("media_workflow client suite: %v", err)
	}
	wfShared, err := wfSuite.NewClient()
	if err != nil {
		log.Fatalf("media_workflow shared client: %v", err)
	}
	wfHost := wfCfg.GetBaseDomain()
	JobCli, err = jobsvc.NewJobServiceClient(wfHost, jobsvc.WithHertzClient(wfShared))
	if err != nil {
		log.Fatalf("job client: %v", err)
	}
	WfCli, err = wfsvc.NewWorkflowServiceClient(wfHost, wfsvc.WithHertzClient(wfShared))
	if err != nil {
		log.Fatalf("workflow client: %v", err)
	}
}
