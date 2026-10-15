ALTER TABLE recordings ADD COLUMN IF NOT EXISTS revision integer NOT NULL DEFAULT 0;
CREATE TABLE IF NOT EXISTS processing_jobs (
 id text PRIMARY KEY,
 recording_id text NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
 kind text NOT NULL CHECK(kind IN ('transcribe','edit')),
 status text NOT NULL CHECK(status IN ('queued','transcribing','aligning','aligned','phonetics','ready','failed')),
 payload jsonb NOT NULL DEFAULT '{}',
 result jsonb,
 error text NOT NULL DEFAULT '',
 base_revision integer NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS one_active_job_per_recording ON processing_jobs(recording_id) WHERE status NOT IN ('ready','failed');
CREATE INDEX IF NOT EXISTS processing_jobs_queue ON processing_jobs(status,created_at);
