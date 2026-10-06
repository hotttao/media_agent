BEGIN;

CREATE FUNCTION media_workflow.protect_frozen_workflow_compilation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(NEW.id,NEW.workflow_id,NEW.workflow_version,NEW.conductor_name,
           NEW.conductor_version,NEW.ir_json,NEW.definition_json,
           NEW.definition_hash,NEW.created_at)
       IS DISTINCT FROM
       ROW(OLD.id,OLD.workflow_id,OLD.workflow_version,OLD.conductor_name,
           OLD.conductor_version,OLD.ir_json,OLD.definition_json,
           OLD.definition_hash,OLD.created_at) THEN
        RAISE EXCEPTION 'workflow compilation payload is immutable';
    END IF;
    IF OLD.status='registered' AND
       ROW(NEW.status,NEW.registered_at,NEW.error_message)
       IS DISTINCT FROM ROW(OLD.status,OLD.registered_at,OLD.error_message) THEN
        RAISE EXCEPTION 'registered workflow compilation cannot regress';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER frozen_workflow_compilation
BEFORE UPDATE ON media_workflow.workflow_compilations
FOR EACH ROW EXECUTE FUNCTION media_workflow.protect_frozen_workflow_compilation();

COMMIT;
