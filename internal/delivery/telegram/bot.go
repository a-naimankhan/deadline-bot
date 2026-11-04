package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"deadline-bot/internal/domain"
	"deadline-bot/internal/logger"
	"deadline-bot/internal/service"
)

// TaskService — то, что telegram-слою нужно от сервиса.
type TaskService interface {
	AddTask(ctx context.Context, in service.AddTaskInput) (domain.Task, error)
	ListActive(ctx context.Context) ([]domain.Task, error)
	Done(ctx context.Context, taskID int64) error
	Snooze(ctx context.Context, taskID int64, d time.Duration) (time.Time, error)
	SnoozeUntilTomorrow(ctx context.Context, taskID int64) (time.Time, error)
}

// sender — минимум Bot API, который нужен слою. *bot.Bot его реализует, в тестах — фейк.
type sender interface {
	SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error)
	AnswerCallbackQuery(ctx context.Context, params *bot.AnswerCallbackQueryParams) (bool, error)
	EditMessageText(ctx context.Context, params *bot.EditMessageTextParams) (*models.Message, error)
}

type Bot struct {
	tg      *bot.Bot
	api     sender
	svc     TaskService
	ownerID int64
	loc     *time.Location
	clock   func() time.Time
	log     *slog.Logger
}

// New создаёт бота. bot.New ходит в Telegram (getMe), поэтому неверный токен — ошибка здесь.
func New(token string, ownerID int64, svc TaskService, loc *time.Location, clock func() time.Time) (*Bot, error) {
	b := &Bot{svc: svc, ownerID: ownerID, loc: loc, clock: clock, log: logger.Get()}
	tg, err := bot.New(token,
		bot.WithDefaultHandler(func(ctx context.Context, _ *bot.Bot, u *models.Update) { b.handle(ctx, u) }),
		bot.WithErrorsHandler(func(err error) { b.log.Error("telegram", "err", err) }),
	)
	if err != nil {
		return nil, fmt.Errorf("init telegram bot: %w", err)
	}
	b.tg, b.api = tg, tg
	return b, nil
}

// Run слушает обновления (long polling) и блокируется до отмены ctx.
func (b *Bot) Run(ctx context.Context) error {
	_, err := b.tg.SetMyCommands(ctx, &bot.SetMyCommandsParams{Commands: []models.BotCommand{
		{Command: "add", Description: "Добавить: Предмет | Название | ДД.ММ ЧЧ:ММ"},
		{Command: "list", Description: "Несданные задачи"},
		{Command: "done", Description: "Сдал: /done <id>"},
		{Command: "snooze", Description: "Отложить: /snooze <id> 3h"},
	}})
	if err != nil {
		b.log.Warn("set bot commands", "err", err)
	}
	b.tg.Start(ctx)
	return nil
}

func (b *Bot) handle(ctx context.Context, u *models.Update) {
	switch {
	case u.Message != nil:
		if u.Message.Chat.ID != b.ownerID {
			b.log.Warn("ignored message from stranger", "chat_id", u.Message.Chat.ID)
			return
		}
		b.handleMessage(ctx, u.Message)
	case u.CallbackQuery != nil:
		if u.CallbackQuery.From.ID != b.ownerID {
			b.log.Warn("ignored callback from stranger", "user_id", u.CallbackQuery.From.ID)
			return
		}
		b.handleCallback(ctx, u.CallbackQuery)
	}
}

func (b *Bot) handleMessage(ctx context.Context, m *models.Message) {
	cmd, args := splitCommand(m.Text)
	var reply string
	switch cmd {
	case "/start", "/help":
		reply = helpText
	case "/add":
		reply = b.cmdAdd(ctx, args)
	case "/list":
		reply = b.cmdList(ctx)
	case "/done":
		reply = b.cmdDone(ctx, args)
	case "/snooze":
		reply = b.cmdSnooze(ctx, args)
	default:
		reply = "Не понял. " + helpText
	}
	b.reply(ctx, m.Chat.ID, reply)
}

// splitCommand: "/add@MyBot  БИС | ..." → ("/add", "БИС | ...").
func splitCommand(text string) (string, string) {
	text = strings.TrimSpace(text)
	cmd, args, _ := strings.Cut(text, " ")
	cmd, _, _ = strings.Cut(cmd, "@")
	return strings.ToLower(cmd), strings.TrimSpace(args)
}

func (b *Bot) cmdAdd(ctx context.Context, args string) string {
	in, err := ParseAdd(args, b.clock(), b.loc)
	if err != nil {
		return b.userError(err)
	}
	t, err := b.svc.AddTask(ctx, in)
	if err != nil {
		return b.userError(err)
	}
	return fmt.Sprintf("Добавил #%d: %s — %s\nДедлайн: %s", t.ID, t.Subject, t.Title, b.fmtTime(t.Deadline))
}

