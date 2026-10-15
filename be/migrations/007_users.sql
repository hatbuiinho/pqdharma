CREATE TABLE IF NOT EXISTS app_users(id text PRIMARY KEY,email text NOT NULL UNIQUE,password_hash text NOT NULL,is_admin boolean NOT NULL DEFAULT false,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS user_sessions(token_hash text PRIMARY KEY,user_id text NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,expires_at timestamptz NOT NULL);
ALTER TABLE recordings ADD COLUMN IF NOT EXISTS owner_id text REFERENCES app_users(id);
ALTER TABLE recordings ADD COLUMN IF NOT EXISTS visibility text NOT NULL DEFAULT 'private' CHECK(visibility IN ('private','shared','public'));
ALTER TABLE recordings ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
CREATE TABLE IF NOT EXISTS recording_shares(recording_id text REFERENCES recordings(id) ON DELETE CASCADE,user_id text REFERENCES app_users(id) ON DELETE CASCADE,PRIMARY KEY(recording_id,user_id));
CREATE INDEX IF NOT EXISTS recordings_owner ON recordings(owner_id);
