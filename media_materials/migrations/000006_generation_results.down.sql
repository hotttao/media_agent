DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM media_materials.generated_asset_results) THEN
        RAISE EXCEPTION 'cannot downgrade nonempty generation result receipts';
    END IF;
END $$;
DROP TRIGGER registered_generation_result_immutable ON media_materials.generated_assets;
DROP FUNCTION media_materials.protect_registered_generation_result();
DROP TRIGGER generation_result_receipt_immutable ON media_materials.generated_asset_results;
DROP FUNCTION media_materials.protect_generation_result_receipt();
DROP TABLE media_materials.generated_asset_results;
