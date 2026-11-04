package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"deadline-bot/internal/domain"
	"deadline-bot/internal/logger"
	"deadline-bot/internal/service"
)

const ownerID int64 = 100

type fakeSender struct {
	sent     []*bot.SendMessageParams
	answered []*bot.AnswerCallbackQueryParams
	edited   []*bot.EditMessageTextParams
	sendErr  error
}

func (f *fakeSender) SendMessage(_ context.Context, p *bot.SendMessageParams) (*models.Message, error) {
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	f.sent = append(f.sent, p)
	return &models.Message{}, nil
}

func (f *fakeSender) AnswerCallbackQuery(_ context.Context, p *bot.AnswerCallbackQueryParams) (bool, error) {
	f.answered = append(f.answered, p)
	return true, nil
}

func (f *fakeSender) EditMessageText(_ context.Context, p *bot.EditMessageTextParams) (*models.Message, error) {
	f.edited = append(f.edited, p)
	return &models.Message{}, nil
}

type fakeService struct {
	added      []service.AddTaskInput
	tasks      []domain.Task
	doneIDs    []int64
	snoozed    []time.Duration
	tomorrowID int64
	snoozeAt   time.Time
	err        error
}

func (f *fakeService) AddTask(_ context.Context, in service.AddTaskInput) (domain.Task, error) {
	if f.err != nil {
		return domain.Task{}, f.err
	}
	f.added = append(f.added, in)
	return domain.Task{ID: 1, Subject: in.Subject, Title: in.Title, Deadline: in.Deadline}, nil
}

func (f *fakeService) ListActive(context.Context) ([]domain.Task, error) { return f.tasks, f.err }

func (f *fakeService) Done(_ context.Context, id int64) error {
	if f.err != nil {
		return f.err
	}
	f.doneIDs = append(f.doneIDs, id)
	return nil
}

func (f *fakeService) Snooze(_ context.Context, _ int64, d time.Duration) (time.Time, error) {
	if f.err != nil {
		return time.Time{}, f.err
	}
	f.snoozed = append(f.snoozed, d)
	return f.snoozeAt, nil
}

func (f *fakeService) SnoozeUntilTomorrow(_ context.Context, id int64) (time.Time, error) {
	if f.err != nil {
		return time.Time{}, f.err
	}
	f.tomorrowID = id
	return f.snoozeAt, nil
}

func newTestBot(t *testing.T, svc *fakeService, now time.Time) (*Bot, *fakeSender) {
	t.Helper()
	api := &fakeSender{}
	return &Bot{
		api:     api,
		svc:     svc,
		ownerID: ownerID,
		loc:     almaty(t),
		clock:   func() time.Time { return now },
		log:     logger.Get(),
	}, api
}

func message(chatID int64, text string) *models.Update {
	return &models.Update{Message: &models.Message{Chat: models.Chat{ID: chatID}, Text: text}}
}

func callback(fromID int64, data string) *models.Update {
	return &models.Update{CallbackQuery: &models.CallbackQuery{
		ID:   "cq1",
		From: models.User{ID: fromID},
		Data: data,
		Message: models.MaybeInaccessibleMessage{
			Message: &models.Message{ID: 55, Chat: models.Chat{ID: ownerID}, Text: "🚨 СЕГОДНЯ ДЕДЛАЙН"},
		},
	}}
}

func lastText(t *testing.T, api *fakeSender) string {
	t.Helper()
	if len(api.sent) == 0 {
		t.Fatal("no message sent")
	}
	return api.sent[len(api.sent)-1].Text
}

func TestHandleIgnoresStrangers(t *testing.T) {
	svc := &fakeService{}
	b, api := newTestBot(t, svc, time.Now())

	b.handle(context.Background(), message(999, "/list"))
	b.handle(context.Background(), callback(999, "done:1"))

	if len(api.sent) != 0 || len(api.answered) != 0 || len(svc.doneIDs) != 0 {
		t.Errorf("stranger got a reaction: sent=%d answered=%d done=%v", len(api.sent), len(api.answered), svc.doneIDs)
	}
}

