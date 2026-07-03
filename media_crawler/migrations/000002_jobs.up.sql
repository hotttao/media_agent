CREATE TABLE "media-crawler".jobs (
    job_id         VARCHAR(64) PRIMARY KEY,
    platform       VARCHAR(32) NOT NULL,
    crawler_type   VARCHAR(32) NOT NULL,
    params         JSONB,
    status         VARCHAR(32) NOT NULL DEFAULT 'queued',
    pid            INTEGER,
    checkpoint_id  VARCHAR(128),
    progress       JSONB,
    result_summary JSONB,
    error          TEXT,
    stdout_tail    TEXT,
    queued_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at     TIMESTAMPTZ,
    finished_at    TIMESTAMPTZ
);

CREATE INDEX idx_jobs_status    ON "media-crawler".jobs(status);
CREATE INDEX idx_jobs_platform  ON "media-crawler".jobs(platform);
CREATE INDEX idx_jobs_queued_at ON "media-crawler".jobs(queued_at DESC);
