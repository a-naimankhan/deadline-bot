package scheduler

import (
	"context"
	"time"
)

type Store interface {
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]domain.DueReminder, error)
	MarkSent(ctx context.Context, id int64) error
}

type Notifier interface {
	Send(ctx context.Context, r domain.DueReminder) error
}

type Scheduler struct {
	store    Store
	notifier Notifier
	workers  int
	tick     time.Duration
}

func NewScheduler(store Store, notifier Notifier, workers int, tick time.Duration) *Scheduler {
	return &Scheduler{
		store:    store,
		notifier: notifier,
		workers:  workers,
		tick:     tick,
	}
}

func (s *Scheduler) Run(ctx context.Context) error {

}
