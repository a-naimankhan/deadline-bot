package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"deadline-bot/internal/domain"
)

// TaskRepo — то, что сервису нужно от хранилища.
type TaskRepo interface {
	CreateTask(ctx context.Context, t domain.Task, rem []domain.Reminder) (domain.Task, error)
	ListActive(ctx context.Context) ([]domain.Task, error)
	MarkDone(ctx context.Context, id int64, at time.Time) error
	AddReminder(ctx context.Context, rem domain.Reminder) error
}

type Service struct {
	repo  TaskRepo
	loc   *time.Location
	hour  int
	clock func() time.Time
}

func New(repo TaskRepo, loc *time.Location, hour int, clock func() time.Time) *Service {
	return &Service{repo: repo, loc: loc, hour: hour, clock: clock}
}

type AddTaskInput struct {
	Subject  string
	Title    string
	Details  string
	Deadline time.Time
}

// AddTask создаёт задачу вместе с запланированными напоминаниями.
// Пустой предмет/название или дедлайн не в будущем — domain.ErrInvalidInput.
func (s *Service) AddTask(ctx context.Context, in AddTaskInput) (domain.Task, error) {
	now := s.clock()
	t := domain.Task{
		Subject:  strings.TrimSpace(in.Subject),
		Title:    strings.TrimSpace(in.Title),
		Details:  strings.TrimSpace(in.Details),
		Deadline: in.Deadline.UTC(),
	}
	if t.Subject == "" {
		return domain.Task{}, fmt.Errorf("empty subject: %w", domain.ErrInvalidInput)
	}
	if t.Title == "" {
		return domain.Task{}, fmt.Errorf("empty title: %w", domain.ErrInvalidInput)
	}
	if !t.Deadline.After(now) {
		return domain.Task{}, fmt.Errorf("deadline is not in the future: %w", domain.ErrInvalidInput)
	}

	rem := PlanReminders(t.Deadline, now, s.loc, s.hour)
	created, err := s.repo.CreateTask(ctx, t, rem)
	if err != nil {
		return domain.Task{}, fmt.Errorf("create task: %w", err)
	}
	return created, nil
}

// ListActive возвращает несданные задачи, ближайший дедлайн первым.
func (s *Service) ListActive(ctx context.Context) ([]domain.Task, error) {
	tasks, err := s.repo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active: %w", err)
	}
	return tasks, nil
}

// Done отмечает задачу сданной. Нет задачи или она уже сдана — domain.ErrNotFound.
func (s *Service) Done(ctx context.Context, taskID int64) error {
	if err := s.repo.MarkDone(ctx, taskID, s.clock().UTC()); err != nil {
		return fmt.Errorf("done: %w", err)
	}
	return nil
}

// Snooze ставит напоминание kind=snooze через d от текущего момента.
func (s *Service) Snooze(ctx context.Context, taskID int64, d time.Duration) error {
	if d <= 0 {
		return fmt.Errorf("snooze duration %s: %w", d, domain.ErrInvalidInput)
	}
	return s.addSnooze(ctx, taskID, s.clock().Add(d))
}

// SnoozeUntilTomorrow ставит напоминание на ближайшие hour:00 по локальному времени:
// до hour — сегодня (нажал ночью — значит «до утра»), иначе — завтра.
func (s *Service) SnoozeUntilTomorrow(ctx context.Context, taskID int64) error {
	now := s.clock().In(s.loc)
	y, m, d := now.Date()
	at := time.Date(y, m, d, s.hour, 0, 0, 0, s.loc)
	if !at.After(now) {
		at = time.Date(y, m, d+1, s.hour, 0, 0, 0, s.loc)
	}
	return s.addSnooze(ctx, taskID, at)
}

func (s *Service) addSnooze(ctx context.Context, taskID int64, at time.Time) error {
	err := s.repo.AddReminder(ctx, domain.Reminder{
		TaskID:   taskID,
		Kind:     domain.KindSnooze,
		RemindAt: at.UTC(),
	})
	if err != nil {
		return fmt.Errorf("snooze task %d: %w", taskID, err)
	}
	return nil
}
