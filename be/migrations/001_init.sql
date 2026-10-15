CREATE TABLE IF NOT EXISTS sutras(id text PRIMARY KEY,title text NOT NULL,language text NOT NULL);
CREATE TABLE IF NOT EXISTS recordings(id text PRIMARY KEY,sutra_id text REFERENCES sutras(id),title text NOT NULL,audio_file text NOT NULL,source_url text NOT NULL,status text NOT NULL);
CREATE TABLE IF NOT EXISTS segments(id text PRIMARY KEY,recording_id text REFERENCES recordings(id) ON DELETE CASCADE,position integer NOT NULL,start_ms integer NOT NULL,end_ms integer NOT NULL,text text NOT NULL,CHECK(end_ms>start_ms));
CREATE TABLE IF NOT EXISTS tokens(id text PRIMARY KEY,segment_id text REFERENCES segments(id) ON DELETE CASCADE,position integer NOT NULL,text text NOT NULL,start_ms integer NOT NULL,end_ms integer NOT NULL,end_estimated boolean NOT NULL,phonetic jsonb,CHECK(end_ms>start_ms));
CREATE INDEX IF NOT EXISTS segments_recording_order ON segments(recording_id,position);
CREATE INDEX IF NOT EXISTS tokens_segment_order ON tokens(segment_id,position);
