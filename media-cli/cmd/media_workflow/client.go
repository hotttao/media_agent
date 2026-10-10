package media_workflow

import (
	log "log"

	"context"
	"github.com/cloudwego/hertz/pkg/app/client"
	"github.com/cloudwego/hertz/pkg/protocol"
	configpb "media_agent/hertz_gen/config"
	assetsvc "media_agent/hertz_gen/media_materials/generated_asset_service"
	objectsvc "media_agent/hertz_gen/media_materials/library_object_service"
	matsvc "media_agent/hertz_gen/media_materials/material_service"
	prodsvc "media_agent/hertz_gen/media_materials/product_service"
	vipsvc "media_agent/hertz_gen/media_materials/virtual_ip_service"
	capsvc "media_agent/hertz_gen/media_workflow/capability_service"
	historysvc "media_agent/hertz_gen/media_workflow/historical_publication_service"
	jobsvc "media_agent/hertz_gen/media_workflow/job_service"
	tplsvc "media_agent/hertz_gen/media_workflow/template_service"
	wfsvc "media_agent/hertz_gen/media_workflow/workflow_service"
	"media_agent/hertz_infra/clienthertz"
	rootcmd "media_agent/media_cli/cmd"
	"media_agent/media_cli/cmd/cookie"
)

// caller 侧公开 typed client（企业素材、对象解析及 Workflow Job）。
// 装配参照 cmd/media_auth/user/client.go 的 clienthertz 套件模式。
var (
	MatCli        matsvc.Client
	ObjectCli     objectsvc.Client
	AssetCli      assetsvc.Client
	ProdCli       prodsvc.Client
	VipCli        vipsvc.Client
	JobCli        jobsvc.Client
	WfCli         wfsvc.Client
	HistoryCli    historysvc.Client
	CapabilityCli capsvc.Client
	TemplateCli   tplsvc.Client
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

	// media_materials 的公开读取/解析客户端共享一个治理套件与身份中间件。
	matSuite, err := base.NewAppClientSuite("media_materials")
	if err != nil {
		log.Fatalf("media_materials client suite: %v", err)
	}
	matShared, err := matSuite.NewClient()
	if err != nil {
		log.Fatalf("media_materials shared client: %v", err)
	}
	matHost := matCfg.GetBaseDomain()
	matShared.Use(cookie.New(rootcmd.CookiesDir).Middleware(matHost), materialOrganizationMiddleware)
	MatCli, err = matsvc.NewMaterialServiceClient(matHost, matsvc.WithHertzClient(matShared))
	if err != nil {
		log.Fatalf("material client: %v", err)
	}
	ObjectCli, err = objectsvc.NewLibraryObjectServiceClient(matHost, objectsvc.WithHertzClient(matShared))
	if err != nil {
		log.Fatal("library object client configuration failed")
	}
	AssetCli, err = assetsvc.NewGeneratedAssetServiceClient(matHost, assetsvc.WithHertzClient(matShared))
	if err != nil {
		log.Fatal("generated asset client configuration failed")
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
	wfShared.Use(cookie.New(rootcmd.CookiesDir).Middleware(wfHost))
	CapabilityCli, err = capsvc.NewCapabilityServiceClient(wfHost, capsvc.WithHertzClient(wfShared))
	if err != nil {
		log.Fatal("capability client configuration failed")
	}
	TemplateCli, err = tplsvc.NewTemplateServiceClient(wfHost, tplsvc.WithHertzClient(wfShared))
	if err != nil {
		log.Fatal("template client configuration failed")
	}
	HistoryCli, err = historysvc.NewHistoricalPublicationServiceClient(wfHost, historysvc.WithHertzClient(wfShared))
	if err != nil {
		log.Fatalf("historical publication client: %v", err)
	}
	JobCli, err = jobsvc.NewJobServiceClient(wfHost, jobsvc.WithHertzClient(wfShared))
	if err != nil {
		log.Fatalf("job client: %v", err)
	}
	WfCli, err = wfsvc.NewWorkflowServiceClient(wfHost, wfsvc.WithHertzClient(wfShared))
	if err != nil {
		log.Fatalf("workflow client: %v", err)
	}
}

type materialOrganizationKey struct{}

func materialOrganizationMiddleware(next client.Endpoint) client.Endpoint {
	return func(ctx context.Context, req *protocol.Request, resp *protocol.Response) error {
		if org, _ := ctx.Value(materialOrganizationKey{}).(string); org != "" {
			req.URI().QueryArgs().Set("organization_id", org)
		}
		return next(ctx, req, resp)
	}
}
