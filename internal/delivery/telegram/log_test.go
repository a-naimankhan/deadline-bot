package telegram

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"

	"deadline-bot/internal/domain"
)

// newLoggedBot — тестовый бот, который пишет JSON-лог в буфер.
func newLoggedBot(t *testing.T, svc *fakeService, now time.Time) (*Bot, *bytes.Buffer) {
	t.Helper()
	b, _ := newTestBot(t, svc, now)
	var buf bytes.Buffer
	b.log = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return b, &buf
}

func TestLogging(t *testing.T) {
	loc := almaty(t)
	now := time.Date(2026, time.October, 20, 10, 0, 0, 0, loc)
	ctx := context.Background()

	tests := []struct {
		name    string
		svc     *fakeService
		update  *models.Update
		want    []string
		notWant []string
	}{
		{
			name:    "add ok logs id but not task text",
			svc:     &fakeService{},
			update:  message(ownerID, "/add БИС | Секретная лаба | 23.10 23:59"),
			want:    []string{`"msg":"command handled"`, `"cmd":"/add"`, `"task_id":1`, `"result":"ok"`, `"level":"INFO"`},
			notWant: []string{"Секретная", "БИС"},
		},
		{
			name:    "bad input is info without user text",
			svc:     &fakeService{},
			update:  message(ownerID, "/add БИС | Lab | завтра"),
			want:    []string{`"result":"invalid_input"`, `"level":"INFO"`},
			notWant: []string{"завтра"},
		},
		{
			name:   "not found",
			svc:    &fakeService{err: domain.ErrNotFound},
			update: message(ownerID, "/done 7"),
			want:   []string{`"cmd":"/done"`, `"task_id":7`, `"result":"not_found"`},
		},
		{
			name:   "internal error is logged as error with details",
			svc:    &fakeService{err: errors.New("db is down")},
			update: message(ownerID, "/list"),
			want:   []string{`"level":"ERROR"`, `"result":"error"`, "db is down"},
		},
		{
			name:    "free text is logged as import without the text",
			svc:     &fakeService{},
			update:  message(ownerID, "привет, как дела"),
			want:    []string{`"cmd":"import"`, `"result":"invalid_input"`},
			notWant: []string{"привет"},
		},
		{
			name:    "import draft logs draft_id, not task_id",
			svc:     &fakeService{},
			update:  message(ownerID, teamsFBIS),
			want:    []string{`"cmd":"import"`, `"draft_id":1`},
			notWant: []string{"task_id", "Reading"},
		},
		{
			name:   "unknown slash command",
			svc:    &fakeService{},
			update: message(ownerID, "/foo"),
			want:   []string{`"cmd":"unknown"`},
		},
		{
			name:   "callback",
			svc:    &fakeService{snoozeAt: now},
			update: callback(ownerID, "snooze:7:tomorrow"),
			want:   []string{`"msg":"callback handled"`, `"action":"snooze_tomorrow"`, `"task_id":7`, `"result":"ok"`},
		},
		{
			name:   "unknown callback",
			svc:    &fakeService{},
			update: callback(ownerID, "boom:1"),
			want:   []string{`"msg":"unknown callback"`, `"level":"WARN"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, buf := newLoggedBot(t, tt.svc, now)
			b.handle(ctx, tt.update)
			out := buf.String()
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("log %s\nmissing %s", out, w)
				}
			}
			for _, nw := range tt.notWant {
				if strings.Contains(out, nw) {
					t.Errorf("log %s\nmust not contain %q", out, nw)
				}
			}
		})
	}
}
