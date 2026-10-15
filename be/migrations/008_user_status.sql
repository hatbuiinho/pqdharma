ALTER TABLE app_users ADD COLUMN IF NOT EXISTS disabled boolean NOT NULL DEFAULT false;
