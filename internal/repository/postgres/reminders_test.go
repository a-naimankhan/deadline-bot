package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v5"

	"deadline-bot/internal/domain"
)

func TestAddReminder(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		if err != nil {
			t.Fatal(err)
		}
		defer mock.Close()

		repo := New(mock)
		rem := domain.Reminder{
			TaskID:   1,
			Kind:     domain.KindD1,
			RemindAt: time.Now().UTC(),
		}

		mock.ExpectExec(`INSERT INTO reminders`).
			WithArgs(rem.TaskID, string(rem.Kind), rem.RemindAt.UTC()).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		err = repo.AddReminder(ctx, rem)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("foreign key violation returns ErrNotFound", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		if err != nil {
			t.Fatal(err)
		}
		defer mock.Close()

		repo := New(mock)
		rem := domain.Reminder{
			TaskID:   999,
			Kind:     domain.KindD1,
			RemindAt: time.Now().UTC(),
		}

		pgErr := &pgconn.PgError{Code: "23503"}
		mock.ExpectExec(`INSERT INTO reminders`).
			WithArgs(rem.TaskID, string(rem.Kind), rem.RemindAt.UTC()).
			WillReturnError(pgErr)

		err = repo.AddReminder(ctx, rem)
		if err == nil {
			t.Error("expected error")
		}
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("generic error does not return ErrNotFound", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		if err != nil {
			t.Fatal(err)
		}
		defer mock.Close()

		repo := New(mock)
		rem := domain.Reminder{
			TaskID:   1,
			Kind:     domain.KindD1,
			RemindAt: time.Now().UTC(),
		}

		mock.ExpectExec(`INSERT INTO reminders`).
			WithArgs(rem.TaskID, string(rem.Kind), rem.RemindAt.UTC()).
			WillReturnError(errors.New("boom"))

		err = repo.AddReminder(ctx, rem)
		if err == nil {
			t.Error("expected error")
		}
		if errors.Is(err, domain.ErrNotFound) {
			t.Error("generic error should not be ErrNotFound")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})
}

func TestMarkSent(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		if err != nil {
			t.Fatal(err)
		}
		defer mock.Close()

		repo := New(mock)
		now := time.Now().UTC()

		mock.ExpectExec(`UPDATE reminders SET sent_at`).
			WithArgs(int64(1), now).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		err = repo.MarkSent(ctx, 1, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("zero rows affected returns ErrNotFound", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		if err != nil {
			t.Fatal(err)
		}
		defer mock.Close()

		repo := New(mock)
		now := time.Now().UTC()

		mock.ExpectExec(`UPDATE reminders SET sent_at`).
			WithArgs(int64(999), now).
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))

		err = repo.MarkSent(ctx, 999, now)
		if err == nil {
			t.Error("expected error")
		}
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})
}
