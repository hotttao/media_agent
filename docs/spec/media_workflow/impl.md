# media_workflow 实现规格

> 模板执行引擎。存储 Template / Workflow / Job，接收自包含的 JobInput bundle，渲染 prompt，由外部 worker pull StepRun 执行。**不依赖 media_materials**——execution 只读 bundle + step 产出，零 materials 调用（仅 StepRun 产出时调一次 UpsertGeneratedAsset）。
>
> 领域规则见 `docs/media_workflow/domain/*` 与 `docs/media_workflow/workflow/*`；本文是落地规格（IDL + DB + 关键行为）。

## 1. 边界

- **本服务拥有**：template / workflow / job / step_run 的存储与执行。
- **不拥有**：material / generated_asset / product / virtual_ip / asset_kind（归 media_materials）。
- **id 风格**：text（uuid/nanoid，service 生成），对齐 media_ai。
- **依赖**：media_materials 仅在 workflow 校验期调 `BatchGetAssetKind`（校验 slot.name / output_asset_kind 已注册）；execution 期不调。

## 2. IDL

### 2.1 template.proto

```protobuf
syntax = "proto3";
package media_workflow.template;
import "api.proto";
import "google/api/annotations.proto";

enum TemplateStatus { TEMPLATE_STATUS_UNSPECIFIED = 0; DRAFT = 1; PUBLISHED = 2; ARCHIVED = 3; }
enum ResourceType { RESOURCE_TYPE_UNSPECIFIED = 0; MATERIAL = 1; ASSET = 2; PRODUCT = 3; VIRTUAL_IP = 4; TEXT = 5; }

message TemplateInputSlot {
  string name = 1;                  // prompt_body 占位符；MATERIAL/ASSET 时 = 类型(asset_kind)
  ResourceType resource_type = 2;
  bool required = 3;
  int32 display_order = 4;
}

message Template {
  string id = 1;
  string scope = 2;
  string name = 3;
  string description = 4;
  string prompt_body = 5;
  repeated TemplateInputSlot inputs = 6;
  string output_asset_kind = 7;     // → asset_kinds.name，resource_type=ASSET
  string platform = 8;
  string gen_way = 9;
  string gen_type = 10;
  TemplateStatus status = 11;
  string visibility = 12;           // private/team/public
  int32 version = 13;
  string created_by = 14;
  int64 created_at = 15;
  int64 updated_at = 16;
  bool needs_confirm = 17;          // 产出是否需用户确认（IMAGE/VIDEO 默认 true，TEXT 默认 false）
  int32 candidate_count = 18;       // 一次生成候选数；needs_confirm=true 时 N>1
}

message CreateTemplateReq {
  string scope = 1 [(api.query)="scope"];
  string name = 2 [(api.body)="name"];
  string description = 3 [(api.body)="description"];
  string prompt_body = 4 [(api.body)="prompt_body"];
  repeated TemplateInputSlot inputs = 5 [(api.body)="inputs"];
  string output_asset_kind = 6 [(api.body)="output_asset_kind"];
  string platform = 7 [(api.body)="platform"];
  string gen_way = 8 [(api.body)="gen_way"];
  string gen_type = 9 [(api.body)="gen_type"];
  bool needs_confirm = 10 [(api.body)="needs_confirm"];
  int32 candidate_count = 11 [(api.body)="candidate_count"];
  string created_by = 12 [(api.query)="created_by"];
}
message CreateTemplateResp { string id = 1; int32 version = 2; }

message UpdateTemplateReq {
  string id = 1 [(api.path)="id"];
  string name = 2 [(api.body)="name"];
  string description = 3 [(api.body)="description"];
  string prompt_body = 4 [(api.body)="prompt_body"];
  repeated TemplateInputSlot inputs = 5 [(api.body)="inputs"];
  string output_asset_kind = 6 [(api.body)="output_asset_kind"];
  string platform = 7 [(api.body)="platform"];
  string gen_way = 8 [(api.body)="gen_way"];
  string gen_type = 9 [(api.body)="gen_type"];
}
message UpdateTemplateResp { int32 version = 1; }

message GetTemplateReq { string id = 1 [(api.path)="id"]; }
message GetTemplateVersionReq { string id = 1 [(api.path)="id"]; int32 version = 2 [(api.path)="version"]; }
message ListTemplatesReq {
  string scope = 1 [(api.query)="scope"];
  string name = 2 [(api.query)="name"];
  string visibility = 3 [(api.query)="visibility"];
  int32 page = 4 [(api.query)="page"];
  int32 page_size = 5 [(api.query)="page_size"];
}
message ListTemplatesResp { repeated Template items = 1; int32 total = 2; }

message GrantReq { string id = 1 [(api.path)="id"]; string team_id = 2 [(api.body)="team_id"]; string permission = 3 [(api.body)="permission"]; }
message RevokeReq { string id = 1 [(api.path)="id"]; string team_id = 2 [(api.path)="team_id"]; }
message ChangeVisibilityReq { string id = 1 [(api.path)="id"]; string visibility = 2 [(api.body)="visibility"]; }

service TemplateService {
  rpc CreateTemplate(CreateTemplateReq) returns (CreateTemplateResp) { option (api.post) = "/api/v1/templates"; }
  rpc UpdateTemplate(UpdateTemplateReq) returns (UpdateTemplateResp) { option (api.put) = "/api/v1/templates/:id"; }
  rpc GetTemplate(GetTemplateReq) returns (Template) { option (api.get) = "/api/v1/templates/:id"; }
  rpc GetTemplateVersion(GetTemplateVersionReq) returns (Template) { option (api.get) = "/api/v1/templates/:id/versions/:version"; }
  rpc ListTemplates(ListTemplatesReq) returns (ListTemplatesResp) { option (api.get) = "/api/v1/templates"; }
  rpc ArchiveTemplate(GetTemplateReq) returns (Template) { option (api.post) = "/api/v1/templates/:id/archive"; }
  rpc PublishTemplate(GetTemplateReq) returns (Template) { option (api.post) = "/api/v1/templates/:id/publish"; }
  rpc GrantTemplate(GrantReq) returns (Template) { option (api.post) = "/api/v1/templates/:id/grants"; }
  rpc RevokeTemplate(RevokeReq) returns (Template) { option (api.delete) = "/api/v1/templates/:id/grants/:team_id"; }
  rpc ChangeVisibility(ChangeVisibilityReq) returns (Template) { option (api.post) = "/api/v1/templates/:id/visibility"; }
}
```

