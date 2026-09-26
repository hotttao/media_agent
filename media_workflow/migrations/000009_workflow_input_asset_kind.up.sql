BEGIN;

-- input_schema 是 workflow 发布时冻结的公开输入契约。旧数据从实际的
-- step.slot_binding -> template_input 关系回填 MATERIAL/ASSET 的 asset_kind。
WITH derived AS (
    SELECT ws.workflow_id,
           sb.ref AS name,
           MIN(ti.resource_type) AS resource_type,
           CASE
               WHEN MIN(ti.resource_type) IN ('MATERIAL', 'ASSET')
                    AND COUNT(DISTINCT ti.asset_kind) = 1
               THEN MIN(ti.asset_kind)
               ELSE ''
           END AS asset_kind,
           BOOL_AND(ti.required) AS required
      FROM media_workflow.workflows w
      JOIN media_workflow.workflow_steps ws
        ON ws.workflow_id = w.id AND ws.version = w.version
      JOIN media_workflow.slot_bindings sb
        ON sb.step_id = ws.id AND sb.source = 'WORKFLOW_INPUT'
      JOIN media_workflow.template_inputs ti
        ON ti.template_id = ws.template_id
       AND ti.version = ws.template_version
       AND ti.name = sb.to_input
     GROUP BY ws.workflow_id, sb.ref
), aggregated AS (
    SELECT workflow_id,
           JSONB_AGG(
               JSONB_BUILD_OBJECT(
                   'name', name,
                   'resource_type', resource_type,
                   'asset_kind', asset_kind,
                   'required', required
               ) ORDER BY name
           ) AS input_schema
      FROM derived
     GROUP BY workflow_id
)
UPDATE media_workflow.workflows w
   SET input_schema = a.input_schema
  FROM aggregated a
 WHERE w.id = a.workflow_id;

COMMIT;
