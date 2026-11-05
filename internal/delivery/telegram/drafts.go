package telegram

import (
	"sync"
	"time"

	"deadline-bot/internal/service"
)

// draftTTL — сколько черновик ждёт нажатия «Добавить».
const draftTTL = 24 * time.Hour

// draftStore хранит распознанные, но ещё не подтверждённые задания.
// В памяти: владелец один, после рестарта достаточно переслать текст ещё раз.
// Нулевое значение готово к работе.
type draftStore struct {
	mu     sync.Mutex
	nextID int64
	items  map[int64]draft
}

type draft struct {
	in      service.AddTaskInput
	created time.Time
}

func (s *draftStore) put(in service.AddTaskInput, now time.Time) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items == nil {
		s.items = make(map[int64]draft)
	}
	for id, d := range s.items {
		if now.Sub(d.created) > draftTTL {
			delete(s.items, id)
		}
	}
	s.nextID++
	s.items[s.nextID] = draft{in: in, created: now}
	return s.nextID
}

// attachLatest прикрепляет файл к самому свежему живому черновику.
func (s *draftStore) attachLatest(fileID string, now time.Time) (service.AddTaskInput, int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest int64
	for id, d := range s.items {
		if id > latest && now.Sub(d.created) <= draftTTL {
			latest = id
		}
	}
	if latest == 0 {
		return service.AddTaskInput{}, 0, false
	}
	d := s.items[latest]
	d.in.FileID = fileID
	s.items[latest] = d
	return d.in, latest, true
}

// take достаёт черновик и удаляет его: повторное нажатие кнопки задачу не задвоит.
func (s *draftStore) take(id int64, now time.Time) (service.AddTaskInput, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.items[id]
	if !ok {
		return service.AddTaskInput{}, false
	}
	delete(s.items, id)
	if now.Sub(d.created) > draftTTL {
		return service.AddTaskInput{}, false
	}
	return d.in, true
}
