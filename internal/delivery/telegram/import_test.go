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

// Реальные тексты из Teams Assignments, предмет дописан первой строкой.
const (
	teamsFBIS = `фбис
Reading Exercise: Business Intelligence
Срок заканчивается 10 ноября 2026 г., 23:59
•
Допускается несколько отправок
Инструкции
Read case on pp.512-514 and answer questions on p.514`

	teamsAssignment4 = `БИС
Assignment 4
Срок заканчивается 8 ноября 2026 г., 23:59
•
Допускается несколько отправок
Инструкции
You can do with in word, PowerPoint,…But submit with pdf format.

Add your full name, ID, and practice lessons dates to the first slide.`
)

func TestParseImport(t *testing.T) {
	loc := almaty(t)
	now := time.Date(2026, time.November, 5, 10, 0, 0, 0, loc)

	ok := []struct {
		name, text, subject, title, details string
		want                                time.Time
	}{
		{"teams fbis", teamsFBIS, "ФБИС", "Reading Exercise: Business Intelligence",
			"Read case on pp.512-514 and answer questions on p.514", time.Date(2026, 11, 10, 23, 59, 0, 0, loc)},
		{"teams assignment 4", teamsAssignment4, "БИС", "Assignment 4",
			"You can do with in word, PowerPoint,…But submit with pdf format.\nAdd your full name, ID, and practice lessons dates to the first slide.",
			time.Date(2026, 11, 8, 23, 59, 0, 0, loc)},
		{"no year no time", "БИС\nLab 5\nСрок заканчивается 3 декабря", "БИС", "Lab 5", "",
			time.Date(2026, 12, 3, 23, 59, 0, 0, loc)},
		{"срок сдачи with colon", "БИС\nLab 6\nСрок сдачи: 3 декабря, 18:00", "БИС", "Lab 6", "",
			time.Date(2026, 12, 3, 18, 0, 0, 0, loc)},
		{"multiword subject kept as is", "Бизнес аналитика\nLab\nСрок заканчивается 3 декабря 2026 г., 9:05",
			"Бизнес аналитика", "Lab", "", time.Date(2026, 12, 3, 9, 5, 0, 0, loc)},
		{"crlf and spaces", "  фбис \r\n\r\n  Essay   1 \r\nСрок   заканчивается 10 ноября 2026 г., 23:59\r\n", "ФБИС", "Essay 1", "",
			time.Date(2026, 11, 10, 23, 59, 0, 0, loc)},
	}
	for _, tt := range ok {
		t.Run(tt.name, func(t *testing.T) {
			in, err := ParseImport(tt.text, now, loc)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if in.Subject != tt.subject || in.Title != tt.title || in.Details != tt.details {
				t.Errorf("got %q / %q / %q", in.Subject, in.Title, in.Details)
			}
			if !in.Deadline.Equal(tt.want) || in.Deadline.Location() != time.UTC {
				t.Errorf("deadline = %v, want %v (UTC)", in.Deadline, tt.want)
			}
		})
	}

	bad := []struct{ name, text, wantMsg string }{
		{"no due line", "БИС\nLab 5\nсдать до пятницы", "не нашёл строку со сроком"},
		{"no subject", "Assignment 4\nСрок заканчивается 8 ноября 2026 г., 23:59", "не вижу предмета"},
		{"past", "БИС\nLab\nСрок заканчивается 1 ноября 2026 г., 23:59", "уже прошла"},
		{"too far", "БИС\nLab\nСрок заканчивается 1 декабря 2028 г., 23:59", "дальше чем через год"},
		{"bad month", "БИС\nLab\nСрок заканчивается 1 ноябряя 2026 г., 23:59", "не понял месяц"},
		{"bad day", "БИС\nLab\nСрок заканчивается 31 ноября 2026 г.", "такой даты нет"},
		{"bad time", "БИС\nLab\nСрок заканчивается 10 ноября 2026 г., 25:00", "не понял время"},
		{"long subject", strings.Repeat("слово ", 20) + "\nLab\nСрок заканчивается 10 ноября", "предмет длиннее"},
	}
	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseImport(tt.text, now, loc)
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("err = %q, want %q", err, tt.wantMsg)
			}
		})
	}
}

func TestParseImportTruncatesDetails(t *testing.T) {
	loc := almaty(t)
	now := time.Date(2026, time.November, 5, 10, 0, 0, 0, loc)
	in, err := ParseImport("БИС\nLab\nСрок заканчивается 10 ноября\n"+strings.Repeat("я", 5000), now, loc)
	if err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(in.Details)); n != maxDetailsLen {
		t.Errorf("details len = %d, want %d", n, maxDetailsLen)
	}
}

