// Package wq — очередь длительных операций приложения (применение пресетов
// звука и другие прогоны на минуты): идёт по одной, порядок ждущих можно
// менять, очередь можно поставить на паузу; состояние видит интерфейс.
package wq

import (
	"context"
	"fmt"
	"sync"
)

// Item — запись очереди. State: waiting | running | done | error | canceled.
type Item struct {
	ID          int64  `json:"id"`
	Kind        string `json:"kind"` // 'preset' — применение пресета звука к треку
	Title       string `json:"title"`
	JobID       int64  `json:"job_id"` // трек, к которому относится операция
	State       string `json:"state"`
	Err         string `json:"err,omitempty"`
	ResultJobID int64  `json:"result_job_id,omitempty"` // итог: для пресета — id версии-трека
}

// Snapshot — состояние очереди для интерфейса.
type Snapshot struct {
	Paused bool   `json:"paused"`
	Items  []Item `json:"items"` // в порядке исполнения: идущая, ждущие, последние завершённые
}

const keepDone = 8 // сколько последних завершённых держать для отчёта

type entry struct {
	Item
	run func(context.Context) (int64, error)
}

type Queue struct {
	mu     sync.Mutex
	seq    int64
	wait   []*entry // порядок исполнения (первая — следующая)
	cur    *entry   // идущая; очередь однопоточная — параллельные прогоны перетирали бы файлы
	fin    []Item   // последние завершённые (done/error/canceled)
	paused bool
	wake   chan struct{}
}

func New() *Queue {
	return &Queue{wake: make(chan struct{}, 1)}
}

func (q *Queue) poke() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// Enqueue — поставить операцию в конец очереди; fn вызывается контекстом Run
// (отменяется завершением приложения) и возвращает id результата (для пресета —
// id новой версии-трека; он попадает в снимок для интерфейса).
func (q *Queue) Enqueue(kind, title string, jobID int64, fn func(context.Context) (int64, error)) Item {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.seq++
	e := &entry{Item: Item{ID: q.seq, Kind: kind, Title: title, JobID: jobID, State: "waiting"}, run: fn}
	q.wait = append(q.wait, e)
	item := e.Item
	q.poke()
	return item
}

// Run — цикл исполнителя; крутится горутиной с контекстом приложения.
func (q *Queue) Run(ctx context.Context) {
	for ctx.Err() == nil {
		e := q.take()
		if e == nil {
			select {
			case <-ctx.Done():
				return
			case <-q.wake:
			}
			continue
		}
		res, err := safeRun(ctx, e.run)
		if err != nil {
			e.Err = err.Error()
			e.State = "error"
		} else {
			e.ResultJobID = res
			e.State = "done"
		}
		q.finish(e)
	}
}

func safeRun(ctx context.Context, fn func(context.Context) (int64, error)) (res int64, err error) {
	defer func() {
		if r := recover(); r != nil {
			res, err = 0, fmt.Errorf("panic: %v", r)
		}
	}()
	return fn(ctx)
}

func (q *Queue) take() *entry {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.paused || len(q.wait) == 0 || q.cur != nil {
		return nil
	}
	e := q.wait[0]
	q.wait = q.wait[1:]
	e.State = "running"
	q.cur = e
	return e
}

func (q *Queue) finish(e *entry) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.cur == e {
		q.cur = nil
	}
	q.fin = append(q.fin, e.Item)
	if len(q.fin) > keepDone {
		q.fin = q.fin[len(q.fin)-keepDone:]
	}
	q.poke()
}

// Snapshot — текущее состояние: идущая, ждущие, завершённые.
func (q *Queue) Snapshot() Snapshot {
	q.mu.Lock()
	defer q.mu.Unlock()
	s := Snapshot{Paused: q.paused, Items: make([]Item, 0, 1+len(q.wait)+len(q.fin))}
	if q.cur != nil {
		s.Items = append(s.Items, q.cur.Item)
	}
	for _, e := range q.wait {
		s.Items = append(s.Items, e.Item)
	}
	s.Items = append(s.Items, q.fin...)
	return s
}

// Move — ждущую запись подвинуть среди ждущих: dir < 0 — ближе к исполнению.
func (q *Queue) Move(id int64, dir int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	i := -1
	for k, e := range q.wait {
		if e.ID == id {
			i = k
			break
		}
	}
	if i < 0 {
		return
	}
	j := i + dir
	if j < 0 || j >= len(q.wait) {
		return
	}
	q.wait[i], q.wait[j] = q.wait[j], q.wait[i]
}

// Cancel — ждущую запись убрать (в отчёт попадает как отменённая); идущую
// отменить нельзя — прогон воркера посреди не прервать.
func (q *Queue) Cancel(id int64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for k, e := range q.wait {
		if e.ID == id {
			q.wait = append(q.wait[:k], q.wait[k+1:]...)
			e.State = "canceled"
			q.fin = append(q.fin, e.Item)
			if len(q.fin) > keepDone {
				q.fin = q.fin[len(q.fin)-keepDone:]
			}
			return
		}
	}
}

// SetPaused — пауза: новая запись не начинается, идущая доигрывает до конца.
func (q *Queue) SetPaused(v bool) {
	q.mu.Lock()
	q.paused = v
	q.mu.Unlock()
	q.poke()
}
