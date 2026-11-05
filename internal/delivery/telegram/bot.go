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
	GetTask(ctx context.Context, id int64) (domain.Task, error)
	Done(ctx context.Context, taskID int64) error
	Snooze(ctx context.Context, taskID int64, d time.Duration) (time.Time, error)
	SnoozeUntilTomorrow(ctx context.Context, taskID int64) (time.Time, error)
}

// sender — минимум Bot API, который нужен слою. *bot.Bot его реализует, в тестах — фейк.
type sender interface {
	SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error)
	SendDocument(ctx context.Context, params *bot.SendDocumentParams) (*models.Message, error)
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
	drafts  draftStore
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
	text := m.Text
	if text == "" {
		text = m.Caption // пересланный файл с подписью
	}
	cmd, args := splitCommand(text)
	var fileID string
	if m.Document != nil {
		fileID = m.Document.FileID
	}

	var (
		reply  string
		markup models.ReplyMarkup
		id     int64
		err    error
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
		switch {
		case text != "" && !strings.HasPrefix(cmd, "/"):
			cmd = "import" // сам текст в лог не пишем
			reply, markup, id, err = b.cmdImport(ctx, text, fileID)
		case text == "" && fileID != "":
			cmd = "attach"
			reply, markup, id, err = b.cmdAttach(fileID)
		default:
			cmd = "unknown"
			reply = "Не понял. " + helpText
		}
	}
	if err != nil {
		reply, markup = b.errorText(err, id), nil
	}
	b.send(ctx, m.Chat.ID, reply, markup)
	b.logResult("command handled", "cmd", cmd, idKey(cmd == "import" || cmd == "attach"), id, start, err)
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

// cmdImport распознаёт задание из текста Teams и показывает черновик с кнопками.
// В БД ничего не пишется до нажатия «Добавить». id — номер черновика.
func (b *Bot) cmdImport(ctx context.Context, text, fileID string) (string, models.ReplyMarkup, int64, error) {
	in, err := ParseImport(text, b.clock(), b.loc)
	if err != nil {
		return "", nil, 0, err
	}
	in.FileID = fileID
	active, err := b.svc.ListActive(ctx)
	if err != nil {
		return "", nil, 0, err
	}
	id := b.drafts.put(in, b.clock())
	return b.draftText(in, active), draftKeyboard(id), id, nil
}

// cmdAttach прикрепляет файл без подписи к последнему неподтверждённому черновику
// и показывает черновик заново (старые кнопки при этом продолжают работать).
func (b *Bot) cmdAttach(fileID string) (string, models.ReplyMarkup, int64, error) {
	in, id, ok := b.drafts.attachLatest(fileID, b.clock())
	if !ok {
		return "", nil, 0, invalid("не к чему прикрепить файл. Пришли PDF с текстом задания в подписи " +
			"или сначала текст задания, а потом PDF")
	}
	return "📎 Прикрепил к черновику.\n\n" + b.draftText(in, nil), draftKeyboard(id), id, nil
}

func (b *Bot) draftText(in service.AddTaskInput, active []domain.Task) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Нашёл задание:\n%s — %s\nДедлайн: %s", in.Subject, in.Title, b.fmtTime(in.Deadline))
	if in.Details != "" {
		fmt.Fprintf(&sb, "\n\n%s", truncateRunes(in.Details, 300))
	}
	if in.FileID != "" {
		sb.WriteString("\n📎 Файл прикреплён")
	}
	if dup, ok := findDuplicate(active, in); ok {
		fmt.Fprintf(&sb, "\n\n⚠️ Похоже, это уже есть: #%d %s — %s", dup.ID, dup.Subject, dup.Title)
	}
	return sb.String()
}

func draftKeyboard(id int64) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
		{Text: "Добавить ✅", CallbackData: fmt.Sprintf("%s:%d", cbImport, id)},
		{Text: "Отмена", CallbackData: fmt.Sprintf("%s:%d", cbImportCancel, id)},
	}}}
}

