-- +goose Up
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS provider     text NOT NULL DEFAULT 'local',
  ADD COLUMN IF NOT EXISTS provider_id  text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS email        text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS display_name text NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS users_provider_id_idx
  ON users (provider, provider_id)
  WHERE provider != 'local';

-- +goose Down
DROP INDEX IF EXISTS users_provider_id_idx;
ALTER TABLE users
  DROP COLUMN IF EXISTS provider,
  DROP COLUMN IF EXISTS provider_id,
  DROP COLUMN IF EXISTS email,
  DROP COLUMN IF EXISTS display_name;
