package telegram

import (
	"errors"
	"strings"
	"testing"
	"time"

	"deadline-bot/internal/domain"
)

func almaty(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Almaty")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	return loc
}

func TestParseAdd(t *testing.T) {
	loc := almaty(t)
	now := time.Date(2026, time.October, 20, 10, 0, 0, 0, loc)

	ok := []struct {
		name    string
		args    string
		subject string
		title   string
		want    time.Time
	}{
		{"normal", "БИС | Lab 5 | 23.10 23:59", "БИС", "Lab 5", time.Date(2026, 10, 23, 23, 59, 0, 0, loc)},
		{"extra spaces", "  ФБИС|  Essay   |  23.10    18:00 ", "ФБИС", "Essay", time.Date(2026, 10, 23, 18, 0, 0, 0, loc)},
		{"with year", "БИС | Lab | 05.01.2027 09:30", "БИС", "Lab", time.Date(2027, 1, 5, 9, 30, 0, 0, loc)},
		{"date only means 23:59", "БИС | Lab | 25.10", "БИС", "Lab", time.Date(2026, 10, 25, 23, 59, 0, 0, loc)},
		{"single digits", "БИС | Lab | 3.11 9:05", "БИС", "Lab", time.Date(2026, 11, 3, 9, 5, 0, 0, loc)},
		{"today later", "БИС | Lab | 20.10", "БИС", "Lab", time.Date(2026, 10, 20, 23, 59, 0, 0, loc)},
	}
	for _, tt := range ok {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAdd(tt.args, now, loc)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Subject != tt.subject || got.Title != tt.title {
				t.Errorf("got %q / %q, want %q / %q", got.Subject, got.Title, tt.subject, tt.title)
			}
			if !got.Deadline.Equal(tt.want) {
				t.Errorf("deadline = %s, want %s", got.Deadline.In(loc), tt.want)
			}
			if got.Deadline.Location() != time.UTC {
				t.Errorf("location = %s, want UTC", got.Deadline.Location())
			}
		})
	}

	bad := []struct {
		name    string
		args    string
		errPart string
	}{
		{"empty", "", "формат"},
		{"missing separator", "БИС Lab 5 | 23.10 23:59", "формат"},
		{"too many parts", "БИС | Lab | 23.10 | extra", "формат"},
		{"empty title", "БИС |  | 23.10", "пустыми"},
		{"bad date", "БИС | Lab | завтра", "не понял дату"},
		{"bad time", "БИС | Lab | 23.10 25:00", "не понял дату"},
		{"nonexistent day", "БИС | Lab | 31.02.2027", "не понял дату"},
		{"feb 29 in non-leap year", "БИС | Lab | 29.02", "такой даты нет"},
		{"past date", "БИС | Lab | 19.10 23:59", "уже прошла"},
		{"earlier today", "БИС | Lab | 20.10 09:00", "уже прошла"},
	}
	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseAdd(tt.args, now, loc)
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
			if !strings.Contains(err.Error(), tt.errPart) {
				t.Errorf("err = %q, want it to contain %q", err, tt.errPart)
			}
			if strings.Contains(err.Error(), "invalid input") {
				t.Errorf("user-facing error leaks sentinel text: %q", err)
			}
		})
	}
}

func TestParseSnooze(t *testing.T) {
	ok := []struct {
		args string
		id   int64
		d    time.Duration
	}{
		{"7 3h", 7, 3 * time.Hour},
		{"  12   1h30m ", 12, 90 * time.Minute},
		{"7 45m", 7, 45 * time.Minute},
		{"7 3ч", 7, 3 * time.Hour},
		{"7 1ч30м", 7, 90 * time.Minute},
		{"7 2H", 7, 2 * time.Hour},
	}
	for _, tt := range ok {
		t.Run(tt.args, func(t *testing.T) {
			id, d, err := ParseSnooze(tt.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id != tt.id || d != tt.d {
				t.Errorf("got (%d, %s), want (%d, %s)", id, d, tt.id, tt.d)
			}
		})
	}

	for _, args := range []string{"", "7", "7 3h extra", "abc 3h", "0 3h", "-1 3h", "7 soon", "7 0h", "7 -3h"} {
		t.Run("bad "+args, func(t *testing.T) {
			_, _, err := ParseSnooze(args)
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestParseTaskID(t *testing.T) {
	if id, err := ParseTaskID(" 42 "); err != nil || id != 42 {
		t.Errorf("ParseTaskID(42) = %d, %v", id, err)
	}
	for _, s := range []string{"", "x", "0", "-5", "1.5"} {
		if _, err := ParseTaskID(s); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("ParseTaskID(%q) err = %v, want ErrInvalidInput", s, err)
		}
	}
}
