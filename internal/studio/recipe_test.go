package studio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"partscaster/internal/yue"
)

// Ведущий импорта рецепта
// studio.RecipeRunner{Svc, Owner}.Tick(ctx) — один шаг за тик:
//   первая сессия RecipeImports() со status running → ClaimRecipeImport(id, Owner)
//   (ErrClaimed — ничего не делает) → RecipeImport(id) (recipe + state) → шаг →
//   SaveRecipeImport(id, {state, progress, status, error, root_job_id}).
// Порядок: корень (Submit: seed-режим — тот же сид, plan-режим — Abc = plan_abc), затем ops
// рецепта строго по порядку; генерация ждёт готовности джобы (done → дальше, error → сессия
// error); правки — с id треков и именами файлов, заменёнными по соответствию; сбой шага →
// status error + текст, повтор после ручного status running — с того же шага (готовые не
// переделываются). В конце SetHead(корень, узел head), папка трека, status done, root_job_id.
//
// Рецепт в тестах строится из НАСТОЯЩИХ записей журнала: исходные вызовы идут через
// yue.Journaled / составные операции studio (их params — ровно то, что попадёт в рецепт), а
// замена id на n-id делается по договору экспорта воркера (rcBuild). Так тест не знает формата
// params отдельных op и проверяет круг «журнал → рецепт → воспроизведение».
//
// Воркер «новой установки» — rcNew: сессии импорта и генерации — внешняя граница; звук,
// загрузки, стемы — psFake/secFake.

// rcOrig — исходный воркер: создающие вызовы возвращают ids по очереди, ApplyFx — fxFiles по очереди.
type rcOrig struct {
	*secFake
	ids     []int64
	fxFiles []string
	seeds   map[int64]int64 // фактический сид джобы (список Jobs)
}

func newRcOrig(f *secFake, ids []int64, fx []string) *rcOrig {
	if f == nil {
		f = newSecFake()
	}
	return &rcOrig{secFake: f, ids: ids, fxFiles: fx, seeds: map[int64]int64{}}
}

func (o *rcOrig) take() int64 {
	id := o.ids[0]
	o.ids = o.ids[1:]
	return id
}

func (o *rcOrig) Jobs(context.Context) ([]yue.Job, error) {
	var out []yue.Job
	for id, s := range o.seeds {
		out = append(out, yue.Job{ID: id, Status: "done", Seed: s})
	}
	return out, nil
}

func (o *rcOrig) ContinueJob(_ context.Context, _ int64, _ float64, seed int64, _, _ string) (int64, error) {
	id := o.take()
	if seed == 0 {
		seed = 5151 // сид, выбранный воркером
	}
	o.seeds[id] = seed
	return id, nil
}

func (o *rcOrig) ApplyFx(_ context.Context, _ int64, _ yue.FxRequest) (*yue.DspVariant, error) {
	f := o.fxFiles[0]
	o.fxFiles = o.fxFiles[1:]
	return &yue.DspVariant{File: f}, nil
}

func (o *rcOrig) SetHead(context.Context, int64, int64) error { return nil }

func (o *rcOrig) VariantToTrack(_ context.Context, _ int64, _, _ string, _ int64) (int64, error) {
	id := o.take()
	o.seeds[id] = 0
	return id, nil
}

