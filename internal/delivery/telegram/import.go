package telegram

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"deadline-bot/internal/service"
)

// Импорт задания из текста, скопированного из MS Teams Assignments:
//
//	ФБИС                                           ← предмет (пишешь сам первой строкой)
//	Reading Exercise: Business Intelligence        ← название
//	Срок заканчивается 10 ноября 2026 г., 23:59    ← дедлайн
//	•
//	Допускается несколько отправок
//	Инструкции
//	Read case on pp.512-514 ...                    ← details
//
// Парсер строгий: всё, в чём он не уверен, — ошибка с подсказкой, а не догадка.

const importUsage = `пришли так:
ФБИС
<скопированный текст задания из Teams>

Первая строка — предмет, дальше название и строка «Срок заканчивается 10 ноября 2026 г., 23:59».`

const (
	maxSubjectLen = 50
	maxTitleLen   = 200
	maxDetailsLen = 1000
	maxAhead      = 366 * 24 * time.Hour
)

// «Срок заканчивается 10 ноября 2026 г., 23:59» / «Срок сдачи: 10 ноября, 23:59» / «Срок 10 ноября».
var dueRe = regexp.MustCompile(`(?i)^срок(?:\s+заканчивается|\s+сдачи)?:?\s+(\d{1,2})\s+([а-яё]+)(?:\s+(\d{4}))?(?:\s*г\.?)?,?(?:\s+(\d{1,2}):(\d{2}))?\s*$`)

var monthsRu = map[string]time.Month{
	"января": time.January, "февраля": time.February, "марта": time.March,
	"апреля": time.April, "мая": time.May, "июня": time.June,
	"июля": time.July, "августа": time.August, "сентября": time.September,
	"октября": time.October, "ноября": time.November, "декабря": time.December,
}

// Служебные строки Teams, которые не идут ни в название, ни в details.
var teamsNoise = map[string]bool{
	"•": true,
	"·": true,
	"допускается несколько отправок": true,
	"инструкции": true,
}

// ParseImport разбирает текст задания из Teams. Предмет — строка перед названием.
func ParseImport(text string, now time.Time, loc *time.Location) (service.AddTaskInput, error) {
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			lines = append(lines, l)
		}
	}

	dueIdx := -1
	var m []string
	for i, l := range lines {
		if m = dueRe.FindStringSubmatch(l); m != nil {
			dueIdx = i
			break
		}
	}
	if dueIdx < 0 {
		return service.AddTaskInput{}, invalid("не нашёл строку со сроком; " + importUsage)
	}
	if dueIdx < 2 {
		return service.AddTaskInput{}, invalid("не вижу предмета или названия; " + importUsage)
	}

	title := lines[dueIdx-1]
	subject := normalizeSubject(strings.Join(lines[:dueIdx-1], " "))
	if utf8.RuneCountInString(subject) > maxSubjectLen {
		return service.AddTaskInput{}, invalid(fmt.Sprintf("предмет длиннее %d символов — первой строкой должен быть только предмет", maxSubjectLen))
	}
	if utf8.RuneCountInString(title) > maxTitleLen {
		return service.AddTaskInput{}, invalid(fmt.Sprintf("название длиннее %d символов", maxTitleLen))
	}

	deadline, err := parseRuDate(m, now, loc)
	if err != nil {
		return service.AddTaskInput{}, err
	}

	var details []string
	for _, l := range lines[dueIdx+1:] {
		if !teamsNoise[strings.ToLower(l)] {
			details = append(details, l)
		}
	}

	return service.AddTaskInput{
		Subject:  subject,
		Title:    title,
		Details:  truncateRunes(strings.Join(details, "\n"), maxDetailsLen),
		Deadline: deadline.UTC(),
	}, nil
}

// parseRuDate собирает дату из групп dueRe: день, месяц, [год], [час, минута].
func parseRuDate(m []string, now time.Time, loc *time.Location) (time.Time, error) {
	day, _ := strconv.Atoi(m[1])
	month, ok := monthsRu[strings.ToLower(m[2])]
	if !ok {
		return time.Time{}, invalid(fmt.Sprintf("не понял месяц %q", m[2]))
	}
	year := now.In(loc).Year()
	if m[3] != "" {
		year, _ = strconv.Atoi(m[3])
	}
	hour, minute := 23, 59
	if m[4] != "" {
		hour, _ = strconv.Atoi(m[4])
		minute, _ = strconv.Atoi(m[5])
		if hour > 23 || minute > 59 {
			return time.Time{}, invalid(fmt.Sprintf("не понял время %s:%s", m[4], m[5]))
		}
	}

	t := time.Date(year, month, day, hour, minute, 0, 0, loc)
	if t.Day() != day {
		return time.Time{}, invalid(fmt.Sprintf("такой даты нет: %d %s", day, m[2]))
	}
	if !t.After(now) {
		return time.Time{}, invalid(fmt.Sprintf("дата %s уже прошла", t.Format("02.01.2006 15:04")))
	}
	if t.Sub(now) > maxAhead {
		return time.Time{}, invalid(fmt.Sprintf("дата %s дальше чем через год — проверь год", t.Format("02.01.2006")))
	}
	return t, nil
}

// normalizeSubject: «фбис» → «ФБИС», чтобы один предмет не жил под двумя именами.
// Аббревиатуры (одно слово до 8 букв) — капсом, остальное как есть.
func normalizeSubject(s string) string {
	if !strings.Contains(s, " ") && utf8.RuneCountInString(s) <= 8 {
		return strings.ToUpper(s)
	}
	return s
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}
