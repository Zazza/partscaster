package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"partscaster/internal/yue"
)

// Черновик работы над треком (internal-studio-draft, условие 2, ТК4).
// Инструменты: draft_list {}, draft_get {job_id}, draft_set {job_id, state (object)},
// draft_clear {job_id, confirm} — деструктивное, без confirm=true — ошибка.
// draft_get/draft_list отдают JSON.

type draftFake struct {
	*fakeService
	drafts  map[int64]json.RawMessage
	puts    []int64
	putBody []json.RawMessage
	dels    []int64
}

func (f *draftFake) Drafts(context.Context) ([]yue.Draft, error) {
	var out []yue.Draft
	for id, st := range f.drafts {
		out = append(out, yue.Draft{JobID: id, State: st, UpdatedAt: "2026-10-16T10:00:00"})
	}
	return out, nil
}

func (f *draftFake) Draft(_ context.Context, id int64) (*yue.Draft, error) {
	st, ok := f.drafts[id]
	if !ok {
		return nil, nil
	}
	return &yue.Draft{JobID: id, State: st, UpdatedAt: "2026-10-16T10:00:00"}, nil
}

func (f *draftFake) DraftPut(_ context.Context, id int64, state json.RawMessage) error {
	f.puts = append(f.puts, id)
	f.putBody = append(f.putBody, append(json.RawMessage(nil), state...))
	f.drafts[id] = state
	return nil
}

func (f *draftFake) DraftDelete(_ context.Context, id int64) (bool, error) {
	f.dels = append(f.dels, id)
	_, ok := f.drafts[id]
	delete(f.drafts, id)
	return ok, nil
}

func newDraftServer(t *testing.T) (*Server, *draftFake) {
	t.Helper()
	fake := &draftFake{fakeService: &fakeService{url: "http://w:8091"}, drafts: map[int64]json.RawMessage{}}
	s := NewServer(fake, t.TempDir())
	RegisterWorkflowTools(s)
	RegisterStudioTools(s)
	RegisterLibraryTools(s)
	RegisterInstallTools(s)
	return s, fake
}

// jsonIn — первый JSON-объект или массив в выводе инструмента.
func jsonIn(t *testing.T, out string) any {
	t.Helper()
	i := strings.IndexAny(out, "{[")
	if i < 0 {
		t.Fatalf("в выводе нет JSON: %s", out)
	}
	dec := json.NewDecoder(strings.NewReader(out[i:]))
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("вывод не JSON: %v\n%s", err, out)
	}
	return v
}

func TestDraftClearWithoutConfirmFails(t *testing.T) {
	t.Parallel()
	s, fake := newDraftServer(t)
	fake.drafts[5] = json.RawMessage(`{"v":1}`)
	for _, args := range []map[string]any{
		{"job_id": float64(5)},
		{"job_id": float64(5), "confirm": false},
	} {
		if out, ok := call(t, s, "draft_clear", args); ok {
			t.Fatalf("draft_clear без confirm=true прошёл: %s", out)
		}
	}
	if len(fake.dels) != 0 {
		t.Fatalf("DraftDelete вызван без подтверждения: %v", fake.dels)
	}
}

func TestDraftClearWithConfirm(t *testing.T) {
	t.Parallel()
	s, fake := newDraftServer(t)
	fake.drafts[5] = json.RawMessage(`{"v":1}`)
	out, ok := call(t, s, "draft_clear", map[string]any{"job_id": float64(5), "confirm": true})
	if !ok {
		t.Fatalf("draft_clear failed: %s", out)
	}
	if len(fake.dels) != 1 || fake.dels[0] != 5 {
		t.Fatalf("DraftDelete %v, want [5]", fake.dels)
	}
}

func TestDraftSetNonObjectFails(t *testing.T) {
	t.Parallel()
	s, fake := newDraftServer(t)
	for _, st := range []any{[]any{1.0, 2.0}, "text", 3.0, nil} {
		if out, ok := call(t, s, "draft_set", map[string]any{"job_id": float64(5), "state": st}); ok {
			t.Fatalf("draft_set со state=%v прошёл: %s", st, out)
		}
	}
	if out, ok := call(t, s, "draft_set", map[string]any{"job_id": float64(5)}); ok {
		t.Fatalf("draft_set без state прошёл: %s", out)
	}
	if len(fake.puts) != 0 {
		t.Fatalf("DraftPut вызван с не-объектом: %v", fake.puts)
	}
}

