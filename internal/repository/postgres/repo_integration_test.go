//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"deadline-bot/internal/domain"
	"deadline-bot/migrations"
)

func containsTask(list []domain.DueReminder, taskID int64) bool {
	for _, d := range list {
		if d.TaskID == taskID {
			return true
		}
	}
	return false
}

func TestClaimDueDoesNotReturnSameReminderTwice(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()

	pool, err := NewPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	// В CI база пустая. На уже мигрированной базе goose ничего не делает.
	if err := Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatal(err)
	}
	repo := New(pool)

	now := time.Now().UTC()
	task, err := repo.CreateTask(ctx,
		domain.Task{Subject: "TEST", Title: "claim test", FileID: "file-123", Deadline: now.Add(48 * time.Hour)},
		[]domain.Reminder{{Kind: domain.KindD1, RemindAt: now.Add(-time.Minute)}},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM tasks WHERE id = $1`, task.ID) })

	first, err := repo.ClaimDue(ctx, now, 50)
	if err != nil {
		t.Fatal(err)
	}
	if !containsTask(first, task.ID) {
		t.Fatal("first claim should return the reminder")
	}
	for _, d := range first {
		if d.TaskID == task.ID && d.Task.FileID != "file-123" {
			t.Errorf("claimed task file_id = %q, want file-123", d.Task.FileID)
		}
	}
	got, err := repo.GetTask(ctx, task.ID)
	if err != nil || got.FileID != "file-123" {
		t.Errorf("GetTask file_id = %q, err = %v", got.FileID, err)
	}

	second, err := repo.ClaimDue(ctx, now, 50)
	if err != nil {
		t.Fatal(err)
	}
	if containsTask(second, task.ID) {
		t.Fatal("second claim must not return an already claimed reminder")
	}
}