// findDuplicate: несданная задача с тем же предметом и названием (без учёта регистра)
// или с тем же названием и тем же дедлайном.
func findDuplicate(active []domain.Task, in service.AddTaskInput) (domain.Task, bool) {
	for _, t := range active {
		sameTitle := strings.EqualFold(t.Title, in.Title)
		if sameTitle && (strings.EqualFold(t.Subject, in.Subject) || t.Deadline.Equal(in.Deadline)) {
			return t, true
		}
	}
	return domain.Task{}, false
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
		fmt.Fprintf(&sb, "\n#%d %s — %s", t.ID, t.Subject, t.Title)
		if t.FileID != "" {
			sb.WriteString(" 📎")
		}
		fmt.Fprintf(&sb, "\n   до %s", b.fmtTime(t.Deadline))
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

// Данные кнопок: done:<id>, snooze:<id>:3h, snooze:<id>:tomorrow;
// для черновиков импорта imp:<draft>, impx:<draft>; файл задания file:<id>.
const (
	cbDone           = "done"
	cbSnooze         = "snooze"
	cbSnooze3h       = "3h"
	cbSnoozeTomorrow = "tomorrow"
	cbImport         = "imp"
	cbImportCancel   = "impx"
	cbFile           = "file"
)

// Действия кнопок, как они пишутся в лог.
const (
	actionDone           = "done"
	actionSnooze3h       = "snooze_3h"
	actionSnoozeTomorrow = "snooze_tomorrow"
	actionImport         = "import"
	actionImportCancel   = "import_cancel"
	actionFile           = "file"
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
	case parts[0] == cbImport && len(parts) == 2:
		return actionImport, id, true
	case parts[0] == cbImportCancel && len(parts) == 2:
		return actionImportCancel, id, true
	case parts[0] == cbFile && len(parts) == 2:
		return actionFile, id, true
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
	isDraft := action == actionImport || action == actionImportCancel
	b.logResult("callback handled", "action", action, idKey(isDraft), id, start, err)

	// Сообщение старше 48ч бывает недоступно — тогда просто не редактируем.
	// Пустой status — действие не меняет сообщение (кнопки остаются).
	msg := cq.Message.Message
	if err != nil || msg == nil || status == "" {
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
	switch action {
	case actionImport:
		in, ok := b.drafts.take(id, b.clock())
		if !ok {
			return "", "", errDraftGone
		}
		t, err := b.svc.AddTask(ctx, in)
		if err != nil {
			return "", "", err
		}
		return "Добавлено ✅", fmt.Sprintf("✅ Добавил #%d", t.ID), nil
	case actionImportCancel:
		b.drafts.take(id, b.clock())
		return "Отменено", "✖️ Отменено", nil
	case actionFile:
		return b.sendTaskFile(ctx, id)
	}

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

// sendTaskFile присылает файл задания отдельным сообщением.
func (b *Bot) sendTaskFile(ctx context.Context, id int64) (short, status string, err error) {
	t, err := b.svc.GetTask(ctx, id)
	if err != nil {
		return "", "", err
	}
	if t.FileID == "" {
		return "", "", invalid(fmt.Sprintf("к задаче #%d файл не прикреплён", id))
	}
	if _, err := b.api.SendDocument(ctx, &bot.SendDocumentParams{
		ChatID:   b.ownerID,
		Document: &models.InputFileString{Data: t.FileID},
		Caption:  fmt.Sprintf("%s — %s (#%d)", t.Subject, t.Title, t.ID),
	}); err != nil {
		return "", "", fmt.Errorf("send task file: %w", err)
	}
	return "📎", "", nil
}

// errDraftGone — черновик уже добавлен, отменён или устарел (или бот перезапускался).
var errDraftGone = invalid("черновик устарел — перешли задание ещё раз")

func (b *Bot) reply(ctx context.Context, chatID int64, text string) {
	b.send(ctx, chatID, text, nil)
}

func (b *Bot) send(ctx context.Context, chatID int64, text string, markup models.ReplyMarkup) {
	if _, err := b.api.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text, ReplyMarkup: markup}); err != nil {
		b.log.Error("send reply", "err", err)
	}
}

// logResult пишет одну строку на команду/кнопку: что, с какой задачей, сколько заняло, чем кончилось.
// Текст задач и аргументы не пишем — только id. Ошибки ввода и «не найдено» — штатный исход (info),
// всё остальное — error с подробностями.
func (b *Bot) logResult(msg, key, name, idName string, id int64, start time.Time, err error) {
	attrs := []any{key, name, "duration", time.Since(start).String()}
	if id > 0 {
		attrs = append(attrs, idName, id)
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

// idKey — под каким ключом писать id в лог: у импорта это номер черновика, а не задачи.
func idKey(draft bool) string {
	if draft {
		return "draft_id"
	}
	return "task_id"
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
/snooze <id> <время> — отложить, например /snooze 7 3h

Или просто пришли задание из Teams: первой строкой предмет, дальше скопированный текст.
PDF задания можно прислать с этим текстом в подписи или следом отдельным сообщением.`
