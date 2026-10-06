BEGIN;
ALTER TABLE media_workflow.workflows DROP CONSTRAINT workflows_status_check;
ALTER TABLE media_workflow.workflows ADD CONSTRAINT workflows_status_check
    CHECK (status IN ('draft','publishing','published','publish_failed','archived'));

CREATE TABLE media_workflow.workflow_publications (
    id text PRIMARY KEY CHECK (length(id) BETWEEN 1 AND 256 AND btrim(id)=id),
    workflow_id text NOT NULL REFERENCES media_workflow.workflows(id) ON DELETE RESTRICT,
    workflow_version integer NOT NULL CHECK (workflow_version>0),
    compilation_id text NOT NULL UNIQUE REFERENCES media_workflow.workflow_compilations(id) ON DELETE RESTRICT,
    requested_by text NOT NULL CHECK (length(requested_by) BETWEEN 1 AND 256 AND btrim(requested_by)=requested_by),
    definition_hash text NOT NULL CHECK (definition_hash ~ '^[a-f0-9]{64}$'),
    definition_json jsonb NOT NULL CHECK (jsonb_typeof(definition_json)='object'),
    task_definitions_json jsonb NOT NULL CHECK (jsonb_typeof(task_definitions_json)='array' AND jsonb_array_length(task_definitions_json)>0),
    payload_digest text NOT NULL CHECK (payload_digest ~ '^[a-f0-9]{64}$'),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','registered','failed','superseded')),
    claim_token text,
    claimed_at timestamptz,
    delivery_attempts integer NOT NULL DEFAULT 0 CHECK (delivery_attempts>=0),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error text CHECK (last_error IS NULL OR last_error ~ '^[a-z0-9_]{0,64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    UNIQUE (workflow_id,workflow_version),
    CHECK ((claim_token IS NULL)=(claimed_at IS NULL)),
    CHECK (status='pending' OR claim_token IS NULL)
);
CREATE INDEX workflow_publications_pending_idx ON media_workflow.workflow_publications(next_attempt_at,created_at,id) WHERE status='pending';

CREATE FUNCTION media_workflow.protect_workflow_publication() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='DELETE' THEN RAISE EXCEPTION 'workflow publication audit cannot be deleted'; END IF;
    IF ROW(NEW.id,NEW.workflow_id,NEW.workflow_version,NEW.compilation_id,NEW.requested_by,
           NEW.definition_hash,NEW.definition_json,NEW.task_definitions_json,NEW.payload_digest,NEW.created_at)
       IS DISTINCT FROM ROW(OLD.id,OLD.workflow_id,OLD.workflow_version,OLD.compilation_id,OLD.requested_by,
           OLD.definition_hash,OLD.definition_json,OLD.task_definitions_json,OLD.payload_digest,OLD.created_at) THEN
        RAISE EXCEPTION 'workflow publication payload is immutable';
    END IF;
    IF OLD.status<>'pending' AND NEW IS DISTINCT FROM OLD THEN
        RAISE EXCEPTION 'workflow publication terminal state is immutable';
    END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER immutable_workflow_publication BEFORE UPDATE OR DELETE ON media_workflow.workflow_publications
FOR EACH ROW EXECUTE FUNCTION media_workflow.protect_workflow_publication();
COMMIT;
