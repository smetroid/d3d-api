-- +goose Up
ALTER TABLE element_shares ADD COLUMN IF NOT EXISTS title text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE element_shares DROP COLUMN IF EXISTS title;