func TestCommands(t *testing.T) {
	loc := almaty(t)
	now := time.Date(2026, time.October, 20, 10, 0, 0, 0, loc)
	ctx := context.Background()

	t.Run("add", func(t *testing.T) {
		svc := &fakeService{}
		b, api := newTestBot(t, svc, now)
		b.handle(ctx, message(ownerID, "/add БИС | Lab 5 | 23.10 23:59"))

		if len(svc.added) != 1 || svc.added[0].Subject != "БИС" {
			t.Fatalf("AddTask calls = %+v", svc.added)
		}
		if got := lastText(t, api); !strings.Contains(got, "#1") || !strings.Contains(got, "23.10 (пт) 23:59") {
			t.Errorf("reply = %q", got)
		}
	})

	t.Run("add with bot mention", func(t *testing.T) {
		svc := &fakeService{}
		b, _ := newTestBot(t, svc, now)
		b.handle(ctx, message(ownerID, "/add@DeadlineBot БИС | Lab | 23.10"))
		if len(svc.added) != 1 {
			t.Errorf("AddTask not called for /add@bot")
		}
	})

	t.Run("add bad input shows parser error", func(t *testing.T) {
		svc := &fakeService{}
		b, api := newTestBot(t, svc, now)
		b.handle(ctx, message(ownerID, "/add БИС | Lab | 19.10"))

		if len(svc.added) != 0 {
			t.Error("service must not be called on bad input")
		}
		if got := lastText(t, api); !strings.Contains(got, "уже прошла") {
			t.Errorf("reply = %q", got)
		}
	})

	t.Run("list empty", func(t *testing.T) {
		b, api := newTestBot(t, &fakeService{}, now)
		b.handle(ctx, message(ownerID, "/list"))
		if got := lastText(t, api); !strings.Contains(got, "нет") {
			t.Errorf("reply = %q", got)
		}
	})

	t.Run("list marks overdue", func(t *testing.T) {
		svc := &fakeService{tasks: []domain.Task{
			{ID: 3, Subject: "БИС", Title: "Old", Deadline: now.Add(-time.Hour)},
			{ID: 4, Subject: "ФБИС", Title: "New", Deadline: now.Add(26 * time.Hour)},
		}}
		b, api := newTestBot(t, svc, now)
		b.handle(ctx, message(ownerID, "/list"))
		got := lastText(t, api)
		if !strings.Contains(got, "#3") || !strings.Contains(got, "просрочено") || !strings.Contains(got, "через 1д 2ч") {
			t.Errorf("reply = %q", got)
		}
	})

	t.Run("done not found", func(t *testing.T) {
		b, api := newTestBot(t, &fakeService{err: domain.ErrNotFound}, now)
		b.handle(ctx, message(ownerID, "/done 7"))
		if got := lastText(t, api); !strings.Contains(got, "#7 нет") {
			t.Errorf("reply = %q", got)
		}
	})

	t.Run("done bad id", func(t *testing.T) {
		svc := &fakeService{}
		b, api := newTestBot(t, svc, now)
		b.handle(ctx, message(ownerID, "/done abc"))
		if len(svc.doneIDs) != 0 || !strings.Contains(lastText(t, api), "❌") {
			t.Errorf("done=%v reply=%q", svc.doneIDs, lastText(t, api))
		}
	})

	t.Run("snooze", func(t *testing.T) {
		svc := &fakeService{snoozeAt: now.Add(3 * time.Hour)}
		b, api := newTestBot(t, svc, now)
		b.handle(ctx, message(ownerID, "/snooze 7 3ч"))
		if len(svc.snoozed) != 1 || svc.snoozed[0] != 3*time.Hour {
			t.Fatalf("snoozed = %v", svc.snoozed)
		}
		if got := lastText(t, api); !strings.Contains(got, "13:00") {
			t.Errorf("reply = %q", got)
		}
	})

	t.Run("internal error is not leaked", func(t *testing.T) {
		b, api := newTestBot(t, &fakeService{err: errors.New("pq: connection refused")}, now)
		b.handle(ctx, message(ownerID, "/list"))
		if got := lastText(t, api); strings.Contains(got, "pq") {
			t.Errorf("internal error leaked: %q", got)
		}
	})

	t.Run("unknown command shows help", func(t *testing.T) {
		b, api := newTestBot(t, &fakeService{}, now)
		b.handle(ctx, message(ownerID, "привет"))
		if got := lastText(t, api); !strings.Contains(got, "/add") {
			t.Errorf("reply = %q", got)
		}
	})
}