func TestDraftSetObject(t *testing.T) {
	t.Parallel()
	s, fake := newDraftServer(t)
	st := map[string]any{"v": 1.0, "edits": []any{map[string]any{"childId": -2.0, "db": -6.0}}, "queue": []any{}, "ts": 17.0}
	out, ok := call(t, s, "draft_set", map[string]any{"job_id": float64(9), "state": st})
	if !ok {
		t.Fatalf("draft_set failed: %s", out)
	}
	if len(fake.puts) != 1 || fake.puts[0] != 9 {
		t.Fatalf("DraftPut %v, want [9]", fake.puts)
	}
	var got any
	if err := json.Unmarshal(fake.putBody[0], &got); err != nil {
		t.Fatalf("state не JSON: %s", fake.putBody[0])
	}
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(st)
	if string(a) != string(b) {
		t.Fatalf("state %s, want %s", a, b)
	}
}

func TestDraftGetReturnsJSON(t *testing.T) {
	t.Parallel()
	s, fake := newDraftServer(t)
	fake.drafts[7] = json.RawMessage(`{"v":1,"edits":[],"queue":[],"ts":42}`)
	out, ok := call(t, s, "draft_get", map[string]any{"job_id": float64(7)})
	if !ok {
		t.Fatalf("draft_get failed: %s", out)
	}
	m, isObj := jsonIn(t, out).(map[string]any)
	if !isObj {
		t.Fatalf("draft_get: не объект: %s", out)
	}
	if m["job_id"] != 7.0 {
		t.Fatalf("job_id %v: %s", m["job_id"], out)
	}
	st, _ := m["state"].(map[string]any)
	if st == nil || st["ts"] != 42.0 {
		t.Fatalf("state не тот: %s", out)
	}
	if _, ok := m["updated_at"]; !ok {
		t.Fatalf("нет updated_at: %s", out)
	}
}

func TestDraftGetMissingIsNotCrash(t *testing.T) {
	t.Parallel()
	s, _ := newDraftServer(t)
	// черновика нет — не паника; ответ (ошибка или «нет черновика») — на усмотрение реализации
	_, _ = call(t, s, "draft_get", map[string]any{"job_id": float64(8)})
}

func TestDraftListReturnsJSON(t *testing.T) {
	t.Parallel()
	s, fake := newDraftServer(t)
	fake.drafts[3] = json.RawMessage(`{"v":1,"ts":1}`)
	out, ok := call(t, s, "draft_list", map[string]any{})
	if !ok {
		t.Fatalf("draft_list failed: %s", out)
	}
	arr, isArr := jsonIn(t, out).([]any)
	if !isArr || len(arr) != 1 {
		t.Fatalf("draft_list: want массив из 1: %s", out)
	}
	m, _ := arr[0].(map[string]any)
	if m == nil || m["job_id"] != 3.0 {
		t.Fatalf("элемент %v", arr[0])
	}
}

func TestDraftListEmptyIsJSON(t *testing.T) {
	t.Parallel()
	s, _ := newDraftServer(t)
	out, ok := call(t, s, "draft_list", map[string]any{})
	if !ok {
		t.Fatalf("draft_list failed: %s", out)
	}
	if arr, isArr := jsonIn(t, out).([]any); !isArr || len(arr) != 0 {
		t.Fatalf("пустой список — пустой JSON-массив: %s", out)
	}
}

// Журнал правок черновик не пишет (это не правка трека).
func TestDraftToolsNoJournal(t *testing.T) {
	t.Parallel()
	fake := &draftFake{fakeService: &fakeService{url: "http://w:8091"}, drafts: map[int64]json.RawMessage{}}
	rec := &draftJournal{draftFake: fake}
	s2 := NewServer(rec, t.TempDir())
	RegisterWorkflowTools(s2)
	RegisterStudioTools(s2)
	RegisterLibraryTools(s2)
	RegisterInstallTools(s2)
	if out, ok := call(t, s2, "draft_set", map[string]any{"job_id": float64(4), "state": map[string]any{"v": 1.0}}); !ok {
		t.Fatalf("draft_set: %s", out)
	}
	if out, ok := call(t, s2, "draft_clear", map[string]any{"job_id": float64(4), "confirm": true}); !ok {
		t.Fatalf("draft_clear: %s", out)
	}
	if len(rec.ops) != 0 {
		t.Fatalf("черновик записан в журнал: %v", rec.ops)
	}
}

type draftJournal struct {
	*draftFake
	ops []string
}

func (d *draftJournal) RecordOp(_ context.Context, _ int64, op yue.JobOp) error {
	d.ops = append(d.ops, op.Op)
	return nil
}
