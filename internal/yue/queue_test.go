package yue

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// Клиент общей очереди операций воркера (internal-shared-queue, контракт «Go» и «Воркер: HTTP»):
// пути/методы/тела, заголовки X-Partscaster-Op / X-Partscaster-Source, декод QueueSnapshot/QueueOp,
// 404/409 → *StatusError. Внешняя граница — HTTP воркера (httptest).

type queueReq struct {
	method, path string
	body         map[string]any
	op, source   string
	hasOp        bool
	hasSource    bool
}

// queueServer — фейковый воркер: отвечает status/resp, записывает каждый запрос.
func queueServer(t *testing.T, status int, resp string) (*Client, func() []queueReq) {
	t.Helper()
	var mu sync.Mutex
	var reqs []queueReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := queueReq{method: r.Method, path: r.URL.Path, body: map[string]any{}}
		_, q.hasOp = r.Header[http.CanonicalHeaderKey("X-Partscaster-Op")]
		_, q.hasSource = r.Header[http.CanonicalHeaderKey("X-Partscaster-Source")]
		q.op = r.Header.Get("X-Partscaster-Op")
		q.source = r.Header.Get("X-Partscaster-Source")
		b, _ := io.ReadAll(r.Body)
		if len(b) > 0 {
			if err := json.Unmarshal(b, &q.body); err != nil {
				t.Errorf("тело не JSON-объект: %q", b)
			}
		}
		mu.Lock()
		reqs = append(reqs, q)
		mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL), func() []queueReq {
		mu.Lock()
		defer mu.Unlock()
		return append([]queueReq(nil), reqs...)
	}
}

func lastReq(t *testing.T, reqs func() []queueReq) queueReq {
	t.Helper()
	r := reqs()
	if len(r) == 0 {
		t.Fatal("воркер не получил ни одного запроса")
	}
	return r[len(r)-1]
}

func wantStatus(t *testing.T, err error, code int) {
	t.Helper()
	var se *StatusError
	if !errors.As(err, &se) || se.Code != code {
		t.Fatalf("ожидался *StatusError %d, получено %v", code, err)
	}
}

const opJSON = `{"id":5,"kind":"preset","title":"Warm Wire — пресет","job_id":463,"source":"mcp",
"state":"running","pos":3,"result_job_id":0,"error":"","cancel_requested":true}`

func TestQueueListDecodesSnapshot(t *testing.T) {
	t.Parallel()
	c, reqs := queueServer(t, http.StatusOK, `{"paused":true,"items":[`+opJSON+`,
{"id":6,"kind":"generate","title":"Gone","job_id":502,"source":"app","state":"done","pos":4,
"result_job_id":812,"error":"","cancel_requested":false},
{"id":7,"kind":"stems","title":"x","job_id":1,"source":"worker","state":"error","pos":5,
"result_job_id":0,"error":"нет памяти","cancel_requested":false}]}`)
	s, err := c.QueueList(context.Background())
	if err != nil {
		t.Fatalf("QueueList: %v", err)
	}
	r := lastReq(t, reqs)
	if r.method != http.MethodGet || r.path != "/queue" {
		t.Fatalf("запрос %s %s, нужен GET /queue", r.method, r.path)
	}
	if !s.Paused {
		t.Error("paused=true не прочитан")
	}
	if len(s.Items) != 3 {
		t.Fatalf("items: %d, нужно 3", len(s.Items))
	}
	op := s.Items[0]
	if op.ID != 5 || op.Kind != "preset" || op.Title != "Warm Wire — пресет" || op.JobID != 463 ||
		op.Source != "mcp" || op.State != "running" || op.Pos != 3 || !op.CancelRequested {
		t.Errorf("поля операции прочитаны не так: %+v", op)
	}
	if s.Items[1].ResultJobID != 812 || s.Items[1].State != "done" {
		t.Errorf("итог-версия: %+v", s.Items[1])
	}
	if s.Items[2].Error != "нет памяти" || s.Items[2].State != "error" {
		t.Errorf("итог-ошибка: %+v", s.Items[2])
	}
}

// ТК7 (краевой): пустая очередь — пустой список, без ошибки, paused читается.
func TestQueueListEmpty(t *testing.T) {
	t.Parallel()
	c, _ := queueServer(t, http.StatusOK, `{"paused":false,"items":[]}`)
	s, err := c.QueueList(context.Background())
	if err != nil {
		t.Fatalf("QueueList: %v", err)
	}
	if s == nil || s.Paused || len(s.Items) != 0 {
		t.Fatalf("пустая очередь: %+v", s)
	}
}