func draftButtons(t *testing.T, api *fakeSender) (add, cancel string) {
	t.Helper()
	kb, ok := api.sent[len(api.sent)-1].ReplyMarkup.(*models.InlineKeyboardMarkup)
	if !ok || len(kb.InlineKeyboard) != 1 || len(kb.InlineKeyboard[0]) != 2 {
		t.Fatalf("draft keyboard = %+v", api.sent[len(api.sent)-1].ReplyMarkup)
	}
	return kb.InlineKeyboard[0][0].CallbackData, kb.InlineKeyboard[0][1].CallbackData
}

func TestImportFlow(t *testing.T) {
	loc := almaty(t)
	now := time.Date(2026, time.November, 5, 10, 0, 0, 0, loc)
	ctx := context.Background()

	t.Run("draft then add", func(t *testing.T) {
		svc := &fakeService{}
		b, api := newTestBot(t, svc, now)
		b.handle(ctx, message(ownerID, teamsFBIS))

		if len(svc.added) != 0 {
			t.Fatal("nothing must be saved before confirmation")
		}
		got := lastText(t, api)
		for _, want := range []string{"ФБИС — Reading Exercise", "10.11 (вт) 23:59", "Read case"} {
			if !strings.Contains(got, want) {
				t.Errorf("draft %q lacks %q", got, want)
			}
		}

		add, _ := draftButtons(t, api)
		b.handle(ctx, callback(ownerID, add))
		if len(svc.added) != 1 || svc.added[0].Subject != "ФБИС" || svc.added[0].Details == "" {
			t.Fatalf("AddTask calls = %+v", svc.added)
		}
		if len(api.edited) != 1 || !strings.Contains(api.edited[0].Text, "✅ Добавил #1") {
			t.Errorf("edited = %+v", api.edited)
		}

		// Повторное нажатие не задваивает задачу.
		b.handle(ctx, callback(ownerID, add))
		if len(svc.added) != 1 {
			t.Errorf("second press added again: %d", len(svc.added))
		}
		if a := api.answered[len(api.answered)-1].Text; !strings.Contains(a, "устарел") {
			t.Errorf("answer = %q", a)
		}
	})

	t.Run("cancel", func(t *testing.T) {
		svc := &fakeService{}
		b, api := newTestBot(t, svc, now)
		b.handle(ctx, message(ownerID, teamsAssignment4))
		add, cancel := draftButtons(t, api)

		b.handle(ctx, callback(ownerID, cancel))
		b.handle(ctx, callback(ownerID, add))
		if len(svc.added) != 0 {
			t.Errorf("cancelled draft was added: %+v", svc.added)
		}
	})

	t.Run("expired draft", func(t *testing.T) {
		svc := &fakeService{}
		b, api := newTestBot(t, svc, now)
		b.handle(ctx, message(ownerID, teamsAssignment4))
		add, _ := draftButtons(t, api)

		b.clock = func() time.Time { return now.Add(draftTTL + time.Minute) }
		b.handle(ctx, callback(ownerID, add))
		if len(svc.added) != 0 {
			t.Error("expired draft was added")
		}
	})

	t.Run("duplicate warning", func(t *testing.T) {
		svc := &fakeService{tasks: []domain.Task{{ID: 7, Subject: "БИС", Title: "assignment 4"}}}
		b, api := newTestBot(t, svc, now)
		b.handle(ctx, message(ownerID, teamsAssignment4))
		if got := lastText(t, api); !strings.Contains(got, "⚠️ Похоже, это уже есть: #7") {
			t.Errorf("reply = %q", got)
		}
	})

	t.Run("unparsable text shows hint", func(t *testing.T) {
		svc := &fakeService{}
		b, api := newTestBot(t, svc, now)
		b.handle(ctx, message(ownerID, "привет"))
		got := lastText(t, api)
		if !strings.Contains(got, "не нашёл строку со сроком") {
			t.Errorf("reply = %q", got)
		}
		if api.sent[len(api.sent)-1].ReplyMarkup != nil {
			t.Error("error reply must have no buttons")
		}
	})

	t.Run("caption of forwarded file", func(t *testing.T) {
		svc := &fakeService{}
		b, api := newTestBot(t, svc, now)
		u := &models.Update{Message: &models.Message{Chat: models.Chat{ID: ownerID}, Caption: teamsFBIS}}
		b.handle(ctx, u)
		if got := lastText(t, api); !strings.Contains(got, "Нашёл задание") {
			t.Errorf("reply = %q", got)
		}
	})
}

