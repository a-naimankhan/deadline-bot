package service

import (
	"slices"
	"time"

	"deadline-bot/internal/domain"
)

// «Душение» в день дедлайна: nag каждые nagEvery после dday, не больше maxNags.
const (
	nagEvery = 2 * time.Hour
	maxNags  = 6
)

// PlanReminders строит напоминания d2, d1, dday в hour:00 по loc относительно даты дедлайна.
// Если dday не раньше дедлайна (например, дедлайн в 09:00), он сдвигается на deadline − 1ч.
// После dday идут nag каждые nagEvery (не больше maxNags): все до дедлайна и один первый после него.
// Сданные задачи ClaimDue не забирает, поэтому nag после «Сдал» просто не придут.
// Напоминания с RemindAt <= now отбрасываются. Результат отсортирован по времени, в UTC.
func PlanReminders(deadline, now time.Time, loc *time.Location, hour int) []domain.Reminder {
	local := deadline.In(loc)
	y, m, d := local.Date()
	at := func(daysBefore int) time.Time {
		return time.Date(y, m, d-daysBefore, hour, 0, 0, 0, loc)
	}

	dday := at(0)
	if !dday.Before(deadline) {
		dday = deadline.Add(-time.Hour)
	}

	candidates := []domain.Reminder{
		{Kind: domain.KindD2, RemindAt: at(2)},
		{Kind: domain.KindD1, RemindAt: at(1)},
		{Kind: domain.KindDDay, RemindAt: dday},
	}
	for k := 1; k <= maxNags; k++ {
		nag := dday.Add(time.Duration(k) * nagEvery)
		candidates = append(candidates, domain.Reminder{Kind: domain.KindNag, RemindAt: nag})
		if !nag.Before(deadline) {
			break
		}
	}

	var out []domain.Reminder
	for _, r := range candidates {
		if !r.RemindAt.After(now) {
			continue
		}
		r.RemindAt = r.RemindAt.UTC()
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b domain.Reminder) int { return a.RemindAt.Compare(b.RemindAt) })
	return out
}
