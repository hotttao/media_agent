BEGIN;

UPDATE media_workflow.workflows
   SET input_schema = COALESCE(
       (SELECT JSONB_AGG(entry - 'asset_kind')
          FROM JSONB_ARRAY_ELEMENTS(input_schema) AS entry),
       '[]'::jsonb
   );

COMMIT;
