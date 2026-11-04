package telegram

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"golang.org/x/time/rate"

	"deadline-bot/internal/domain"
)

// Telegram допускает около одного сообщения в секунду в один чат.
const sendInterval = time.Second

// Notifier отправляет напоминания владельцу. Реализует scheduler.Notifier.
type Notifier struct {
	api     sender
	chatID  int64
	loc     *time.Location
	clock   func() time.Time
	limiter *rate.Limiter
}

// Notifier создаёт отправщика напоминаний поверх того же клиента Telegram.
func (b *Bot) Notifier() *Notifier {
	return newNotifier(b.api, b.ownerID, b.loc, b.clock)
}

func newNotifier(api sender, chatID int64, loc *time.Location, clock func() time.Time) *Notifier {
	return &Notifier{
		api:     api,
		chatID:  chatID,
		loc:     loc,
		clock:   clock,
		limiter: rate.NewLimiter(rate.Every(sendInterval), 1),
	}
}

// Send отправляет напоминание с кнопками. Ошибка — напоминание не доставлено (планировщик повторит).
func (n *Notifier) Send(ctx context.Context, r domain.DueReminder) error {
	if err := n.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limit wait: %w", err)
	}
	_, err := n.api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      n.chatID,
		Text:        reminderText(r, n.clock(), n.loc),
		ReplyMarkup: reminderKeyboard(r.TaskID),
	})
	if err != nil {
		return fmt.Errorf("send reminder %d: %w", r.ID, err)
	}
	return nil
}

func reminderText(r domain.DueReminder, now time.Time, loc *time.Location) string {
	t := r.Task
	what := fmt.Sprintf("%s — %s (#%d)", t.Subject, t.Title, t.ID)
	deadline := formatLocal(t.Deadline, loc)

	switch r.Kind {
	case domain.KindD2:
		return fmt.Sprintf("📌 Через 2 дня дедлайн\n%s\nДо %s", what, deadline)
	case domain.KindD1:
		return fmt.Sprintf("⚠️ Завтра дедлайн\n%s\nДо %s", what, deadline)
	case domain.KindDDay:
		return fmt.Sprintf("🚨 СЕГОДНЯ ДЕДЛАЙН, СДАЙ!\n%s\nДо %s", what, deadline)
	case domain.KindNag:
		if !t.Deadline.After(now) {
			return fmt.Sprintf("⛔ ДЕДЛАЙН ПРОШЁЛ, А ТЫ НЕ СДАЛ!\n%s\nБыл до %s", what, deadline)
		}
		return fmt.Sprintf("🔔 ВСЁ ЕЩЁ НЕ СДАНО! Осталось %s\n%s\nДо %s",
			humanDuration(t.Deadline.Sub(now)), what, deadline)
	default: // snooze
		return fmt.Sprintf("⏰ Напоминаю\n%s\nДо %s", what, deadline)
	}
}

func reminderKeyboard(taskID int64) *models.InlineKeyboardMarkup {
	id := strconv.FormatInt(taskID, 10)
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "Сдал ✅", CallbackData: cbDone + ":" + id}},
		{
			{Text: "Отложить на 3ч", CallbackData: cbSnooze + ":" + id + ":" + cbSnooze3h},
			{Text: "До завтра", CallbackData: cbSnooze + ":" + id + ":" + cbSnoozeTomorrow},
		},
	}}
}

var weekdays = [...]string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}

// formatLocal: «23.10 (пт) 23:59» по loc.
func formatLocal(t time.Time, loc *time.Location) string {
	l := t.In(loc)
	return fmt.Sprintf("%s (%s) %s", l.Format("02.01"), weekdays[l.Weekday()], l.Format("15:04"))
}

// humanDuration: «2д 3ч», «3ч 59м», «15м». Меньше минуты — «<1м».
func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return "<1м"
	}
	days := int(d / (24 * time.Hour))
	hours := int(d % (24 * time.Hour) / time.Hour)
	mins := int(d % time.Hour / time.Minute)
	switch {
	case days > 0:
		return fmt.Sprintf("%dд %dч", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dч %dм", hours, mins)
	default:
		return fmt.Sprintf("%dм", mins)
	}
}
