-- =====================================================================
-- media_workflow — 初始化 schema（template + workflow）
-- =====================================================================
-- 范围: template / template_inputs / template_versions / template_grants
--       workflows / workflow_steps / step_bindings / workflow_grants
-- job / step_runs 在 000002_job.up.sql（P3）
-- id: text（uuid/nanoid，service 生成）
-- =====================================================================

BEGIN;

CREATE SCHEMA IF NOT EXISTS media_workflow;

-- ─────────────────────────────────────────────────────────────────────
-- 1. templates —— 提示词模板（当前版本）
-- ─────────────────────────────────────────────────────────────────────
-- output_asset_kind → media_materials.asset_kinds.name (resource_type=ASSET)
-- needs_confirm: 产出是否需用户确认；candidate_count: 一次生成几个候选
-- 锁列: scope 不可改
CREATE TABLE IF NOT EXISTS media_workflow.templates (
    id                 text PRIMARY KEY,
    scope              text NOT NULL,
    name               text NOT NULL,
    description        text,
    prompt_body        text NOT NULL,
    output_asset_kind  text NOT NULL,
    platform           text NOT NULL,
    gen_way            text NOT NULL,
    gen_type           text,
    needs_confirm      boolean NOT NULL DEFAULT false,
    candidate_count    int NOT NULL DEFAULT 1,
    status             text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published','archived')),
    visibility         text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','team','public')),
    version            int NOT NULL DEFAULT 1,
    created_by         text NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    deleted_at         timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS templates_scope_name_version_idx
    ON media_workflow.templates (scope, name, version) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS templates_scope_status_idx
    ON media_workflow.templates (scope, status);
CREATE INDEX IF NOT EXISTS templates_created_by_idx
    ON media_workflow.templates (created_by);
CREATE INDEX IF NOT EXISTS templates_output_kind_idx
    ON media_workflow.templates (output_asset_kind);

-- ─────────────────────────────────────────────────────────────────────
-- 2. template_inputs —— 当前版本的 input slots
-- ─────────────────────────────────────────────────────────────────────
-- name = slot 名 + prompt_body 占位符；MATERIAL/ASSET 时 = asset_kind（类型）
CREATE TABLE IF NOT EXISTS media_workflow.template_inputs (
    id            text PRIMARY KEY,
    template_id   text NOT NULL REFERENCES media_workflow.templates(id) ON DELETE CASCADE,
    version       int NOT NULL,
    name          text NOT NULL,
    resource_type text NOT NULL CHECK (resource_type IN ('MATERIAL','ASSET','PRODUCT','VIRTUAL_IP','TEXT')),
    required      boolean NOT NULL DEFAULT false,
    display_order int NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS template_inputs_tpl_ver_name_idx
    ON media_workflow.template_inputs (template_id, version, name);
CREATE INDEX IF NOT EXISTS template_inputs_lookup_idx
    ON media_workflow.template_inputs (template_id, version);

-- ─────────────────────────────────────────────────────────────────────
-- 3. template_versions —— 历史版本快照（update 时追加；workflow 按 (template_id, version) 查）
-- ─────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS media_workflow.template_versions (
    id                text PRIMARY KEY,
    template_id       text NOT NULL REFERENCES media_workflow.templates(id) ON DELETE CASCADE,
    version           int NOT NULL,
    prompt_body       text NOT NULL,
    output_asset_kind text NOT NULL,
    platform          text NOT NULL,
    gen_way           text NOT NULL,
    gen_type          text,
    needs_confirm     boolean NOT NULL DEFAULT false,
    candidate_count   int NOT NULL DEFAULT 1,
    inputs_snapshot   jsonb NOT NULL,           -- [{name, resource_type, required, display_order}]
    created_by        text NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS template_versions_tpl_ver_idx
    ON media_workflow.template_versions (template_id, version);

-- ─────────────────────────────────────────────────────────────────────
-- 4. template_grants —— template 资源授权
-- ─────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS media_workflow.template_grants (
    template_id text NOT NULL REFERENCES media_workflow.templates(id) ON DELETE CASCADE,
    team_id     text NOT NULL,
    permission  text NOT NULL CHECK (permission IN ('viewer','editor')),
    granted_by  text NOT NULL,
    granted_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (template_id, team_id)
);
CREATE INDEX IF NOT EXISTS template_grants_team_idx
    ON media_workflow.template_grants (team_id);

-- ─────────────────────────────────────────────────────────────────────
-- 5. workflows —— 工作流（DAG）
-- ─────────────────────────────────────────────────────────────────────
-- input_schema: 派生快照 [{name, resource_type, required}]
CREATE TABLE IF NOT EXISTS media_workflow.workflows (
    id              text PRIMARY KEY,
    scope           text NOT NULL,
    name            text NOT NULL,
    description     text,
    output_step_key text NOT NULL,
    status          text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published','archived')),
    visibility      text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','team','public')),
    version         int NOT NULL DEFAULT 1,
    input_schema    jsonb NOT NULL DEFAULT '[]',
    created_by      text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS workflows_scope_name_version_idx
    ON media_workflow.workflows (scope, name, version) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS workflows_scope_status_idx
    ON media_workflow.workflows (scope, status, updated_at);
CREATE INDEX IF NOT EXISTS workflows_created_by_idx
    ON media_workflow.workflows (created_by);

-- ─────────────────────────────────────────────────────────────────────
-- 6. workflow_steps —— DAG 节点
-- ─────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS media_workflow.workflow_steps (
    id               text PRIMARY KEY,
    workflow_id      text NOT NULL REFERENCES media_workflow.workflows(id) ON DELETE CASCADE,
    version          int NOT NULL,
    step_key         text NOT NULL,
    step_name        text,
    step_order       int NOT NULL,
    template_id      text NOT NULL,
    template_version int NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS workflow_steps_key_idx
    ON media_workflow.workflow_steps (workflow_id, version, step_key);
CREATE UNIQUE INDEX IF NOT EXISTS workflow_steps_order_idx
    ON media_workflow.workflow_steps (workflow_id, version, step_order);
CREATE INDEX IF NOT EXISTS workflow_steps_template_idx
    ON media_workflow.workflow_steps (template_id, template_version);

-- ─────────────────────────────────────────────────────────────────────
-- 7. step_bindings —— step 输入绑定（source 三选一）
-- ─────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS media_workflow.step_bindings (
    id           text PRIMARY KEY,
    step_id      text NOT NULL REFERENCES media_workflow.workflow_steps(id) ON DELETE CASCADE,
    to_input     text NOT NULL,                      -- slot.name
    source       text NOT NULL CHECK (source IN ('WORKFLOW_INPUT','STEP_OUTPUT','CONST')),
    ref          text,                                -- STEP_OUTPUT → step.key
    const_value  text,                                -- CONST 字面值
    display_order int NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CHECK (
        (source = 'WORKFLOW_INPUT' AND ref IS NULL      AND const_value IS NULL) OR
        (source = 'STEP_OUTPUT'    AND ref IS NOT NULL  AND const_value IS NULL) OR
        (source = 'CONST'          AND const_value IS NOT NULL AND ref IS NULL)
    )
);
CREATE UNIQUE INDEX IF NOT EXISTS step_bindings_step_input_idx
    ON media_workflow.step_bindings (step_id, to_input);

-- ─────────────────────────────────────────────────────────────────────
-- 8. workflow_grants —— workflow 资源授权
-- ─────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS media_workflow.workflow_grants (
    workflow_id text NOT NULL REFERENCES media_workflow.workflows(id) ON DELETE CASCADE,
    team_id     text NOT NULL,
    permission  text NOT NULL CHECK (permission IN ('viewer','editor')),
    granted_by  text NOT NULL,
    granted_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workflow_id, team_id)
);
CREATE INDEX IF NOT EXISTS workflow_grants_team_idx
    ON media_workflow.workflow_grants (team_id);

COMMIT;
