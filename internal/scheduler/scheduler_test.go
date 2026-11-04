package scheduler

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"deadline-bot/internal/domain"
)

type fakeStore struct {
	mu        sync.Mutex
	batches   [][]domain.DueReminder // каждый вызов ClaimDue отдаёт следующую пачку
	claimErrs []error                // ошибка для i-го вызова ClaimDue (если есть)
	calls     int
	marked    []int64
	markedAt  []time.Time
	markCtxOK []bool // был ли ctx MarkSent жив в момент вызова
}

func (f *fakeStore) ClaimDue(_ context.Context, _ time.Time, limit int) ([]domain.DueReminder, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.calls
	f.calls++
	if i < len(f.claimErrs) && f.claimErrs[i] != nil {
		return nil, f.claimErrs[i]
	}
	if len(f.batches) == 0 {
		return nil, nil
	}
	b := f.batches[0]
	f.batches = f.batches[1:]
	if len(b) > limit {
		b = b[:limit]
	}
	return b, nil
}

func (f *fakeStore) MarkSent(ctx context.Context, id int64, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.marked = append(f.marked, id)
	f.markedAt = append(f.markedAt, at)
	f.markCtxOK = append(f.markCtxOK, ctx.Err() == nil)
	return nil
}

func (f *fakeStore) markedIDs() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Sorted(slices.Values(f.marked))
}

type fakeNotifier struct {
	mu     sync.Mutex
	sent   []int64
	failID int64
	onSend func() // вызывается при каждой отправке (например, чтобы отменить ctx)
}

func (f *fakeNotifier) Send(_ context.Context, r domain.DueReminder) error {
	if f.onSend != nil {
		f.onSend()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.ID == f.failID {
		return errors.New("telegram is down")
	}
	f.sent = append(f.sent, r.ID)
	return nil
}

func (f *fakeNotifier) sentIDs() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Sorted(slices.Values(f.sent))
}

func due(ids ...int64) []domain.DueReminder {
	out := make([]domain.DueReminder, 0, len(ids))
	for _, id := range ids {
		out = append(out, domain.DueReminder{Reminder: domain.Reminder{ID: id, TaskID: id * 10, Kind: domain.KindDDay}})
	}
	return out
}

var fixedNow = time.Date(2026, time.October, 23, 7, 0, 0, 0, time.UTC)

// runUntil запускает планировщик и останавливает его, когда cond выполнится (или по таймауту).
func runUntil(t *testing.T, s *Scheduler, ctx context.Context, cancel context.CancelFunc, cond func() bool) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	deadline := time.After(2 * time.Second)
	for !cond() {
		select {
		case <-deadline:
			cancel()
			t.Fatal("condition not met in time")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}

func newTestScheduler(store Store, n Notifier) *Scheduler {
	return NewScheduler(store, n, 3, 10*time.Millisecond, func() time.Time { return fixedNow })
}

func TestRunSendsAndMarks(t *testing.T) {
	store := &fakeStore{batches: [][]domain.DueReminder{due(1, 2)}}
	n := &fakeNotifier{}
	ctx, cancel := context.WithCancel(context.Background())

	runUntil(t, newTestScheduler(store, n), ctx, cancel, func() bool { return len(store.markedIDs()) == 2 })

	if got := n.sentIDs(); !slices.Equal(got, []int64{1, 2}) {
		t.Errorf("sent = %v, want [1 2]", got)
	}
	for _, at := range store.markedAt {
		if !at.Equal(fixedNow) {
			t.Errorf("marked at %s, want %s", at, fixedNow)
		}
	}
}

func TestSendErrorIsNotMarked(t *testing.T) {
	store := &fakeStore{batches: [][]domain.DueReminder{due(1, 2, 3)}}
	n := &fakeNotifier{failID: 2}
	ctx, cancel := context.WithCancel(context.Background())

	runUntil(t, newTestScheduler(store, n), ctx, cancel, func() bool { return len(store.markedIDs()) == 2 })

	if got := store.markedIDs(); !slices.Equal(got, []int64{1, 3}) {
		t.Errorf("marked = %v, want [1 3] (failed send must not be marked)", got)
	}
}

func TestClaimErrorDoesNotStopLoop(t *testing.T) {
	store := &fakeStore{
		claimErrs: []error{errors.New("db hiccup")},
		batches:   [][]domain.DueReminder{due(5)},
	}
	n := &fakeNotifier{}
	ctx, cancel := context.WithCancel(context.Background())

	runUntil(t, newTestScheduler(store, n), ctx, cancel, func() bool { return len(store.markedIDs()) == 1 })

	if got := n.sentIDs(); !slices.Equal(got, []int64{5}) {
		t.Errorf("sent = %v, want [5]", got)
	}
}

func TestMarkSentSurvivesShutdown(t *testing.T) {
	store := &fakeStore{batches: [][]domain.DueReminder{due(1)}}
	ctx, cancel := context.WithCancel(context.Background())
	// Shutdown приходит прямо во время отправки: напоминание уже ушло, пометка должна пройти.
	n := &fakeNotifier{onSend: cancel}

	runUntil(t, newTestScheduler(store, n), ctx, cancel, func() bool { return len(store.markedIDs()) == 1 })

	store.mu.Lock()
	defer store.mu.Unlock()
	if !store.markCtxOK[0] {
		t.Error("MarkSent got a cancelled ctx; it must outlive shutdown")
	}
}

func TestRunStopsWithNothingToDo(t *testing.T) {
	store := &fakeStore{}
	ctx, cancel := context.WithCancel(context.Background())
	runUntil(t, newTestScheduler(store, &fakeNotifier{}), ctx, cancel, func() bool {
		store.mu.Lock()
		defer store.mu.Unlock()
		return store.calls >= 2 // первый проход сразу + хотя бы один тик
	})
}
