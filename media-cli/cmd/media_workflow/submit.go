package media_workflow

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	reachpb "media_agent/hertz_gen/model/media_materials/reachability"
	prodpb "media_agent/hertz_gen/model/media_materials/product"
	vippb "media_agent/hertz_gen/model/media_materials/virtual_ip"
	jobpb "media_agent/hertz_gen/model/media_workflow/job"
	wfpb "media_agent/hertz_gen/model/media_workflow/workflow"
)

var submitJobCmd = &cobra.Command{
	Use:   "submit-job",
	Short: "assemble JobInput bundle (reachability 校验 + 解析) and submit to media_workflow",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		userID, _ := cmd.Flags().GetString("user-id")
		workflowID, _ := cmd.Flags().GetString("workflow-id")
		version, _ := cmd.Flags().GetInt("workflow-version")
		productID, _ := cmd.Flags().GetString("product-id")
		vipID, _ := cmd.Flags().GetString("virtual-ip-id")
		return runSubmitJob(ctx, userID, workflowID, version, productID, vipID)
	},
}

func init() {
	submitJobCmd.Flags().String("user-id", "", "submitter user id (for reachability viewer check)")
	submitJobCmd.Flags().String("workflow-id", "", "workflow id")
	submitJobCmd.Flags().Int("workflow-version", 0, "workflow version (0 = latest)")
	submitJobCmd.Flags().String("product-id", "", "product input id (optional)")
	submitJobCmd.Flags().String("virtual-ip-id", "", "virtual_ip input id (optional)")
	_ = submitJobCmd.MarkFlagRequired("workflow-id")
}

// runSubmitJob caller 组装 JobInput bundle 并提交。
// 流程（bundle 模式，见 docs/ddd/media_materials/workflow/0001-reachability/reachability.md）：
//  1. GetWorkflow 拿 scope（可达性锚点）
//  2. CheckProductAccessible / CheckVirtualIPAccessible 校验容器可达
//  3. GetProduct / GetVirtualIp 填 ProductData / VirtualIpData（reachability 不返回结构化字段）
//  4. ResolveAssetByKind + AssetRead 解析 workflow_input slot 的 asset（url/content 填 bundle）
//  5. SubmitJob 提交自包含 bundle
func runSubmitJob(ctx context.Context, userID, workflowID string, version int, productID, vipID string) error {
	// 1. workflow scope
	wf, _, err := WfCli.GetWorkflow(ctx, &wfpb.GetWorkflowReq{Id: workflowID})
	if err != nil {
		return fmt.Errorf("get workflow: %w", err)
	}
	scope := wf.GetScope()
	if version == 0 {
		version = int(wf.GetVersion())
	}

	bundle := &jobpb.JobInput{}

	// 2 + 3. product 输入
	if productID != "" {
		acc, _, err := ReachCli.CheckProductAccessible(ctx, &reachpb.CheckProductAccessibleReq{
			WorkflowScope: scope, ProductId: productID,
		})
		if err != nil {
			return fmt.Errorf("check product accessible: %w", err)
		}
		if !acc.GetAccessible() {
			return fmt.Errorf("product %s not accessible: %s", productID, acc.GetReason())
		}
		p, _, err := ProdCli.GetProduct(ctx, &prodpb.GetProductReq{Id: productID})
		if err != nil {
			return fmt.Errorf("get product: %w", err)
		}
		bundle.Product = &jobpb.ProductData{
			Id: p.GetId(), Name: p.GetName(), TargetAudience: p.GetTargetAudience(),
			ProductDetails: p.GetProductDetails(), DisplayActions: p.GetDisplayActions(),
		}
	}

	// 2 + 3. virtual_ip 输入
	if vipID != "" {
		acc, _, err := ReachCli.CheckVirtualIPAccessible(ctx, &reachpb.CheckVirtualIPAccessibleReq{
			UserId: userID, WorkflowScope: scope, VirtualIpId: vipID,
		})
		if err != nil {
			return fmt.Errorf("check virtual_ip accessible: %w", err)
		}
		if !acc.GetAccessible() {
			return fmt.Errorf("virtual_ip %s not accessible: %s", vipID, acc.GetReason())
		}
		v, _, err := VipCli.GetVirtualIp(ctx, &vippb.GetVirtualIpReq{Id: vipID})
		if err != nil {
			return fmt.Errorf("get virtual_ip: %w", err)
		}
		bundle.VirtualIp = &jobpb.VirtualIpData{
			Id: v.GetId(), Name: v.GetName(), AvatarUrl: v.GetAvatarUrl(),
			FullBodyUrl: v.GetFullBodyUrl(), ThreeViewUrl: v.GetThreeViewUrl(), NineViewUrl: v.GetNineViewUrl(),
		}
	}

	// 4. ResolveAssetByKind + AssetRead 解析 input_schema 里的 asset slot
	for _, entry := range wf.GetInputSchema().GetEntries() {
		ownerKind, ownerID := ownerForEntry(entry.GetName(), productID, vipID)
		if ownerID == "" {
			continue
		}
		resolved, _, err := ReachCli.ResolveAssetByKind(ctx, &reachpb.ResolveAssetByKindReq{
			OwnerKind: ownerKind, OwnerId: ownerID, AssetKind: entry.GetName(),
		})
		if err != nil {
			return fmt.Errorf("resolve asset by kind %q: %w", entry.GetName(), err)
		}
		read, _, err := ReachCli.AssetRead(ctx, &reachpb.AssetReadReq{
			ResourceType: resolved.GetResourceType(), Id: resolved.GetAssetId(),
		})
		if err != nil {
			return fmt.Errorf("asset read %s: %w", resolved.GetAssetId(), err)
		}
		switch resolved.GetResourceType() {
		case "MATERIAL":
			bundle.Material = append(bundle.Material, &jobpb.MaterialData{Name: entry.GetName(), Url: read.GetUrl(), Content: read.GetContent()})
		case "ASSET":
			bundle.Asset = append(bundle.Asset, &jobpb.AssetData{Name: entry.GetName(), Url: read.GetUrl(), Content: read.GetContent()})
		}
	}

	// 5. SubmitJob
	resp, _, err := JobCli.SubmitJob(ctx, &jobpb.SubmitJobReq{
		WorkflowId: workflowID, WorkflowVersion: int32(version), Input: bundle, SubmittedBy: userID,
	})
	if err != nil {
		return fmt.Errorf("submit job: %w", err)
	}
	fmt.Printf("submitted job_id=%s status=%s\n", resp.GetJobId(), resp.GetStatus())
	return nil
}

// ownerForEntry 简化：当前按 product 优先，vip 次之解析 asset。
// 完整实现需 workflow.input_schema 标注每个 entry 的 owner_kind（product/virtual_ip）。
func ownerForEntry(name, productID, vipID string) (string, string) {
	if productID != "" {
		return "product", productID
	}
	if vipID != "" {
		return "virtual_ip", vipID
	}
	return "", ""
}
