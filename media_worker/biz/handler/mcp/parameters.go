package mcp

import contract "media_agent/media_worker/biz/shared/media_management"

type slot struct {
	Name         string `json:"name"`
	ResourceType string `json:"resource_type"`
	AssetKind    string `json:"asset_kind"`
	Required     bool   `json:"required"`
	DisplayOrder int    `json:"display_order"`
}
type expression struct {
	Op    string       `json:"op"`
	Slot  string       `json:"slot"`
	Items []expression `json:"items"`
}

func (e expression) command() contract.Expression {
	v := contract.Expression{Op: e.Op, Slot: e.Slot}
	for _, i := range e.Items {
		v.Items = append(v.Items, i.command())
	}
	return v
}

type binding struct {
	Source     int    `json:"source"`
	Ref        string `json:"ref"`
	ConstValue string `json:"const_value"`
}
type step struct {
	Key             string             `json:"key"`
	Name            string             `json:"name"`
	TemplateID      string             `json:"template_id"`
	TemplateVersion int                `json:"template_version"`
	Order           int                `json:"step_order"`
	ApprovalPolicy  string             `json:"approval_policy"`
	ReusePolicy     string             `json:"reuse_policy"`
	Bindings        map[string]binding `json:"slot_bindings"`
}
type mediaInput struct {
	Name       string `json:"name"`
	AssetKind  string `json:"asset_kind"`
	ObjectPath string `json:"object_path"`
	Content    string `json:"content"`
}
type jobInput struct {
	ProductID   string       `json:"product_id"`
	VirtualIPID string       `json:"virtual_ip_id"`
	Assets      []mediaInput `json:"asset"`
	Materials   []mediaInput `json:"material"`
}
type parameters struct {
	ID                string                `json:"id"`
	Scope             string                `json:"scope"`
	Name              string                `json:"name"`
	Description       string                `json:"description"`
	Visibility        string                `json:"visibility"`
	Status            string                `json:"status"`
	Page              int                   `json:"page"`
	PageSize          int                   `json:"page_size"`
	Version           int                   `json:"version"`
	PromptBody        string                `json:"prompt_body"`
	OutputAssetKind   string                `json:"output_asset_kind"`
	CapabilityKey     string                `json:"capability_key"`
	CapabilityVersion int                   `json:"capability_version"`
	CandidateCount    int                   `json:"candidate_count"`
	NeedsConfirm      bool                  `json:"needs_confirm"`
	Inputs            []slot                `json:"inputs"`
	InvocationMapping map[string]expression `json:"invocation_mapping"`
	Steps             []step                `json:"steps"`
	OutputStepKey     string                `json:"output_step_key"`
	WorkflowID        string                `json:"workflow_id"`
	WorkflowVersion   int                   `json:"workflow_version"`
	IdempotencyKey    string                `json:"idempotency_key"`
	Input             jobInput              `json:"input"`
	Provider          string                `json:"provider"`
	GenerationMethod  string                `json:"generation_method"`
	GenerationType    string                `json:"generation_type"`
}

func (p parameters) command() contract.Params {
	c := contract.Params{ID: p.ID, Scope: p.Scope, Name: p.Name, Description: p.Description, Visibility: p.Visibility, Status: p.Status, Page: p.Page, PageSize: p.PageSize, Version: p.Version, PromptBody: p.PromptBody, OutputAssetKind: p.OutputAssetKind, CapabilityKey: p.CapabilityKey, CapabilityVersion: p.CapabilityVersion, CandidateCount: p.CandidateCount, NeedsConfirm: p.NeedsConfirm, OutputStepKey: p.OutputStepKey, WorkflowID: p.WorkflowID, WorkflowVersion: p.WorkflowVersion, IdempotencyKey: p.IdempotencyKey, Provider: p.Provider, GenerationMethod: p.GenerationMethod, GenerationType: p.GenerationType, InvocationMapping: map[string]contract.Expression{}, Input: contract.JobInput{ProductID: p.Input.ProductID, VirtualIPID: p.Input.VirtualIPID}}
	if c.Page == 0 {
		c.Page = 1
	}
	if c.PageSize == 0 {
		c.PageSize = 20
	}
	for _, i := range p.Inputs {
		c.Inputs = append(c.Inputs, contract.Slot{Name: i.Name, ResourceType: i.ResourceType, AssetKind: i.AssetKind, Required: i.Required, DisplayOrder: i.DisplayOrder})
	}
	for k, v := range p.InvocationMapping {
		c.InvocationMapping[k] = v.command()
	}
	for _, s := range p.Steps {
		v := contract.Step{Key: s.Key, Name: s.Name, TemplateID: s.TemplateID, TemplateVersion: s.TemplateVersion, Order: s.Order, ApprovalPolicy: s.ApprovalPolicy, ReusePolicy: s.ReusePolicy, Bindings: map[string]contract.Binding{}}
		for k, b := range s.Bindings {
			v.Bindings[k] = contract.Binding{Source: b.Source, Ref: b.Ref, ConstValue: b.ConstValue}
		}
		c.Steps = append(c.Steps, v)
	}
	for _, i := range p.Input.Assets {
		c.Input.Assets = append(c.Input.Assets, contract.MediaInput{Name: i.Name, AssetKind: i.AssetKind, ObjectPath: i.ObjectPath, Content: i.Content})
	}
	for _, i := range p.Input.Materials {
		c.Input.Materials = append(c.Input.Materials, contract.MediaInput{Name: i.Name, AssetKind: i.AssetKind, ObjectPath: i.ObjectPath, Content: i.Content})
	}
	return c
}