func TestQueueAddPostsOp(t *testing.T) {
	t.Parallel()
	c, reqs := queueServer(t, http.StatusOK, opJSON)
	op, err := c.QueueAdd(context.Background(), QueueOpIn{Kind: "preset", Title: "Warm Wire — пресет", JobID: 463})
	if err != nil {
		t.Fatalf("QueueAdd: %v", err)
	}
	r := lastReq(t, reqs)
	if r.method != http.MethodPost || r.path != "/queue/ops" {
		t.Fatalf("запрос %s %s, нужен POST /queue/ops", r.method, r.path)
	}
	if r.body["kind"] != "preset" || r.body["title"] != "Warm Wire — пресет" || r.body["job_id"] != float64(463) {
		t.Fatalf("тело: %v", r.body)
	}
	if op == nil || op.ID != 5 || op.State != "running" {
		t.Fatalf("ответ: %+v", op)
	}
}

func TestQueueBeatPath(t *testing.T) {
	t.Parallel()
	c, reqs := queueServer(t, http.StatusOK, opJSON)
	op, err := c.QueueBeat(context.Background(), 5)
	if err != nil {
		t.Fatalf("QueueBeat: %v", err)
	}
	r := lastReq(t, reqs)
	if r.method != http.MethodPost || r.path != "/queue/ops/5/beat" {
		t.Fatalf("запрос %s %s", r.method, r.path)
	}
	if !op.CancelRequested {
		t.Error("cancel_requested из ответа beat не прочитан")
	}
}

// Неизвестная операция (пропала, interrupted) — 404 как *StatusError: по нему studio узнаёт ErrInterrupted.
func TestQueueBeatNotFoundIsStatusError(t *testing.T) {
	t.Parallel()
	c, _ := queueServer(t, http.StatusNotFound, `{"detail":"нет операции"}`)
	_, err := c.QueueBeat(context.Background(), 99)
	wantStatus(t, err, http.StatusNotFound)
}

func TestQueueFinishPostsResult(t *testing.T) {
	t.Parallel()
	c, reqs := queueServer(t, http.StatusOK, `{"id":5,"state":"done","result_job_id":812}`)
	op, err := c.QueueFinish(context.Background(), 5, QueueFinishIn{State: "done", ResultJobID: 812})
	if err != nil {
		t.Fatalf("QueueFinish: %v", err)
	}
	r := lastReq(t, reqs)
	if r.method != http.MethodPost || r.path != "/queue/ops/5/finish" {
		t.Fatalf("запрос %s %s", r.method, r.path)
	}
	if r.body["state"] != "done" || r.body["result_job_id"] != float64(812) {
		t.Fatalf("тело: %v", r.body)
	}
	if op.State != "done" || op.ResultJobID != 812 {
		t.Fatalf("ответ: %+v", op)
	}
}

func TestQueueFinishErrorText(t *testing.T) {
	t.Parallel()
	c, reqs := queueServer(t, http.StatusOK, `{"id":5,"state":"error","error":"нет стемов"}`)
	if _, err := c.QueueFinish(context.Background(), 5, QueueFinishIn{State: "error", Error: "нет стемов"}); err != nil {
		t.Fatalf("QueueFinish: %v", err)
	}
	if b := lastReq(t, reqs).body; b["state"] != "error" || b["error"] != "нет стемов" {
		t.Fatalf("тело: %v", b)
	}
}

func TestQueueFinishConflictAndNotFound(t *testing.T) {
	t.Parallel()
	for _, code := range []int{http.StatusConflict, http.StatusNotFound} {
		c, _ := queueServer(t, code, `{"detail":"нельзя"}`)
		_, err := c.QueueFinish(context.Background(), 5, QueueFinishIn{State: "done"})
		wantStatus(t, err, code)
	}
}

func TestQueueMovePostsDir(t *testing.T) {
	t.Parallel()
	for _, dir := range []int{-1, 1} {
		c, reqs := queueServer(t, http.StatusOK, `{"moved":true}`)
		if err := c.QueueMove(context.Background(), 8, dir); err != nil {
			t.Fatalf("QueueMove(%d): %v", dir, err)
		}
		r := lastReq(t, reqs)
		if r.method != http.MethodPost || r.path != "/queue/ops/8/move" || r.body["dir"] != float64(dir) {
			t.Fatalf("запрос %s %s %v", r.method, r.path, r.body)
		}
	}
}

