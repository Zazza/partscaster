package mcp

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"partscaster/internal/yue"
)

// Инструменты общей очереди (internal-shared-queue, условие 14, ТК16; контракт «Go»/MCP):
// queue_list, queue_move {id, dir}, queue_cancel {id, confirm}, queue_pause {paused}.
// Внешняя граница — yue.Service (фейк).

type moveCall struct {
	id  int64
	dir int
}

type queueFake struct {
	*fakeService
	snap    *yue.QueueSnapshot
	lists   int
	moves   []moveCall
	cancels []int64
	pauses  []bool
	err     error
}

func (f *queueFake) QueueList(ctx context.Context) (*yue.QueueSnapshot, error) {
	f.lists++
	if f.err != nil {
		return nil, f.err
	}
	return f.snap, nil
}

func (f *queueFake) QueueMove(ctx context.Context, id int64, dir int) error {
	f.moves = append(f.moves, moveCall{id, dir})
	return f.err
}

func (f *queueFake) QueueCancel(ctx context.Context, id int64) error {
	f.cancels = append(f.cancels, id)
	return f.err
}

func (f *queueFake) QueuePause(ctx context.Context, paused bool) error {
	f.pauses = append(f.pauses, paused)
	return f.err
}

func newQueueServer(t *testing.T) (*Server, *queueFake) {
	t.Helper()
	fake := &queueFake{fakeService: &fakeService{url: "http://w:8091"}, snap: &yue.QueueSnapshot{}}
	s := NewServer(fake, t.TempDir())
	RegisterWorkflowTools(s)
	RegisterStudioTools(s)
	RegisterLibraryTools(s)
	RegisterInstallTools(s)
	return s, fake
}

func TestQueueListShowsOps(t *testing.T) {
	t.Parallel()
	s, fake := newQueueServer(t)
	fake.snap = &yue.QueueSnapshot{Paused: true, Items: []yue.QueueOp{
		{ID: 31, Kind: "generate", Title: "Gone", JobID: 502, Source: "app", State: "running", Pos: 1},
		{ID: 32, Kind: "preset", Title: "Warm Wire — пресет", JobID: 463, Source: "mcp", State: "waiting", Pos: 2},
	}}
	out, ok := call(t, s, "queue_list", map[string]any{})
	if !ok {
		t.Fatalf("queue_list: %s", out)
	}
	if fake.lists != 1 {
		t.Fatalf("QueueList вызван %d раз", fake.lists)
	}
	for _, want := range []string{"31", "32", "Gone", "Warm Wire — пресет", "waiting", "running"} {
		if !strings.Contains(out, want) {
			t.Errorf("в ответе нет %q: %s", want, out)
		}
	}
}

// Краевой: пустая очередь — не ошибка.
func TestQueueListEmpty(t *testing.T) {
	t.Parallel()
	s, _ := newQueueServer(t)
	if out, ok := call(t, s, "queue_list", map[string]any{}); !ok {
		t.Fatalf("пустая очередь — не ошибка: %s", out)
	}
}

// Краевой: воркер недоступен — ошибка инструмента, а не паника.
func TestQueueListWorkerDown(t *testing.T) {
	t.Parallel()
	s, fake := newQueueServer(t)
	fake.err = errors.New("dial tcp: connection refused")
	if out, ok := call(t, s, "queue_list", map[string]any{}); ok {
		t.Fatalf("ожидалась ошибка, получено %s", out)
	}
}

func TestQueueMoveCallsService(t *testing.T) {
	t.Parallel()
	for _, dir := range []int{-1, 1} {
		s, fake := newQueueServer(t)
		out, ok := call(t, s, "queue_move", map[string]any{"id": float64(32), "dir": float64(dir)})
		if !ok {
			t.Fatalf("queue_move dir=%d: %s", dir, out)
		}
		if len(fake.moves) != 1 || fake.moves[0] != (moveCall{32, dir}) {
			t.Fatalf("QueueMove: %+v, нужно {32 %d}", fake.moves, dir)
		}
	}
}

// Неверные аргументы (нет id, dir не −1/1) — отказ, сервис не вызван.
func TestQueueMoveBadArgs(t *testing.T) {
	t.Parallel()
	for _, args := range []map[string]any{
		{"dir": float64(1)},
		{"id": float64(32)},
		{"id": float64(32), "dir": float64(0)},
		{"id": float64(32), "dir": float64(2)},
	} {
		s, fake := newQueueServer(t)
		if out, ok := call(t, s, "queue_move", args); ok {
			t.Fatalf("%v: ожидался отказ, получено %s", args, out)
		}
		if len(fake.moves) != 0 {
			t.Fatalf("%v: сервис не должен вызываться: %v", args, fake.moves)
		}
	}
}

// ТК8: move идущей/завершённой (409 воркера) — ошибка инструмента.
func TestQueueMoveConflictIsError(t *testing.T) {
	t.Parallel()
	s, fake := newQueueServer(t)
	fake.err = &yue.StatusError{Code: http.StatusConflict, Msg: "операция не ждёт"}
	if out, ok := call(t, s, "queue_move", map[string]any{"id": float64(31), "dir": float64(-1)}); ok {
		t.Fatalf("ожидалась ошибка: %s", out)
	}
}

// ТК16: queue_cancel без confirm=true — отказ, сервис не вызван.
func TestQueueCancelRequiresConfirm(t *testing.T) {
	t.Parallel()
	for _, args := range []map[string]any{
		{"id": float64(32)},
		{"id": float64(32), "confirm": false},
	} {
		s, fake := newQueueServer(t)
		if out, ok := call(t, s, "queue_cancel", args); ok {
			t.Fatalf("%v: отмена без подтверждения должна отказать, получено %s", args, out)
		}
		if len(fake.cancels) != 0 {
			t.Fatalf("%v: сервис не должен вызываться: %v", args, fake.cancels)
		}
	}
	s, fake := newQueueServer(t)
	if out, ok := call(t, s, "queue_cancel", map[string]any{"id": float64(32), "confirm": true}); !ok {
		t.Fatalf("queue_cancel с confirm: %s", out)
	}
	if len(fake.cancels) != 1 || fake.cancels[0] != 32 {
		t.Fatalf("QueueCancel: %v", fake.cancels)
	}
}

// Краевой: неизвестный id (404) — ошибка инструмента.
func TestQueueCancelUnknownIsError(t *testing.T) {
	t.Parallel()
	s, fake := newQueueServer(t)
	fake.err = &yue.StatusError{Code: http.StatusNotFound, Msg: "нет операции"}
	if out, ok := call(t, s, "queue_cancel", map[string]any{"id": float64(999), "confirm": true}); ok {
		t.Fatalf("ожидалась ошибка: %s", out)
	}
}

func TestQueuePauseCallsService(t *testing.T) {
	t.Parallel()
	s, fake := newQueueServer(t)
	for _, p := range []bool{true, false} {
		if out, ok := call(t, s, "queue_pause", map[string]any{"paused": p}); !ok {
			t.Fatalf("queue_pause %v: %s", p, out)
		}
	}
	if len(fake.pauses) != 2 || !fake.pauses[0] || fake.pauses[1] {
		t.Fatalf("QueuePause: %v, нужно [true false]", fake.pauses)
	}
}