func TestCallbacks(t *testing.T) {
	loc := almaty(t)
	now := time.Date(2026, time.October, 20, 12, 0, 0, 0, loc)
	ctx := context.Background()

	t.Run("done removes buttons", func(t *testing.T) {
		svc := &fakeService{}
		b, api := newTestBot(t, svc, now)
		b.handle(ctx, callback(ownerID, "done:7"))

		if len(svc.doneIDs) != 1 || svc.doneIDs[0] != 7 {
			t.Fatalf("done = %v", svc.doneIDs)
		}
		if len(api.answered) != 1 {
			t.Fatal("callback must always be answered")
		}
		if len(api.edited) != 1 {
			t.Fatal("message must be edited")
		}
		e := api.edited[0]
		if e.MessageID != 55 || e.ReplyMarkup != nil || !strings.Contains(e.Text, "✅ Сдано") {
			t.Errorf("edit = %+v", e)
		}
	})

	t.Run("snooze 3h", func(t *testing.T) {
		svc := &fakeService{snoozeAt: now.Add(3 * time.Hour)}
		b, api := newTestBot(t, svc, now)
		b.handle(ctx, callback(ownerID, "snooze:7:3h"))
		if len(svc.snoozed) != 1 || svc.snoozed[0] != 3*time.Hour {
			t.Fatalf("snoozed = %v", svc.snoozed)
		}
		if !strings.Contains(api.edited[0].Text, "15:00") {
			t.Errorf("edit text = %q", api.edited[0].Text)
		}
	})

	t.Run("snooze tomorrow", func(t *testing.T) {
		svc := &fakeService{snoozeAt: now.Add(24 * time.Hour)}
		b, _ := newTestBot(t, svc, now)
		b.handle(ctx, callback(ownerID, "snooze:7:tomorrow"))
		if svc.tomorrowID != 7 {
			t.Errorf("SnoozeUntilTomorrow id = %d", svc.tomorrowID)
		}
	})

	t.Run("already done keeps message, still answers", func(t *testing.T) {
		b, api := newTestBot(t, &fakeService{err: domain.ErrNotFound}, now)
		b.handle(ctx, callback(ownerID, "done:7"))
		if len(api.answered) != 1 || !strings.Contains(api.answered[0].Text, "#7 нет") {
			t.Errorf("answered = %+v", api.answered)
		}
		if len(api.edited) != 0 {
			t.Error("message must not be edited on error")
		}
	})

	for _, data := range []string{"", "done", "done:x", "snooze:7", "snooze:7:5h", "boom:7"} {
		t.Run("bad data "+data, func(t *testing.T) {
			svc := &fakeService{}
			b, api := newTestBot(t, svc, now)
			b.handle(ctx, callback(ownerID, data))
			if len(api.answered) != 1 || len(api.edited) != 0 || len(svc.doneIDs)+len(svc.snoozed) != 0 {
				t.Errorf("answered=%d edited=%d", len(api.answered), len(api.edited))
			}
		})
	}

	t.Run("inaccessible message is not edited", func(t *testing.T) {
		svc := &fakeService{}
		b, api := newTestBot(t, svc, now)
		u := callback(ownerID, "done:7")
		u.CallbackQuery.Message = models.MaybeInaccessibleMessage{}
		b.handle(ctx, u)
		if len(svc.doneIDs) != 1 || len(api.answered) != 1 || len(api.edited) != 0 {
			t.Errorf("done=%v answered=%d edited=%d", svc.doneIDs, len(api.answered), len(api.edited))
		}
	})
}