> Render / ParseSlotSpec 是内部 RPC（被 workflow_execution / 前端预览调），不在 HTTP 暴露；走 service-to-service。

### 2.2 workflow.proto

```protobuf
syntax = "proto3";
package media_workflow.workflow;
import "api.proto";
import "media_workflow/template.proto";  // ResourceType / TemplateStatus 复用

enum WorkflowStatus { WORKFLOW_STATUS_UNSPECIFIED = 0; DRAFT = 1; PUBLISHED = 2; ARCHIVED = 3; }
enum WorkflowInputSource { WIS_UNSPECIFIED = 0; WORKFLOW_INPUT = 1; STEP_OUTPUT = 2; CONST = 3; }

message WorkflowInputBinding {
  WorkflowInputSource source = 1;
  string ref = 2;          // STEP_OUTPUT → step.key；其余空
  string const_value = 3;  // CONST 时字面值
}
message WorkflowStep {
  string key = 1;
  string name = 2;
  int32 step_order = 3;
  string template_id = 4;
  int32 template_version = 5;
  map<string, WorkflowInputBinding> input_bindings = 6;  // key = slot.name
}
message InputSchemaEntry {
  string name = 1;
  template.ResourceType resource_type = 2;
  bool required = 3;
}
message WorkflowInputSchema { repeated InputSchemaEntry entries = 1; }

message Workflow {
  string id = 1;
  string scope = 2;
  string name = 3;
  string description = 4;
  string output_step_key = 5;
  repeated WorkflowStep steps = 6;
  WorkflowStatus status = 7;
  string visibility = 8;
  int32 version = 9;
  WorkflowInputSchema input_schema = 10;  // 派生快照
  string created_by = 11;
  int64 created_at = 12;
  int64 updated_at = 13;
}

message CreateWorkflowReq {
  string scope = 1 [(api.query)="scope"];
  string name = 2 [(api.body)="name"];
  string description = 3 [(api.body)="description"];
  string output_step_key = 4 [(api.body)="output_step_key"];
  repeated WorkflowStep steps = 5 [(api.body)="steps"];
  string created_by = 6 [(api.query)="created_by"];
}
message CreateWorkflowResp { string id = 1; int32 version = 2; }
message UpdateWorkflowReq {
  string id = 1 [(api.path)="id"];
  string name = 2 [(api.body)="name"];
  string description = 3 [(api.body)="description"];
  string output_step_key = 4 [(api.body)="output_step_key"];
  repeated WorkflowStep steps = 5 [(api.body)="steps"];
}
message UpdateWorkflowResp { int32 version = 1; }

message ValidateWorkflowReq {
  string scope = 1 [(api.body)="scope"];
  string output_step_key = 2 [(api.body)="output_step_key"];
  repeated WorkflowStep steps = 3 [(api.body)="steps"];
}
message DerivedEdge { string from = 1; string to = 2; string to_input = 3; }
message RejectReason { string code = 1; string message = 2; string subject = 3; }
message ValidationResult {
  bool valid = 1;
  repeated string roots = 2;
  string output_step_key = 3;
  repeated DerivedEdge derived_edges = 4;
  WorkflowInputSchema input_schema = 5;
  repeated RejectReason reject_reasons = 6;
}

// 通用入参（{id} / {id, version} / 分页）—— 跨 service 复用
message GetByIdReq { string id = 1 [(api.path)="id"]; }
message ListWorkflowsReq {
  string scope = 1 [(api.query)="scope"];
  string name = 2 [(api.query)="name"];
  string status = 3 [(api.query)="status"];
  string visibility = 4 [(api.query)="visibility"];
  int32 page = 5; int32 page_size = 6;
}
message ListWorkflowsResp { repeated Workflow items = 1; int32 total = 2; }

service WorkflowService {
  rpc CreateWorkflow(CreateWorkflowReq) returns (CreateWorkflowResp) { option (api.post) = "/api/v1/workflows"; }
  rpc UpdateWorkflow(UpdateWorkflowReq) returns (UpdateWorkflowResp) { option (api.put) = "/api/v1/workflows/:id"; }
  rpc GetWorkflow(GetByIdReq) returns (Workflow) { option (api.get) = "/api/v1/workflows/:id"; }
  rpc ListWorkflows(ListWorkflowsReq) returns (ListWorkflowsResp) { option (api.get) = "/api/v1/workflows"; }
  rpc ArchiveWorkflow(GetByIdReq) returns (Workflow) { option (api.post) = "/api/v1/workflows/:id/archive"; }
  rpc PublishWorkflow(GetByIdReq) returns (Workflow) { option (api.post) = "/api/v1/workflows/:id/publish"; }
  rpc ValidateWorkflow(ValidateWorkflowReq) returns (ValidationResult) { option (api.post) = "/api/v1/workflows/validate"; }
  rpc GrantWorkflow(GrantReq) returns (Workflow) { option (api.post) = "/api/v1/workflows/:id/grants"; }
  rpc RevokeWorkflow(RevokeReq) returns (Workflow) { option (api.delete) = "/api/v1/workflows/:id/grants/:team_id"; }
  rpc ChangeWorkflowVisibility(ChangeVisibilityReq) returns (Workflow) { option (api.post) = "/api/v1/workflows/:id/visibility"; }
}
```

