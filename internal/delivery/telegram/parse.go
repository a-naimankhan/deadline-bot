package telegram

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"deadline-bot/internal/domain"
	"deadline-bot/internal/service"
)

// Тексты ошибок парсера показываются владельцу как есть, поэтому они на русском.
// Все ошибки оборачивают domain.ErrInvalidInput.

const addUsage = "формат: /add Предмет | Название | ДД.ММ ЧЧ:ММ"

// Форматы даты в /add. Без года — текущий год, без времени — 23:59.
var (
	layoutsWithYear = []string{"2.1.2006 15:04", "2.1.2006"}
	layoutsNoYear   = []string{"2.1 15:04", "2.1"}
)

// ParseAdd разбирает аргументы /add: «Предмет | Название | дата».
func ParseAdd(args string, now time.Time, loc *time.Location) (service.AddTaskInput, error) {
	parts := strings.Split(args, "|")
	if len(parts) != 3 {
		return service.AddTaskInput{}, invalid(addUsage)
	}
	subject := strings.TrimSpace(parts[0])
	title := strings.TrimSpace(parts[1])
	if subject == "" || title == "" {
		return service.AddTaskInput{}, invalid("предмет и название не могут быть пустыми; " + addUsage)
	}

	deadline, err := parseDeadline(parts[2], now, loc)
	if err != nil {
		return service.AddTaskInput{}, err
	}
	return service.AddTaskInput{Subject: subject, Title: title, Deadline: deadline.UTC()}, nil
}

func parseDeadline(raw string, now time.Time, loc *time.Location) (time.Time, error) {
	s := strings.Join(strings.Fields(raw), " ")
	hasTime := strings.Contains(s, ":")

	t, ok := tryLayouts(s, layoutsWithYear, loc)
	if !ok {
		t, ok = tryLayouts(s, layoutsNoYear, loc)
		if !ok {
			return time.Time{}, invalid(fmt.Sprintf("не понял дату %q; %s", s, addUsage))
		}
		// Год подставляем после разбора: 29.02 без года проверяется уже в нужном году.
		withYear := time.Date(now.In(loc).Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, loc)
		if withYear.Day() != t.Day() {
			return time.Time{}, invalid(fmt.Sprintf("такой даты нет: %q", s))
		}
		t = withYear
	}
	if !hasTime {
		t = time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 0, 0, loc)
	}

	if !t.After(now) {
		return time.Time{}, invalid(fmt.Sprintf("дата %s уже прошла", t.Format("02.01.2006 15:04")))
	}
	return t, nil
}

func tryLayouts(s string, layouts []string, loc *time.Location) (time.Time, bool) {
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l, s, loc); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

const snoozeUsage = "формат: /snooze <id> <время>, например /snooze 7 3h или 1h30m"

// ParseSnooze разбирает аргументы /snooze: «<id> <длительность>».
// Длительность — как в time.ParseDuration; русские «ч» и «м» тоже понимаются (3ч, 1ч30м).
func ParseSnooze(args string) (int64, time.Duration, error) {
	fields := strings.Fields(args)
	if len(fields) != 2 {
		return 0, 0, invalid(snoozeUsage)
	}
	id, err := ParseTaskID(fields[0])
	if err != nil {
		return 0, 0, err
	}

	raw := strings.NewReplacer("ч", "h", "м", "m").Replace(strings.ToLower(fields[1]))
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, 0, invalid(fmt.Sprintf("не понял время %q; %s", fields[1], snoozeUsage))
	}
	if d <= 0 {
		return 0, 0, invalid("время должно быть больше нуля")
	}
	return id, d, nil
}

// ParseTaskID разбирает id задачи (для /done и /snooze).
func ParseTaskID(s string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || id <= 0 {
		return 0, invalid(fmt.Sprintf("id задачи должен быть положительным числом, а не %q", s))
	}
	return id, nil
}

// inputError — текст для пользователя без хвоста «: invalid input», но errors.Is видит ErrInvalidInput.
type inputError struct{ msg string }

func (e inputError) Error() string { return e.msg }
func (e inputError) Unwrap() error { return domain.ErrInvalidInput }

func invalid(msg string) error {
	return inputError{msg: msg}
}
