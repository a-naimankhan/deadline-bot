package domain

import "time"

type Task struct {
	ID        int64
	Subject   string
	Title     string
	Details   string
	FileID    string // file_id вложения в Telegram, пусто — без файла
	Deadline  time.Time
	DoneAt    *time.Time
	CreatedAt time.Time
}

func (t Task) IsDone() bool {
	return t.DoneAt != nil
}