### 2.3 job.proto

```protobuf
syntax = "proto3";
package media_workflow.job;
import "api.proto";

// —— 自包含 bundle：caller 从 media_materials 解析后填入 ——
message MaterialData { string name = 1; string url = 2; string content = 3; }   // IMAGE/VIDEO→url, TEXT→content
message AssetData { string name = 1; string url = 2; string content = 3; }       // generated_asset
message ProductData { string id = 1; string name = 2; string target_audience = 3; string product_details = 4; string display_actions = 5; }
message VirtualIpData { string id = 1; string name = 2; string avatar_url = 3; string full_body_url = 4; string three_view_url = 5; string nine_view_url = 6; }

message JobInput {
  ProductData product = 1;            // 可 null
  VirtualIpData virtual_ip = 2;       // 可 null
  repeated AssetData asset = 3;       // name 唯一
  repeated MaterialData material = 4; // name 唯一
}

enum JobStatus {
  JOB_STATUS_UNSPECIFIED = 0; PENDING = 1; RUNNING = 2; NEED_CONFIRM = 3;
  SUCCEEDED = 4; FAILED = 5; CANCELLED = 6;
}
enum StepRunStatus {
  SR_UNSPECIFIED = 0; BLOCKED = 1; PENDING = 2; CLAIMED = 3;
  NEED_CONFIRM = 4; SUCCEEDED = 5; FAILED = 6; SKIPPED = 7;
}

// 候选产物（worker 一次生成 N 个，用户选 1）
message Candidate {
  int32 index = 1;           // 1-based
  string url = 2;            // IMAGE/VIDEO
  string content = 3;        // TEXT
  bool is_primary = 4;       // worker 标记主图（非 confirm 时自动取 primary）
}

message StepRun {
  string id = 1;
  string job_id = 2;
  string step_key = 3;
  int32 step_order = 4;
  string template_id = 5;
  int32 template_version = 6;
  StepRunStatus status = 7;
  int32 retry_count = 8;
  string resolved_inputs = 9;          // jsonb: {slot_name: {resource_type,id,url,content,...}}
  string rendered_prompt = 10;
  repeated Candidate candidates = 11;  // NEED_CONFIRM 时的 N 候选
  int32 selected_index = 12;           // 用户选定的候选 index
  string output_asset_id = 13;         // → generated_assets.id（选定后落库）
  string failure_reason = 14;
  string claimed_by = 15;              // worker 标识
  int64 claimed_at = 16;
  int64 started_at = 17;
  int64 finished_at = 18;
}

message Job {
  string id = 1;
  string scope = 2;
  string workflow_id = 3;
  int32 workflow_version = 4;
  JobInput input = 5;
  JobStatus status = 6;
  int32 retry_count = 7;
  string retry_history = 8;            // jsonb
  string submitted_by = 9;
  int64 submitted_at = 10;
  int64 started_at = 11;
  int64 finished_at = 12;
  string failure_reason = 13;
}

// —— SubmitJob ——
message SubmitJobReq {
  string workflow_id = 1 [(api.query)="workflow_id"];
  int32 workflow_version = 2 [(api.body)="workflow_version"];
  JobInput input = 3 [(api.body)="input"];
  string submitted_by = 4 [(api.query)="submitted_by"];
}
message SubmitJobResp { string job_id = 1; JobStatus status = 2; }
message GetJobReq { string id = 1 [(api.path)="id"]; }
message GetJobResp { Job job = 1; repeated StepRun step_runs = 2; }
message ListJobsReq {
  string scope = 1 [(api.query)="scope"]; string workflow_id = 2 [(api.query)="workflow_id"];
  string status = 3 [(api.query)="status"]; int32 page = 4; int32 page_size = 5;
}
message ListJobsResp { repeated Job items = 1; int32 total = 2; }

// —— worker 侧：claim / complete / fail ——
message ClaimStepRunReq {
  string platform = 1 [(api.query)="platform"];
  string gen_way = 2 [(api.query)="gen_way"];
  string gen_type = 3 [(api.query)="gen_type"];
  string worker_id = 4 [(api.query)="worker_id"];
}
message ClaimAsset { string label = 1; string url = 2; string content = 3; }  // 输入 asset（slot 的 url/content）
message ClaimStepRunResp {
  string step_run_id = 1;
  string job_id = 2;
  string rendered_prompt = 3;
  repeated ClaimAsset input_assets = 4;   // 渲染后的输入 asset
  string platform = 5;
  string gen_way = 6;
  string gen_type = 7;
  string asset_kind = 8;                  // = template.output_asset_kind
  int32 candidate_count = 9;              // worker 据此生成 N 候选
}
message CompleteStepRunReq {
  string step_run_id = 1 [(api.path)="step_run_id"];
  repeated Candidate candidates = 2 [(api.body)="candidates"];
}
message FailStepRunReq {
  string step_run_id = 1 [(api.path)="step_run_id"];
  string reason = 2 [(api.body)="reason"];
}

// —— 用户侧：confirm / regenerate ——
message ConfirmStepRunReq {
  string step_run_id = 1 [(api.path)="step_run_id"];
  int32 selected_index = 2 [(api.body)="selected_index"];
}
message RegenerateStepRunReq { string step_run_id = 1 [(api.path)="step_run_id"]; }

// —— retry ——
message RetryJobReq { string id = 1 [(api.path)="id"]; string reason = 2 [(api.body)="reason"]; JobInput input_override = 3 [(api.body)="input_override"]; }
message RetryStepRunReq { string step_run_id = 1 [(api.path)="step_run_id"]; string reason = 2 [(api.body)="reason"]; JobInput input_override = 3 [(api.body)="input_override"]; }
message GetStepRunReq { string step_run_id = 1 [(api.path)="step_run_id"]; }

// —— SSE 事件 ——
enum EventType {
  ET_UNSPECIFIED = 0;
  ET_JOB_STATUS = 1;          // job 状态变更
  ET_STEP_STATUS = 2;         // step_run 状态变更
  ET_STEP_NEED_CONFIRM = 3;   // step 待确认（带 candidates）
}
message JobEvent {
  string job_id = 1;
  int64 version = 2;          // 单调递增，前端按 (job_id, version) 幂等
  EventType type = 3;
  int64 timestamp = 4;
  string payload = 5;         // jsonb：status_change / {step_run_id, candidates[]} 等
}

service JobService {
  // 用户 / 前端
  rpc SubmitJob(SubmitJobReq) returns (SubmitJobResp) { option (api.post) = "/api/v1/jobs"; }
  rpc GetJob(GetJobReq) returns (GetJobResp) { option (api.get) = "/api/v1/jobs/:id"; }
  rpc ListJobs(ListJobsReq) returns (ListJobsResp) { option (api.get) = "/api/v1/jobs"; }
  rpc CancelJob(GetJobReq) returns (Job) { option (api.post) = "/api/v1/jobs/:id/cancel"; }
  rpc RetryJob(RetryJobReq) returns (Job) { option (api.post) = "/api/v1/jobs/:id/retry"; }
  rpc ConfirmStepRun(ConfirmStepRunReq) returns (StepRun) { option (api.post) = "/api/v1/step-runs/:step_run_id/confirm"; }
  rpc RegenerateStepRun(RegenerateStepRunReq) returns (StepRun) { option (api.post) = "/api/v1/step-runs/:step_run_id/regenerate"; }
  rpc RetryStepRun(RetryStepRunReq) returns (StepRun) { option (api.post) = "/api/v1/step-runs/:step_run_id/retry"; }
  rpc GetStepRun(GetStepRunReq) returns (StepRun) { option (api.get) = "/api/v1/step-runs/:step_run_id"; }
  // worker
  rpc ClaimStepRun(ClaimStepRunReq) returns (ClaimStepRunResp) { option (api.post) = "/api/v1/step-runs/claim"; }
  rpc CompleteStepRun(CompleteStepRunReq) returns (StepRun) { option (api.post) = "/api/v1/step-runs/:step_run_id/complete"; }
  rpc FailStepRun(FailStepRunReq) returns (StepRun) { option (api.post) = "/api/v1/step-runs/:step_run_id/fail"; }
  // SSE
  rpc StreamJobEvents(GetJobReq) returns (stream JobEvent) { option (api.get) = "/api/v1/jobs/:id/events"; }
}
```

