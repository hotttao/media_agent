CREATE TABLE IF NOT EXISTS platform_settings (
    key text PRIMARY KEY,
    value jsonb NOT NULL,
    version bigint NOT NULL DEFAULT 1,
    updated_by text NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT platform_settings_version_check CHECK (version > 0)
);

CREATE TABLE IF NOT EXISTS platform_setting_outbox (
    id uuid PRIMARY KEY,
    setting_key text NOT NULL REFERENCES platform_settings(key) ON DELETE CASCADE,
    version bigint NOT NULL,
    event_type text NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    delivered_at timestamptz,
    CONSTRAINT platform_setting_outbox_event_type_check CHECK (event_type = 'platform_setting.changed'),
    UNIQUE (setting_key, version)
);

CREATE INDEX IF NOT EXISTS idx_platform_setting_outbox_pending
    ON platform_setting_outbox (created_at) WHERE delivered_at IS NULL;
