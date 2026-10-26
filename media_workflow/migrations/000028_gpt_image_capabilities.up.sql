BEGIN;
INSERT INTO media_workflow.execution_capabilities
 (capability_key,version,provider,mode,generation_method,generation_type,input_schema,output_asset_kind,output_media_type,conductor_task_def,execution_target,retry_policy,timeout_seconds,status,published_at)
VALUES
 ('gpt.t2i.text_prompt',1,'gpt','image_generation','t2i','text_prompt',
  '[{"name":"prompt","type":"string","required":true}]',
  '','IMAGE','gpt_t2i_text_prompt_v1','personal_worker','{"retry_count":0}',1200,'published',now()),
 ('gpt.i2i.multi_image_prompt',1,'gpt','image_generation','i2i','multi_image_prompt',
  '[{"name":"prompt","type":"string","required":true},{"name":"images","type":"resource_ref_list","required":true,"media_type":"IMAGE","min_items":1,"max_items":4}]',
  '','IMAGE','gpt_i2i_multi_image_v1','personal_worker','{"retry_count":0}',1200,'published',now());
COMMIT;