> worker 不做认证（按需求）。ClaimStepRun 无可领任务时返回 404。

## 3. 数据库

> 迁移工具 golang-migrate，SQL-first，版本表 `schema_migrations`。id = text(uuid)。

### 3.1 templates / template_inputs / template_versions / template_grants

```sql
CREATE TABLE templates (
  id text PRIMARY KEY,
  scope text NOT NULL,
  name text NOT NULL,
  description text,
  prompt_body text NOT NULL,
  output_asset_kind text NOT NULL,
  platform text NOT NULL,
  gen_way text NOT NULL,
  gen_type text,
  needs_confirm bool NOT NULL DEFAULT false,
  candidate_count int NOT NULL DEFAULT 1,
  status text NOT NULL DEFAULT 'draft',       -- draft/published/archived
  visibility text NOT NULL DEFAULT 'private', -- private/team/public
  version int NOT NULL DEFAULT 1,
  created_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE UNIQUE INDEX uk_templates_scope_name_version ON templates (scope, name, version) WHERE deleted_at IS NULL;
CREATE INDEX idx_templates_scope_status ON templates (scope, status);
CREATE INDEX idx_templates_created_by ON templates (created_by);
CREATE INDEX idx_templates_output_kind ON templates (output_asset_kind);

-- 当前版本的 input slots
CREATE TABLE template_inputs (
  id text PRIMARY KEY,
  template_id text NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
  version int NOT NULL,                        -- 与 templates.version 对齐
  name text NOT NULL,
  resource_type text NOT NULL,                 -- MATERIAL/ASSET/PRODUCT/VIRTUAL_IP/TEXT
  required bool NOT NULL DEFAULT false,
  display_order int NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uk_template_inputs ON template_inputs (template_id, version, name);
CREATE INDEX idx_template_inputs_lookup ON template_inputs (template_id, version);

-- 历史版本快照（update 时追加；workflow 按 (template_id, version) 查）
CREATE TABLE template_versions (
  id text PRIMARY KEY,
  template_id text NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
  version int NOT NULL,
  prompt_body text NOT NULL,
  output_asset_kind text NOT NULL,
  platform text NOT NULL,
  gen_way text NOT NULL,
  gen_type text,
  needs_confirm bool NOT NULL DEFAULT false,
  candidate_count int NOT NULL DEFAULT 1,
  inputs_snapshot jsonb NOT NULL,              -- [{name, resource_type, required, display_order}]
  created_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uk_template_versions ON template_versions (template_id, version);

CREATE TABLE template_grants (
  template_id text NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
  team_id text NOT NULL,
  permission text NOT NULL CHECK (permission IN ('viewer','editor')),
  granted_by text NOT NULL,
  granted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (template_id, team_id)
);
CREATE INDEX idx_template_grants_team ON template_grants (team_id);
```

