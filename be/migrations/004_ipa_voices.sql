-- Keep legacy IPA unlabelled: its accent cannot be inferred from the text.
ALTER TABLE recordings ADD COLUMN IF NOT EXISTS default_ipa_voice text CHECK(default_ipa_voice IN ('en-gb','en-us'));
ALTER TABLE recordings ALTER COLUMN default_ipa_voice SET DEFAULT 'en-gb';
CREATE TABLE IF NOT EXISTS token_ipa (
 token_id text NOT NULL REFERENCES tokens(id) ON DELETE CASCADE,
 voice text NOT NULL CHECK(voice IN ('en-gb','en-us')),
 phonetic jsonb NOT NULL,
 PRIMARY KEY(token_id,voice)
);
CREATE TABLE IF NOT EXISTS recording_ipa_versions (
 recording_id text NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
 voice text NOT NULL CHECK(voice IN ('en-gb','en-us')),
 revision integer NOT NULL,
 generated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(recording_id,voice)
);
ALTER TABLE processing_jobs DROP CONSTRAINT IF EXISTS processing_jobs_kind_check;
ALTER TABLE processing_jobs ADD CONSTRAINT processing_jobs_kind_check CHECK(kind IN ('transcribe','edit','ipa'));
