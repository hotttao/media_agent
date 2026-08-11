CREATE SCHEMA IF NOT EXISTS media_agent_gateway;

CREATE TABLE media_agent_gateway.agent_sessions (
    id uuid PRIMARY KEY,
    user_id text NOT NULL,
    workspace_id text NOT NULL,
    agent_id text NOT NULL,
    runtime_session_id text,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX agent_sessions_user_updated_idx
    ON media_agent_gateway.agent_sessions (user_id, updated_at DESC);

CREATE TABLE media_agent_gateway.turns (
    id uuid PRIMARY KEY,
    session_id uuid NOT NULL REFERENCES media_agent_gateway.agent_sessions(id) ON DELETE CASCADE,
    idempotency_key text NOT NULL,
    status text NOT NULL DEFAULT 'running',
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    UNIQUE (session_id, idempotency_key)
);
CREATE UNIQUE INDEX turns_one_running_per_session_idx
    ON media_agent_gateway.turns (session_id) WHERE status = 'running';

CREATE TABLE media_agent_gateway.messages (
    id uuid PRIMARY KEY,
    session_id uuid NOT NULL REFERENCES media_agent_gateway.agent_sessions(id) ON DELETE CASCADE,
    turn_id uuid NOT NULL REFERENCES media_agent_gateway.turns(id) ON DELETE CASCADE,
    role text NOT NULL,
    content jsonb NOT NULL,
    sequence bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (session_id, sequence)
);

CREATE TABLE media_agent_gateway.universal_events (
    id uuid PRIMARY KEY,
    session_id uuid NOT NULL REFERENCES media_agent_gateway.agent_sessions(id) ON DELETE CASCADE,
    turn_id uuid NOT NULL REFERENCES media_agent_gateway.turns(id) ON DELETE CASCADE,
    sequence bigint NOT NULL,
    type text NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (turn_id, sequence)
);