### 3.2 workflows / workflow_steps / step_bindings / workflow_grants

```sql
CREATE TABLE workflows (
  id text PRIMARY KEY,
  scope text NOT NULL,
  name text NOT NULL,
  description text,
  output_step_key text NOT NULL,
  status text NOT NULL DEFAULT 'draft',
  visibility text NOT NULL DEFAULT 'private',
  version int NOT NULL DEFAULT 1,
  input_schema jsonb NOT NULL DEFAULT '[]',    -- 派生快照 [{name, resource_type, required}]
  created_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE UNIQUE INDEX uk_workflows_scope_name_version ON workflows (scope, name, version) WHERE deleted_at IS NULL;
CREATE INDEX idx_workflows_scope_status ON workflows (scope, status, updated_at);
CREATE INDEX idx_workflows_created_by ON workflows (created_by);

CREATE TABLE workflow_steps (
  id text PRIMARY KEY,
  workflow_id text NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
  version int NOT NULL,
  step_key text NOT NULL,
  step_name text,
  step_order int NOT NULL,
  template_id text NOT NULL,
  template_version int NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uk_workflow_steps_key ON workflow_steps (workflow_id, version, step_key);
CREATE UNIQUE INDEX uk_workflow_steps_order ON workflow_steps (workflow_id, version, step_order);
CREATE INDEX idx_workflow_steps_template ON workflow_steps (template_id, template_version);

CREATE TABLE step_bindings (
  id text PRIMARY KEY,
  step_id text NOT NULL REFERENCES workflow_steps(id) ON DELETE CASCADE,
  to_input text NOT NULL,                      -- slot.name
  source text NOT NULL CHECK (source IN ('WORKFLOW_INPUT','STEP_OUTPUT','CONST')),
  ref text,                                    -- STEP_OUTPUT → step.key
  const_value text,                            -- CONST 字面值
  display_order int NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (
    (source = 'WORKFLOW_INPUT' AND ref IS NULL     AND const_value IS NULL) OR
    (source = 'STEP_OUTPUT'    AND ref IS NOT NULL AND const_value IS NULL) OR
    (source = 'CONST'          AND const_value IS NOT NULL AND ref IS NULL)
  )
);
CREATE UNIQUE INDEX uk_step_bindings ON step_bindings (step_id, to_input);

CREATE TABLE workflow_grants (
  workflow_id text NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
  team_id text NOT NULL,
  permission text NOT NULL CHECK (permission IN ('viewer','editor')),
  granted_by text NOT NULL,
  granted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (workflow_id, team_id)
);
CREATE INDEX idx_workflow_grants_team ON workflow_grants (team_id);
```