// ТК8: край (moved=false) — без ошибки; move идущей/завершённой (409) и несуществующей (404) — ошибка.
func TestQueueMoveEdgeAndErrors(t *testing.T) {
	t.Parallel()
	c, _ := queueServer(t, http.StatusOK, `{"moved":false}`)
	if err := c.QueueMove(context.Background(), 8, -1); err != nil {
		t.Fatalf("край очереди — не ошибка: %v", err)
	}
	for _, code := range []int{http.StatusConflict, http.StatusNotFound} {
		c, _ := queueServer(t, code, `{"detail":"не ждущая"}`)
		wantStatus(t, c.QueueMove(context.Background(), 8, 1), code)
	}
}

func TestQueueCancelPath(t *testing.T) {
	t.Parallel()
	c, reqs := queueServer(t, http.StatusOK, `{"canceled":true,"running":false}`)
	if err := c.QueueCancel(context.Background(), 11); err != nil {
		t.Fatalf("QueueCancel: %v", err)
	}
	r := lastReq(t, reqs)
	if r.method != http.MethodPost || r.path != "/queue/ops/11/cancel" {
		t.Fatalf("запрос %s %s", r.method, r.path)
	}
}

// ТК12 (краевой): повторная отмена завершённой — воркер отвечает {canceled:false}; клиент не падает.
func TestQueueCancelRepeatIsNotError(t *testing.T) {
	t.Parallel()
	c, _ := queueServer(t, http.StatusOK, `{"canceled":false,"running":false}`)
	if err := c.QueueCancel(context.Background(), 11); err != nil {
		t.Fatalf("повторная отмена: %v", err)
	}
}

func TestQueueCancelNotFound(t *testing.T) {
	t.Parallel()
	c, _ := queueServer(t, http.StatusNotFound, `{"detail":"нет операции"}`)
	wantStatus(t, c.QueueCancel(context.Background(), 404404), http.StatusNotFound)
}

func TestQueuePausePostsFlag(t *testing.T) {
	t.Parallel()
	for _, p := range []bool{true, false} {
		c, reqs := queueServer(t, http.StatusOK, `{"paused":true}`)
		if err := c.QueuePause(context.Background(), p); err != nil {
			t.Fatalf("QueuePause(%v): %v", p, err)
		}
		r := lastReq(t, reqs)
		if r.method != http.MethodPost || r.path != "/queue/pause" || r.body["paused"] != p {
			t.Fatalf("запрос %s %s %v", r.method, r.path, r.body)
		}
	}
}

func TestWithOpRoundTrip(t *testing.T) {
	t.Parallel()
	if got := OpFrom(context.Background()); got != 0 {
		t.Fatalf("ctx без операции: OpFrom=%d, нужно 0", got)
	}
	if got := OpFrom(WithOp(context.Background(), 42)); got != 42 {
		t.Fatalf("OpFrom(WithOp(42))=%d", got)
	}
}

// Метка операции идёт в каждый запрос клиента, у которого в ctx есть op (в т.ч. в старые методы),
// и не ставится, если op в ctx нет.
func TestOpHeaderSentFromContext(t *testing.T) {
	t.Parallel()
	c, reqs := queueServer(t, http.StatusOK, `{"retried":true}`)
	if err := c.RetryJob(WithOp(context.Background(), 42), 3); err != nil {
		t.Fatalf("RetryJob: %v", err)
	}
	if r := lastReq(t, reqs); r.op != "42" {
		t.Fatalf("X-Partscaster-Op=%q, нужно 42", r.op)
	}
	if err := c.RetryJob(context.Background(), 3); err != nil {
		t.Fatalf("RetryJob: %v", err)
	}
	if r := lastReq(t, reqs); r.hasOp {
		t.Fatalf("без op в ctx заголовка X-Partscaster-Op быть не должно: %q", r.op)
	}
}

// Источник (SetSource) — в каждом запросе: и в новых методах очереди, и в старых.
func TestSourceHeaderOnEveryRequest(t *testing.T) {
	t.Parallel()
	c, reqs := queueServer(t, http.StatusOK, opJSON)
	c.SetSource("mcp")
	ctx := context.Background()
	_, _ = c.QueueAdd(ctx, QueueOpIn{Kind: "preset", Title: "t", JobID: 1})
	_, _ = c.QueueBeat(ctx, 5)
	_ = c.RetryJob(ctx, 3)
	_, _ = c.QueueList(ctx)
	got := reqs()
	if len(got) != 4 {
		t.Fatalf("запросов %d, нужно 4", len(got))
	}
	for _, r := range got {
		if r.source != "mcp" {
			t.Errorf("%s %s: X-Partscaster-Source=%q, нужно mcp", r.method, r.path, r.source)
		}
	}
	c.SetSource("app")
	_, _ = c.QueueList(ctx)
	if r := lastReq(t, reqs); r.source != "app" {
		t.Errorf("после SetSource(app): %q", r.source)
	}
}