func pdfMessage(caption string) *models.Update {
	return &models.Update{Message: &models.Message{
		Chat:     models.Chat{ID: ownerID},
		Caption:  caption,
		Document: &models.Document{FileID: "pdf-1", FileName: "assignment.pdf", MimeType: "application/pdf"},
	}}
}

func TestImportWithFile(t *testing.T) {
	loc := almaty(t)
	now := time.Date(2026, time.November, 5, 10, 0, 0, 0, loc)
	ctx := context.Background()

	t.Run("pdf with caption", func(t *testing.T) {
		svc := &fakeService{}
		b, api := newTestBot(t, svc, now)
		b.handle(ctx, pdfMessage(teamsAssignment4))
		if got := lastText(t, api); !strings.Contains(got, "📎 Файл прикреплён") {
			t.Errorf("draft = %q", got)
		}
		add, _ := draftButtons(t, api)
		b.handle(ctx, callback(ownerID, add))
		if len(svc.added) != 1 || svc.added[0].FileID != "pdf-1" {
			t.Fatalf("added = %+v", svc.added)
		}
	})

	t.Run("text then pdf attaches to latest draft", func(t *testing.T) {
		svc := &fakeService{}
		b, api := newTestBot(t, svc, now)
		b.handle(ctx, message(ownerID, teamsFBIS))
		b.handle(ctx, message(ownerID, teamsAssignment4))
		b.handle(ctx, pdfMessage(""))

		got := lastText(t, api)
		if !strings.Contains(got, "Прикрепил") || !strings.Contains(got, "Assignment 4") {
			t.Errorf("reply = %q", got)
		}
		add, _ := draftButtons(t, api)
		b.handle(ctx, callback(ownerID, add))
		if len(svc.added) != 1 || svc.added[0].Title != "Assignment 4" || svc.added[0].FileID != "pdf-1" {
			t.Fatalf("added = %+v", svc.added)
		}
	})

	t.Run("old draft buttons see the attached file", func(t *testing.T) {
		svc := &fakeService{}
		b, api := newTestBot(t, svc, now)
		b.handle(ctx, message(ownerID, teamsFBIS))
		oldAdd, _ := draftButtons(t, api)
		b.handle(ctx, pdfMessage(""))
		b.handle(ctx, callback(ownerID, oldAdd))
		if len(svc.added) != 1 || svc.added[0].FileID != "pdf-1" {
			t.Fatalf("added = %+v", svc.added)
		}
	})

	t.Run("pdf without caption and no draft", func(t *testing.T) {
		svc := &fakeService{}
		b, api := newTestBot(t, svc, now)
		b.handle(ctx, pdfMessage(""))
		if got := lastText(t, api); !strings.Contains(got, "не к чему прикрепить") {
			t.Errorf("reply = %q", got)
		}
	})

	t.Run("file button sends document and keeps buttons", func(t *testing.T) {
		svc := &fakeService{tasks: []domain.Task{{ID: 7, Subject: "БИС", Title: "Lab", FileID: "pdf-1"}}}
		b, api := newTestBot(t, svc, now)
		b.handle(ctx, callback(ownerID, "file:7"))
		if len(api.docs) != 1 {
			t.Fatalf("docs = %d, want 1", len(api.docs))
		}
		if f, ok := api.docs[0].Document.(*models.InputFileString); !ok || f.Data != "pdf-1" {
			t.Errorf("document = %+v", api.docs[0].Document)
		}
		if len(api.edited) != 0 {
			t.Error("reminder must keep its buttons after file request")
		}
	})

	t.Run("file button on task without file", func(t *testing.T) {
		svc := &fakeService{tasks: []domain.Task{{ID: 7, Subject: "БИС", Title: "Lab"}}}
		b, api := newTestBot(t, svc, now)
		b.handle(ctx, callback(ownerID, "file:7"))
		if len(api.docs) != 0 {
			t.Error("nothing to send")
		}
		if a := api.answered[0].Text; !strings.Contains(a, "файл не прикреплён") {
			t.Errorf("answer = %q", a)
		}
	})

	t.Run("list marks tasks with files", func(t *testing.T) {
		svc := &fakeService{tasks: []domain.Task{{ID: 7, Subject: "БИС", Title: "Lab", FileID: "pdf-1",
			Deadline: now.Add(48 * time.Hour)}}}
		b, api := newTestBot(t, svc, now)
		b.handle(ctx, message(ownerID, "/list"))
		if got := lastText(t, api); !strings.Contains(got, "#7 БИС — Lab 📎") {
			t.Errorf("list = %q", got)
		}
	})
}
