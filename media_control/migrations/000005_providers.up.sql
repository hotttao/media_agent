CREATE TABLE IF NOT EXISTS providers (
    id text PRIMARY KEY,
    name text NOT NULL,
    provider_type text NOT NULL,
    endpoint text NOT NULL,
    capabilities jsonb NOT NULL,
    status text NOT NULL DEFAULT 'active',
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT providers_status_check CHECK (status IN ('active', 'disabled'))
);
