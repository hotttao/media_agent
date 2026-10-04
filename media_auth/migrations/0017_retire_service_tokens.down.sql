-- Automatic downgrade must never restore a retired credential write path.
DO $$ BEGIN
    RAISE EXCEPTION 'credential retirement cannot be automatically rolled back';
END $$;
