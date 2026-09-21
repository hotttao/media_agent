CREATE TABLE media_storage.stored_objects (
    object_path text PRIMARY KEY,
    namespace text NOT NULL,
    driver text NOT NULL,
    backend_key text NOT NULL,
    content_type text NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
    checksum text NOT NULL,
    state text NOT NULL CHECK (state IN ('active', 'pending_delete', 'deleted')),
    delete_after timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX stored_objects_namespace_state_idx
    ON media_storage.stored_objects(namespace, state, updated_at);
