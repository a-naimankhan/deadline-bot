package domain

import "time"

type Task struct {
	ID        int64
	Subject   string
	Title     string
	Details   string
	Deadline  time.Time
	DoneAt    *time.Time
	CreatedAt time.Time
}

func (t Task) IsDone() bool {
	return t.DoneAt != nil
}