### 3.3 jobs / step_runs

```sql
CREATE TABLE jobs (
  id text PRIMARY KEY,
  scope text NOT NULL,
  workflow_id text NOT NULL,
  workflow_version int NOT NULL,
  input jsonb NOT NULL,                        -- JobInput bundle 快照
  status text NOT NULL DEFAULT 'pending',      -- pending/running/need_confirm/succeeded/failed/cancelled
  retry_count int NOT NULL DEFAULT 0,
  retry_history jsonb NOT NULL DEFAULT '[]',   -- [{retry_at, reason, input_override?}]
  submitted_by text NOT NULL,
  submitted_at timestamptz NOT NULL DEFAULT now(),
  started_at timestamptz,
  finished_at timestamptz,
  failure_reason text
);
CREATE INDEX idx_jobs_scope_status ON jobs (scope, status, submitted_at DESC);
CREATE INDEX idx_jobs_workflow ON jobs (workflow_id, workflow_version);
CREATE INDEX idx_jobs_submitted_by ON jobs (submitted_by, submitted_at DESC);

CREATE TABLE step_runs (
  id text PRIMARY KEY,
  job_id text NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  step_key text NOT NULL,
  step_order int NOT NULL,
  template_id text NOT NULL,
  template_version int NOT NULL,
  status text NOT NULL DEFAULT 'blocked',     -- blocked/pending/claimed/need_confirm/succeeded/failed/skipped
  retry_count int NOT NULL DEFAULT 0,
  resolved_inputs jsonb NOT NULL,              -- {slot_name: {resource_type,id,url,content,...}}
  rendered_prompt text,
  candidates jsonb NOT NULL DEFAULT '[]',      -- [{index,url,content,is_primary}] NEED_CONFIRM 时 N 候选
  selected_index int,                          -- 用户选定的候选
  output_asset_id text,                        -- → media_materials.generated_assets.id
  failure_reason text,
  claimed_by text,
  claimed_at timestamptz,
  started_at timestamptz,
  finished_at timestamptz
);
CREATE UNIQUE INDEX uk_step_runs ON step_runs (job_id, step_key);
CREATE INDEX idx_step_runs_claim ON step_runs (status, claimed_at);  -- CLAIMED 超时扫描
CREATE INDEX idx_step_runs_job_status ON step_runs (job_id, status);
```

