BEGIN;

CREATE TABLE media_workflow.historical_video_pushes (
    id text PRIMARY KEY,
    organization_id text NOT NULL,
    workspace_id text NOT NULL,
    owner_user_id text NOT NULL,
    source_system text NOT NULL DEFAULT 'media_ai',
    source_record_id text NOT NULL,
    source_digest text NOT NULL,
    product_id text NOT NULL,
    virtual_ip_id text NOT NULL,
    source_scene_id text NOT NULL DEFAULT '',
    template_name text NOT NULL DEFAULT '',
    title text NOT NULL DEFAULT '',
    content text NOT NULL DEFAULT '',
    source_status text NOT NULL DEFAULT '',
    is_qualified boolean NOT NULL DEFAULT false,
    is_published boolean NOT NULL DEFAULT false,
    video_object_path text,
    video_sha256 text,
    thumbnail_object_path text,
    thumbnail_sha256 text,
    manual_clip_object_path text,
    manual_clip_sha256 text,
    source_created_at timestamptz NOT NULL,
    source_updated_at timestamptz NOT NULL,
    imported_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT historical_video_pushes_source_digest_check CHECK (source_digest ~ '^[0-9a-f]{64}$'),
    CONSTRAINT historical_video_pushes_video_pair_check CHECK ((video_object_path IS NULL AND video_sha256 IS NULL) OR (video_object_path IS NOT NULL AND video_object_path <> '' AND video_sha256 ~ '^[0-9a-f]{64}$')),
    CONSTRAINT historical_video_pushes_thumbnail_pair_check CHECK ((thumbnail_object_path IS NULL AND thumbnail_sha256 IS NULL) OR (thumbnail_object_path IS NOT NULL AND thumbnail_object_path <> '' AND thumbnail_sha256 ~ '^[0-9a-f]{64}$')),
    CONSTRAINT historical_video_pushes_manual_clip_pair_check CHECK ((manual_clip_object_path IS NULL AND manual_clip_sha256 IS NULL) OR (manual_clip_object_path IS NOT NULL AND manual_clip_object_path <> '' AND manual_clip_sha256 ~ '^[0-9a-f]{64}$')),
    UNIQUE (workspace_id, source_system, source_record_id)
);

CREATE INDEX historical_video_pushes_workspace_idx ON media_workflow.historical_video_pushes (workspace_id, source_created_at DESC);

CREATE TABLE media_workflow.historical_video_push_members (
    push_id text NOT NULL REFERENCES media_workflow.historical_video_pushes(id) ON DELETE CASCADE,
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    source_video_id text NOT NULL,
    job_id text NOT NULL REFERENCES media_workflow.jobs(id),
    PRIMARY KEY (push_id, ordinal),
    UNIQUE (push_id, source_video_id),
    UNIQUE (push_id, job_id)
);

COMMIT;
