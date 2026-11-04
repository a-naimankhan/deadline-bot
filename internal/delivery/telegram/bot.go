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
	start := time.Now()
	cmd, args := splitCommand(m.Text)

	var (
		reply string
		id    int64
		err   error
	)
	switch cmd {
	case "/start", "/help":
		reply = helpText
	case "/add":
		reply, id, err = b.cmdAdd(ctx, args)
	case "/list":
		reply, err = b.cmdList(ctx)
	case "/done":
		reply, id, err = b.cmdDone(ctx, args)
	case "/snooze":
		reply, id, err = b.cmdSnooze(ctx, args)
	default:
		cmd = "unknown" // произвольный текст в лог не пишем
		reply = "Не понял. " + helpText
	}
	if err != nil {
		reply = b.errorText(err, id)
	}
	b.reply(ctx, m.Chat.ID, reply)
	b.logResult("command handled", "cmd", cmd, id, start, err)
}

// splitCommand: "/add@MyBot  БИС | ..." → ("/add", "БИС | ...").
func splitCommand(text string) (string, string) {
	text = strings.TrimSpace(text)
	cmd, args, _ := strings.Cut(text, " ")
	cmd, _, _ = strings.Cut(cmd, "@")
	return strings.ToLower(cmd), strings.TrimSpace(args)
}

func (b *Bot) cmdAdd(ctx context.Context, args string) (string, int64, error) {
	in, err := ParseAdd(args, b.clock(), b.loc)
	if err != nil {
		return "", 0, err
	}
	t, err := b.svc.AddTask(ctx, in)
	if err != nil {
		return "", 0, err
	}
	return fmt.Sprintf("Добавил #%d: %s — %s\nДедлайн: %s", t.ID, t.Subject, t.Title, b.fmtTime(t.Deadline)), t.ID, nil
}

func (b *Bot) cmdList(ctx context.Context) (string, error) {
	tasks, err := b.svc.ListActive(ctx)
	if err != nil {
		return "", err
	}
	if len(tasks) == 0 {
		return "Несданных задач нет 🎉", nil
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
	return sb.String(), nil
}

func (b *Bot) cmdDone(ctx context.Context, args string) (string, int64, error) {
	id, err := ParseTaskID(args)
	if err != nil {
		return "", 0, err
	}
	if err := b.svc.Done(ctx, id); err != nil {
		return "", id, err
	}
	return fmt.Sprintf("✅ #%d сдано. Больше не напомню.", id), id, nil
}

func (b *Bot) cmdSnooze(ctx context.Context, args string) (string, int64, error) {
	id, d, err := ParseSnooze(args)
	if err != nil {
		return "", 0, err
	}
	at, err := b.svc.Snooze(ctx, id, d)
	if err != nil {
		return "", id, err
	}
	return fmt.Sprintf("⏰ Напомню про #%d %s", id, b.fmtTime(at)), id, nil
}

// Данные кнопок: done:<id>, snooze:<id>:3h, snooze:<id>:tomorrow.
const (
	cbDone           = "done"
	cbSnooze         = "snooze"
	cbSnooze3h       = "3h"
	cbSnoozeTomorrow = "tomorrow"
)

// Действия кнопок, как они пишутся в лог.
const (
	actionDone           = "done"
	actionSnooze3h       = "snooze_3h"
	actionSnoozeTomorrow = "snooze_tomorrow"
)

// parseCallback разбирает данные кнопки. ok=false — кнопка не наша или битая.
func parseCallback(data string) (action string, id int64, ok bool) {
	parts := strings.Split(data, ":")
	if len(parts) < 2 {
		return "", 0, false
	}
	id, err := ParseTaskID(parts[1])
	if err != nil {
		return "", 0, false
	}
	switch {
	case parts[0] == cbDone && len(parts) == 2:
		return actionDone, id, true
	case parts[0] == cbSnooze && len(parts) == 3 && parts[2] == cbSnooze3h:
		return actionSnooze3h, id, true
	case parts[0] == cbSnooze && len(parts) == 3 && parts[2] == cbSnoozeTomorrow:
		return actionSnoozeTomorrow, id, true
	}
	return "", 0, false
}

func (b *Bot) handleCallback(ctx context.Context, cq *models.CallbackQuery) {
	start := time.Now()
	action, id, ok := parseCallback(cq.Data)
	if !ok {
		b.answer(ctx, cq.ID, "Неизвестная кнопка")
		b.log.Warn("unknown callback")
		return
	}

	short, status, err := b.runCallback(ctx, action, id)
	if err != nil {
		short = b.errorText(err, id)
	}
	b.answer(ctx, cq.ID, short)
	b.logResult("callback handled", "action", action, id, start, err)

	// Сообщение старше 48ч бывает недоступно — тогда просто не редактируем.
	msg := cq.Message.Message
	if err != nil || msg == nil {
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

// runCallback выполняет действие кнопки. short — всплывающий ответ,
// status — строка, которую дописываем в сообщение с напоминанием.
func (b *Bot) runCallback(ctx context.Context, action string, id int64) (short, status string, err error) {
	if action == actionDone {
		if err := b.svc.Done(ctx, id); err != nil {
			return "", "", err
		}
		return "Сдано ✅", "✅ Сдано", nil
	}

	var at time.Time
	if action == actionSnooze3h {
		at, err = b.svc.Snooze(ctx, id, 3*time.Hour)
	} else {
		at, err = b.svc.SnoozeUntilTomorrow(ctx, id)
	}
	if err != nil {
		return "", "", err
	}
	return "Отложено", "⏰ Отложено до " + b.fmtTime(at), nil
}

func (b *Bot) answer(ctx context.Context, callbackID, text string) {
	if _, err := b.api.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: callbackID,
		Text:            text,
	}); err != nil {
		b.log.Warn("answer callback", "err", err)
	}
}

func (b *Bot) reply(ctx context.Context, chatID int64, text string) {
	if _, err := b.api.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text}); err != nil {
		b.log.Error("send reply", "err", err)
	}
}

// logResult пишет одну строку на команду/кнопку: что, с какой задачей, сколько заняло, чем кончилось.
// Текст задач и аргументы не пишем — только id. Ошибки ввода и «не найдено» — штатный исход (info),
// всё остальное — error с подробностями.
func (b *Bot) logResult(msg, key, name string, id int64, start time.Time, err error) {
	attrs := []any{key, name, "duration", time.Since(start).String()}
	if id > 0 {
		attrs = append(attrs, "task_id", id)
	}
	switch {
	case err == nil:
		b.log.Info(msg, append(attrs, "result", "ok")...)
	case errors.Is(err, domain.ErrInvalidInput):
		b.log.Info(msg, append(attrs, "result", "invalid_input")...)
	case errors.Is(err, domain.ErrNotFound):
		b.log.Info(msg, append(attrs, "result", "not_found")...)
	default:
		b.log.Error(msg, append(attrs, "result", "error", "err", err)...)
	}
}

// errorText превращает ошибку в текст для владельца. Ошибки парсера показываем как есть
// (они на русском), внутренние наружу не выдаём.
func (b *Bot) errorText(err error, id int64) string {
	var ie inputError
	switch {
	case errors.As(err, &ie):
		return "❌ " + ie.Error()
	case errors.Is(err, domain.ErrInvalidInput):
		return "❌ Проверь данные. " + helpText
	case errors.Is(err, domain.ErrNotFound):
		return fmt.Sprintf("Задачи #%d нет или она уже сдана.", id)
	default:
		return "Что-то сломалось, попробуй позже."
	}
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
