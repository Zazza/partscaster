package yue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// yue.Journaled(svc) после УСПЕШНОГО вызова пишет одну запись журнала через
// svc.(OpRecorder).RecordOp(ctx, jobID, JobOp{Op, Params, Result, Refs}):
//   Submit с ParentID → submit (params — заявка, сид фактический, result job:<id>),
//   ContinueJob → continue, FragmentJob → fragment, SubmitOverdub → overdub,
//   VariantToTrack → variant_track (params file, title, voice_src; result job:<id>),
//   ApplyFx не-превью → fx (result — файл), UpdateJob → update,
//   SetHead → set_head.
// Не пишут: ошибка вызова, превью fx, Submit без родителя, контекст WithoutJournal.
// Сбой записи не роняет вызов. yue.Record пишет через OpRecorder, если контекст не «без журнала».
// Клиент: RecordOp → POST /jobs/{id}/ops; ClaimRecipeImport 409 → ErrClaimed.
//
// Воркер — jnFake: только внешние границы (методы Service, которые зовёт тест, и RecordOp).
// Остальные методы не реализованы: их вызов — паника (нарушение контракта).

type jnRec struct {
	jobID int64
	op    JobOp
}

type jnFake struct {
	Service
	mu       sync.Mutex
	recs     []jnRec
	recErr   error // RecordOp падает
	callErr  error // вызовы сервиса падают
	newID    int64 // id, который возвращают создающие вызовы
	newSeed  int64 // фактический сид новой джобы (в списке Jobs)
	fxFile   string
	calls    []string
	updTitle *string
}

func newJnFake() *jnFake { return &jnFake{newID: 77, newSeed: 31337, fxFile: "dsp-fx-1.flac"} }

func (f *jnFake) RecordOp(_ context.Context, jobID int64, op JobOp) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recs = append(f.recs, jnRec{jobID, op})
	return f.recErr
}

func (f *jnFake) call(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, name)
	return f.callErr
}

func (f *jnFake) Jobs(context.Context) ([]Job, error) {
	return []Job{
		{ID: 5, Status: "done", Seed: 111},
		{ID: f.newID, Status: "queued", Seed: f.newSeed},
	}, nil
}

func (f *jnFake) Submit(_ context.Context, _ SubmitParams) (int64, error) {
	if err := f.call("submit"); err != nil {
		return 0, err
	}
	return f.newID, nil
}

func (f *jnFake) ContinueJob(_ context.Context, _ int64, _ float64, _ int64, _, _ string) (int64, error) {
	if err := f.call("continue"); err != nil {
		return 0, err
	}
	return f.newID, nil
}

func (f *jnFake) FragmentJob(_ context.Context, _ int64, _, _ float64, _ int64, _, _ string) (int64, error) {
	if err := f.call("fragment"); err != nil {
		return 0, err
	}
	return f.newID, nil
}

func (f *jnFake) SubmitOverdub(_ context.Context, _ int64, _, _ string, _ float64, _ string, _ int64) (int64, error) {
	if err := f.call("overdub"); err != nil {
		return 0, err
	}
	return f.newID, nil
}

func (f *jnFake) VariantToTrack(_ context.Context, _ int64, _, _ string, _ int64) (int64, error) {
	if err := f.call("variant_track"); err != nil {
		return 0, err
	}
	return f.newID, nil
}

func (f *jnFake) ApplyFx(_ context.Context, _ int64, req FxRequest) (*DspVariant, error) {
	if err := f.call("fx"); err != nil {
		return nil, err
	}
	if req.Preview {
		return &DspVariant{File: "preview-fx-1.flac"}, nil
	}
	return &DspVariant{File: f.fxFile}, nil
}

func (f *jnFake) MakeStems(_ context.Context, _ int64) (map[string]any, error) {
	if err := f.call("stems"); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func (f *jnFake) UpdateJob(_ context.Context, id int64, title, _ *string) (*Job, error) {
	if err := f.call("update"); err != nil {
		return nil, err
	}
	f.updTitle = title
	return &Job{ID: id}, nil
}

func (f *jnFake) SetHead(_ context.Context, _, _ int64) error {
	return f.call("set_head")
}

func jnParams(t *testing.T, op JobOp) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(op.Params, &m); err != nil {
		t.Fatalf("params %s — не JSON-объект: %v", op.Params, err)
	}
	return m
}

