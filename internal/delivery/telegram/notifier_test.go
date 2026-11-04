package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"

	"deadline-bot/internal/domain"
)

func dueReminder(kind domain.ReminderKind, deadline time.Time) domain.DueReminder {
	return domain.DueReminder{
		Reminder: domain.Reminder{ID: 9, TaskID: 7, Kind: kind},
		Task:     domain.Task{ID: 7, Subject: "БИС", Title: "Lab 5", Deadline: deadline},
	}
}

func TestNotifierSend(t *testing.T) {
	loc := almaty(t)
	now := time.Date(2026, time.October, 23, 12, 0, 0, 0, loc)
	deadline := time.Date(2026, time.October, 23, 23, 59, 0, 0, loc)
	ctx := context.Background()

	t.Run("dday is loud and has buttons", func(t *testing.T) {
		api := &fakeSender{}
		n := newNotifier(api, ownerID, loc, func() time.Time { return now })
		if err := n.Send(ctx, dueReminder(domain.KindDDay, deadline)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		p := api.sent[0]
		if p.ChatID != ownerID {
			t.Errorf("chat = %v, want owner", p.ChatID)
		}
		if !strings.Contains(p.Text, "СЕГОДНЯ ДЕДЛАЙН, СДАЙ") || !strings.Contains(p.Text, "#7") {
			t.Errorf("text = %q", p.Text)
		}
		kb, ok := p.ReplyMarkup.(*models.InlineKeyboardMarkup)
		if !ok {
			t.Fatalf("reply markup = %T", p.ReplyMarkup)
		}
		var data []string
		for _, row := range kb.InlineKeyboard {
			for _, btn := range row {
				data = append(data, btn.CallbackData)
			}
		}
		want := "done:7 snooze:7:3h snooze:7:tomorrow"
		if got := strings.Join(data, " "); got != want {
			t.Errorf("buttons = %q, want %q", got, want)
		}
	})

	t.Run("buttons round-trip through callback handler", func(t *testing.T) {
		// Каждая кнопка напоминания должна распознаваться обработчиком.
		for _, row := range reminderKeyboard(7).InlineKeyboard {
			for _, btn := range row {
				svc := &fakeService{snoozeAt: now}
				b, _ := newTestBot(t, svc, now)
				if _, status := b.applyCallback(ctx, btn.CallbackData); status == "" {
					t.Errorf("button %q not handled", btn.CallbackData)
				}
			}
		}
	})

	t.Run("send error is returned", func(t *testing.T) {
		boom := errors.New("boom")
		n := newNotifier(&fakeSender{sendErr: boom}, ownerID, loc, func() time.Time { return now })
		if err := n.Send(ctx, dueReminder(domain.KindD1, deadline)); !errors.Is(err, boom) {
			t.Errorf("err = %v, want boom", err)
		}
	})

	t.Run("cancelled ctx sends nothing", func(t *testing.T) {
		api := &fakeSender{}
		n := newNotifier(api, ownerID, loc, func() time.Time { return now })
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		if err := n.Send(cctx, dueReminder(domain.KindD1, deadline)); !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
		if len(api.sent) != 0 {
			t.Error("must not send with cancelled ctx")
		}
	})
}

func TestReminderText(t *testing.T) {
	loc := almaty(t)
	deadline := time.Date(2026, time.October, 23, 23, 59, 0, 0, loc)

	tests := []struct {
		kind domain.ReminderKind
		now  time.Time
		want string
	}{
		{domain.KindD2, deadline.Add(-60 * time.Hour), "Через 2 дня"},
		{domain.KindD1, deadline.Add(-36 * time.Hour), "Завтра"},
		{domain.KindDDay, deadline.Add(-12 * time.Hour), "СЕГОДНЯ ДЕДЛАЙН"},
		{domain.KindNag, deadline.Add(-(3*time.Hour + 59*time.Minute)), "Осталось 3ч 59м"},
		{domain.KindNag, deadline.Add(time.Minute), "ДЕДЛАЙН ПРОШЁЛ"},
		{domain.KindSnooze, deadline.Add(-5 * time.Hour), "Напоминаю"},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind)+" "+tt.want, func(t *testing.T) {
			got := reminderText(dueReminder(tt.kind, deadline), tt.now, loc)
			if !strings.Contains(got, tt.want) || !strings.Contains(got, "23.10 (пт) 23:59") {
				t.Errorf("text = %q, want it to contain %q and the deadline", got, tt.want)
			}
		})
	}
}

func TestHumanDuration(t *testing.T) {
	tests := map[time.Duration]string{
		30 * time.Second:              "<1м",
		15 * time.Minute:              "15м",
		3*time.Hour + 59*time.Minute:  "3ч 59м",
		26*time.Hour + 30*time.Minute: "1д 2ч",
		50 * time.Hour:                "2д 2ч",
	}
	for d, want := range tests {
		if got := humanDuration(d); got != want {
			t.Errorf("humanDuration(%s) = %q, want %q", d, got, want)
		}
	}
}
