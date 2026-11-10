BEGIN;
ALTER TABLE media_workflow.execution_capabilities DROP CONSTRAINT execution_capabilities_output_contract_check;
ALTER TABLE media_workflow.execution_capabilities ADD CONSTRAINT execution_capabilities_output_contract_check CHECK (
    (provider = 'wx_channel' AND capability_key = 'wx_channel.download.record' AND version > 0
     AND generation_method = 'download' AND generation_type = 'record'
     AND output_media_type = 'VIDEO' AND output_asset_kind = '' AND mode = 'reference_import'
     AND execution_target = 'personal_worker' AND conductor_task_def ~ '^[A-Za-z][A-Za-z0-9_]{0,255}$') OR
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
INSERT INTO media_workflow.execution_capabilities
(capability_key,version,provider,mode,generation_method,generation_type,input_schema,output_asset_kind,output_media_type,conductor_task_def,execution_target,retry_policy,timeout_seconds,status,published_at)
VALUES ('wx_channel.download.record',1,'wx_channel','reference_import','download','record',
 '[{"name":"record_id","type":"string","required":true}]','','VIDEO',
 'wx_channel_download_record_v1','personal_worker','{"retry_count":0}',2400,'published',now());
COMMIT;