func jnNum(m map[string]any, k string) (float64, bool) {
	v, ok := m[k].(float64)
	return v, ok
}

// jnOne — ровно одна запись, op как ожидается.
func jnOne(t *testing.T, f *jnFake, op string) jnRec {
	t.Helper()
	if len(f.recs) != 1 {
		t.Fatalf("записей журнала %d (%+v), want 1 (%s)", len(f.recs), f.recs, op)
	}
	if f.recs[0].op.Op != op {
		t.Fatalf("op %q, want %q", f.recs[0].op.Op, op)
	}
	return f.recs[0]
}

// Submit с родителем и сидом 0 → submit, params — заявка (родитель, стиль) с ФАКТИЧЕСКИМ
// сидом новой джобы из списка, result job:<новый id>.
func TestJournaledSubmitWithParentRecordsActualSeed(t *testing.T) {
	t.Parallel()
	f := newJnFake()
	svc := Journaled(f)
	id, err := svc.Submit(context.Background(), SubmitParams{Title: "кусок", Style: "post-punk", Lyrics: "la",
		Seed: 0, ParentID: 5, Role: "section", Abc: "X:1"})
	if err != nil || id != f.newID {
		t.Fatalf("Submit = %d, %v; want %d, nil", id, err, f.newID)
	}
	r := jnOne(t, f, "submit")
	if r.jobID != 5 && r.jobID != f.newID {
		t.Errorf("запись у трека %d, want родитель 5 или новый %d", r.jobID, f.newID)
	}
	if r.op.Result != fmt.Sprintf("job:%d", f.newID) {
		t.Errorf("result %q, want job:%d", r.op.Result, f.newID)
	}
	p := jnParams(t, r.op)
	if s, _ := jnNum(p, "seed"); int64(s) != f.newSeed {
		t.Errorf("seed в params %v, want фактический %d (не 0)", p["seed"], f.newSeed)
	}
	if pid, _ := jnNum(p, "parent_id"); int64(pid) != 5 {
		t.Errorf("parent_id в params %v, want 5", p["parent_id"])
	}
	if p["style"] != "post-punk" || p["abc"] != "X:1" || p["role"] != "section" {
		t.Errorf("params не заявка: %v", p)
	}
}

// Сид задан явно → в params он и остаётся.
func TestJournaledSubmitExplicitSeed(t *testing.T) {
	t.Parallel()
	f := newJnFake()
	if _, err := Journaled(f).Submit(context.Background(), SubmitParams{Style: "s", Seed: 42, ParentID: 5}); err != nil {
		t.Fatal(err)
	}
	if s, _ := jnNum(jnParams(t, jnOne(t, f, "submit").op), "seed"); s != 42 {
		t.Errorf("seed %v, want 42", s)
	}
}

// Корень (Submit без родителя) не журналируется.
func TestJournaledSubmitRootNotRecorded(t *testing.T) {
	t.Parallel()
	f := newJnFake()
	id, err := Journaled(f).Submit(context.Background(), SubmitParams{Style: "s"})
	if err != nil || id != f.newID {
		t.Fatalf("Submit = %d, %v", id, err)
	}
	if len(f.recs) != 0 {
		t.Errorf("записи %+v, want нет (корень)", f.recs)
	}
}

// Генерации от трека: continue/fragment/overdub — одна запись, result job:<id>, сид фактический.
func TestJournaledGenerationsRecordActualSeed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		op string
		do func(Service) (int64, error)
	}{
		{"continue", func(s Service) (int64, error) {
			return s.ContinueJob(context.Background(), 5, 95.2, 0, "X:1", "louder")
		}},
		{"fragment", func(s Service) (int64, error) {
			return s.FragmentJob(context.Background(), 5, 10, 20, 0, "", "")
		}},
		{"overdub", func(s Service) (int64, error) {
			return s.SubmitOverdub(context.Background(), 5, "guitar", "", 0.8, "", 0)
		}},
	}
	for _, c := range cases {
		t.Run(c.op, func(t *testing.T) {
			t.Parallel()
			f := newJnFake()
			id, err := c.do(Journaled(f))
			if err != nil || id != f.newID {
				t.Fatalf("%s = %d, %v; want %d", c.op, id, err, f.newID)
			}
			r := jnOne(t, f, c.op)
			if r.jobID != 5 && r.jobID != f.newID {
				t.Errorf("запись у трека %d, want 5 или %d", r.jobID, f.newID)
			}
			if r.op.Result != fmt.Sprintf("job:%d", f.newID) {
				t.Errorf("result %q, want job:%d", r.op.Result, f.newID)
			}
			if s, _ := jnNum(jnParams(t, r.op), "seed"); int64(s) != f.newSeed {
				t.Errorf("seed %v, want фактический %d", s, f.newSeed)
			}
		})
	}
}

