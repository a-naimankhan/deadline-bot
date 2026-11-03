-- +goose Up
ALTER TABLE reminders ADD COLUMN claimed_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE reminders DROP COLUMN claimed_at;
