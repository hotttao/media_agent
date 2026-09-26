BEGIN;

ALTER TABLE media_workflow.templates
    ADD COLUMN IF NOT EXISTS capability_key text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS capability_version int NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS invocation_mapping jsonb NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE media_workflow.template_versions
    ADD COLUMN IF NOT EXISTS capability_key text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS capability_version int NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS invocation_mapping jsonb NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE media_workflow.template_inputs
    ADD COLUMN IF NOT EXISTS asset_kind text;
UPDATE media_workflow.template_inputs
SET asset_kind = name
WHERE asset_kind IS NULL OR asset_kind = '';
ALTER TABLE media_workflow.template_inputs
    ALTER COLUMN asset_kind SET NOT NULL;

CREATE TABLE IF NOT EXISTS media_workflow.execution_capabilities (
    capability_key      text NOT NULL,
    version             int NOT NULL,
    provider            text NOT NULL,
    mode                text NOT NULL,
    input_schema        jsonb NOT NULL,
    output_asset_kind   text NOT NULL,
    conductor_task_def  text NOT NULL,
    retry_policy        jsonb NOT NULL DEFAULT '{"retry_count":3,"retry_logic":"FIXED","retry_delay_seconds":1}'::jsonb,
    timeout_seconds     int NOT NULL DEFAULT 600,
    status              text NOT NULL DEFAULT 'published' CHECK (status IN ('draft','published','archived')),
    created_at          timestamptz NOT NULL DEFAULT now(),
    published_at        timestamptz,
    PRIMARY KEY (capability_key, version),
    UNIQUE (conductor_task_def)
);

CREATE TABLE IF NOT EXISTS media_workflow.workflow_compilations (
    id                    text PRIMARY KEY,
    workflow_id           text NOT NULL REFERENCES media_workflow.workflows(id) ON DELETE CASCADE,
    workflow_version      int NOT NULL,
    conductor_name        text NOT NULL,
    conductor_version     int NOT NULL,
    ir_json               jsonb NOT NULL,
    definition_json       jsonb NOT NULL,
    definition_hash       text NOT NULL,
    status                text NOT NULL CHECK (status IN ('pending','registering','registered','failed')),
    error_message         text,
    created_at            timestamptz NOT NULL DEFAULT now(),
    registered_at         timestamptz,
    UNIQUE (workflow_id, workflow_version),
    UNIQUE (workflow_id, workflow_version, definition_hash)
);

ALTER TABLE media_workflow.jobs
    ADD COLUMN IF NOT EXISTS conductor_workflow_id text,
    ADD COLUMN IF NOT EXISTS compilation_hash text;

INSERT INTO media_workflow.execution_capabilities
    (capability_key, version, provider, mode, input_schema, output_asset_kind, conductor_task_def, status, published_at)
VALUES
    ('media.image.model', 1, 'client', 'image_generation',
     '[{"name":"prompt","type":"string","required":true},{"name":"resources","type":"resource_ref_list","required":false}]',
     'model_image', 'media_image_model_v1', 'published', now()),
    ('media.image.style', 1, 'client', 'image_generation',
     '[{"name":"prompt","type":"string","required":true},{"name":"resources","type":"resource_ref_list","required":false}]',
     'style_image', 'media_image_style_v1', 'published', now()),
    ('media.image.first_frame', 1, 'client', 'image_generation',
     '[{"name":"prompt","type":"string","required":true},{"name":"resources","type":"resource_ref_list","required":false}]',
     'first_frame', 'media_image_first_frame_v1', 'published', now()),
    ('media.video.generate', 1, 'client', 'video_generation',
     '[{"name":"prompt","type":"string","required":true},{"name":"resources","type":"resource_ref_list","required":false}]',
     'ai_video', 'media_video_generate_v1', 'published', now()),
    ('media.video.clip', 1, 'client', 'video_editing',
     '[{"name":"prompt","type":"string","required":true},{"name":"resources","type":"resource_ref_list","required":true}]',
     'final_video', 'media_video_clip_v1', 'published', now()),
    ('media.content.ai_fill', 1, 'client', 'content_generation',
     '[{"name":"prompt","type":"string","required":true},{"name":"resources","type":"resource_ref_list","required":true}]',
     'final_video', 'media_content_ai_fill_v1', 'published', now()),
    ('media.publish.feishu', 1, 'client', 'publishing',
     '[{"name":"prompt","type":"string","required":true},{"name":"resources","type":"resource_ref_list","required":true}]',
     'final_video', 'media_publish_feishu_v1', 'published', now())
ON CONFLICT (capability_key, version) DO NOTHING;

COMMIT;
