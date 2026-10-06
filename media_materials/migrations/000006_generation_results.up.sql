CREATE TABLE media_materials.generated_asset_results (
    step_run_id text NOT NULL CHECK (length(step_run_id) BETWEEN 1 AND 256 AND step_run_id=btrim(step_run_id)),
    result_key text NOT NULL CHECK (result_key ~ '^[0-9a-f]{64}$'),
    asset_id text NOT NULL UNIQUE REFERENCES media_materials.generated_assets(id) ON DELETE RESTRICT,
    request_digest text NOT NULL CHECK (request_digest ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (step_run_id, result_key)
);

CREATE FUNCTION media_materials.protect_generation_result_receipt() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'generation result receipts are immutable';
END;
$$;
CREATE TRIGGER generation_result_receipt_immutable BEFORE UPDATE OR DELETE
ON media_materials.generated_asset_results FOR EACH ROW
EXECUTE FUNCTION media_materials.protect_generation_result_receipt();

CREATE FUNCTION media_materials.protect_registered_generation_result() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM media_materials.generated_asset_results WHERE asset_id=OLD.id)
       AND (to_jsonb(NEW)-'deleted_at'-'updated_at') IS DISTINCT FROM (to_jsonb(OLD)-'deleted_at'-'updated_at') THEN
        RAISE EXCEPTION 'registered generation result is immutable';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER registered_generation_result_immutable BEFORE UPDATE
ON media_materials.generated_assets FOR EACH ROW
EXECUTE FUNCTION media_materials.protect_registered_generation_result();
