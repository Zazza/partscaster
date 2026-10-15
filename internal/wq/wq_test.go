// Спецификация очереди операций (wq): исполняет по одной в порядке ждущих,
// Move меняет порядок среди ждущих, Cancel убирает ждущую, пауза не пускает
// следующую пока не снять, завершённые попадают в отчёт (с ошибкой — с текстом).
package wq

import (
	"context"
	"errors"
	"testing"
	"time"
)

func runQueue() (*Queue, context.CancelFunc) {
	q := New()
	ctx, cancel := context.WithCancel(context.Background())
	go q.Run(ctx)
	return q, cancel
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("условие не наступило")
}

func TestOrderSerial(t *testing.T) {
	q, cancel := runQueue()
	defer cancel()
	var order []int64
	done := make(chan struct{})
	q.Enqueue("preset", "a", 1, func(context.Context) (int64, error) { order = append(order, 1); return 0, nil })
	q.Enqueue("preset", "b", 2, func(context.Context) (int64, error) { order = append(order, 2); return 0, nil })
	q.Enqueue("preset", "c", 3, func(context.Context) (int64, error) { order = append(order, 3); close(done); return 0, nil })
	<-done
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Fatalf("порядок %v, want [1 2 3]", order)
	}
}

func TestMoveReordersWaiting(t *testing.T) {
	q, cancel := runQueue()
	defer cancel()
	block := make(chan struct{})
	release := make(chan struct{})
	q.Enqueue("preset", "blocker", 1, func(context.Context) (int64, error) { <-block; <-release; return 0, nil })
	q.Enqueue("preset", "b", 2, func(context.Context) (int64, error) { <-release; return 0, nil })
	c := q.Enqueue("preset", "c", 3, func(context.Context) (int64, error) { <-release; return 0, nil })
	q.Move(c.ID, -1) // c выше b
	waitFor(t, func() bool {
		s := q.Snapshot()
		return len(s.Items) >= 2 && s.Items[1].Title == "c"
	})
	close(block)
	close(release)
}

func TestCancelWaiting(t *testing.T) {
	q, cancel := runQueue()
	defer cancel()
	release := make(chan struct{})
	q.Enqueue("preset", "a", 1, func(context.Context) (int64, error) { <-release; return 0, nil })
	it := q.Enqueue("preset", "b", 2, func(context.Context) (int64, error) { return 0, nil })
	q.Cancel(it.ID)
	s := q.Snapshot()
	if len(s.Items) != 2 { // идущая a + отменённая b
		t.Fatalf("записей %d (%v), want 2", len(s.Items), s.Items)
	}
	if last := s.Items[len(s.Items)-1]; last.State != "canceled" {
		t.Fatalf("отменённая запись: %+v", last)
	}
	close(release)
}

func TestPauseHoldsNext(t *testing.T) {
	q, cancel := runQueue()
	defer cancel()
	release := make(chan struct{})
	first := make(chan struct{})
	q.Enqueue("preset", "a", 1, func(context.Context) (int64, error) { close(first); <-release; return 0, nil })
	<-first
	q.SetPaused(true)
	started := make(chan struct{})
	q.Enqueue("preset", "b", 2, func(context.Context) (int64, error) { close(started); return 0, nil })
	select {
	case <-started:
		t.Fatal("под паузой началась вторая запись")
	case <-time.After(80 * time.Millisecond):
	}
	if !q.Snapshot().Paused {
		t.Fatal("пауза не отражена в снимке")
	}
	q.SetPaused(false)
	close(release) // первая доигрывает — только тогда вторая может начаться
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("после снятия паузы вторая запись не началась")
	}
}

func TestErrorRecorded(t *testing.T) {
	q, cancel := runQueue()
	defer cancel()
	done := make(chan struct{})
	q.Enqueue("preset", "bad", 1, func(context.Context) (int64, error) { return 0, errors.New("воркер недоступен") })
	q.Enqueue("preset", "ok", 2, func(context.Context) (int64, error) { close(done); return 0, nil })
	<-done
	var bad, ok Item
	for _, it := range q.Snapshot().Items {
		switch it.Title {
		case "bad":
			bad = it
		case "ok":
			ok = it
		}
	}
	if bad.State != "error" || bad.Err != "воркер недоступен" {
		t.Fatalf("ошибочная запись: %+v", bad)
	}
	if ok.State != "done" {
		t.Fatalf("успешная запись: %+v", ok)
	}
}

// Успешная операция прикладывает итог (для пресета — id версии-трека) к записи
// очереди: интерфейс по нему находит и показывает готовую версию.
func TestResultRecorded(t *testing.T) {
	q, cancel := runQueue()
	defer cancel()
	done := make(chan Item, 1)
	q.Enqueue("preset", "ок", 7, func(context.Context) (int64, error) { return 42, nil })
	waitFor(t, func() bool {
		for _, it := range q.Snapshot().Items {
			if it.Title == "ок" && it.State == "done" {
				done <- it
				return true
			}
		}
		return false
	})
	it := <-done
	if it.ResultJobID != 42 {
		t.Fatalf("итог записи: %+v, want result_job_id 42", it)
	}
}
