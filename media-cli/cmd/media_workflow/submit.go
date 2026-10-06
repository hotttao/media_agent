package media_workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	assetpb "media_agent/hertz_gen/model/media_materials/generated_asset"
	objectpb "media_agent/hertz_gen/model/media_materials/library_object"
	matpb "media_agent/hertz_gen/model/media_materials/material"
	prodpb "media_agent/hertz_gen/model/media_materials/product"
	vippb "media_agent/hertz_gen/model/media_materials/virtual_ip"
	jobpb "media_agent/hertz_gen/model/media_workflow/job"
	wfpb "media_agent/hertz_gen/model/media_workflow/workflow"
)

type submitSelection struct {
	organization, workspace, workflow, product, virtualIP string
	idempotencyKey                                        string
	version                                               int
	materials, assets                                     map[string]string
	dryRun                                                bool
}

var submitJobCmd = newSubmitJobCommand()

func newSubmitJobCommand() *cobra.Command {
	var selection submitSelection
	c := &cobra.Command{Use: "submit-job", Short: "选择企业素材并提交自包含 Workflow 输入", Args: cobra.NoArgs}
	c.Flags().StringVar(&selection.organization, "organization-id", "", "素材所属企业")
	c.Flags().StringVar(&selection.workspace, "workspace-id", "", "任务所属 Workspace")
	c.Flags().StringVar(&selection.workflow, "workflow-id", "", "已发布 Workflow ID")
	c.Flags().StringVar(&selection.idempotencyKey, "idempotency-key", "", "同一次提交重用稳定键；省略则每次创建新 Job")
	c.Flags().IntVar(&selection.version, "workflow-version", 0, "发布版本（0=当前版本）")
	c.Flags().StringVar(&selection.product, "product-id", "", "Product ID")
	c.Flags().StringVar(&selection.virtualIP, "virtual-ip-id", "", "Virtual IP ID")
	c.Flags().StringToStringVar(&selection.materials, "material", nil, "输入名=Material ID，可重复指定")
	c.Flags().StringToStringVar(&selection.assets, "asset", nil, "输入名=Generated Asset ID，可重复指定")
	c.Flags().BoolVar(&selection.dryRun, "dry-run", false, "验证并组装输入，不创建 Job；不输出媒体 URL")
	_ = c.MarkFlagRequired("organization-id")
	_ = c.MarkFlagRequired("workspace-id")
	_ = c.MarkFlagRequired("workflow-id")
	c.RunE = func(c *cobra.Command, _ []string) error {
		request, err := buildSubmitRequest(c.Context(), selection)
		if err != nil {
			return err
		}
		if selection.dryRun {
			_, err = fmt.Fprintf(c.OutOrStdout(), "input validated workflow_id=%s version=%d materials=%d assets=%d; no Job submitted\n", request.WorkflowId, request.WorkflowVersion, len(request.Input.Material), len(request.Input.Asset))
			return err
		}
		result, response, err := JobCli.SubmitJob(c.Context(), request)
		if response != nil && response.StatusCode() == 409 {
			return fmt.Errorf("submission key conflicts with different parameters")
		}
		if err != nil {
			// Generated Hertz clients discard rawResponse on HTTP errors and
			// wrap status_code/body as JSON. Inspect only the numeric status;
			// never return that error string or its upstream body to the user.
			var failure struct {
				Status int `json:"status_code"`
			}
			if json.Unmarshal([]byte(err.Error()), &failure) == nil && failure.Status == 409 {
				return fmt.Errorf("submission key conflicts with different parameters")
			}
			return fmt.Errorf("submit outcome unconfirmed; query execution or reuse the same idempotency key, do not create a new key automatically")
		}
		if response == nil || response.StatusCode() != 200 || result.GetJobId() == "" {
			return fmt.Errorf("gateway did not acknowledge a Job")
		}
		_, err = fmt.Fprintf(c.OutOrStdout(), "submitted job_id=%s status=%s\n", result.GetJobId(), result.GetStatus())
		return err
	}
	return c
}

