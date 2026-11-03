package service

import (
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

func TestPlanReminders(t *testing.T) {
	loc := almaty(t)
	local := func(month time.Month, day, hour, min int) time.Time {
		return time.Date(2026, month, day, hour, min, 0, 0, loc)
	}
	oct := func(day, hour, min int) time.Time { return local(time.October, day, hour, min) }

	type want struct {
		kind domain.ReminderKind
		at   time.Time
	}
	// nags: nag-и в указанные локальные часы; час 24 — полночь следующего дня.
	nags := func(month time.Month, day int, hours ...int) []want {
		var out []want
		for _, h := range hours {
			out = append(out, want{domain.KindNag, local(month, day, h, 0)})
		}
		return out
	}
	join := func(parts ...[]want) []want {
		var out []want
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}

	tests := []struct {
		name     string
		deadline time.Time
		now      time.Time
		want     []want
	}{
		{
			name:     "deadline in 5 days: d2, d1, dday and 6 nags",
			deadline: oct(25, 23, 59),
			now:      oct(20, 10, 0),
			want: join(
				[]want{
					{domain.KindD2, oct(23, 12, 0)},
					{domain.KindD1, oct(24, 12, 0)},
					{domain.KindDDay, oct(25, 12, 0)},
				},
				nags(time.October, 25, 14, 16, 18, 20, 22, 24),
			),
		},
		{
			name:     "deadline in 1.5 days drops d2",
			deadline: oct(22, 12, 0),
			now:      oct(20, 22, 0),
			want: []want{
				{domain.KindD1, oct(21, 12, 0)},
				{domain.KindDDay, oct(22, 11, 0)},
				{domain.KindNag, oct(22, 13, 0)},
			},
		},
		{
			name:     "deadline tonight",
			deadline: oct(20, 23, 59),
			now:      oct(20, 10, 0),
			want: join(
				[]want{{domain.KindDDay, oct(20, 12, 0)}},
				nags(time.October, 20, 14, 16, 18, 20, 22, 24),
			),
		},
		{
			name:     "deadline tonight, added after noon: only remaining nags",
			deadline: oct(20, 23, 59),
			now:      oct(20, 15, 0),
			want:     nags(time.October, 20, 16, 18, 20, 22, 24),
		},
		{
			name:     "deadline 18:00: nags stop at first one after deadline",
			deadline: oct(20, 18, 0),
			now:      oct(20, 10, 0),
			want: join(
				[]want{{domain.KindDDay, oct(20, 12, 0)}},
				nags(time.October, 20, 14, 16, 18),
			),
		},
		{
			name:     "deadline today 09:00 moves dday to 08:00",
			deadline: oct(22, 9, 0),
			now:      oct(20, 10, 0),
			want: []want{
				{domain.KindD2, oct(20, 12, 0)},
				{domain.KindD1, oct(21, 12, 0)},
				{domain.KindDDay, oct(22, 8, 0)},
				{domain.KindNag, oct(22, 10, 0)},
			},
		},
		{
			name:     "month boundary",
			deadline: local(time.November, 1, 23, 59),
			now:      oct(25, 10, 0),
			want: join(
				[]want{
					{domain.KindD2, oct(30, 12, 0)},
					{domain.KindD1, oct(31, 12, 0)},
					{domain.KindDDay, local(time.November, 1, 12, 0)},
				},
				nags(time.November, 1, 14, 16, 18, 20, 22, 24),
			),
		},
		{
			name:     "deadline already passed",
			deadline: oct(20, 9, 0),
			now:      oct(20, 10, 0),
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PlanReminders(tt.deadline, tt.now, loc, 12)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d reminders %+v, want %d", len(got), got, len(tt.want))
			}
			for i, w := range tt.want {
				if got[i].Kind != w.kind {
					t.Errorf("[%d] kind = %s, want %s", i, got[i].Kind, w.kind)
				}
				if !got[i].RemindAt.Equal(w.at) {
					t.Errorf("[%d] at = %s, want %s", i, got[i].RemindAt.In(loc), w.at)
				}
				if got[i].RemindAt.Location() != time.UTC {
					t.Errorf("[%d] location = %s, want UTC", i, got[i].RemindAt.Location())
				}
			}
		})
	}
}
