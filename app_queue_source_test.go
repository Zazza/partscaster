package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"partscaster/internal/yue"
)

// ТК18 (условие 16): приложение ставит применение пресета из окна в общую очередь воркера
// операцией источника «app» и сразу отдаёт id операции воркера (работа — когда дойдёт очередь).
// Клиент — тот, что передан в NewApp (как в main.go); воркер — httptest. Фоновая цепочка после
// постановки на фейковом воркере может упасть — тест от её исхода не зависит.

type queuedPost struct {
	in     yue.QueueOpIn
	source string
}

func TestApplySoundPresetFromWindowQueuesAppOp(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var posts []queuedPost
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/queue/ops" {
			var in yue.QueueOpIn
			_ = json.NewDecoder(r.Body).Decode(&in)
			mu.Lock()
			posts = append(posts, queuedPost{in, r.Header.Get("X-Partscaster-Source")})
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(yue.QueueOp{ID: 9001, Kind: in.Kind, Title: in.Title, JobID: in.JobID,
				Source: r.Header.Get("X-Partscaster-Source"), State: "waiting", Client: true})
			return
		}
		http.NotFound(w, r) // остальное фоновой цепочке не нужно для проверки
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := NewApp(yue.New(srv.URL), &fakePlayer{})
	a.ctx = ctx // startup не вызываем: его фоновые циклы к тесту не относятся

	done := make(chan struct{})
	var id int64
	var err error
	go func() {
		defer close(done)
		id, err = a.YueApplySoundPresetWith(463, yue.SoundPreset{ID: 7, Name: "Пост-панк"})
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("применение пресета не вернулось сразу — ждёт работы, а не постановки в очередь")
	}
	if err != nil {
		t.Fatalf("YueApplySoundPresetWith: %v", err)
	}
	if id != 9001 {
		t.Fatalf("вернулся id %d, ждали id операции воркера 9001", id)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(posts) == 0 {
		t.Fatal("POST /queue/ops не отправлен — операция не в общей очереди воркера")
	}
	p := posts[0]
	if p.in.Kind != "preset" || p.in.JobID != 463 {
		t.Fatalf("операция в очереди: %+v, ждали kind=preset job_id=463", p.in)
	}
	if p.source != "app" {
		t.Fatalf("X-Partscaster-Source=%q, ждали app", p.source)
	}
}
