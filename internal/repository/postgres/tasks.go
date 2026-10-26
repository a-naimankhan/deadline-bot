package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"deadline-bot/internal/domain"
)

const taskColumns = `id, subject, title, details, deadline, done_at, created_at`

// scanTask читает строку задачи и приводит все времена к UTC.
func scanTask(row pgx.Row) (domain.Task, error) {
	var t domain.Task
	err := row.Scan(&t.ID, &t.Subject, &t.Title, &t.Details, &t.Deadline, &t.DoneAt, &t.CreatedAt)
	if err != nil {
		return domain.Task{}, err
	}
	normalizeTask(&t)
	return t, nil
}

func normalizeTask(t *domain.Task) {
	t.Deadline = t.Deadline.UTC()
	t.CreatedAt = t.CreatedAt.UTC()
	if t.DoneAt != nil {
		u := t.DoneAt.UTC()
		t.DoneAt = &u
	}
}

// CreateTask создаёт задачу и все её напоминания в одной транзакции.
func (r *Repo) CreateTask(ctx context.Context, t domain.Task, rem []domain.Reminder) (domain.Task, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Task{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // после Commit это no-op

	err = tx.QueryRow(ctx, `
		INSERT INTO tasks (subject, title, details, deadline)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`,
		t.Subject, t.Title, t.Details, t.Deadline.UTC(),
	).Scan(&t.ID, &t.CreatedAt)
	if err != nil {
		return domain.Task{}, fmt.Errorf("insert task: %w", err)
	}
	t.CreatedAt = t.CreatedAt.UTC()

	for _, x := range rem {
		_, err := tx.Exec(ctx,
			`INSERT INTO reminders (task_id, kind, remind_at) VALUES ($1, $2, $3)`,
			t.ID, string(x.Kind), x.RemindAt.UTC(),
		)
		if err != nil {
			return domain.Task{}, fmt.Errorf("insert reminder %q: %w", x.Kind, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Task{}, fmt.Errorf("commit: %w", err)
	}
	return t, nil
}

// ListActive возвращает несданные задачи, ближайший дедлайн первым.
func (r *Repo) ListActive(ctx context.Context) ([]domain.Task, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+taskColumns+` FROM tasks WHERE done_at IS NULL ORDER BY deadline ASC`)
	if err != nil {
		return nil, fmt.Errorf("list active tasks: %w", err)
	}
	tasks, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Task, error) {
		return scanTask(row)
	})
	if err != nil {
		return nil, fmt.Errorf("scan active tasks: %w", err)
	}
	return tasks, nil
}

// GetTask возвращает задачу или domain.ErrNotFound.
func (r *Repo) GetTask(ctx context.Context, id int64) (domain.Task, error) {
	t, err := scanTask(r.pool.QueryRow(ctx,
		`SELECT `+taskColumns+` FROM tasks WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Task{}, fmt.Errorf("task %d: %w", id, domain.ErrNotFound)
	}
	if err != nil {
		return domain.Task{}, fmt.Errorf("get task %d: %w", id, err)
	}
	return t, nil
}

// MarkDone ставит done_at. Если задачи нет  или она уже сдана, возвращает domain.ErrNotFound.
func (r *Repo) MarkDone(ctx context.Context, id int64, at time.Time) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE tasks SET done_at = $2 WHERE id = $1 AND done_at IS NULL`, id, at.UTC())
	if err != nil {
		return fmt.Errorf("mark task %d done: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("task %d: %w", id, domain.ErrNotFound)
	}
	return nil
}
