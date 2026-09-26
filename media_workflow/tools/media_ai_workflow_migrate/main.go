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
	"strings"
	"time"
)

const workflowName = "media_ai · 完整带货视频生成（联调样本）"

type config struct {
	baseURL           string
	scope             string
	sessionCookieName string
	sessionCookie     string
	bearerToken       string
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
	flag.StringVar(&cfg.sessionCookieName, "session-cookie-name", env("MEDIA_WORKFLOW_SESSION_COOKIE_NAME", "ory_kratos_session"), "会话 cookie 名")
	flag.StringVar(&cfg.sessionCookie, "session-cookie", env("MEDIA_E2E_KRATOS_SESSION_COOKIE", ""), "会话 cookie")
	flag.StringVar(&cfg.bearerToken, "bearer-token", env("MEDIA_WORKFLOW_BEARER_TOKEN", ""), "Bearer token")
	flag.BoolVar(&cfg.dryRun, "dry-run", false, "只输出迁移定义")
	flag.Parse()
	cfg.baseURL = strings.TrimRight(cfg.baseURL, "/")
	if !cfg.dryRun && cfg.sessionCookie == "" && cfg.bearerToken == "" {
		log.Fatal("需要 MEDIA_E2E_KRATOS_SESSION_COOKIE、--session-cookie 或 --bearer-token")
	}

	specs := fixtureTemplates()
	if cfg.dryRun {
		encoded, _ := json.MarshalIndent(map[string]any{"templates": specs, "workflow": workflowName}, "", "  ")
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
	encoded, _ := json.Marshal(map[string]any{"workflow_id": workflow.ID, "workflow_version": workflow.Version, "name": workflowName})
	fmt.Println(string(encoded))
}

func fixtureTemplates() []templateSpec {
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
		{Key: "model", Name: "media_ai · 生成模特图", Prompt: "生成模特商品图；人物={{.virtual_ip.asset_ref}}；商品={{.product.asset_ref}}", OutputAssetKind: "model_image", CapabilityKey: "media.image.model", Slots: []map[string]any{container("virtual_ip", "VIRTUAL_IP", "virtual_ip"), container("product", "PRODUCT", "product")}, Invocation: invocation("virtual_ip", "product")},
		{Key: "style", Name: "media_ai · 生成定妆图", Prompt: "生成定妆图；模特={{.model.asset_ref}}；姿势={{.pose.asset_ref}}", OutputAssetKind: "style_image", CapabilityKey: "media.image.style", Slots: []map[string]any{asset("model", "model_image"), material("pose", "pose")}, Invocation: invocation("model", "pose")},
		{Key: "first_frame", Name: "media_ai · 生成首帧", Prompt: "生成视频首帧；定妆图={{.style.asset_ref}}；场景={{.scene.asset_ref}}", OutputAssetKind: "first_frame", CapabilityKey: "media.image.first_frame", Slots: []map[string]any{asset("style", "style_image"), material("scene", "scene")}, Invocation: invocation("style", "scene")},
		{Key: "video", Name: "media_ai · 生成动作视频", Prompt: "生成动作视频；首帧={{.first_frame.asset_ref}}；动作={{.movement.asset_ref}}", OutputAssetKind: "ai_video", CapabilityKey: "media.video.generate", Slots: []map[string]any{asset("first_frame", "first_frame"), material("movement", "movement")}, Invocation: invocation("first_frame", "movement")},
		{Key: "clip", Name: "media_ai · 剪辑成片", Prompt: "把两段动作视频剪辑为完整成片", OutputAssetKind: "final_video", CapabilityKey: "media.video.clip", Slots: []map[string]any{asset("video_a", "ai_video"), asset("video_b", "ai_video")}, Invocation: invocation("video_a", "video_b")},
		{Key: "ai_fill", Name: "media_ai · 生成发布文案", Prompt: "根据成片生成标题和发布文案；视频={{.final_video.asset_ref}}", OutputAssetKind: "final_video", CapabilityKey: "media.content.ai_fill", Slots: []map[string]any{asset("final_video", "final_video")}, Invocation: invocation("final_video")},
		{Key: "publish", Name: "media_ai · 发布到飞书", Prompt: "发布已完成的视频与文案", OutputAssetKind: "final_video", CapabilityKey: "media.publish.feishu", Slots: []map[string]any{asset("final_video", "final_video"), asset("publish_copy", "final_video")}, Invocation: invocation("final_video", "publish_copy")},
	}
}

func (c *client) createAndPublishTemplate(ctx context.Context, spec templateSpec) (resourceRef, error) {
	payload := map[string]any{
		"name": spec.Name, "description": "从 media_ai scripts/workflow-cli 迁移的单样本联调模板",
		"prompt_body": spec.Prompt, "inputs": spec.Slots, "output_asset_kind": spec.OutputAssetKind,
		"platform": "client", "gen_way": spec.CapabilityKey, "gen_type": "media_ai_fixture",
		"needs_confirm": false, "candidate_count": 1,
		"capability_key": spec.CapabilityKey, "capability_version": 1, "invocation_mapping": spec.Invocation,
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
		step("video_a", "video", 4, map[string]any{"first_frame": binding(2, "first_frame"), "movement": binding(1, "movement_a")}),
		step("video_b", "video", 5, map[string]any{"first_frame": binding(2, "first_frame"), "movement": binding(1, "movement_b")}),
		step("clip", "clip", 6, map[string]any{"video_a": binding(2, "video_a"), "video_b": binding(2, "video_b")}),
		step("ai_fill", "ai_fill", 7, map[string]any{"final_video": binding(2, "clip")}),
		step("publish", "publish", 8, map[string]any{"final_video": binding(2, "clip"), "publish_copy": binding(2, "ai_fill")}),
	}
	payload := map[string]any{"name": workflowName, "description": "media_ai 七阶段完整视频生成流程；本轮只迁移一个联调样本", "output_step_key": "publish", "steps": steps}
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
