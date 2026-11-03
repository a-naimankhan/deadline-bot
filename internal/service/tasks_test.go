package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"deadline-bot/internal/domain"
)

type fakeRepo struct {
	created   []domain.Task
	createdRe [][]domain.Reminder
	added     []domain.Reminder
	doneID    int64
	doneAt    time.Time
	err       error
}

func (f *fakeRepo) CreateTask(_ context.Context, t domain.Task, rem []domain.Reminder) (domain.Task, error) {
	if f.err != nil {
		return domain.Task{}, f.err
	}
	t.ID = int64(len(f.created) + 1)
	f.created = append(f.created, t)
	f.createdRe = append(f.createdRe, rem)
	return t, nil
}

func (f *fakeRepo) ListActive(context.Context) ([]domain.Task, error) {
	return f.created, f.err
}

func (f *fakeRepo) MarkDone(_ context.Context, id int64, at time.Time) error {
	if f.err != nil {
		return f.err
	}
	f.doneID, f.doneAt = id, at
	return nil
}

func (f *fakeRepo) AddReminder(_ context.Context, rem domain.Reminder) error {
	if f.err != nil {
		return f.err
	}
	f.added = append(f.added, rem)
	return nil
}

func newTestService(t *testing.T, repo *fakeRepo, now time.Time) *Service {
	t.Helper()
	return New(repo, almaty(t), 12, func() time.Time { return now })
}

func TestAddTask(t *testing.T) {
	loc := almaty(t)
	now := time.Date(2026, time.October, 20, 10, 0, 0, 0, loc)
	ctx := context.Background()

	t.Run("success plans reminders", func(t *testing.T) {
		repo := &fakeRepo{}
		svc := newTestService(t, repo, now)

		task, err := svc.AddTask(ctx, AddTaskInput{
			Subject:  "  БИС ",
			Title:    "Lab 5",
			Deadline: time.Date(2026, time.October, 23, 23, 59, 0, 0, loc),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.ID != 1 || task.Subject != "БИС" {
			t.Errorf("task = %+v, want ID 1 and trimmed subject", task)
		}
		if task.Deadline.Location() != time.UTC {
			t.Errorf("deadline location = %s, want UTC", task.Deadline.Location())
		}
		// d2, d1, dday + 6 nag
		if got := len(repo.createdRe[0]); got != 9 {
			t.Errorf("reminders = %d, want 9", got)
		}
	})

	invalid := []struct {
		name string
		in   AddTaskInput
	}{
		{"empty subject", AddTaskInput{Subject: " ", Title: "Lab", Deadline: now.Add(time.Hour)}},
		{"empty title", AddTaskInput{Subject: "БИС", Title: "", Deadline: now.Add(time.Hour)}},
		{"deadline now", AddTaskInput{Subject: "БИС", Title: "Lab", Deadline: now}},
		{"deadline in past", AddTaskInput{Subject: "БИС", Title: "Lab", Deadline: now.Add(-time.Hour)}},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{}
			_, err := newTestService(t, repo, now).AddTask(ctx, tt.in)
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
			if len(repo.created) != 0 {
				t.Error("repo must not be called on invalid input")
			}
		})
	}

	t.Run("repo error is wrapped", func(t *testing.T) {
		boom := errors.New("boom")
		_, err := newTestService(t, &fakeRepo{err: boom}, now).AddTask(ctx, AddTaskInput{
			Subject: "БИС", Title: "Lab", Deadline: now.Add(48 * time.Hour),
		})
		if !errors.Is(err, boom) {
			t.Errorf("err = %v, want wrapped boom", err)
		}
	})
}

func TestListActive(t *testing.T) {
	now := time.Date(2026, time.October, 20, 5, 0, 0, 0, time.UTC)
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		repo := &fakeRepo{created: []domain.Task{{ID: 1}, {ID: 2}}}
		tasks, err := newTestService(t, repo, now).ListActive(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tasks) != 2 {
			t.Errorf("tasks = %d, want 2", len(tasks))
		}
	})

	t.Run("repo error is wrapped", func(t *testing.T) {
		boom := errors.New("boom")
		_, err := newTestService(t, &fakeRepo{err: boom}, now).ListActive(ctx)
		if !errors.Is(err, boom) {
			t.Errorf("err = %v, want wrapped boom", err)
		}
	})
}

func TestDone(t *testing.T) {
	now := time.Date(2026, time.October, 20, 5, 0, 0, 0, time.UTC)
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		repo := &fakeRepo{}
		if err := newTestService(t, repo, now).Done(ctx, 7); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.doneID != 7 || !repo.doneAt.Equal(now) {
			t.Errorf("MarkDone(%d, %s), want (7, %s)", repo.doneID, repo.doneAt, now)
		}
	})

	t.Run("not found passes through", func(t *testing.T) {
		repo := &fakeRepo{err: domain.ErrNotFound}
		err := newTestService(t, repo, now).Done(ctx, 7)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
}

func TestSnooze(t *testing.T) {
	loc := almaty(t)
	now := time.Date(2026, time.October, 20, 15, 30, 0, 0, loc)
	ctx := context.Background()

	t.Run("3h", func(t *testing.T) {
		repo := &fakeRepo{}
		if err := newTestService(t, repo, now).Snooze(ctx, 3, 3*time.Hour); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := repo.added[0]
		if got.TaskID != 3 || got.Kind != domain.KindSnooze || !got.RemindAt.Equal(now.Add(3*time.Hour)) {
			t.Errorf("reminder = %+v", got)
		}
		if got.RemindAt.Location() != time.UTC {
			t.Errorf("location = %s, want UTC", got.RemindAt.Location())
		}
	})

	t.Run("non-positive duration", func(t *testing.T) {
		err := newTestService(t, &fakeRepo{}, now).Snooze(ctx, 3, 0)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("until tomorrow", func(t *testing.T) {
		repo := &fakeRepo{}
		if err := newTestService(t, repo, now).SnoozeUntilTomorrow(ctx, 3); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := time.Date(2026, time.October, 21, 12, 0, 0, 0, loc)
		if got := repo.added[0].RemindAt; !got.Equal(want) {
			t.Errorf("at = %s, want %s", got.In(loc), want)
		}
	})

	untilTomorrow := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{"late evening", time.Date(2026, time.October, 20, 23, 30, 0, 0, loc), time.Date(2026, time.October, 21, 12, 0, 0, 0, loc)},
		{"after midnight is same day noon", time.Date(2026, time.October, 21, 0, 30, 0, 0, loc), time.Date(2026, time.October, 21, 12, 0, 0, 0, loc)},
		{"exactly noon is next day", time.Date(2026, time.October, 21, 12, 0, 0, 0, loc), time.Date(2026, time.October, 22, 12, 0, 0, 0, loc)},
	}
	for _, tt := range untilTomorrow {
		t.Run("until tomorrow "+tt.name, func(t *testing.T) {
			repo := &fakeRepo{}
			if err := newTestService(t, repo, tt.now).SnoozeUntilTomorrow(ctx, 3); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := repo.added[0].RemindAt; !got.Equal(tt.want) {
				t.Errorf("at = %s, want %s", got.In(loc), tt.want)
			}
		})
	}

	t.Run("task not found", func(t *testing.T) {
		err := newTestService(t, &fakeRepo{err: domain.ErrNotFound}, now).Snooze(ctx, 99, time.Hour)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
}
