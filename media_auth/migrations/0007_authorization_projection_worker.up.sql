BEGIN;

ALTER TABLE media_auth.authorization_outbox
    ADD COLUMN IF NOT EXISTS claimed_at timestamptz,
    ADD COLUMN IF NOT EXISTS next_attempt_at timestamptz NOT NULL DEFAULT now();

CREATE TABLE IF NOT EXISTS media_auth.authorization_projection_checkpoints (
    aggregate_type text NOT NULL,
    aggregate_id   text NOT NULL,
    sequence       bigint NOT NULL,
    event_id       text NOT NULL,
    projected_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (aggregate_type, aggregate_id)
);

CREATE INDEX IF NOT EXISTS authorization_outbox_claim_idx
    ON media_auth.authorization_outbox (next_attempt_at, created_at)
    WHERE delivered_at IS NULL;

COMMIT;
