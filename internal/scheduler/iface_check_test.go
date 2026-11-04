package scheduler_test

import (
	"deadline-bot/internal/delivery/telegram"
	"deadline-bot/internal/repository/postgres"
	"deadline-bot/internal/scheduler"
)

var (
	_ scheduler.Store    = (*postgres.Repo)(nil)
	_ scheduler.Notifier = (*telegram.Notifier)(nil)
)
