package yue

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Черновик работы над треком (internal-studio-draft, условия 1–2, ТК2).
// Контракт клиента:
//   Drafts(ctx)              → GET /drafts               → []Draft
//   Draft(ctx, id)           → GET /jobs/{id}/draft      → *Draft; 404 → (nil, nil) — «черновика нет»
//   DraftPut(ctx, id, state) → PUT /jobs/{id}/draft {"state": state}
//   DraftDelete(ctx, id)     → DELETE /jobs/{id}/draft   → deleted bool
// Журнал правок черновик не пишет (это не правка трека).
// GET /jobs: признак has_draft доходит до приложения.

type draftReq struct {
	method, path string
	body         []byte
}

// draftServer — воркер, записывающий запросы и отвечающий status/resp.
func draftServer(t *testing.T, status int, resp string) (*Client, *[]draftReq) {
	t.Helper()
	var mu sync.Mutex
	got := &[]draftReq{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		*got = append(*got, draftReq{r.Method, r.URL.Path, b})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL), got
}

func oneReq(t *testing.T, got *[]draftReq) draftReq {
	t.Helper()
	if len(*got) != 1 {
		t.Fatalf("ожидался один запрос, было %d: %+v", len(*got), *got)
	}
	return (*got)[0]
}

func sameJSON(t *testing.T, a, b []byte) bool {
	t.Helper()
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		t.Fatalf("not JSON: %q", a)
	}
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatalf("not JSON: %q", b)
	}
	xa, _ := json.Marshal(x)
	ya, _ := json.Marshal(y)
	return string(xa) == string(ya)
}

func TestDraftsList(t *testing.T) {
	t.Parallel()
	c, got := draftServer(t, 200,
		`[{"job_id":7,"state":{"v":1,"edits":[],"queue":[],"ts":5},"updated_at":"2026-10-16T10:00:00"},
		  {"job_id":9,"state":{"v":1},"updated_at":"2026-10-16T11:00:00"}]`)
	ds, err := c.Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	r := oneReq(t, got)
	if r.method != http.MethodGet || r.path != "/drafts" {
		t.Fatalf("request: %s %s", r.method, r.path)
	}
	if len(ds) != 2 || ds[0].JobID != 7 || ds[1].JobID != 9 {
		t.Fatalf("decoded %+v", ds)
	}
	if ds[0].UpdatedAt != "2026-10-16T10:00:00" {
		t.Fatalf("updated_at %q", ds[0].UpdatedAt)
	}
	if !sameJSON(t, ds[0].State, []byte(`{"v":1,"edits":[],"queue":[],"ts":5}`)) {
		t.Fatalf("state %s", ds[0].State)
	}
}

func TestDraftsListEmpty(t *testing.T) {
	t.Parallel()
	c, _ := draftServer(t, 200, `[]`)
	ds, err := c.Drafts(context.Background())
	if err != nil || len(ds) != 0 {
		t.Fatalf("Drafts: %v %+v", err, ds)
	}
}

func TestDraftGet(t *testing.T) {
	t.Parallel()
	c, got := draftServer(t, 200, `{"job_id":12,"state":{"v":1,"ts":3},"updated_at":"u"}`)
	d, err := c.Draft(context.Background(), 12)
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	r := oneReq(t, got)
	if r.method != http.MethodGet || r.path != "/jobs/12/draft" {
		t.Fatalf("request: %s %s", r.method, r.path)
	}
	if d == nil || d.JobID != 12 || d.UpdatedAt != "u" || !sameJSON(t, d.State, []byte(`{"v":1,"ts":3}`)) {
		t.Fatalf("decoded %+v", d)
	}
}

func TestDraftGet404IsNoDraft(t *testing.T) {
	t.Parallel()
	c, _ := draftServer(t, 404, `{"detail":"draft not found"}`)
	d, err := c.Draft(context.Background(), 12)
	if err != nil {
		t.Fatalf("404 — «черновика нет», не ошибка: %v", err)
	}
	if d != nil {
		t.Fatalf("expected nil draft, got %+v", d)
	}
}

func TestDraftGetServerErrorIsError(t *testing.T) {
	t.Parallel()
	c, _ := draftServer(t, 500, `{"detail":"boom"}`)
	if _, err := c.Draft(context.Background(), 12); err == nil {
		t.Fatal("500 должен быть ошибкой, а не «черновика нет»")
	}
}

