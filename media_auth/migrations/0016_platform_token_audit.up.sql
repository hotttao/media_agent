BEGIN;
CREATE TABLE media_auth.platform_token_audit_outbox (
    id text PRIMARY KEY CHECK (id ~ '^[A-Za-z0-9_-]{1,128}$'),
    user_id text NOT NULL REFERENCES media_auth.users(id),
    credential_id text NOT NULL REFERENCES media_auth.platform_tokens(id),
    operation text NOT NULL CHECK (operation IN ('issued','acquired','introspected','rotated','revoked')),
    caller_service text NOT NULL,
    purpose text NOT NULL,
    occurred_at timestamptz NOT NULL,
    delivered_at timestamptz,
    lease_id text CHECK (lease_id IS NULL OR lease_id ~ '^[A-Za-z0-9_-]{1,128}$'),
    lease_until timestamptz,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((lease_id IS NULL) = (lease_until IS NULL)),
    CHECK (delivered_at IS NULL OR lease_id IS NULL),
    CHECK ((caller_service IN ('media_agent_gateway','media_control_schedule_worker') AND purpose='offline-task' AND operation IN ('issued','acquired','introspected','rotated'))
        OR (caller_service='media_auth' AND purpose='login-bootstrap' AND operation='issued')
        OR (caller_service='media_auth' AND purpose='user-revocation' AND operation='revoked'))
);
CREATE INDEX platform_token_audit_pending_idx ON media_auth.platform_token_audit_outbox(next_attempt_at,occurred_at,id) WHERE delivered_at IS NULL;
COMMIT;
