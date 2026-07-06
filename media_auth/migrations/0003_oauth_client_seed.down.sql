BEGIN;
DELETE FROM media_auth.oauth_clients WHERE id IN ('media_materials', 'media_workflow');
COMMIT;
