package domain

import "time"

type ReminderKind string

const (
	KindD2     ReminderKind = "d2"
	KindD1     ReminderKind = "d1"
	KindDDay   ReminderKind = "dday"
	KindNag    ReminderKind = "nag"
	KindSnooze ReminderKind = "snooze"
)

type Reminder struct {
	ID       int64
	TaskID   int64
	Kind     ReminderKind
	RemindAt time.Time // UTC
}

// DueReminder возвращается планировщику вместе с задачей, чтобы собрать текст
type DueReminder struct {
	Reminder
	Task Task
}
