-- +goose Up
CREATE TABLE tasks (
                       id         BIGSERIAL PRIMARY KEY,
                       subject    TEXT NOT NULL,
                       title      TEXT NOT NULL,
                       details    TEXT NOT NULL DEFAULT '',
                       deadline   TIMESTAMPTZ NOT NULL,
                       done_at    TIMESTAMPTZ,
                       created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE reminders (
                           id        BIGSERIAL PRIMARY KEY,
                           task_id   BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
                           kind      TEXT NOT NULL,          -- d2, d1, dday, nag, snooze
                           remind_at TIMESTAMPTZ NOT NULL,
                           sent_at   TIMESTAMPTZ
);
CREATE INDEX reminders_due_idx ON reminders (remind_at) WHERE sent_at IS NULL;

CREATE TABLE netacad_goals (
                               id           BIGSERIAL PRIMARY KEY,
                               total_topics INT NOT NULL,
                               per_day      INT NOT NULL,
                               done_topics  INT NOT NULL DEFAULT 0,
                               start_date   DATE NOT NULL,
                               end_date     DATE NOT NULL,
                               active       BOOLEAN NOT NULL DEFAULT true
);

-- +goose Down
DROP TABLE netacad_goals;
DROP TABLE reminders;
DROP TABLE tasks;