// variant_track: params file, title, voice_src; result job:<id>.
func TestJournaledVariantToTrack(t *testing.T) {
	t.Parallel()
	f := newJnFake()
	id, err := Journaled(f).VariantToTrack(context.Background(), 5, "dsp-fx-1.flac", "Тёплый · хор", 3)
	if err != nil || id != f.newID {
		t.Fatalf("VariantToTrack = %d, %v", id, err)
	}
	r := jnOne(t, f, "variant_track")
	p := jnParams(t, r.op)
	if p["file"] != "dsp-fx-1.flac" || p["title"] != "Тёплый · хор" {
		t.Errorf("params %v, want file и title вызова", p)
	}
	if v, _ := jnNum(p, "voice_src"); v != 3 {
		t.Errorf("voice_src %v, want 3", p["voice_src"])
	}
	if r.op.Result != fmt.Sprintf("job:%d", f.newID) {
		t.Errorf("result %q, want job:%d", r.op.Result, f.newID)
	}
}

// ApplyFx не-превью → fx у этого трека, result — файл варианта; превью — не пишет.
func TestJournaledApplyFx(t *testing.T) {
	t.Parallel()
	f := newJnFake()
	from := 1.5
	if _, err := Journaled(f).ApplyFx(context.Background(), 5, FxRequest{Source: "mix", From: &from,
		Chain: []map[string]any{{"type": "eq"}}}); err != nil {
		t.Fatal(err)
	}
	r := jnOne(t, f, "fx")
	if r.jobID != 5 || r.op.Result != f.fxFile {
		t.Errorf("запись %d/%q, want 5/%q", r.jobID, r.op.Result, f.fxFile)
	}
	p := jnParams(t, r.op)
	if p["source"] != "mix" {
		t.Errorf("params %v, want заявка fx (source mix)", p)
	}

	g := newJnFake()
	if _, err := Journaled(g).ApplyFx(context.Background(), 5, FxRequest{Source: "mix", Preview: true,
		From: &from, To: &from}); err != nil {
		t.Fatal(err)
	}
	if len(g.recs) != 0 {
		t.Errorf("превью записано: %+v", g.recs)
	}
}

// update / set_head — запись у трека вызова.
func TestJournaledUpdateSetHead(t *testing.T) {
	t.Parallel()
	cases := []struct {
		op string
		do func(Service) error
	}{
		{"update", func(s Service) error {
			_, err := s.UpdateJob(context.Background(), 5, strp("Новое имя"), nil)
			return err
		}},
		{"set_head", func(s Service) error { return s.SetHead(context.Background(), 5, 9) }},
	}
	for _, c := range cases {
		t.Run(c.op, func(t *testing.T) {
			t.Parallel()
			f := newJnFake()
			if err := c.do(Journaled(f)); err != nil {
				t.Fatal(err)
			}
			if r := jnOne(t, f, c.op); r.jobID != 5 {
				t.Errorf("запись у трека %d, want 5", r.jobID)
			}
		})
	}
}

// Ошибка вызова — записи нет, ошибка возвращена.
func TestJournaledErrorNotRecorded(t *testing.T) {
	t.Parallel()
	calls := map[string]func(Service) error{
		"submit": func(s Service) error {
			_, err := s.Submit(context.Background(), SubmitParams{ParentID: 5})
			return err
		},
		"continue": func(s Service) error { _, err := s.ContinueJob(context.Background(), 5, 1, 0, "", ""); return err },
		"fragment": func(s Service) error { _, err := s.FragmentJob(context.Background(), 5, 1, 2, 0, "", ""); return err },
		"overdub": func(s Service) error {
			_, err := s.SubmitOverdub(context.Background(), 5, "", "", 1, "", 0)
			return err
		},
		"variant_track": func(s Service) error {
			_, err := s.VariantToTrack(context.Background(), 5, "a.flac", "", 0)
			return err
		},
		"fx": func(s Service) error {
			_, err := s.ApplyFx(context.Background(), 5, FxRequest{Source: "mix"})
			return err
		},
		"update":   func(s Service) error { _, err := s.UpdateJob(context.Background(), 5, strp("x"), nil); return err },
		"set_head": func(s Service) error { return s.SetHead(context.Background(), 5, 9) },
	}
	for name, do := range calls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newJnFake()
			f.callErr = errors.New("воркер упал")
			if err := do(Journaled(f)); err == nil {
				t.Fatal("ошибка вызова потеряна")
			}
			if len(f.recs) != 0 {
				t.Errorf("записи при ошибке: %+v", f.recs)
			}
		})
	}
}

