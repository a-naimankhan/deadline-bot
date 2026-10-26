package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v2"

	"deadline-bot/internal/domain"
)

func TestCreateTask(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		if err != nil {
			t.Fatal(err)
		}
		defer mock.Close()

		repo := New(mock)

		deadline := time.Now().UTC().Add(24 * time.Hour)
		task := domain.Task{
			Subject:  "TEST",
			Title:    "Test Task",
			Details:  "Details",
			Deadline: deadline,
		}
		reminders := []domain.Reminder{
			{Kind: domain.KindD1, RemindAt: deadline.Add(-24 * time.Hour)},
		}

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO tasks`).
			WithArgs(task.Subject, task.Title, task.Details, deadline).
			WillReturnRows(pgxmock.NewRows([]string{"id", "created_at"}).
				AddRow(int64(1), time.Now().UTC()))

		mock.ExpectExec(`INSERT INTO reminders`).
			WithArgs(int64(1), string(reminders[0].Kind), reminders[0].RemindAt.UTC()).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		mock.ExpectCommit()

		result, err := repo.CreateTask(ctx, task, reminders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ID != 1 {
			t.Errorf("expected ID 1, got %d", result.ID)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("begin error", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		if err != nil {
			t.Fatal(err)
		}
		defer mock.Close()

		repo := New(mock)
		mock.ExpectBegin().WillReturnError(pgx.ErrTxClosed)

		_, err = repo.CreateTask(ctx, domain.Task{}, nil)
		if err == nil {
			t.Error("expected error")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("insert task error", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		if err != nil {
			t.Fatal(err)
		}
		defer mock.Close()

		repo := New(mock)
		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO tasks`).
			WillReturnError(pgx.ErrTxClosed)
		mock.ExpectRollback()

		_, err = repo.CreateTask(ctx, domain.Task{}, nil)
		if err == nil {
			t.Error("expected error")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("insert reminder error", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		if err != nil {
			t.Fatal(err)
		}
		defer mock.Close()

		repo := New(mock)
		task := domain.Task{Subject: "TEST", Title: "Test", Deadline: time.Now().UTC()}
		reminders := []domain.Reminder{{Kind: domain.KindD1, RemindAt: time.Now().UTC()}}

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO tasks`).
			WillReturnRows(pgxmock.NewRows([]string{"id", "created_at"}).AddRow(int64(1), time.Now().UTC()))
		mock.ExpectExec(`INSERT INTO reminders`).
			WillReturnError(pgx.ErrTxClosed)
		mock.ExpectRollback()

		_, err = repo.CreateTask(ctx, task, reminders)
		if err == nil {
			t.Error("expected error")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("commit error", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		if err != nil {
			t.Fatal(err)
		}
		defer mock.Close()

		repo := New(mock)
		task := domain.Task{Subject: "TEST", Title: "Test", Deadline: time.Now().UTC()}

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO tasks`).
			WillReturnRows(pgxmock.NewRows([]string{"id", "created_at"}).AddRow(int64(1), time.Now().UTC()))
		mock.ExpectCommit().WillReturnError(pgx.ErrTxClosed)

		_, err = repo.CreateTask(ctx, task, nil)
		if err == nil {
			t.Error("expected error")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})
}

func TestListActive(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		if err != nil {
			t.Fatal(err)
		}
		defer mock.Close()

		repo := New(mock)

		now := time.Now().UTC()
		rows := pgxmock.NewRows([]string{"id", "subject", "title", "details", "deadline", "done_at", "created_at"}).
			AddRow(int64(1), "TEST", "Task1", "Details1", now.Add(24*time.Hour), nil, now).
			AddRow(int64(2), "TEST", "Task2", "Details2", now.Add(48*time.Hour), nil, now)

		mock.ExpectQuery(`SELECT `+taskColumns+` FROM tasks WHERE done_at IS NULL`).
			WillReturnRows(rows)

		tasks, err := repo.ListActive(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tasks) != 2 {
			t.Errorf("expected 2 tasks, got %d", len(tasks))
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("query error", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		if err != nil {
			t.Fatal(err)
		}
		defer mock.Close()

		repo := New(mock)
		mock.ExpectQuery(`SELECT `+taskColumns+` FROM tasks WHERE done_at IS NULL`).
			WillReturnError(pgx.ErrTxClosed)

		_, err = repo.ListActive(ctx)
		if err == nil {
			t.Error("expected error")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("empty result", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		if err != nil {
			t.Fatal(err)
		}
		defer mock.Close()

		repo := New(mock)
		rows := pgxmock.NewRows([]string{"id", "subject", "title", "details", "deadline", "done_at", "created_at"})

		mock.ExpectQuery(`SELECT `+taskColumns+` FROM tasks WHERE done_at IS NULL`).
			WillReturnRows(rows)

		tasks, err := repo.ListActive(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tasks) != 0 {
			t.Errorf("expected 0 tasks, got %d", len(tasks))
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})
}

func TestGetTask(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		if err != nil {
			t.Fatal(err)
		}
		defer mock.Close()

		repo := New(mock)
		now := time.Now().UTC()

		mock.ExpectQuery(`SELECT `+taskColumns+` FROM tasks WHERE id = \$1`).
			WithArgs(int64(1)).
			WillReturnRows(pgxmock.NewRows([]string{"id", "subject", "title", "details", "deadline", "done_at", "created_at"}).
				AddRow(int64(1), "TEST", "Task1", "Details1", now.Add(24*time.Hour), nil, now))

		task, err := repo.GetTask(ctx, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.ID != 1 {
			t.Errorf("expected ID 1, got %d", task.ID)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		if err != nil {
			t.Fatal(err)
		}
		defer mock.Close()

		repo := New(mock)
		mock.ExpectQuery(`SELECT `+taskColumns+` FROM tasks WHERE id = \$1`).
			WithArgs(int64(999)).
			WillReturnError(pgx.ErrNoRows)

		_, err = repo.GetTask(ctx, 999)
		if err == nil {
			t.Error("expected error")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("query error", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		if err != nil {
			t.Fatal(err)
		}
		defer mock.Close()

		repo := New(mock)
		mock.ExpectQuery(`SELECT `+taskColumns+` FROM tasks WHERE id = \$1`).
			WillReturnError(pgx.ErrTxClosed)

		_, err = repo.GetTask(ctx, 1)
		if err == nil {
			t.Error("expected error")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})
}

func TestMarkDone(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		if err != nil {
			t.Fatal(err)
		}
		defer mock.Close()

		repo := New(mock)
		now := time.Now().UTC()

		mock.ExpectExec(`UPDATE tasks SET done_at = \$2 WHERE id = \$1 AND done_at IS NULL`).
			WithArgs(int64(1), now).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		err = repo.MarkDone(ctx, 1, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		if err != nil {
			t.Fatal(err)
		}
		defer mock.Close()

		repo := New(mock)
		now := time.Now().UTC()

		mock.ExpectExec(`UPDATE tasks SET done_at = \$2 WHERE id = \$1 AND done_at IS NULL`).
			WithArgs(int64(999), now).
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))

		err = repo.MarkDone(ctx, 999, now)
		if err == nil {
			t.Error("expected error")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("exec error", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		if err != nil {
			t.Fatal(err)
		}
		defer mock.Close()

		repo := New(mock)
		now := time.Now().UTC()

		mock.ExpectExec(`UPDATE tasks SET done_at = \$2 WHERE id = \$1 AND done_at IS NULL`).
			WillReturnError(pgx.ErrTxClosed)

		err = repo.MarkDone(ctx, 1, now)
		if err == nil {
			t.Error("expected error")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})
}
