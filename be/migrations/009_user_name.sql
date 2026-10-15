ALTER TABLE app_users ADD COLUMN IF NOT EXISTS name text NOT NULL DEFAULT '';
UPDATE app_users SET name=left(split_part(email,'@',1),100) WHERE name='';
