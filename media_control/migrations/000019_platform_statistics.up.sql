CREATE TABLE IF NOT EXISTS platform_statistics (
    metric_key text NOT NULL,
    period_start timestamptz NOT NULL,
    dimensions jsonb NOT NULL,
    dimensions_hash char(64) NOT NULL,
    value numeric NOT NULL,
    refreshed_at timestamptz NOT NULL,
    PRIMARY KEY (metric_key, period_start, dimensions_hash)
);

CREATE INDEX IF NOT EXISTS idx_platform_statistics_period
    ON platform_statistics (period_start DESC, metric_key);