## 4. 关键行为

### 4.1 Workflow 校验（闸 1，Create/Update/Validate）
按 `docs/media_workflow/workflow/0001-workflow_validation.md` 的 A–E 阶段执行：DAG 结构 → slot/binding → input_schema 派生快照 → template 引用 + 跨资源 ACL → scope → asset_kind 注册（调 media_materials.BatchGetAssetKind）。失败 → 不写库，返回 reject_reasons。

### 4.2 SubmitJob 校验（闸 2）
bundle ↔ workflow.input_schema：required 全覆盖、material[]/asset[] name 唯一、无多余类型、PRODUCT/VIRTUAL_IP 非空（如 required）。通过后创建 Job（PENDING）+ StepRuns：root step = PENDING，非 root = BLOCKED。Job → RUNNING。

### 4.3 Execution（pull 模式 + NEED_CONFIRM）

**media_workflow 是协调器 + 渲染器，不是执行器**。外部 worker 主动拉 StepRun，跑完回报。

```
SubmitJob
  → Job RUNNING；root StepRun = PENDING，其余 = BLOCKED
  → SSE: ET_JOB_STATUS(running)

worker 轮询 ClaimStepRun(platform, gen_way, gen_type)
  → 找一个 PENDING 且 template.platform/gen_way/gen_type 匹配的 StepRun
  → 加载 Job.input(bundle) + template(version)
  → 按 binding.source 解析 slot 值（WORKFLOW_INPUT 从 bundle 按 slot.name 取；STEP_OUTPUT 从前驱 step_run.output_asset_id 取；CONST 取 const_value）
  → 渲染 prompt（{{.name.url}}/{{.name.content}} 替换）
  → 返回 rendered_prompt + input_assets + candidate_count
  → StepRun: PENDING → CLAIMED（记 claimed_by/claimed_at）
  → SSE: ET_STEP_STATUS(claimed)

worker 真生成（调 GPT/即梦，产 N 候选）→ 上传拿 urls
  → CompleteStepRun(step_run_id, candidates[N])
    → needs_confirm=true：存 candidates；StepRun → NEED_CONFIRM；Job → NEED_CONFIRM
       → SSE: ET_STEP_NEED_CONFIRM(candidates)  前端弹确认
    → needs_confirm=false：取 primary candidate；落 UpsertGeneratedAsset → output_asset_id；
       StepRun → SUCCEEDED；推进 DAG（下游 BLOCKED→PENDING）；Job → RUNNING
       → SSE: ET_STEP_STATUS(succeeded) + ET_JOB_STATUS(running)

用户确认（NEED_CONFIRM）
  → ConfirmStepRun(step_run_id, selected_index)
    → 取选定 candidate；落 UpsertGeneratedAsset → output_asset_id
    → StepRun → SUCCEEDED；推进 DAG；Job → RUNNING
    → SSE: ET_STEP_STATUS(succeeded) + ET_JOB_STATUS(running)
  或 RegenerateStepRun → StepRun → PENDING（重拉重生成）；Job → RUNNING
  或 CancelJob → Job CANCELLED

worker 失败 → FailStepRun → StepRun FAILED；Job FAILED
  → SSE: ET_STEP_STATUS(failed) + ET_JOB_STATUS(failed)

sink StepRun SUCCEEDED → Job SUCCEEDED → SSE: ET_JOB_STATUS(succeeded)
```

