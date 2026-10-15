CREATE TABLE IF NOT EXISTS recording_assets (
 recording_id text NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
 kind text NOT NULL,
 object_key text NOT NULL,
 content_type text NOT NULL,
 size_bytes bigint NOT NULL,
 sha256 text NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(recording_id,kind)
);
CREATE TABLE IF NOT EXISTS storage_exports (
 recording_id text PRIMARY KEY REFERENCES recordings(id) ON DELETE CASCADE,
 fingerprint text NOT NULL,
 object_key text NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now()
);
