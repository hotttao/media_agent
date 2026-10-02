package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	texttemplate "text/template"
	"time"

	templatedomain "media_agent/media_workflow/biz/domain/template"
)

const workflowName = "media_ai · 图片与动作视频生成（联调样本）"

type config struct {
	baseURL           string
	scope             string
	workflowName      string
	sessionCookieName string
	sessionCookie     string
	sessionStatePath  string
	bearerToken       string
	archiveWorkflowID string
	dryRun            bool
}

type client struct {
	config config
	http   *http.Client
}

type resourceRef struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

type templateRecord struct {
	resourceRef
	Name              string                           `json:"name"`
	PromptBody        string                           `json:"prompt_body"`
	OutputAssetKind   string                           `json:"output_asset_kind"`
	CapabilityKey     string                           `json:"capability_key"`
	InvocationMapping templatedomain.InvocationMapping `json:"invocation_mapping"`
	Status            int                              `json:"status"`
}

type templateSpec struct {
	Key             string
	Name            string
	Prompt          string
	OutputAssetKind string
	CapabilityKey   string
	Slots           []map[string]any
	Invocation      map[string]any
}

func main() {
	var cfg config
	flag.StringVar(&cfg.baseURL, "target-url", env("MEDIA_WORKFLOW_URL", "http://127.0.0.1:18080"), "统一网关地址")
	flag.StringVar(&cfg.scope, "scope", env("MEDIA_WORKFLOW_SCOPE", "ai_clothing"), "目标 Scope")
	flag.StringVar(&cfg.workflowName, "workflow-name", workflowName, "样本 Workflow 名称；重建发布时使用新名称，保留旧版本")
	flag.StringVar(&cfg.sessionCookieName, "session-cookie-name", env("MEDIA_WORKFLOW_SESSION_COOKIE_NAME", "ory_kratos_session"), "会话 cookie 名")
	flag.StringVar(&cfg.sessionCookie, "session-cookie", env("MEDIA_E2E_KRATOS_SESSION_COOKIE", ""), "会话 cookie")
	flag.StringVar(&cfg.sessionStatePath, "session-state", "", "Playwright storage-state.json 路径（从中读取会话 Cookie）")
	flag.StringVar(&cfg.bearerToken, "bearer-token", env("MEDIA_WORKFLOW_BEARER_TOKEN", ""), "Bearer token")
	flag.StringVar(&cfg.archiveWorkflowID, "archive-workflow-id", "", "新 Workflow 发布成功后归档旧联调 Workflow ID")
	promptDir := flag.String("prompt-dir", env("GPT_IMAGE2_PROMPT_DIR", "/home/tao/code/github/gpt_image2/prompts"), "gpt_image2 提示词目录")
	flag.BoolVar(&cfg.dryRun, "dry-run", false, "只输出迁移定义")
	flag.Parse()
	cfg.baseURL = strings.TrimRight(cfg.baseURL, "/")
	if cfg.sessionCookie == "" && cfg.sessionStatePath != "" {
		var err error
		cfg.sessionCookie, err = sessionCookieFromState(cfg.sessionStatePath, cfg.sessionCookieName)
		if err != nil {
			log.Fatalf("读取浏览器会话状态: %v", err)
		}
	}
	if !cfg.dryRun && cfg.sessionCookie == "" && cfg.bearerToken == "" {
		log.Fatal("需要 MEDIA_E2E_KRATOS_SESSION_COOKIE、--session-cookie 或 --bearer-token")
	}

	specs, err := fixtureTemplates(*promptDir)
	if err != nil {
		log.Fatalf("读取 gpt_image2 提示词: %v", err)
	}
	if cfg.dryRun {
		encoded, _ := json.MarshalIndent(map[string]any{"templates": specs, "workflow": cfg.workflowName}, "", "  ")
		fmt.Println(string(encoded))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c := &client{config: cfg, http: &http.Client{Timeout: 30 * time.Second}}
	templates := make(map[string]resourceRef, len(specs))
	for _, spec := range specs {
		ref, err := c.createAndPublishTemplate(ctx, spec)
		if err != nil {
			log.Fatalf("迁移 Template %s: %v", spec.Key, err)
		}
		templates[spec.Key] = ref
		log.Printf("Template ready: %s -> %s@%d", spec.Key, ref.ID, ref.Version)
	}
	workflow, err := c.createAndPublishWorkflow(ctx, templates)
	if err != nil {
		log.Fatalf("迁移 Workflow: %v", err)
	}
	if cfg.archiveWorkflowID != "" {
		if cfg.archiveWorkflowID == workflow.ID {
			log.Fatal("旧 Workflow ID 不得与新 Workflow ID 相同")
		}
		if err := c.json(ctx, http.MethodPost, "/api/v1/workflows/"+url.PathEscape(cfg.archiveWorkflowID)+"/archive", nil, nil); err != nil {
			log.Fatalf("新 Workflow %s 已发布，但旧 Workflow %s 归档失败: %v", workflow.ID, cfg.archiveWorkflowID, err)
		}
	}
	encoded, _ := json.Marshal(map[string]any{"workflow_id": workflow.ID, "workflow_version": workflow.Version, "name": cfg.workflowName})
	fmt.Println(string(encoded))
}

func sessionCookieFromState(path, name string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var state struct {
		Cookies []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"cookies"`
	}
	if err := json.Unmarshal(contents, &state); err != nil {
		return "", err
	}
	for _, cookie := range state.Cookies {
		if cookie.Name == name && cookie.Value != "" {
			return cookie.Value, nil
		}
	}
	return "", fmt.Errorf("会话状态中没有 %s Cookie", name)
}

func fixtureTemplates(promptDir string) ([]templateSpec, error) {
	readPrompt := func(name string) (string, error) {
		contents, err := os.ReadFile(filepath.Join(promptDir, name))
		if err != nil {
			return "", fmt.Errorf("%s: %w", name, err)
		}
		return strings.TrimSpace(string(contents)), nil
	}
	modelPrompt, err := readPrompt("03_模特图.md")
	if err != nil {
		return nil, err
	}
	stylePrompt, err := readPrompt("04_定妆图.md")
	if err != nil {
		return nil, err
	}
	firstFramePrompt, err := readPrompt("05_首帧图.md")
	if err != nil {
		return nil, err
	}
	videoPrompt, err := readPrompt("06_文生视频提示词.md")
	if err != nil {
		return nil, err
	}
	// gpt_image2 用字符串替换“动作细节”；本服务用 Go Template 渲染槽位上下文。
	videoPrompt = strings.ReplaceAll(videoPrompt, "{{ 动作细节 }}", "{{.movement.content}}")
	if videoPrompt == "" {
		return nil, errors.New("06_文生视频提示词.md 为空")
	}
	for _, prompt := range []string{modelPrompt, stylePrompt, firstFramePrompt, videoPrompt} {
		if _, err := texttemplate.New("prompt").Option("missingkey=error").Parse(prompt); err != nil {
			return nil, fmt.Errorf("提示词不符合模板语法: %w", err)
		}
	}
	asset := func(name, kind string) map[string]any {
		return map[string]any{"name": name, "asset_kind": kind, "resource_type": "ASSET", "required": true}
	}
	material := func(name, kind string) map[string]any {
		return map[string]any{"name": name, "asset_kind": kind, "resource_type": "MATERIAL", "required": true}
	}
	container := func(name, resourceType, kind string) map[string]any {
		return map[string]any{"name": name, "asset_kind": kind, "resource_type": resourceType, "required": true}
	}
	invocation := func(slots ...string) map[string]any {
		items := make([]map[string]any, 0, len(slots))
		for _, slot := range slots {
			items = append(items, map[string]any{"op": "asset_ref", "slot": slot})
		}
		return map[string]any{
			"prompt":    map[string]any{"op": "render_template"},
			"resources": map[string]any{"op": "list", "items": items},
		}
	}
	return []templateSpec{
		{Key: "model", Name: "media_ai · 生成模特图", Prompt: modelPrompt, OutputAssetKind: "model_image", CapabilityKey: "media.image.model", Slots: []map[string]any{container("virtual_ip", "VIRTUAL_IP", "virtual_ip"), container("product", "PRODUCT", "product")}, Invocation: invocation("virtual_ip", "product")},
		{Key: "style", Name: "media_ai · 生成定妆图", Prompt: stylePrompt, OutputAssetKind: "style_image", CapabilityKey: "media.image.style", Slots: []map[string]any{asset("model", "model_image"), material("pose", "pose")}, Invocation: invocation("model", "pose")},
		{Key: "first_frame", Name: "media_ai · 生成首帧", Prompt: firstFramePrompt, OutputAssetKind: "first_frame", CapabilityKey: "media.image.first_frame", Slots: []map[string]any{asset("style", "style_image"), material("scene", "scene")}, Invocation: invocation("style", "scene")},
		{Key: "video", Name: "media_ai · 生成动作视频", Prompt: videoPrompt, OutputAssetKind: "ai_video", CapabilityKey: "media.video.generate", Slots: []map[string]any{asset("first_frame", "first_frame"), material("movement", "movement")}, Invocation: invocation("first_frame")},
	}, nil
}

func (c *client) createAndPublishTemplate(ctx context.Context, spec templateSpec) (resourceRef, error) {
	var listed struct {
		Items []templateRecord `json:"items"`
	}
	query := url.Values{"scope": {c.config.scope}, "name": {spec.Name}, "page_size": {"200"}}
	if err := c.json(ctx, http.MethodGet, "/api/v1/templates?"+query.Encode(), nil, &listed); err != nil {
		return resourceRef{}, err
	}
	payload := map[string]any{
		"name": spec.Name, "description": "从 media_ai scripts/workflow-cli 迁移的单样本联调模板",
		"prompt_body": spec.Prompt, "inputs": spec.Slots, "output_asset_kind": spec.OutputAssetKind,
		"platform": "client", "gen_way": spec.CapabilityKey, "gen_type": "media_ai_fixture",
		"needs_confirm": false, "candidate_count": 1,
		"capability_key": spec.CapabilityKey, "capability_version": 1, "invocation_mapping": spec.Invocation,
	}
	for _, existing := range listed.Items {
		if existing.Name != spec.Name {
			continue
		}
		if existing.Status == 3 {
			return resourceRef{}, fmt.Errorf("Template %q 已归档，不能复用", spec.Name)
		}
		if existing.PromptBody == spec.Prompt && existing.OutputAssetKind == spec.OutputAssetKind && existing.CapabilityKey == spec.CapabilityKey && sameInvocationMapping(existing.InvocationMapping, spec.Invocation) {
			if existing.Status == 1 {
				if err := c.json(ctx, http.MethodPost, "/api/v1/templates/"+url.PathEscape(existing.ID)+"/publish", nil, nil); err != nil {
					return resourceRef{}, err
				}
			}
			return existing.resourceRef, nil
		}
		var updated resourceRef
		if err := c.json(ctx, http.MethodPut, "/api/v1/templates/"+url.PathEscape(existing.ID), payload, &updated); err != nil {
			return resourceRef{}, err
		}
		if updated.Version <= existing.Version {
			return resourceRef{}, fmt.Errorf("Template %q 更新后没有递增版本", spec.Name)
		}
		if existing.Status == 1 {
			if err := c.json(ctx, http.MethodPost, "/api/v1/templates/"+url.PathEscape(existing.ID)+"/publish", nil, nil); err != nil {
				return resourceRef{}, err
			}
		}
		return resourceRef{ID: existing.ID, Version: updated.Version}, nil
	}
	var created resourceRef
	if err := c.json(ctx, http.MethodPost, "/api/v1/templates?scope="+url.QueryEscape(c.config.scope), payload, &created); err != nil {
		return resourceRef{}, err
	}
	if created.ID == "" {
		return resourceRef{}, errors.New("CreateTemplate 响应缺少 id")
	}
	var published map[string]any
	if err := c.json(ctx, http.MethodPost, "/api/v1/templates/"+url.PathEscape(created.ID)+"/publish", nil, &published); err != nil {
		return resourceRef{}, err
	}
	return created, nil
}

func sameInvocationMapping(current templatedomain.InvocationMapping, proposed map[string]any) bool {
	encoded, err := json.Marshal(proposed)
	if err != nil {
		return false
	}
	var want templatedomain.InvocationMapping
	if err := json.Unmarshal(encoded, &want); err != nil {
		return false
	}
	gotJSON, err := json.Marshal(current)
	if err != nil {
		return false
	}
	wantJSON, err := json.Marshal(want)
	return err == nil && bytes.Equal(gotJSON, wantJSON)
}

func (c *client) createAndPublishWorkflow(ctx context.Context, templates map[string]resourceRef) (resourceRef, error) {
	binding := func(source int, ref string) map[string]any { return map[string]any{"source": source, "ref": ref} }
	step := func(key, templateKey string, order int, bindings map[string]any) map[string]any {
		ref := templates[templateKey]
		return map[string]any{"key": key, "name": key, "step_order": order, "template_id": ref.ID, "template_version": ref.Version, "slot_bindings": bindings, "approval_policy": "when_multiple_candidates", "reuse_policy": "force_new"}
	}
	steps := []map[string]any{
		step("model_image", "model", 1, map[string]any{"virtual_ip": binding(1, "virtual_ip"), "product": binding(1, "product")}),
		step("style_image", "style", 2, map[string]any{"model": binding(2, "model_image"), "pose": binding(1, "pose")}),
		step("first_frame", "first_frame", 3, map[string]any{"style": binding(2, "style_image"), "scene": binding(1, "scene")}),
		step("video", "video", 4, map[string]any{"first_frame": binding(2, "first_frame"), "movement": binding(1, "movement")}),
	}
	name := c.config.workflowName
	if name == "" {
		name = workflowName
	}
	payload := map[string]any{"name": name, "description": "media_ai 图片与动作视频生成流程；不包含成片、发布文案与飞书发布", "output_step_key": "video", "steps": steps}
	var created resourceRef
	if err := c.json(ctx, http.MethodPost, "/api/v1/workflows?scope="+url.QueryEscape(c.config.scope), payload, &created); err != nil {
		return resourceRef{}, err
	}
	if created.ID == "" {
		return resourceRef{}, errors.New("CreateWorkflow 响应缺少 id")
	}
	var published map[string]any
	if err := c.json(ctx, http.MethodPost, "/api/v1/workflows/"+url.PathEscape(created.ID)+"/publish", nil, &published); err != nil {
		return resourceRef{}, err
	}
	return created, nil
}

func (c *client) json(ctx context.Context, method, endpoint string, payload, result any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.config.baseURL+endpoint, body)
	if err != nil {
		return err
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.config.sessionCookie != "" {
		request.AddCookie(&http.Cookie{Name: c.config.sessionCookieName, Value: c.config.sessionCookie})
	}
	if c.config.bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+c.config.bearerToken)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s %s: HTTP %d: %s", method, endpoint, response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	if result != nil && len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, result); err != nil {
			return err
		}
	}
	return nil
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
