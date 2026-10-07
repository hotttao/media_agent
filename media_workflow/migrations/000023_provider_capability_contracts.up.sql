BEGIN;

ALTER TABLE media_workflow.execution_capabilities
    ADD COLUMN output_media_type text,
    ADD COLUMN generation_method text,
    ADD COLUMN generation_type text;
ALTER TABLE media_workflow.execution_capabilities ADD CONSTRAINT execution_capabilities_output_contract_check CHECK (
    (output_media_type IS NULL AND generation_method IS NULL AND generation_type IS NULL AND btrim(output_asset_kind) <> '') OR
    (output_media_type IS NOT NULL AND generation_method IS NOT NULL AND generation_type IS NOT NULL
     AND output_asset_kind = '' AND provider ~ '^[a-z][a-z0-9_]{0,63}$' AND provider <> 'client'
	 AND version > 0 AND conductor_task_def ~ '^[A-Za-z][A-Za-z0-9_]{0,255}$'
     AND generation_method IN ('t2i','i2i','t2v','i2v')
     AND ((generation_type IN ('first_frame_prompt','multi_image_prompt') AND generation_method IN ('i2i','i2v'))
          OR (generation_type = 'text_prompt' AND generation_method IN ('t2i','t2v')))
     AND ((generation_method IN ('t2i','i2i') AND output_media_type = 'IMAGE' AND mode = 'image_generation')
          OR (generation_method IN ('t2v','i2v') AND output_media_type = 'VIDEO' AND mode = 'video_generation'))
     AND capability_key = provider || '.' || generation_method || '.' || generation_type ||
         CASE WHEN execution_target = 'managed_worker' THEN '.managed' ELSE '' END)
);

CREATE UNIQUE INDEX execution_capabilities_provider_identity
ON media_workflow.execution_capabilities(provider,generation_method,generation_type,execution_target,version)
WHERE output_media_type IS NOT NULL;

CREATE FUNCTION media_workflow.protect_provider_capability_contract() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.output_media_type IS NOT NULL THEN
            RAISE EXCEPTION 'provider capability contract cannot be deleted';
        END IF;
        RETURN OLD;
    END IF;
    IF OLD.output_media_type IS NULL AND NEW.output_media_type IS NOT NULL THEN
        RAISE EXCEPTION 'legacy capability contract cannot be converted in place';
    END IF;
    IF OLD.output_media_type IS NOT NULL AND OLD.status IN ('published','archived') THEN
        IF (to_jsonb(NEW) - 'status') IS DISTINCT FROM (to_jsonb(OLD) - 'status') THEN
            RAISE EXCEPTION 'published provider capability contract is immutable';
        END IF;
        IF NEW.status <> OLD.status AND NOT (OLD.status = 'published' AND NEW.status = 'archived') THEN
            RAISE EXCEPTION 'provider capability lifecycle cannot be reversed';
        END IF;
    END IF;
    RETURN NEW;
END $$;

CREATE TRIGGER protect_provider_capability_contract
BEFORE UPDATE OR DELETE ON media_workflow.execution_capabilities
FOR EACH ROW EXECUTE FUNCTION media_workflow.protect_provider_capability_contract();

INSERT INTO media_workflow.execution_capabilities
    (capability_key,version,provider,mode,generation_method,generation_type,input_schema,output_asset_kind,output_media_type,conductor_task_def,execution_target,retry_policy,timeout_seconds,status,published_at)
VALUES
    ('jimeng.i2i.first_frame_prompt',1,'jimeng','image_generation','i2i','first_frame_prompt',
     '[{"name":"prompt","type":"string","required":true},{"name":"images","type":"resource_ref_list","required":true,"media_type":"IMAGE","min_items":1,"max_items":1}]',
     '','IMAGE','jimeng_i2i_first_frame_v1','personal_worker','{"retry_count":3,"retry_logic":"FIXED","retry_delay_seconds":1}',600,'published',now()),
    ('jimeng.i2i.multi_image_prompt',1,'jimeng','image_generation','i2i','multi_image_prompt',
     '[{"name":"prompt","type":"string","required":true},{"name":"images","type":"resource_ref_list","required":true,"media_type":"IMAGE","min_items":1}]',
     '','IMAGE','jimeng_i2i_multi_image_v1','personal_worker','{"retry_count":3,"retry_logic":"FIXED","retry_delay_seconds":1}',600,'published',now()),
    ('doubao.i2v.first_frame_prompt',1,'doubao','video_generation','i2v','first_frame_prompt',
     '[{"name":"prompt","type":"string","required":true},{"name":"images","type":"resource_ref_list","required":true,"media_type":"IMAGE","min_items":1,"max_items":1}]',
     '','VIDEO','doubao_i2v_first_frame_v1','personal_worker','{"retry_count":3,"retry_logic":"FIXED","retry_delay_seconds":1}',600,'published',now()),
    ('doubao.i2v.multi_image_prompt',1,'doubao','video_generation','i2v','multi_image_prompt',
     '[{"name":"prompt","type":"string","required":true},{"name":"images","type":"resource_ref_list","required":true,"media_type":"IMAGE","min_items":1}]',
     '','VIDEO','doubao_i2v_multi_image_v1','personal_worker','{"retry_count":3,"retry_logic":"FIXED","retry_delay_seconds":1}',600,'published',now());

COMMIT;
