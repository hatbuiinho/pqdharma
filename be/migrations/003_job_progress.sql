ALTER TABLE processing_jobs ADD COLUMN IF NOT EXISTS progress integer NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100);
ALTER TABLE processing_jobs ADD COLUMN IF NOT EXISTS progress_message text NOT NULL DEFAULT '';
UPDATE processing_jobs SET progress=100 WHERE status='ready' AND progress<>100;