func TestDraftPut(t *testing.T) {
	t.Parallel()
	c, got := draftServer(t, 200, `{"job_id":4,"state":{"v":1},"updated_at":"u"}`)
	state := json.RawMessage(`{"v":1,"edits":[{"childId":-2,"db":-6}],"queue":[],"ts":17}`)
	if err := c.DraftPut(context.Background(), 4, state); err != nil {
		t.Fatalf("DraftPut: %v", err)
	}
	r := oneReq(t, got)
	if r.method != http.MethodPut || r.path != "/jobs/4/draft" {
		t.Fatalf("request: %s %s", r.method, r.path)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(r.body, &body); err != nil {
		t.Fatalf("body is not JSON object: %q", r.body)
	}
	if len(body) != 1 {
		t.Fatalf("body must contain only state: %s", r.body)
	}
	if !sameJSON(t, body["state"], state) {
		t.Fatalf("state sent %s", body["state"])
	}
}

func TestDraftPutErrors(t *testing.T) {
	t.Parallel()
	for _, code := range []int{404, 422} {
		c, _ := draftServer(t, code, `{"detail":"x"}`)
		if err := c.DraftPut(context.Background(), 4, json.RawMessage(`{"v":1}`)); err == nil {
			t.Fatalf("%d должен вернуться ошибкой", code)
		}
	}
}

func TestDraftDelete(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		resp string
		want bool
	}{{`{"deleted":true}`, true}, {`{"deleted":false}`, false}} {
		c, got := draftServer(t, 200, tc.resp)
		ok, err := c.DraftDelete(context.Background(), 31)
		if err != nil {
			t.Fatalf("DraftDelete: %v", err)
		}
		r := oneReq(t, got)
		if r.method != http.MethodDelete || r.path != "/jobs/31/draft" {
			t.Fatalf("request: %s %s", r.method, r.path)
		}
		if ok != tc.want {
			t.Fatalf("deleted %v, want %v", ok, tc.want)
		}
	}
}

// GET /jobs: has_draft воркера не теряется по пути в приложение (фронт получает
// Job через JSON биндинга).
func TestJobsCarryHasDraft(t *testing.T) {
	t.Parallel()
	c, _ := draftServer(t, 200,
		`[{"id":1,"title":"a","status":"done","has_draft":true},{"id":2,"title":"b","status":"done","has_draft":false}]`)
	jobs, err := c.Jobs(context.Background())
	if err != nil || len(jobs) != 2 {
		t.Fatalf("Jobs: %v %+v", err, jobs)
	}
	for i, want := range []bool{true, false} {
		b, _ := json.Marshal(jobs[i])
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		if v, ok := m["has_draft"].(bool); !ok || v != want {
			t.Fatalf("job %d: has_draft=%v (json %s), want %v", jobs[i].ID, m["has_draft"], b, want)
		}
	}
}

// Журнал правок черновик не пишет: вызовы через Journaled доходят до сервиса,
// записей журнала нет.
type draftJnFake struct {
	Service
	mu    sync.Mutex
	recs  []JobOp
	calls []string
}

func (f *draftJnFake) RecordOp(_ context.Context, _ int64, op JobOp) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recs = append(f.recs, op)
	return nil
}

func (f *draftJnFake) note(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, s)
}

func (f *draftJnFake) Drafts(context.Context) ([]Draft, error) { f.note("Drafts"); return nil, nil }
func (f *draftJnFake) Draft(context.Context, int64) (*Draft, error) {
	f.note("Draft")
	return nil, nil
}
func (f *draftJnFake) DraftPut(context.Context, int64, json.RawMessage) error {
	f.note("DraftPut")
	return nil
}
func (f *draftJnFake) DraftDelete(context.Context, int64) (bool, error) {
	f.note("DraftDelete")
	return true, nil
}

func TestJournaledDraftsNotRecorded(t *testing.T) {
	t.Parallel()
	f := &draftJnFake{}
	svc := Journaled(f)
	ctx := context.Background()
	if _, err := svc.Drafts(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Draft(ctx, 3); err != nil {
		t.Fatal(err)
	}
	if err := svc.DraftPut(ctx, 3, json.RawMessage(`{"v":1}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DraftDelete(ctx, 3); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.calls, ","); got != "Drafts,Draft,DraftPut,DraftDelete" {
		t.Fatalf("calls %s", got)
	}
	if len(f.recs) != 0 {
		t.Fatalf("черновик не пишет журнал, записано: %+v", f.recs)
	}
}
