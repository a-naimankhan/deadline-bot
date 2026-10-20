-- +goose Up
ALTER TABLE reminders ADD COLUMN claimed_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE reminders DROP COLUMN claimed_at;



UPDATE reminders SET claimed_at = now()
WHERE id IN (
    SELECT id FROM reminders
    WHERE sent_at IS NULL
      AND remind_at <= $1
      AND (claimed_at IS NULL OR claimed_at < now() - interval '5 minutes')
    ORDER BY remind_at
    LIMIT $2
  FOR UPDATE SKIP LOCKED
              )
              RETURNING id, task_id, kind;