// Use authorized public resource reads, explicit selections and signed URLs.
// Identity comes from gateway credentials, never from a caller-supplied User ID.
func buildSubmitRequest(ctx context.Context, s submitSelection) (*jobpb.SubmitJobReq, error) {
	if s.idempotencyKey != "" {
		if len(s.idempotencyKey) > 160 {
			return nil, fmt.Errorf("invalid submission key")
		}
		for _, ch := range []byte(s.idempotencyKey) {
			if ch < 33 || ch > 126 {
				return nil, fmt.Errorf("invalid submission key")
			}
		}
	}
	if s.organization == "" || s.workspace == "" || s.workflow == "" || s.version < 0 {
		return nil, fmt.Errorf("organization, workspace, workflow and a nonnegative version are required")
	}
	if WfCli == nil || JobCli == nil || ProdCli == nil || VipCli == nil || MatCli == nil || ObjectCli == nil || AssetCli == nil {
		return nil, fmt.Errorf("Workflow/Materials clients are not configured")
	}
	ctx = context.WithValue(ctx, materialOrganizationKey{}, s.organization)
	wf, _, err := WfCli.GetWorkflow(ctx, &wfpb.GetWorkflowReq{Id: s.workflow})
	if err != nil {
		return nil, fmt.Errorf("workflow read failed")
	}
	if int32(wf.GetStatus()) != 2 || wf.GetVersion() < 1 {
		return nil, fmt.Errorf("workflow must be published")
	}
	if s.version != 0 && s.version != int(wf.GetVersion()) {
		return nil, fmt.Errorf("selected version does not match the current published input schema")
	}
	entries := map[string]*wfpb.InputSchemaEntry{}
	for _, entry := range wf.GetInputSchema().GetEntries() {
		entries[entry.Name] = entry
	}
	for name, id := range s.materials {
		if id == "" || entries[name] == nil || entries[name].ResourceType != "MATERIAL" {
			return nil, fmt.Errorf("invalid Material input selection")
		}
	}
	for name, id := range s.assets {
		if id == "" || entries[name] == nil || entries[name].ResourceType != "ASSET" {
			return nil, fmt.Errorf("invalid Generated Asset input selection")
		}
	}
	for _, entry := range entries {
		selected := false
		switch entry.ResourceType {
		case "PRODUCT":
			selected = s.product != ""
		case "VIRTUAL_IP":
			selected = s.virtualIP != ""
		case "MATERIAL":
			selected = s.materials[entry.Name] != ""
		case "ASSET":
			selected = s.assets[entry.Name] != ""
		default:
			return nil, fmt.Errorf("unsupported workflow input type")
		}
		if entry.Required && !selected {
			return nil, fmt.Errorf("required input %s is not selected", entry.Name)
		}
	}
	resolve := func(path string) (string, error) {
		if path == "" {
			return "", nil
		}
		resolved, _, err := ObjectCli.ResolveLibraryObject(ctx, &objectpb.ResolveLibraryObjectReq{OrganizationId: s.organization, ObjectPath: path})
		if err != nil || resolved.GetUrl() == "" {
			return "", fmt.Errorf("selected media object is not accessible")
		}
		return resolved.GetUrl(), nil
	}
	input := &jobpb.JobInput{}
	if s.product != "" {
		p, _, err := ProdCli.GetProduct(ctx, &prodpb.GetProductReq{Id: s.product, OrganizationId: s.organization})
		if err != nil || p.GetOrganizationId() != s.organization || p.GetScope() != wf.GetScope() {
			return nil, fmt.Errorf("selected Product is outside the workflow scope or inaccessible")
		}
		path := ""
		for _, image := range p.GetImages() {
			if path == "" || image.GetIsMain() {
				path = image.GetObjectPath()
			}
			if image.GetIsMain() {
				break
			}
		}
		imageURL, err := resolve(path)
		if err != nil {
			return nil, err
		}
		input.Product = &jobpb.ProductData{Id: p.Id, Name: p.Name, TargetAudience: p.TargetAudience, ProductDetails: p.ProductDetails, DisplayActions: p.DisplayActions, ImageUrl: imageURL}
	}
	if s.virtualIP != "" {
		v, _, err := VipCli.GetVirtualIp(ctx, &vippb.GetVirtualIpReq{Id: s.virtualIP, OrganizationId: s.organization})
		if err != nil || v.GetOrganizationId() != s.organization || v.GetScope() != wf.GetScope() {
			return nil, fmt.Errorf("selected Virtual IP is outside the workflow scope or inaccessible")
		}
		urls := make([]string, 4)
		for i, path := range []string{v.AvatarObjectPath, v.FullBodyObjectPath, v.ThreeViewObjectPath, v.NineViewObjectPath} {
			urls[i], err = resolve(path)
			if err != nil {
				return nil, err
			}
		}
		input.VirtualIp = &jobpb.VirtualIpData{Id: v.Id, Name: v.Name, AvatarUrl: urls[0], FullBodyUrl: urls[1], ThreeViewUrl: urls[2], NineViewUrl: urls[3]}
	}
	names := make([]string, 0, len(s.materials)+len(s.assets))
	for name := range s.materials {
		names = append(names, name)
	}
	for name := range s.assets {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry := entries[name]
		if id := s.materials[name]; id != "" {
			m, _, err := MatCli.GetMaterial(ctx, &matpb.GetMaterialReq{Id: id, OrganizationId: s.organization})
			if err != nil || m.GetOrganizationId() != s.organization || m.GetScope() != wf.GetScope() || m.GetAssetKind() != entry.AssetKind {
				return nil, fmt.Errorf("selected Material does not match input %s", name)
			}
			resourceURL, err := resolve(m.ObjectPath)
			if err != nil {
				return nil, err
			}
			content := m.Content
			if content == "" {
				content = m.Prompt
			}
			if content == "" {
				content = m.Description
			}
			if resourceURL == "" && content == "" {
				return nil, fmt.Errorf("selected Material input %s has no media or content", name)
			}
			input.Material = append(input.Material, &jobpb.MaterialData{Name: name, AssetKind: m.AssetKind, Url: resourceURL, Content: content})
		} else {
			a, _, err := AssetCli.GetGeneratedAsset(ctx, &assetpb.GetGeneratedAssetReq{Id: s.assets[name]})
			if err != nil || a.GetScope() != wf.GetScope() || a.GetAssetKind() != entry.AssetKind {
				return nil, fmt.Errorf("selected Generated Asset does not match input %s", name)
			}
			resourceURL, err := resolve(a.ObjectPath)
			if err != nil {
				return nil, err
			}
			if resourceURL == "" && a.Content == "" {
				return nil, fmt.Errorf("selected Generated Asset input %s has no media or content", name)
			}
			input.Asset = append(input.Asset, &jobpb.AssetData{Name: name, AssetKind: a.AssetKind, Url: resourceURL, Content: a.Content})
		}
	}
	return &jobpb.SubmitJobReq{WorkflowId: s.workflow, WorkflowVersion: wf.Version, Input: input, OrganizationId: s.organization, WorkspaceId: s.workspace, IdempotencyKey: s.idempotencyKey}, nil
}
