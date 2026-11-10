BEGIN;
CREATE TABLE media_auth.applications (
 id text PRIMARY KEY, name text NOT NULL, secret_hash text NOT NULL,
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','disabled')),
 credential_version bigint NOT NULL DEFAULT 1 CHECK(credential_version > 0),
 created_by text NOT NULL REFERENCES media_auth.users(id), created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE media_auth.users ADD COLUMN managed_by text REFERENCES media_auth.applications(id);
ALTER TABLE media_auth.users ADD CONSTRAINT users_managed_identity_check CHECK
 (managed_by IS NULL OR (role = 'user' AND password_hash IS NULL AND kratos_identity_id IS NULL));
CREATE INDEX users_managed_by_idx ON media_auth.users(managed_by) WHERE managed_by IS NOT NULL;
CREATE TABLE media_auth.application_access_credentials (
 token_hash text PRIMARY KEY, application_id text NOT NULL REFERENCES media_auth.applications(id),
 user_id text NOT NULL REFERENCES media_auth.users(id), external_id text NOT NULL,
 application_version bigint NOT NULL, auth_version bigint NOT NULL, expires_at timestamptz NOT NULL
);
CREATE INDEX application_credentials_expiry_idx ON media_auth.application_access_credentials(application_id,expires_at);
COMMIT;