**DAG 推进**：StepRun → SUCCEEDED 时，检查所有下游 StepRun 的上游是否都 SUCCEEDED → 下游 BLOCKED → PENDING（可被 claim）。

**Claim 超时**：后台扫描 `status=claimed AND claimed_at < now()-10min` → 回 PENDING（worker 崩溃保护）。

**产物落库**：StepRun SUCCEEDED 时，拿选定 candidate 的 url/content 调 `media_materials.UpsertGeneratedAsset`（带 job_id/step_run_id/template_id/template_version/source_virtual_ip_id/source_product_id）→ output_asset_id。

### 4.4 Retry
仅 `Job.status=failed` 可 RetryJob；上限 MaxRetries（默认 3）。succeeded StepRun 保留；failed/skipped/need_confirm 重置（failed→pending, need_confirm→pending 重拉，skipped→blocked 重新评估）；retry_count+1，retry_history 追加。`input_override` 可覆盖 bundle 部分值。

### 4.5 单步能力 + Publish

- `RetryStepRun`（`POST /api/v1/step-runs/:step_run_id/retry`）：`failed/skipped -> pending` + `retry_count+1`，重跑该 step + 下游；`input_override` 可选覆盖 bundle 部分值。与 `RetryJob` 边界：前者单步+下游，后者整 Job。与 `RegenerateStepRun` 边界：前者 failed/skipped 修复重跑，后者 need_confirm 重新生成候选。详见 [workflow_execution](../../media_workflow/workflow/0002-workflow_execution/workflow_execution.md) §8。
- `GetStepRun`（`GET /api/v1/step-runs/:step_run_id`）：只读返回单条 step_run 状态 + resolved_inputs + rendered_prompt + output_asset_id + failure_reason。
- `PublishTemplate` / `PublishWorkflow`（`POST /:id/publish`）：`draft -> published`，owner；补全状态机 `draft ↔ published -> archived` 中 published 的产生路径。

## 5. 配置

```yaml
# conf/local.yaml
supported_scopes: [ai_clothing, ai_guoxue]   # 仅用于本服务资源的 scope 校验
job:
  max_retries: 3
  claim_timeout_seconds: 600                  # CLAIMED 超时回 PENDING
sse:
  event_retention_seconds: 3600               # Last-Event-ID 补推窗口
```

## 6. 与 media_materials 的契约（仅校验期 + 产出期）

| RPC | 调用时机 | 用途 |
|:---|:---|:---|
| `BatchGetAssetKind(names)` | Workflow 校验期 | 校验 slot.name / output_asset_kind 已注册、resource_type/media_type 匹配 |
| `UpsertGeneratedAsset(...)` | StepRun SUCCEEDED 时（CompleteStepRun / ConfirmStepRun） | 落 generated_asset + 生成记录 |

> SubmitJob 校验与 execution 渲染都**不调** media_materials 读取——bundle 自包含。仅在 StepRun 产出时调一次 UpsertGeneratedAsset。下游 worker 不经 media_workflow 访问 materials——worker 直接拿 bundle 渲染出的 url 生成，产物 url 由 worker 上传后回报。
