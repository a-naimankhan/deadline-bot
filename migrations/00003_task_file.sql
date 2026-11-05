-- +goose Up
-- file_id вложения в Telegram (PDF задания). Сам файл хранит Telegram, у нас только ссылка.
ALTER TABLE tasks ADD COLUMN file_id TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE tasks DROP COLUMN file_id;