// rcRoot — корень рецепта (поля договора root).
type rcRoot struct {
	Title       string  `json:"title"`
	Style       string  `json:"style"`
	Lyrics      string  `json:"lyrics"`
	Seed        int64   `json:"seed"`
	Cot         string  `json:"cot"`
	Arc         string  `json:"arc"`
	MaxTokens   int64   `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
	Cfg         float64 `json:"cfg"`
	Abc         *string `json:"abc"`
	PlanAbc     *string `json:"plan_abc"`
}

func rcStr(s string) *string { return &s }

func rcDefaultRoot() rcRoot {
	return rcRoot{Title: "Тёплый провод", Style: "post-punk, baritone", Lyrics: "[verse]\nla la", Seed: 4242,
		Cot: "full", Arc: "build", MaxTokens: 3000, Temperature: 1.1, Cfg: 1.5, PlanAbc: rcStr("X:plan")}
}

// rcIDKeys — ключи params, чьи значения-id треков экспорт заменяет на n-id.
var rcIDKeys = map[string]bool{"parent_id": true, "child_id": true, "job_id": true, "voice_src": true, "head_id": true}

func rcSubst(v any, nodes map[int64]string) any {
	switch x := v.(type) {
	case map[string]any:
		for k, vv := range x {
			if f, ok := vv.(float64); ok && rcIDKeys[k] {
				if n, ok := nodes[int64(f)]; ok && f != 0 {
					x[k] = n
					continue
				}
			}
			x[k] = rcSubst(vv, nodes)
		}
	case []any:
		for i := range x {
			x[i] = rcSubst(x[i], nodes)
		}
	}
	return v
}

// rcBuild — рецепт по договору экспорта: n1 — корень rootID, далее n2… по порядку появления
// result "job:<id>" в журнале; node — n-id трека записи; result/refs/ключи-id — на n-id.
func rcBuild(t *testing.T, rootID int64, root rcRoot, recs []jrEntry, folder string, headID int64) json.RawMessage {
	t.Helper()
	nodes := map[int64]string{rootID: "n1"}
	for _, e := range recs {
		if s, ok := strings.CutPrefix(e.op.Result, "job:"); ok {
			id, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				t.Fatalf("result %q", e.op.Result)
			}
			if _, seen := nodes[id]; !seen {
				nodes[id] = fmt.Sprintf("n%d", len(nodes)+1)
			}
		}
	}
	var ops []map[string]any
	for _, e := range recs {
		node, ok := nodes[e.jobID]
		if !ok {
			t.Fatalf("запись %s у трека %d вне рецепта (узлы %v)", e.op.Op, e.jobID, nodes)
		}
		var params any
		if err := json.Unmarshal(e.op.Params, &params); err != nil {
			t.Fatalf("params %s: %v", e.op.Params, err)
		}
		result := e.op.Result
		if s, ok := strings.CutPrefix(result, "job:"); ok {
			id, _ := strconv.ParseInt(s, 10, 64)
			result = "job:" + nodes[id]
		}
		refs := []any{}
		for _, r := range e.op.Refs {
			if n, ok := nodes[r]; ok {
				refs = append(refs, n)
			} else {
				refs = append(refs, r)
			}
		}
		ops = append(ops, map[string]any{"op": e.op.Op, "node": node, "params": rcSubst(params, nodes),
			"result": result, "refs": refs})
	}
	rootMap := map[string]any{}
	b, _ := json.Marshal(root)
	_ = json.Unmarshal(b, &rootMap)
	rootMap["id"] = "n1"
	head, ok := nodes[headID]
	if !ok {
		t.Fatalf("head %d вне рецепта", headID)
	}
	out, err := json.Marshal(map[string]any{
		"format": "yue-track-recipe", "version": 1, "exported_at": "2026-10-11T12:00:00+03:00",
		"worker_model": "yue2-test", "stems_model": "htdemucs",
		"track": map[string]any{"title": root.Title, "folder": folder, "head": head},
		"root":  rootMap, "ops": ops, "warnings": []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// rcMainRecipe — корень 10 → continue (11, сид воркера 5151) → fx на 11 (dsp-fx-a.flac)
// → variant_track(11, dsp-fx-a.flac) = 12; head 12, папка «Альбом».
func rcMainRecipe(t *testing.T, root rcRoot) json.RawMessage {
	t.Helper()
	orig := newRcOrig(nil, []int64{11, 12}, []string{"dsp-fx-a.flac"})
	rec := &jrRec{Service: orig}
	j := yue.Journaled(rec)
	ctx := context.Background()
	if _, err := j.ContinueJob(ctx, 10, 95.2, 0, "X:cont", "louder"); err != nil {
		t.Fatal(err)
	}
	from := 30.0
	if _, err := j.ApplyFx(ctx, 11, rcFxReq(&from, "")); err != nil {
		t.Fatal(err)
	}
	if _, err := j.VariantToTrack(ctx, 11, "dsp-fx-a.flac", "Тёплый провод · вариант", 0); err != nil {
		t.Fatal(err)
	}
	if len(rec.recs) != 3 {
		t.Fatalf("журнал исходной установки: %d записей, want 3 (continue, fx, variant_track) — см. тесты Journaled", len(rec.recs))
	}
	return rcBuild(t, 10, root, rec.recs, "Альбом", 12)
}

func rcFxReq(from *float64, file string) yue.FxRequest {
	return yue.FxRequest{Source: "mix", From: from, File: file, Label: "тёплый",
		Chain: []map[string]any{{"type": "eq", "low": 2.0}, {"type": "comp", "ratio": 3.0}}}
}

type rcCont struct {
	jobID         int64
	from          float64
	seed          int64
	abc, styleAdd string
}

type rcFx struct {
	jobID int64
	req   yue.FxRequest
}

type rcVar struct {
	jobID       int64
	file, title string
	voiceSrc    int64
}

type rcNew struct {
	*psFake
	t        *testing.T
	sess     map[string]any // поля сессии без recipe/state
	recipe   json.RawMessage
	state    json.RawMessage
	claimErr error
	mu       sync.Mutex // захват продлевается из другой горутины во время шага
	owners   []string
	saves    int
	saveErr  error                        // SaveRecipeImport отвечает ошибкой (ErrClaimed — сессию забрал другой)
	updates  []map[string]json.RawMessage // присланные поля SaveRecipeImport по порядку
	fxDelay  time.Duration                // ApplyFx держит шаг
	fxClaims int                          // захватов, пришедших во время ApplyFx
	inFx     bool
	stealFx  bool // во время ApplyFx сессию забирает другой: Save и Claim → ErrClaimed
	// failSaveAfterFx — первое сохранение после ApplyFx отвечает этой ошибкой (один раз)
	failSaveAfterFx error
	saveErrOnce     error
	failedSaves     int
	// afterFailState — после неудачного сохранения на воркере оказывается это состояние (вёл другой)
	afterFailState json.RawMessage
	// thenSaveErr — ошибка следующего сохранения после неудачного (досохранение)
	thenSaveErr error
	progress    []string // все присланные progress по порядку

	nextID    int64
	jobs      []yue.Job
	newStatus string // статус новых джоб в списке ("" — done)

	calls    []string
	submits  []yue.SubmitParams
	conts    []rcCont
	fxs      []rcFx
	fxErr    error // ApplyFx падает (один раз)
	variants []rcVar
	overdubs []rcOverdub
	heads    [][2]int64
	folders  map[int64]string
}

func newRcNew(t *testing.T, pf *psFake, recipe json.RawMessage, mode string) *rcNew {
	if pf == nil {
		pf = &psFake{engFake: newEngFake(t, newSecFake())}
	}
	return &rcNew{psFake: pf, t: t, recipe: recipe, nextID: 100, folders: map[int64]string{},
		sess: map[string]any{"id": 1, "title": "Тёплый провод", "status": "running", "progress": "",
			"error": "", "mode": mode, "created_at": "2026-10-11T12:00:00"}}
}

func (f *rcNew) status() string { s, _ := f.sess["status"].(string); return s }

func (f *rcNew) decode(withBody bool) *yue.RecipeImport {
	m := map[string]any{}
	for k, v := range f.sess {
		m[k] = v
	}
	if withBody {
		m["recipe"] = f.recipe
		if len(f.state) > 0 {
			m["state"] = f.state
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		f.t.Fatal(err)
	}
	var ri yue.RecipeImport
	if err := json.Unmarshal(b, &ri); err != nil {
		f.t.Fatalf("сессия %s не читается в yue.RecipeImport (поля договора GET /recipes/imports/{id}): %v", b, err)
	}
	return &ri
}

func (f *rcNew) RecipeImports(context.Context) ([]yue.RecipeImport, error) {
	return []yue.RecipeImport{*f.decode(false)}, nil
}

func (f *rcNew) RecipeImport(_ context.Context, id int64) (*yue.RecipeImport, error) {
	if id != 1 {
		return nil, &yue.StatusError{Code: 404, Msg: "нет сессии"}
	}
	return f.decode(true), nil
}

func (f *rcNew) ClaimRecipeImport(_ context.Context, _ int64, owner string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.owners = append(f.owners, owner)
	if f.inFx {
		f.fxClaims++
	}
	return f.claimErr
}

// SaveRecipeImport — как PATCH воркера: присланные поля меняются, отсутствующие/пустые — нет.
func (f *rcNew) SaveRecipeImport(_ context.Context, id int64, u yue.RecipeImportUpdate) (*yue.RecipeImport, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saves++
	b, err := json.Marshal(u)
	if err != nil {
		f.t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		f.t.Fatalf("RecipeImportUpdate %s: %v", b, err)
	}
	f.updates = append(f.updates, m)
	if f.saveErrOnce != nil {
		err := f.saveErrOnce
		f.saveErrOnce, f.thenSaveErr = f.thenSaveErr, nil
		if f.failedSaves == 0 {
			f.failedSaves = len(f.updates)
		}
		if f.afterFailState != nil {
			f.state, f.afterFailState = f.afterFailState, nil
		}
		return nil, err
	}
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	if v, ok := m["state"]; ok && string(v) != "null" && string(v) != `""` {
		f.state = append(json.RawMessage(nil), v...)
	}
	for _, k := range []string{"status", "progress", "error"} {
		if v, ok := m[k]; ok && string(v) != "null" {
			var s string
			if err := json.Unmarshal(v, &s); err != nil {
				f.t.Fatalf("%s = %s", k, v)
			}
			if k != "status" || s != "" {
				f.sess[k] = s
			}
			if k == "progress" && s != "" {
				f.progress = append(f.progress, s)
			}
		}
	}
	if v, ok := m["root_job_id"]; ok && string(v) != "null" {
		var n int64
		if err := json.Unmarshal(v, &n); err == nil && n != 0 {
			f.sess["root_job_id"] = n
		}
	}
	return f.decode(true), nil
}

func (f *rcNew) create(title string) int64 {
	id := f.nextID
	f.nextID++
	st := f.newStatus
	if st == "" {
		st = "done"
	}
	f.jobs = append(f.jobs, yue.Job{ID: id, Title: title, Status: st, AudioFile: "audio.flac", DurationSec: psDur})
	return id
}

func (f *rcNew) setStatus(id int64, st string) {
	for i := range f.jobs {
		if f.jobs[i].ID == id {
			f.jobs[i].Status = st
		}
	}
}

func (f *rcNew) Jobs(context.Context) ([]yue.Job, error) {
	return append([]yue.Job(nil), f.jobs...), nil
}

func (f *rcNew) Submit(_ context.Context, p yue.SubmitParams) (int64, error) {
	f.calls = append(f.calls, "submit")
	f.submits = append(f.submits, p)
	return f.create(p.Title), nil
}

func (f *rcNew) ContinueJob(_ context.Context, id int64, from float64, seed int64, abc, styleAdd string) (int64, error) {
	f.calls = append(f.calls, "continue")
	f.conts = append(f.conts, rcCont{id, from, seed, abc, styleAdd})
	return f.create("продолжение"), nil
}

func (f *rcNew) ApplyFx(_ context.Context, id int64, req yue.FxRequest) (*yue.DspVariant, error) {
	f.calls = append(f.calls, "fx")
	f.fxs = append(f.fxs, rcFx{id, req})
	f.mu.Lock()
	f.inFx = true
	f.mu.Unlock()
	time.Sleep(f.fxDelay)
	f.mu.Lock()
	f.inFx = false
	if f.stealFx {
		f.saveErr, f.claimErr = yue.ErrClaimed, yue.ErrClaimed
	}
	if f.failSaveAfterFx != nil {
		f.saveErrOnce, f.failSaveAfterFx = f.failSaveAfterFx, nil
	}
	f.mu.Unlock()
	if f.fxErr != nil {
		err := f.fxErr
		f.fxErr = nil
		return nil, err
	}
	return &yue.DspVariant{File: fmt.Sprintf("dsp-fx-new%d.flac", len(f.fxs))}, nil
}

type rcOverdub struct {
	jobID         int64
	style, lyrics string
	gain          float64
	abc           string
	seed          int64
}

func (f *rcNew) SubmitOverdub(_ context.Context, id int64, style, lyrics string, gain float64, abc string, seed int64) (int64, error) {
	f.calls = append(f.calls, "overdub")
	f.overdubs = append(f.overdubs, rcOverdub{id, style, lyrics, gain, abc, seed})
	return f.create(style), nil
}

func (f *rcNew) VariantToTrack(_ context.Context, id int64, file, title string, voiceSrc int64) (int64, error) {
	f.calls = append(f.calls, "variant_track")
	f.variants = append(f.variants, rcVar{id, file, title, voiceSrc})
	return f.create(title), nil
}

func (f *rcNew) SetHead(_ context.Context, id, head int64) error {
	f.calls = append(f.calls, "set_head")
	f.heads = append(f.heads, [2]int64{id, head})
	return nil
}

func (f *rcNew) UpdateJob(_ context.Context, id int64, title, folder *string) (*yue.Job, error) {
	f.calls = append(f.calls, "update")
	if folder != nil {
		f.folders[id] = *folder
	}
	return &yue.Job{ID: id}, nil
}

func (f *rcNew) count(name string) int {
	n := 0
	for _, c := range f.calls {
		if c == name {
			n++
		}
	}
	return n
}

// rcTicks — тики, пока сессия running (не больше max).
func rcTicks(t *testing.T, f *rcNew, max int) *RecipeRunner {
	t.Helper()
	r := &RecipeRunner{Svc: f, Owner: "app"}
	for i := 0; i < max && f.status() == "running"; i++ {
		r.Tick(withoutLiveLayer(context.Background())) // механика импорта; слой «живой звук» — своими тестами
	}
	return r
}

func rcRootID(f *rcNew) int64 {
	switch v := f.sess["root_job_id"].(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	}
	return 0
}

// seed-режим — Submit (тот же сид и поля корня), ContinueJob(новый корень, тот же from,
// сид из журнала, abc, style_add), ApplyFx на новом продолжении, VariantToTrack по НОВОМУ имени
// файла; в конце SetHead(корень, новый head), папка «Альбом», status done, root_job_id.
func TestRecipeRunnerSeedModeReplaysInOrder(t *testing.T) {
	t.Parallel()
	root := rcDefaultRoot()
	f := newRcNew(t, nil, rcMainRecipe(t, root), "seed")
	rcTicks(t, f, 40)

	if f.status() != "done" {
		t.Fatalf("сессия %v (error %q), want done; вызовы %v", f.sess["status"], f.sess["error"], f.calls)
	}
	var gen []string
	for _, c := range f.calls {
		if c != "set_head" && c != "update" {
			gen = append(gen, c)
		}
	}
	if want := []string{"submit", "continue", "fx", "variant_track"}; !reflect.DeepEqual(gen, want) {
		t.Fatalf("вызовы %v, want %v по порядку", gen, want)
	}
	s := f.submits[0]
	if s.Seed != root.Seed || s.Style != root.Style || s.Lyrics != root.Lyrics || s.Cot != root.Cot ||
		s.Arc != root.Arc || s.MaxTokens != root.MaxTokens || s.Temperature != root.Temperature || s.Cfg != root.Cfg {
		t.Errorf("Submit %+v, want поля корня %+v (тот же сид)", s, root)
	}
	if s.ParentID != 0 || s.Draft {
		t.Errorf("Submit корня: parent %d draft %v, want 0/false", s.ParentID, s.Draft)
	}
	if s.Abc != "" {
		t.Errorf("seed-режим: Abc %q, want пусто (в исходной заявке abc не было)", s.Abc)
	}
	if want := (rcCont{100, 95.2, 5151, "X:cont", "louder"}); f.conts[0] != want {
		t.Errorf("ContinueJob %+v, want %+v (новый корень, фактический сид из журнала)", f.conts[0], want)
	}
	fx := f.fxs[0]
	from := 30.0
	if want := rcFxReq(&from, ""); fx.jobID != 101 || !reflect.DeepEqual(fx.req, want) {
		t.Errorf("ApplyFx(%d, %+v), want (101, %+v)", fx.jobID, fx.req, want)
	}
	if want := (rcVar{101, "dsp-fx-new1.flac", "Тёплый провод · вариант", 0}); f.variants[0] != want {
		t.Errorf("VariantToTrack %+v, want %+v (новое имя файла)", f.variants[0], want)
	}
	if len(f.heads) != 1 || f.heads[0] != [2]int64{100, 102} {
		t.Errorf("SetHead %v, want [(100, 102)]", f.heads)
	}
	if f.folders[100] != "Альбом" {
		t.Errorf("папка корня %q, want «Альбом»", f.folders[100])
	}
	if rcRootID(f) != 100 {
		t.Errorf("root_job_id %v, want 100", f.sess["root_job_id"])
	}
	if len(f.owners) == 0 || f.owners[0] != "app" {
		t.Errorf("захват владельцем %v, want app", f.owners)
	}
}

// seed-режим с abc в исходной заявке — корень с тем же abc.
func TestRecipeRunnerSeedModeKeepsRequestAbc(t *testing.T) {
	t.Parallel()
	root := rcDefaultRoot()
	root.Abc = rcStr("X:request")
	f := newRcNew(t, nil, rcMainRecipe(t, root), "seed")
	rcTicks(t, f, 40)
	if len(f.submits) != 1 || f.submits[0].Abc != "X:request" || f.submits[0].Seed != root.Seed {
		t.Errorf("Submit %+v, want Abc X:request, сид %d", f.submits, root.Seed)
	}
}

// plan-режим — корень рендерится по plan_abc.
func TestRecipeRunnerPlanModeRendersPlanAbc(t *testing.T) {
	t.Parallel()
	root := rcDefaultRoot()
	f := newRcNew(t, nil, rcMainRecipe(t, root), "plan")
	rcTicks(t, f, 40)
	if len(f.submits) != 1 || f.submits[0].Abc != "X:plan" {
		t.Fatalf("Submit %+v, want Abc = plan_abc X:plan", f.submits)
	}
	if f.submits[0].Style != root.Style || f.submits[0].Lyrics != root.Lyrics {
		t.Errorf("Submit %+v, want стиль и текст корня", f.submits[0])
	}
	if f.status() != "done" {
		t.Errorf("сессия %v, want done", f.sess["status"])
	}
}

// Один шаг за тик и ожидание генерации: пока корень не готов — дальше не идёт.
func TestRecipeRunnerWaitsForGeneration(t *testing.T) {
	t.Parallel()
	f := newRcNew(t, nil, rcMainRecipe(t, rcDefaultRoot()), "seed")
	f.newStatus = "running"
	rcTicks(t, f, 5)
	if (&RecipeRunner{Svc: f, Owner: "app"}).Tick(context.Background()) {
		t.Error("Tick = true во время ожидания генерации, want false")
	}
	if f.count("submit") != 1 || f.count("continue") != 0 {
		t.Fatalf("вызовы %v, want только submit, пока корень не готов", f.calls)
	}
	if f.status() != "running" {
		t.Fatalf("сессия %v, want running во время ожидания", f.sess["status"])
	}
	f.newStatus = ""
	f.setStatus(100, "done")
	rcTicks(t, f, 40)
	if f.status() != "done" || f.count("submit") != 1 || f.count("continue") != 1 {
		t.Errorf("после готовности: сессия %v, вызовы %v", f.sess["status"], f.calls)
	}
}

// Генерация упала → сессия error с текстом, дальше не идёт.
func TestRecipeRunnerGenerationErrorStops(t *testing.T) {
	t.Parallel()
	f := newRcNew(t, nil, rcMainRecipe(t, rcDefaultRoot()), "seed")
	f.newStatus = "error"
	rcTicks(t, f, 10)
	if f.status() != "error" {
		t.Fatalf("сессия %v, want error (корень упал)", f.sess["status"])
	}
	if e, _ := f.sess["error"].(string); e == "" {
		t.Error("текст ошибки пуст")
	}
	if f.count("continue") != 0 || f.count("set_head") != 0 {
		t.Errorf("вызовы после сбоя: %v", f.calls)
	}
}

// Сбой на 3-м узле (fx) → error; повтор после ручного running продолжает с 3-го:
// корень и продолжение не перевызываются.
func TestRecipeRunnerResumesAfterFailure(t *testing.T) {
	t.Parallel()
	f := newRcNew(t, nil, rcMainRecipe(t, rcDefaultRoot()), "seed")
	f.fxErr = errors.New("движок выключен")
	rcTicks(t, f, 20)
	if f.status() != "error" {
		t.Fatalf("сессия %v, want error после сбоя fx; вызовы %v", f.sess["status"], f.calls)
	}
	if e, _ := f.sess["error"].(string); e == "" {
		t.Error("текст ошибки пуст")
	}
	if f.count("variant_track") != 0 || f.count("set_head") != 0 {
		t.Errorf("после сбоя шли дальше: %v", f.calls)
	}

	f.sess["status"] = "running" // человек перезапустил сессию
	rcTicks(t, f, 20)
	if f.status() != "done" {
		t.Fatalf("после повтора сессия %v (%v), want done", f.sess["status"], f.sess["error"])
	}
	if f.count("submit") != 1 || f.count("continue") != 1 {
		t.Errorf("submit %d, continue %d, want по 1 (готовые не переделываются): %v",
			f.count("submit"), f.count("continue"), f.calls)
	}
	if f.count("fx") != 2 || f.fxs[1].jobID != 101 {
		t.Errorf("fx %d раз, второй у %d; want повтор fx у 101", f.count("fx"), f.fxs[len(f.fxs)-1].jobID)
	}
	if len(f.variants) != 1 || f.variants[0].file != "dsp-fx-new2.flac" {
		t.Errorf("VariantToTrack %+v, want файл повторного fx dsp-fx-new2.flac", f.variants)
	}
}

// Сессию ведёт другой (ErrClaimed) — ничего не делается и не сохраняется.
func TestRecipeRunnerClaimedDoesNothing(t *testing.T) {
	t.Parallel()
	f := newRcNew(t, nil, rcMainRecipe(t, rcDefaultRoot()), "seed")
	f.claimErr = yue.ErrClaimed
	if (&RecipeRunner{Svc: f, Owner: "app"}).Tick(context.Background()) {
		t.Error("Tick = true, want false: сессию держит другой")
	}
	rcTicks(t, f, 3)
	if len(f.calls) != 0 || f.saves != 0 {
		t.Errorf("вызовы %v, сохранений %d, want ничего (ведёт другой)", f.calls, f.saves)
	}
	if f.status() != "running" {
		t.Errorf("статус %v, want running", f.sess["status"])
	}
}

// Нет сессий running (done/error) — ничего не делается, захвата нет.
func TestRecipeRunnerIgnoresFinishedSessions(t *testing.T) {
	t.Parallel()
	for _, st := range []string{"done", "error"} {
		f := newRcNew(t, nil, rcMainRecipe(t, rcDefaultRoot()), "seed")
		f.sess["status"] = st
		r := &RecipeRunner{Svc: f, Owner: "app"}
		if r.Tick(context.Background()) {
			t.Errorf("%s: Tick = true, want false (нечего делать)", st)
		}
		if len(f.calls) != 0 || f.saves != 0 || len(f.owners) != 0 {
			t.Errorf("%s: вызовы %v, сохранений %d, захваты %v; want ничего", st, f.calls, f.saves, f.owners)
		}
	}
}

// Цепочка правок: второй fx берёт вариант первого (File) — имя заменено на новое;
// variant_track — по новому имени второго.
func TestRecipeRunnerRemapsFilesBetweenEdits(t *testing.T) {
	t.Parallel()
	orig := newRcOrig(nil, []int64{11, 12}, []string{"dsp-fx-a.flac", "dsp-fx-b.flac"})
	rec := &jrRec{Service: orig}
	j := yue.Journaled(rec)
	ctx := context.Background()
	if _, err := j.ContinueJob(ctx, 10, 60, 77, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := j.ApplyFx(ctx, 11, rcFxReq(nil, "")); err != nil {
		t.Fatal(err)
	}
	if _, err := j.ApplyFx(ctx, 11, rcFxReq(nil, "dsp-fx-a.flac")); err != nil {
		t.Fatal(err)
	}
	if _, err := j.VariantToTrack(ctx, 11, "dsp-fx-b.flac", "v", 0); err != nil {
		t.Fatal(err)
	}
	f := newRcNew(t, nil, rcBuild(t, 10, rcDefaultRoot(), rec.recs, "", 12), "seed")
	rcTicks(t, f, 40)

	if f.status() != "done" {
		t.Fatalf("сессия %v (%v), want done", f.sess["status"], f.sess["error"])
	}
	if len(f.fxs) != 2 || f.fxs[1].req.File != "dsp-fx-new1.flac" || f.fxs[1].jobID != 101 {
		t.Errorf("второй ApplyFx %+v, want File dsp-fx-new1.flac у 101", f.fxs)
	}
	if len(f.variants) != 1 || f.variants[0].file != "dsp-fx-new2.flac" {
		t.Errorf("VariantToTrack %+v, want dsp-fx-new2.flac", f.variants)
	}
	if f.conts[0].seed != 77 {
		t.Errorf("сид продолжения %d, want 77 (заданный в исходном вызове)", f.conts[0].seed)
	}
	if len(f.folders) != 0 {
		t.Errorf("папка выставлена %v, want нет (в рецепте пусто)", f.folders)
	}
}

// splice-правка — job_id кусков переназначены на новые треки; склейка у нового корня.
func TestRecipeRunnerSpliceRemapsJobIDs(t *testing.T) {
	t.Parallel()
	src := spliceSetup(t) // звук у 11 (spliceA) и 12 (spliceB)
	orig := newRcOrig(src, []int64{spliceB}, nil)
	rec := &jrRec{Service: orig}
	j := yue.Journaled(rec)
	ctx := context.Background()
	if _, err := j.ContinueJob(ctx, spliceA, 3, 9, "", ""); err != nil {
		t.Fatal(err)
	}
	parts := []SplicePart{{JobID: spliceA, From: 0, To: 3}, {JobID: spliceB, From: 3, To: 6}}
	if _, err := Splice(ctx, j, spliceA, parts, 0.05); err != nil {
		t.Fatalf("исходная склейка: %v", err)
	}
	if len(rec.recs) != 2 {
		t.Fatalf("журнал исходной установки %d записей, want 2 (continue, splice)", len(rec.recs))
	}

	nf := newSecFake()
	nf.files[key(100, "audio.flac")] = src.files[key(spliceA, "audio.flac")]
	nf.files[key(101, "audio.flac")] = src.files[key(spliceB, "audio.flac")]
	f := newRcNew(t, &psFake{engFake: newEngFake(t, nf)}, rcBuild(t, spliceA, rcDefaultRoot(), rec.recs, "", spliceA), "seed")
	rcTicks(t, f, 40)

	if f.status() != "done" {
		t.Fatalf("сессия %v (%v), want done", f.sess["status"], f.sess["error"])
	}
	for _, k := range []string{key(100, "audio.flac"), key(101, "audio.flac")} {
		if !contains(nf.fetched, k) {
			t.Errorf("кусок %s не скачан: %v", k, nf.fetched)
		}
	}
	for _, k := range nf.fetched {
		if strings.HasPrefix(k, fmt.Sprintf("%d/", spliceA)) || strings.HasPrefix(k, fmt.Sprintf("%d/", spliceB)) {
			t.Errorf("скачан старый трек %s", k)
		}
	}
	if len(nf.uploadedTo) != 1 || nf.uploadedTo[0] != 100 {
		t.Errorf("склейка загружена к %v, want [100] (новый корень)", nf.uploadedTo)
	}
	// head = корень: SetHead не обязателен (корень и так основная версия), но если зван — на сам корень
	for _, h := range f.heads {
		if h[0] != 100 || (h[1] != 100 && h[1] != 0) {
			t.Errorf("SetHead %v, want корень 100 (head = корень)", h)
		}
	}
}

// sound_preset — заново ApplySoundPreset с пресетом из рецепта; узел-версия = его
// результат (head → эта версия).
func TestRecipeRunnerSoundPresetReapplied(t *testing.T) {
	t.Parallel()
	src := psSetup(t, nil)
	rec := &jrRec{Service: src}
	p := psPreset(9, nil, []yue.PresetStep{{Chain: "level", Params: map[string]float64{"gain": -6, "ceiling": -0.5}}})
	if _, err := ApplySoundPreset(withoutLiveLayer(context.Background()), yue.Journaled(rec), psJobID, p); err != nil {
		t.Fatalf("исходный пресет: %v", err)
	}
	if len(rec.recs) != 1 {
		t.Fatalf("журнал исходной установки %d записей, want 1 (sound_preset)", len(rec.recs))
	}

	dst := psSetup(t, nil)
	for k, v := range dst.files {
		if rest, ok := strings.CutPrefix(k, fmt.Sprintf("%d/", psJobID)); ok {
			dst.files[key(100, rest)] = v
		}
	}
	dst.uploads = map[string][]byte{}
	f := newRcNew(t, dst, rcBuild(t, psJobID, rcDefaultRoot(), rec.recs, "", psChild), "seed")
	rcTicks(t, f, 40)

	if f.status() != "done" {
		t.Fatalf("сессия %v (%v), want done", f.sess["status"], f.sess["error"])
	}
	final := fmt.Sprintf("dsp-preset-%d.flac", p.ID)
	if _, ok := dst.uploads[final]; !ok {
		t.Errorf("финал пресета %s не загружен: %v", final, keys(dst.uploads))
	}
	if len(f.variants) != 1 || f.variants[0].jobID != 100 || f.variants[0].file != final {
		t.Fatalf("версия пресета %+v, want VariantToTrack(100, %s)", f.variants, final)
	}
	if len(f.heads) != 1 || f.heads[0] != [2]int64{100, 101} {
		t.Errorf("SetHead %v, want (100, 101) — версия пресета", f.heads)
	}
}

// rcWithDurations — рецепт с durations {nK: секунды}.
func rcWithDurations(t *testing.T, raw json.RawMessage, d map[string]float64) json.RawMessage {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m["durations"] = d
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// rcDriftWords — признаки строки «длительность отличается, окна правок могли сместиться».
func rcHasDrift(lines []string) bool {
	for _, l := range lines {
		low := strings.ToLower(l)
		for _, w := range []string{"длительн", "смест", "смещ", "окна"} {
			if strings.Contains(low, w) {
				return true
			}
		}
	}
	return false
}

// Новая длительность узла отличается от durations[nK] > 2 % → строка в progress сессии
// (окна правок могли сместиться); ≤ 2 % — такой строки нет. Новые джобы фейка — psDur секунд.
func TestRecipeRunnerDurationDrift(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		n2    float64
		drift bool
	}{
		{"5 %", psDur * 1.05, true},
		{"1 %", psDur * 1.01, false},
		{"-3 %", psDur / 1.03, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw := rcWithDurations(t, rcMainRecipe(t, rcDefaultRoot()),
				map[string]float64{"n1": psDur, "n2": tc.n2, "n3": psDur})
			f := newRcNew(t, nil, raw, "seed")
			rcTicks(t, f, 40)
			if f.status() != "done" {
				t.Fatalf("сессия %v (%v), want done — расхождение длительности не ошибка", f.sess["status"], f.sess["error"])
			}
			if got := rcHasDrift(f.progress); got != tc.drift {
				t.Errorf("строка о длительности в progress: %v, want %v; progress %q", got, tc.drift, f.progress)
			}
		})
	}
}

// Долгий шаг (ApplyFx дольше интервала продления) → ClaimRecipeImport зовётся
// повторно ВО ВРЕМЯ шага. Не параллельный: подменяет recipeLeaseEvery пакета.
func TestRecipeRunnerRenewsClaimDuringLongStep(t *testing.T) {
	old := recipeLeaseEvery
	recipeLeaseEvery = 20 * time.Millisecond
	defer func() { recipeLeaseEvery = old }()

	f := newRcNew(t, nil, rcMainRecipe(t, rcDefaultRoot()), "seed")
	f.fxDelay = 200 * time.Millisecond
	rcTicks(t, f, 40)
	if f.status() != "done" {
		t.Fatalf("сессия %v (%v), want done", f.sess["status"], f.sess["error"])
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fxClaims < 2 {
		t.Errorf("захватов во время ApplyFx %d, want ≥ 2 (продление раз в 20 мс за 200 мс шага)", f.fxClaims)
	}
	for _, o := range f.owners {
		if o != "app" {
			t.Errorf("продление владельцем %q, want app", o)
		}
	}
}

// Сохранение состояния несёт owner ведущего (воркер отвергнет устаревшее состояние чужого).
func TestRecipeRunnerSaveCarriesOwner(t *testing.T) {
	t.Parallel()
	f := newRcNew(t, nil, rcMainRecipe(t, rcDefaultRoot()), "seed")
	rcTicks(t, f, 40)
	if f.status() != "done" || len(f.updates) == 0 {
		t.Fatalf("сессия %v, сохранений %d", f.sess["status"], len(f.updates))
	}
	for i, u := range f.updates {
		var o string
		if v, ok := u["owner"]; !ok || json.Unmarshal(v, &o) != nil || o != "app" {
			t.Errorf("сохранение %d: owner %s, want \"app\"", i, u["owner"])
		}
	}
}

// SaveRecipeImport ответил ErrClaimed (сессию забрал другой) → ведущий не делает
// следующий шаг ни в этом тике, ни в следующих.
func TestRecipeRunnerStopsWhenSaveClaimed(t *testing.T) {
	t.Parallel()
	f := newRcNew(t, nil, rcMainRecipe(t, rcDefaultRoot()), "seed")
	f.stealFx = true
	r := &RecipeRunner{Svc: f, Owner: "app"}
	for i := 0; i < 20; i++ {
		before := f.count("fx")
		did := r.Tick(context.Background())
		if before == 0 && f.count("fx") == 1 && did {
			t.Error("Tick шага fx = true, want false: сохранение отвергнуто (ErrClaimed)")
		}
	}
	if f.count("fx") != 1 || f.count("variant_track") != 0 || f.count("set_head") != 0 {
		t.Errorf("вызовы %v, want остановка после fx (сессию держит другой)", f.calls)
	}
}

func rcLastUpdate(t *testing.T, f *rcNew) map[string]json.RawMessage {
	t.Helper()
	if len(f.updates) != 1 {
		t.Fatalf("сохранений %d, want 1", len(f.updates))
	}
	return f.updates[0]
}

func rcField(u map[string]json.RawMessage, k string) (string, bool) {
	v, ok := u[k]
	if !ok || string(v) == "null" {
		return "", false
	}
	var s string
	if json.Unmarshal(v, &s) != nil {
		return "", false
	}
	return s, true
}

// «Повторить»: status running, error "" (тот же шаг — state не трогается).
func TestRecipeImportActionRetry(t *testing.T) {
	t.Parallel()
	f := newRcNew(t, nil, rcMainRecipe(t, rcDefaultRoot()), "seed")
	f.sess["status"], f.sess["error"] = "error", "движок выключен"
	f.state = json.RawMessage(`{"next":1,"waiting":0,"nodes":{"n1":100,"n2":101},"files":{}}`)
	if err := RecipeImportAction(context.Background(), f, 1, "retry"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	u := rcLastUpdate(t, f)
	if st, _ := rcField(u, "status"); st != "running" {
		t.Errorf("status %s, want running", u["status"])
	}
	if e, ok := rcField(u, "error"); !ok || e != "" {
		t.Errorf("error %s, want \"\" (сброшен)", u["error"])
	}
	if string(f.state) != `{"next":1,"waiting":0,"nodes":{"n1":100,"n2":101},"files":{}}` {
		t.Errorf("state изменён: %s — повтор должен идти с того же шага", f.state)
	}
}

// «Убрать»: status dismissed.
func TestRecipeImportActionDismiss(t *testing.T) {
	t.Parallel()
	f := newRcNew(t, nil, rcMainRecipe(t, rcDefaultRoot()), "seed")
	f.sess["status"] = "error"
	if err := RecipeImportAction(context.Background(), f, 1, "dismiss"); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	if st, _ := rcField(rcLastUpdate(t, f), "status"); st != "dismissed" {
		t.Errorf("status %s, want dismissed", f.updates[0]["status"])
	}
}

// Неизвестное действие — ошибка, ничего не сохраняется; ошибка сервиса — возвращается.
func TestRecipeImportActionUnknownAndServiceError(t *testing.T) {
	t.Parallel()
	f := newRcNew(t, nil, rcMainRecipe(t, rcDefaultRoot()), "seed")
	for _, a := range []string{"", "delete", "RETRY"} {
		if err := RecipeImportAction(context.Background(), f, 1, a); err == nil {
			t.Errorf("действие %q: ошибки нет", a)
		}
	}
	if len(f.updates) != 0 {
		t.Errorf("сохранения при неизвестном действии: %v", f.updates)
	}
	g := newRcNew(t, nil, rcMainRecipe(t, rcDefaultRoot()), "seed")
	g.saveErr = errors.New("воркер недоступен")
	if err := RecipeImportAction(context.Background(), g, 1, "retry"); err == nil {
		t.Error("ошибка сервиса потеряна")
	}
}

// testdata/recipe_e2e.json (python-тест сверяет её с build_recipe — golden) проигрывается
// ведущим: все вызовы — с новыми id и новыми именами файлов; variant_track из файла fx — новое имя
// fx, из overdub-{nK}.flac — overdub-<новый id overdub-джобы>.flac; итог — SetHead на новую версию.
func TestRecipeRunnerReplaysE2EFixture(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("testdata", "recipe_e2e.json"))
	if err != nil {
		t.Fatalf("фикстура: %v", err)
	}
	f := newRcNew(t, nil, raw, "seed")
	rcTicks(t, f, 60)
	if f.status() != "done" {
		t.Fatalf("сессия %v (%v), want done; вызовы %v", f.sess["status"], f.sess["error"], f.calls)
	}
	var gen []string
	for _, c := range f.calls {
		if c != "set_head" && c != "update" {
			gen = append(gen, c)
		}
	}
	// новые id: n1=100, n2=101 (continue), n3=102 (variant), n4=103 (overdub), n5=104 (variant)
	if want := []string{"submit", "continue", "fx", "variant_track", "overdub", "variant_track"}; !reflect.DeepEqual(gen, want) {
		t.Fatalf("вызовы %v, want %v", gen, want)
	}
	if s := f.submits[0]; s.Seed != 123 || s.Style != "dark rock, male baritone" || s.ParentID != 0 {
		t.Errorf("корень %+v, want сид 123 и стиль корня", s)
	}
	if c := f.conts[0]; c.jobID != 100 || c.from != 30 || c.seed != 77 {
		t.Errorf("ContinueJob %+v, want (100, 30, сид 77)", c)
	}
	if f.fxs[0].jobID != 101 {
		t.Errorf("ApplyFx у %d, want 101 (новое продолжение)", f.fxs[0].jobID)
	}
	if want := (rcVar{101, "dsp-fx-new1.flac", "warm", 0}); f.variants[0] != want {
		t.Errorf("VariantToTrack из fx %+v, want %+v (новое имя файла fx)", f.variants[0], want)
	}
	if o := f.overdubs[0]; o.jobID != 102 || o.style != "warm strings" || o.seed != 88 || o.gain != 0.5 {
		t.Errorf("SubmitOverdub %+v, want (102, warm strings, gain 0.5, сид 88)", o)
	}
	if want := (rcVar{102, "overdub-103.flac", "with strings", 0}); f.variants[1] != want {
		t.Errorf("VariantToTrack из overdub %+v, want %+v (overdub-<новый id>.flac)", f.variants[1], want)
	}
	if len(f.heads) == 0 || f.heads[len(f.heads)-1] != [2]int64{100, 104} {
		t.Errorf("SetHead %v, want последним (100, 104) — новая версия n5", f.heads)
	}
	for _, h := range f.heads {
		if h[0] != 100 || h[1] != 104 {
			t.Errorf("SetHead %v, want только (100, 104)", h)
		}
	}
	if f.folders[100] != "Альбом" || rcRootID(f) != 100 {
		t.Errorf("папка %q, root_job_id %v; want Альбом, 100", f.folders[100], f.sess["root_job_id"])
	}
}

// rcNext — state.next присланного сохранения (-100 — state нет).
func rcNext(t *testing.T, u map[string]json.RawMessage) int {
	t.Helper()
	v, ok := u["state"]
	if !ok || string(v) == "null" {
		return -100
	}
	var st struct {
		Next int `json:"next"`
	}
	if err := json.Unmarshal(v, &st); err != nil {
		t.Fatalf("state %s: %v", v, err)
	}
	return st.Next
}

// Шаг fx сделан, PATCH не принят (обычная ошибка) → следующий тик досохраняет тот же
// ход (next продвинут за fx), ApplyFx второй раз не зовётся.
func TestRecipeRunnerResavesStepAfterSaveError(t *testing.T) {
	t.Parallel()
	f := newRcNew(t, nil, rcMainRecipe(t, rcDefaultRoot()), "seed")
	f.failSaveAfterFx = errors.New("воркер: 500")
	rcTicks(t, f, 40)
	if f.status() != "done" {
		t.Fatalf("сессия %v (%v), want done", f.sess["status"], f.sess["error"])
	}
	if f.count("fx") != 1 {
		t.Errorf("ApplyFx вызван %d раз, want 1 (шаг не повторяется); вызовы %v", f.count("fx"), f.calls)
	}
	if f.failedSaves == 0 || f.failedSaves >= len(f.updates) {
		t.Fatalf("неудачное сохранение %d из %d — сценарий не сработал", f.failedSaves, len(f.updates))
	}
	failed, next := f.updates[f.failedSaves-1], f.updates[f.failedSaves]
	if n := rcNext(t, failed); n != 2 {
		t.Errorf("непринятый ход: next %d, want 2 (за fx — индекс 1 в ops)", n)
	}
	if n := rcNext(t, next); n != 2 {
		t.Errorf("следующее сохранение: next %d, want 2 (досохранён тот же ход)", n)
	}
}

// ErrClaimed — ход не досохраняется: состояние воркера главнее, шаг идёт от него
// (в фейке другой ведущий ничего не сделал — fx повторяется по состоянию воркера).
func TestRecipeRunnerNoResaveAfterClaimed(t *testing.T) {
	t.Parallel()
	f := newRcNew(t, nil, rcMainRecipe(t, rcDefaultRoot()), "seed")
	f.failSaveAfterFx = yue.ErrClaimed
	rcTicks(t, f, 40)
	if f.status() != "done" {
		t.Fatalf("сессия %v (%v), want done", f.sess["status"], f.sess["error"])
	}
	if f.failedSaves == 0 {
		t.Fatal("сценарий не сработал")
	}
	if f.count("fx") != 2 {
		t.Errorf("ApplyFx %d раз, want 2 (свой ход не досохранён, шаг от состояния воркера)", f.count("fx"))
	}
}

// В журнале set_head на продолжение (n2), а в рецепте head = n1 → в конце
// SetHead(новый корень, 0): давний выбор из журнала не остаётся.
func TestRecipeRunnerFinalHeadRootResetsHead(t *testing.T) {
	t.Parallel()
	orig := newRcOrig(nil, []int64{11}, nil)
	rec := &jrRec{Service: orig}
	j := yue.Journaled(rec)
	ctx := context.Background()
	if _, err := j.ContinueJob(ctx, 10, 40, 5, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := j.SetHead(ctx, 10, 11); err != nil {
		t.Fatal(err)
	}
	if len(rec.recs) != 2 || rec.recs[1].op.Op != "set_head" {
		t.Fatalf("журнал %+v, want continue + set_head", rec.recs)
	}
	f := newRcNew(t, nil, rcBuild(t, 10, rcDefaultRoot(), rec.recs, "", 10), "seed")
	rcTicks(t, f, 40)
	if f.status() != "done" {
		t.Fatalf("сессия %v (%v), want done", f.sess["status"], f.sess["error"])
	}
	if len(f.heads) == 0 || f.heads[len(f.heads)-1] != [2]int64{100, 0} {
		t.Errorf("SetHead %v, want последним (100, 0) — основная версия = сам трек", f.heads)
	}
}

// Имя ведущего уникально на экземпляр и содержит вид программы.
func TestRecipeOwnerUnique(t *testing.T) {
	t.Parallel()
	a, b := RecipeOwner("app"), RecipeOwner("app")
	if a == b {
		t.Errorf("RecipeOwner дважды = %q, want разные", a)
	}
	if !strings.Contains(a, "app") || !strings.Contains(b, "app") {
		t.Errorf("%q / %q, want содержат app", a, b)
	}
	if m := RecipeOwner("mcp"); !strings.Contains(m, "mcp") {
		t.Errorf("RecipeOwner(mcp) = %q, want содержит mcp", m)
	}
}

// Сохранение хода A не принято, а на воркере уже другое состояние (вёл B: next продвинут,
// своё соответствие) → A свой старый ход не пишет и fx ради него не повторяет; идёт от состояния B.
func TestRecipeRunnerDropsUnsavedWhenWorkerStateChanged(t *testing.T) {
	t.Parallel()
	f := newRcNew(t, nil, rcMainRecipe(t, rcDefaultRoot()), "seed")
	f.failSaveAfterFx = errors.New("воркер: 500")
	f.afterFailState = json.RawMessage(`{"next":3,"waiting":0,"nodes":{"n1":100,"n2":101,"n3":555},"files":{"n2/dsp-fx-a.flac":"dsp-fx-B.flac"}}`)
	rcTicks(t, f, 40)
	if f.failedSaves == 0 {
		t.Fatal("сценарий не сработал")
	}
	for i, u := range f.updates[f.failedSaves:] {
		if rcNext(t, u) == 2 {
			t.Errorf("сохранение %d после сбоя несёт старый ход A (next 2) — перезаписало состояние B", f.failedSaves+i)
		}
	}
	if f.count("fx") != 1 || f.count("variant_track") != 0 {
		t.Errorf("вызовы %v, want fx один раз и без variant_track (его сделал B)", f.calls)
	}
	if f.status() != "done" {
		t.Fatalf("сессия %v (%v), want done от состояния B", f.sess["status"], f.sess["error"])
	}
	if len(f.heads) == 0 || f.heads[len(f.heads)-1] != [2]int64{100, 555} {
		t.Errorf("SetHead %v, want (100, 555) — head по соответствию B", f.heads)
	}
}

// Досохранение получило ErrClaimed → в этом же тике шаг не выполняется.
func TestRecipeRunnerResaveClaimedSkipsStep(t *testing.T) {
	t.Parallel()
	f := newRcNew(t, nil, rcMainRecipe(t, rcDefaultRoot()), "seed")
	f.failSaveAfterFx = errors.New("воркер: 500")
	f.thenSaveErr = yue.ErrClaimed
	r := &RecipeRunner{Svc: f, Owner: "app"}
	for i := 0; i < 20 && f.count("fx") == 0; i++ {
		r.Tick(context.Background())
	}
	if f.count("fx") != 1 || f.failedSaves == 0 {
		t.Fatalf("сценарий не сработал: вызовы %v, неудачных сохранений %d", f.calls, f.failedSaves)
	}
	saves := len(f.updates)
	if r.Tick(context.Background()) {
		t.Error("Tick = true, want false: досохранение отвергнуто (ErrClaimed)")
	}
	if len(f.updates) != saves+1 {
		t.Fatalf("сохранений в тике %d, want 1 (досохранение)", len(f.updates)-saves)
	}
	if f.count("fx") != 1 || f.count("variant_track") != 0 || f.count("continue") != 1 {
		t.Errorf("вызовы %v: в тике с ErrClaimed шаг выполнен", f.calls)
	}
}
