CREATE TABLE IF NOT EXISTS cluster_projections (
    cluster_id text PRIMARY KEY,
    status text NOT NULL,
    capacity jsonb NOT NULL,
    observed_at timestamptz NOT NULL,
    CONSTRAINT cluster_projections_status_check CHECK (status IN ('online','degraded','offline','unknown'))
);

CREATE INDEX IF NOT EXISTS idx_cluster_projections_observed
    ON cluster_projections (observed_at DESC);
