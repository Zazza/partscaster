package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"partscaster/internal/yue"
)

// ТК17 (условие 15): клиент воркера, которым работает MCP-сервер (WorkerClient — его создаёт
// cmd/partscaster-mcp), помечает свои запросы источником «mcp». Постановка операции в общую
// очередь (POST /queue/ops) несёт X-Partscaster-Source: mcp — в приложении она видна как «ИИ».
// Внешняя граница — воркер (httptest).

type seenReq struct {
	method, path, source string
}

func sourceWorker(t *testing.T) (*httptest.Server, func() []seenReq) {
	t.Helper()
	var mu sync.Mutex
	var seen []seenReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, seenReq{r.Method, r.URL.Path, r.Header.Get("X-Partscaster-Source")})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/queue/ops":
			var in yue.QueueOpIn
			_ = json.NewDecoder(r.Body).Decode(&in)
			_ = json.NewEncoder(w).Encode(yue.QueueOp{ID: 4242, Kind: in.Kind, Title: in.Title, JobID: in.JobID,
				Source: r.Header.Get("X-Partscaster-Source"), State: "running", Client: true})
		case r.Method == http.MethodGet && r.URL.Path == "/queue":
			_ = json.NewEncoder(w).Encode(yue.QueueSnapshot{Items: []yue.QueueOp{}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []seenReq {
		mu.Lock()
		defer mu.Unlock()
		return append([]seenReq(nil), seen...)
	}
}

type queueAdder interface {
	QueueAdd(ctx context.Context, in yue.QueueOpIn) (*yue.QueueOp, error)
}

func TestWorkerClientQueueAddSourceMCP(t *testing.T) {
	t.Parallel()
	srv, seen := sourceWorker(t)
	c, ok := WorkerClient(srv.URL).(queueAdder)
	if !ok {
		t.Fatal("клиент MCP не умеет ставить операцию в очередь воркера (QueueAdd)")
	}
	op, err := c.QueueAdd(context.Background(), yue.QueueOpIn{Kind: "preset", Title: "пост-панк", JobID: 463})
	if err != nil {
		t.Fatalf("QueueAdd: %v", err)
	}
	if op.ID != 4242 {
		t.Fatalf("id операции воркера не вернулся: %+v", op)
	}
	var post *seenReq
	for _, r := range seen() {
		if r.method == http.MethodPost && r.path == "/queue/ops" {
			r := r
			post = &r
		}
	}
	if post == nil {
		t.Fatalf("POST /queue/ops не отправлен: %+v", seen())
	}
	if post.source != "mcp" {
		t.Fatalf("POST /queue/ops: X-Partscaster-Source=%q, ждали mcp", post.source)
	}
}

func TestWorkerClientQueueListSourceMCP(t *testing.T) {
	t.Parallel()
	srv, seen := sourceWorker(t)
	c, ok := WorkerClient(srv.URL).(queueView)
	if !ok {
		t.Fatal("клиент MCP не даёт список очереди")
	}
	if _, err := c.QueueList(context.Background()); err != nil {
		t.Fatalf("QueueList: %v", err)
	}
	got := seen()
	if len(got) == 0 || got[0].path != "/queue" || got[0].source != "mcp" {
		t.Fatalf("GET /queue без источника mcp: %+v", got)
	}
}