// Сбой записи журнала не роняет вызов: результат тот же.
func TestJournaledRecordFailureKeepsResult(t *testing.T) {
	t.Parallel()
	f := newJnFake()
	f.recErr = errors.New("журнал недоступен")
	svc := Journaled(f)
	id, err := svc.ContinueJob(context.Background(), 5, 1, 0, "", "")
	if err != nil || id != f.newID {
		t.Errorf("ContinueJob = %d, %v; want %d, nil при сбое журнала", id, err, f.newID)
	}
	v, err := svc.ApplyFx(context.Background(), 5, FxRequest{Source: "mix"})
	if err != nil || v == nil || v.File != f.fxFile {
		t.Errorf("ApplyFx = %+v, %v; want вариант при сбое журнала", v, err)
	}
	if err := svc.SetHead(context.Background(), 5, 9); err != nil {
		t.Errorf("SetHead: %v при сбое журнала", err)
	}
}

// Контекст «без журнала» (внутренние вызовы составной операции) — записей нет, вызовы идут.
func TestJournaledWithoutJournalContext(t *testing.T) {
	t.Parallel()
	f := newJnFake()
	svc := Journaled(f)
	ctx := WithoutJournal(context.Background())
	if _, err := svc.ApplyFx(ctx, 5, FxRequest{Source: "mix"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.VariantToTrack(ctx, 5, "a.flac", "t", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Submit(ctx, SubmitParams{ParentID: 5}); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 3 {
		t.Errorf("вызовы %v, want 3 прошли к сервису", f.calls)
	}
	if len(f.recs) != 0 {
		t.Errorf("записи в контексте без журнала: %+v", f.recs)
	}
}

// Прочие вызовы (стемы) проходят насквозь без записи.
func TestJournaledPassThroughNotRecorded(t *testing.T) {
	t.Parallel()
	f := newJnFake()
	if _, err := Journaled(f).MakeStems(context.Background(), 5); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 || f.calls[0] != "stems" {
		t.Errorf("вызовы %v, want [stems]", f.calls)
	}
	if len(f.recs) != 0 {
		t.Errorf("стемы записаны: %+v", f.recs)
	}
}

// yue.Record: пишет через OpRecorder; в т.ч. через обёртку Journaled (её получают App/MCP);
// контекст «без журнала» — не пишет; сервис без RecordOp и сбой записи — без паники и ошибки.
func TestRecordHelper(t *testing.T) {
	t.Parallel()
	op := JobOp{Op: "splice", Params: json.RawMessage(`{"parts":[]}`), Result: "dsp-splice-1.flac", Refs: []int64{11, 12}}

	f := newJnFake()
	Record(context.Background(), f, 5, op)
	r := jnOne(t, f, "splice")
	if r.jobID != 5 || r.op.Result != op.Result || len(r.op.Refs) != 2 || r.op.Refs[0] != 11 || r.op.Refs[1] != 12 {
		t.Errorf("запись %+v, want %+v у 5", r, op)
	}

	g := newJnFake()
	Record(context.Background(), Journaled(g), 5, op)
	jnOne(t, g, "splice")

	h := newJnFake()
	Record(WithoutJournal(context.Background()), h, 5, op)
	Record(WithoutJournal(context.Background()), Journaled(h), 5, op)
	if len(h.recs) != 0 {
		t.Errorf("записи в контексте без журнала: %+v", h.recs)
	}

	type plain struct{ Service }
	Record(context.Background(), plain{}, 5, op) // не OpRecorder — тихо ничего

	e := newJnFake()
	e.recErr = errors.New("сбой")
	Record(context.Background(), e, 5, op) // сбой — в лог, без паники
}

// Клиент: RecordOp и сессии импорта.

type jnReq struct {
	method, path string
	body         map[string]any
}

func jnServer(t *testing.T, code int, resp string, got *jnReq) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path = r.Method, r.URL.Path
		data, _ := io.ReadAll(r.Body)
		if len(data) > 0 {
			_ = json.Unmarshal(data, &got.body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = io.WriteString(w, resp)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL)
}

func TestClientRecordOpPostsJournalEntry(t *testing.T) {
	t.Parallel()
	var got jnReq
	c := jnServer(t, 200, `{"id":1}`, &got)
	err := c.RecordOp(context.Background(), 42, JobOp{Op: "fx", Params: json.RawMessage(`{"source":"mix"}`),
		Result: "dsp-fx-1.flac", Refs: []int64{7}})
	if err != nil {
		t.Fatalf("RecordOp: %v", err)
	}
	if got.method != http.MethodPost || got.path != "/jobs/42/ops" {
		t.Fatalf("запрос %s %s, want POST /jobs/42/ops", got.method, got.path)
	}
	if got.body["op"] != "fx" || got.body["result"] != "dsp-fx-1.flac" {
		t.Errorf("тело %v", got.body)
	}
	if p, ok := got.body["params"].(map[string]any); !ok || p["source"] != "mix" {
		t.Errorf("params %v, want объект", got.body["params"])
	}
	if refs, ok := got.body["refs"].([]any); !ok || len(refs) != 1 || refs[0] != float64(7) {
		t.Errorf("refs %v, want [7]", got.body["refs"])
	}
}

func TestClientRecordOpError(t *testing.T) {
	t.Parallel()
	var got jnReq
	c := jnServer(t, 422, `{"detail":"unknown op"}`, &got)
	if err := c.RecordOp(context.Background(), 42, JobOp{Op: "zzz", Params: json.RawMessage(`{}`)}); err == nil {
		t.Error("422 без ошибки")
	}
}

func TestClientClaimRecipeImport(t *testing.T) {
	t.Parallel()
	var got jnReq
	c := jnServer(t, 200, `{"id":3}`, &got)
	if err := c.ClaimRecipeImport(context.Background(), 3, "app"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if got.method != http.MethodPost || got.path != "/recipes/imports/3/claim" || got.body["owner"] != "app" {
		t.Errorf("запрос %s %s %v", got.method, got.path, got.body)
	}

	c409 := jnServer(t, 409, `{"detail":"claimed"}`, &got)
	if err := c409.ClaimRecipeImport(context.Background(), 3, "app"); !errors.Is(err, ErrClaimed) {
		t.Errorf("409 → %v, want ErrClaimed", err)
	}
	c500 := jnServer(t, 500, `{"detail":"boom"}`, &got)
	if err := c500.ClaimRecipeImport(context.Background(), 3, "app"); err == nil || errors.Is(err, ErrClaimed) {
		t.Errorf("500 → %v, want ошибка, не ErrClaimed", err)
	}
}

func TestClientImportRecipeAndRecipe(t *testing.T) {
	t.Parallel()
	var got jnReq
	c := jnServer(t, 200, `{"id":9}`, &got)
	id, err := c.ImportRecipe(context.Background(), json.RawMessage(`{"format":"yue-track-recipe","version":1}`), "plan")
	if err != nil || id != 9 {
		t.Fatalf("ImportRecipe = %d, %v", id, err)
	}
	if got.method != http.MethodPost || got.path != "/recipes/import" || got.body["mode"] != "plan" {
		t.Errorf("запрос %s %s %v", got.method, got.path, got.body)
	}
	if r, ok := got.body["recipe"].(map[string]any); !ok || r["format"] != "yue-track-recipe" {
		t.Errorf("recipe %v, want объект рецепта", got.body["recipe"])
	}

	c2 := jnServer(t, 200, `{"format":"yue-track-recipe","version":1}`, &got)
	raw, err := c2.Recipe(context.Background(), 5)
	if err != nil || got.method != http.MethodGet || got.path != "/jobs/5/recipe" {
		t.Fatalf("Recipe: %v; %s %s", err, got.method, got.path)
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil || m["format"] != "yue-track-recipe" {
		t.Errorf("ответ %s", raw)
	}

	c3 := jnServer(t, 422, `{"detail":"корень — черновик"}`, &got)
	if _, err := c3.Recipe(context.Background(), 5); err == nil {
		t.Error("422 экспорта без ошибки")
	}
}