func (b *Bot) cmdList(ctx context.Context) string {
	tasks, err := b.svc.ListActive(ctx)
	if err != nil {
		return b.userError(err)
	}
	if len(tasks) == 0 {
		return "Несданных задач нет 🎉"
	}
	now := b.clock()
	var sb strings.Builder
	sb.WriteString("Несданные задачи:\n")
	for _, t := range tasks {
		fmt.Fprintf(&sb, "\n#%d %s — %s\n   до %s", t.ID, t.Subject, t.Title, b.fmtTime(t.Deadline))
		if !t.Deadline.After(now) {
			sb.WriteString(" ⛔ просрочено")
		} else {
			fmt.Fprintf(&sb, " (через %s)", humanDuration(t.Deadline.Sub(now)))
		}
	}
	return sb.String()
}

func (b *Bot) cmdDone(ctx context.Context, args string) string {
	id, err := ParseTaskID(args)
	if err != nil {
		return b.userError(err)
	}
	if err := b.svc.Done(ctx, id); err != nil {
		return b.taskError(id, err)
	}
	return fmt.Sprintf("✅ #%d сдано. Больше не напомню.", id)
}

func (b *Bot) cmdSnooze(ctx context.Context, args string) string {
	id, d, err := ParseSnooze(args)
	if err != nil {
		return b.userError(err)
	}
	at, err := b.svc.Snooze(ctx, id, d)
	if err != nil {
		return b.taskError(id, err)
	}
	return fmt.Sprintf("⏰ Напомню про #%d %s", id, b.fmtTime(at))
}

// Данные кнопок: done:<id>, snooze:<id>:3h, snooze:<id>:tomorrow.
const (
	cbDone           = "done"
	cbSnooze         = "snooze"
	cbSnooze3h       = "3h"
	cbSnoozeTomorrow = "tomorrow"
)

func (b *Bot) handleCallback(ctx context.Context, cq *models.CallbackQuery) {
	short, status := b.applyCallback(ctx, cq.Data)

	if _, err := b.api.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: cq.ID,
		Text:            short,
	}); err != nil {
		b.log.Warn("answer callback", "err", err)
	}

	// Сообщение старше 48ч бывает недоступно — тогда просто не редактируем.
	msg := cq.Message.Message
	if status == "" || msg == nil {
		return
	}
	// Редактируем текст без ReplyMarkup — кнопки исчезают.
	if _, err := b.api.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    msg.Chat.ID,
		MessageID: msg.ID,
		Text:      msg.Text + "\n\n" + status,
	}); err != nil {
		b.log.Warn("edit reminder message", "err", err)
	}
}

// applyCallback выполняет действие кнопки. short — всплывающий ответ,
// status — строка, которую дописываем в сообщение (пусто — сообщение не трогаем).
func (b *Bot) applyCallback(ctx context.Context, data string) (short, status string) {
	parts := strings.Split(data, ":")
	if len(parts) < 2 {
		return "Неизвестная кнопка", ""
	}
	id, err := ParseTaskID(parts[1])
	if err != nil {
		return "Неизвестная кнопка", ""
	}

	switch {
	case parts[0] == cbDone && len(parts) == 2:
		if err := b.svc.Done(ctx, id); err != nil {
			return b.taskError(id, err), ""
		}
		return "Сдано ✅", "✅ Сдано"
	case parts[0] == cbSnooze && len(parts) == 3:
		var at time.Time
		switch parts[2] {
		case cbSnooze3h:
			at, err = b.svc.Snooze(ctx, id, 3*time.Hour)
		case cbSnoozeTomorrow:
			at, err = b.svc.SnoozeUntilTomorrow(ctx, id)
		default:
			return "Неизвестная кнопка", ""
		}
		if err != nil {
			return b.taskError(id, err), ""
		}
		return "Отложено", "⏰ Отложено до " + b.fmtTime(at)
	}
	return "Неизвестная кнопка", ""
}

func (b *Bot) reply(ctx context.Context, chatID int64, text string) {
	if _, err := b.api.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text}); err != nil {
		b.log.Error("send reply", "err", err)
	}
}

// userError превращает ошибку в текст для владельца. Ошибки ввода показываем как есть
// (тексты парсера на русском), остальное — логируем и отвечаем общей фразой.
func (b *Bot) userError(err error) string {
	var ie inputError
	if errors.As(err, &ie) {
		return "❌ " + ie.Error()
	}
	if errors.Is(err, domain.ErrInvalidInput) {
		return "❌ Проверь данные: " + addUsage
	}
	b.log.Error("handle command", "err", err)
	return "Что-то сломалось, попробуй позже."
}

func (b *Bot) taskError(id int64, err error) string {
	if errors.Is(err, domain.ErrNotFound) {
		return fmt.Sprintf("Задачи #%d нет или она уже сдана.", id)
	}
	return b.userError(err)
}

// fmtTime показывает время по Алматы: «23.10 (пт) 23:59».
func (b *Bot) fmtTime(t time.Time) string {
	return formatLocal(t, b.loc)
}

const helpText = `Команды:
/add Предмет | Название | ДД.ММ ЧЧ:ММ — добавить задачу
/list — несданные задачи
/done <id> — сдал
/snooze <id> <время> — отложить, например /snooze 7 3h`
