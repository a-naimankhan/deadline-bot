package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"deadline-bot/internal/domain"
	"deadline-bot/internal/logger"
)

const (
	// claimBatch — сколько напоминаний забираем за один тик; остальные уйдут на следующем.
	claimBatch = 50
	// markSentTimeout — сколько ждём MarkSent; он не отменяется вместе с ctx,
	// чтобы уже отправленное напоминание успело пометиться и при shutdown.
	markSentTimeout = 5 * time.Second
)

type Store interface {
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]domain.DueReminder, error)
	MarkSent(ctx context.Context, id int64, at time.Time) error
}

type Notifier interface {
	Send(ctx context.Context, r domain.DueReminder) error
}

type Scheduler struct {
	store    Store
	notifier Notifier
	workers  int
	tick     time.Duration
	clock    func() time.Time
	log      *slog.Logger
}

func NewScheduler(store Store, notifier Notifier, workers int, tick time.Duration, clock func() time.Time) *Scheduler {
	return &Scheduler{
		store:    store,
		notifier: notifier,
		workers:  workers,
		tick:     tick,
		clock:    clock,
		log:      logger.Get(),
	}
}

// Run раз в tick забирает готовые напоминания и раздаёт их воркерам.
// Блокируется до отмены ctx; после отмены перестаёт забирать новые, ждёт воркеров и возвращает nil.
// Доставка at-least-once: не отправленное (или не помеченное) напоминание вернётся после истечения аренды.
func (s *Scheduler) Run(ctx context.Context) error {
	jobs := make(chan domain.DueReminder)
	var wg sync.WaitGroup
	for range s.workers {
		wg.Go(func() {
			for r := range jobs {
				s.process(ctx, r)
			}
		})
	}

	ticker := time.NewTicker(s.tick)
	defer ticker.Stop()

	// Первый проход сразу, не ждём минуту после старта.
	for {
		s.dispatch(ctx, jobs)
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return nil
		case <-ticker.C:
		}
	}
}

func (s *Scheduler) dispatch(ctx context.Context, jobs chan<- domain.DueReminder) {
	if ctx.Err() != nil {
		return
	}
	due, err := s.store.ClaimDue(ctx, s.clock(), claimBatch)
	if err != nil {
		// БД моргнула — пробуем на следующем тике, бот не падает.
		s.log.Error("claim due reminders", "err", err)
		return
	}
	for _, r := range due {
		select {
		case jobs <- r:
		case <-ctx.Done():
			// Оставшиеся арендованы и вернутся через claimLease.
			return
		}
	}
}

func (s *Scheduler) process(ctx context.Context, r domain.DueReminder) {
	log := s.log.With("reminder_id", r.ID, "task_id", r.TaskID, "kind", r.Kind)
	if err := s.notifier.Send(ctx, r); err != nil {
		// Не помечаем: аренда истечёт, и напоминание отправится снова.
		log.Error("send reminder", "err", err)
		return
	}

	mctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), markSentTimeout)
	defer cancel()
	if err := s.store.MarkSent(mctx, r.ID, s.clock()); err != nil {
		// Отправили, но не пометили — возможен дубль после аренды. Это цена at-least-once.
		log.Error("mark reminder sent", "err", err)
		return
	}
	log.Info("reminder sent")
}
