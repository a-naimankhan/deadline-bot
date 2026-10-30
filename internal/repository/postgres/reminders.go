package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"deadline-bot/internal/domain"
)

// claimLease: сколько времени напоминание считается «в работе» у воркера.
// Если воркер упал, через это время напоминание заберут снова.
const claimLease = 5 * time.Minute

const pgForeignKeyViolation = "23503"

// AddReminder добавляет напоминание (snooze, nag). Если задачи нет, возвращает domain.ErrNotFound.
func (r *Repo) AddReminder(ctx context.Context, rem domain.Reminder) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO reminders (task_id, kind, remind_at) VALUES ($1, $2, $3)`,
		rem.TaskID, string(rem.Kind), rem.RemindAt.UTC(),
	)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolation {
		return fmt.Errorf("task %d: %w", rem.TaskID, domain.ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("add reminder: %w", err)
	}
	return nil
}

const claimDueSQL = `
WITH due AS (
	SELECT r.id
	FROM reminders r
	JOIN tasks t ON t.id = r.task_id
	WHERE r.sent_at IS NULL
	  AND r.remind_at <= $1
	  AND t.done_at IS NULL
	  AND (r.claimed_at IS NULL OR r.claimed_at < $3)
	ORDER BY r.remind_at
	LIMIT $2
	FOR UPDATE OF r SKIP LOCKED
), claimed AS (
	UPDATE reminders r
	SET claimed_at = $1
	FROM due
	WHERE r.id = due.id
	RETURNING r.id, r.task_id, r.kind, r.remind_at
)
SELECT c.id, c.task_id, c.kind, c.remind_at,
       t.id, t.subject, t.title, t.details, t.deadline, t.done_at, t.created_at
FROM claimed c
JOIN tasks t ON t.id = c.task_id
ORDER BY c.remind_at`

// ClaimDue атомарно «арендует» до limit готовых к отправке напоминаний несданных задач.
func (r *Repo) ClaimDue(ctx context.Context, now time.Time, limit int) ([]domain.DueReminder, error) {
	rows, err := r.db.Query(ctx, claimDueSQL, now.UTC(), limit, now.Add(-claimLease).UTC())
	if err != nil {
		return nil, fmt.Errorf("claim due reminders: %w", err)
	}
	due, err := pgx.CollectRows(rows, scanDue)
	if err != nil {
		return nil, fmt.Errorf("scan due reminders: %w", err)
	}
	return due, nil
}

func scanDue(row pgx.CollectableRow) (domain.DueReminder, error) {
	var d domain.DueReminder
	var kind string
	err := row.Scan(
		&d.ID, &d.TaskID, &kind, &d.RemindAt,
		&d.Task.ID, &d.Task.Subject, &d.Task.Title, &d.Task.Details,
		&d.Task.Deadline, &d.Task.DoneAt, &d.Task.CreatedAt,
	)
	if err != nil {
		return domain.DueReminder{}, err
	}
	d.Kind = domain.ReminderKind(kind)
	d.RemindAt = d.RemindAt.UTC()
	normalizeTask(&d.Task)
	return d, nil
}

// MarkSent помечает напоминание отправленным. Если его нет или оно уже отправлено, возвращает domain.ErrNotFound.
func (r *Repo) MarkSent(ctx context.Context, id int64, at time.Time) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE reminders SET sent_at = $2 WHERE id = $1 AND sent_at IS NULL`, id, at.UTC())
	if err != nil {
		return fmt.Errorf("mark reminder %d sent: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("reminder %d: %w", id, domain.ErrNotFound)
	}
	return nil
}
