package domain

import (
	"time"
	"uuid"
)

type Task struct {
	ID        uuid.UUID
	Subject   string
	Title     string
	Details   string
	Deadline  time.Time
	DoneAt    *time.Time
	CreatedAt time.Time
}

func (t *Task) IsDone() bool
