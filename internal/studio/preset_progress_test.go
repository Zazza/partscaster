package studio

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"partscaster/internal/yue"
)

// Ход применения пресета (internal-preset-progress, условия 3–4; тест-кейсы «Go studio») и промежуточные версии
// (internal-preset-verify-on-apply, условие 11, ТК11: удаляются, без подписей «промежуточная»).
//
// Контракт (имена выбраны тест-автором — в карточке не заданы):
//
//	yue.QueueProgress{Stage string; Pass int; EtaS *float64}
//	Queuer.QueueBeat(ctx, id, progress ...yue.QueueProgress) (*yue.QueueOp, error)
//	ReportStep(ctx, stage string, pass int, eta *float64) — ход операции очереди ctx: сразу битом и дальше
//	  с каждым обычным битом; вне операции — ничего.
//	yue.Service.SetJobParent(ctx, jobID, parentID int64) error — PATCH /jobs/{id} {parent_id}.
//
// Шаги пресета: prepare → parts → levels → render(1) → verify(1) → [fix(N+1) → verify(N+1)]…
// eta — nil до конца первой сверки, потом число ≥ 0.
// Исправляющий проход: прежняя версия удаляется (DeleteJob); ни переименований, ни смены родителя — у трека
// остаётся одна версия, итоговая, с названием «<название> · <пресет>». Внешняя граница — воркер (vfFake из
// preset_verify_test.go + очередь).

// prgBeat — бит, пришедший воркеру: с ходом или без.
type prgBeat struct {
	has bool
	p   yue.QueueProgress
	at  time.Time
}

// prgQueue — очередь воркера: операция сразу идёт (running), биты записываются.
type prgQueue struct {
	mu       sync.Mutex
	beats    []prgBeat
	finished []yue.QueueFinishIn
}

func (q *prgQueue) QueueAdd(_ context.Context, in yue.QueueOpIn) (*yue.QueueOp, error) {
	return &yue.QueueOp{ID: fakeOpID, Kind: in.Kind, Title: in.Title, JobID: in.JobID, State: "running"}, nil
}

func (q *prgQueue) QueueBeat(_ context.Context, id int64, progress ...yue.QueueProgress) (*yue.QueueOp, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	b := prgBeat{at: time.Now()}
	if len(progress) > 0 {
		b.has, b.p = true, progress[0]
	}
	q.beats = append(q.beats, b)
	return &yue.QueueOp{ID: id, State: "running"}, nil
}

func (q *prgQueue) QueueFinish(_ context.Context, id int64, in yue.QueueFinishIn) (*yue.QueueOp, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.finished = append(q.finished, in)
	return &yue.QueueOp{ID: id, State: in.State, ResultJobID: in.ResultJobID}, nil
}

func (q *prgQueue) progress() []prgBeat {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out []prgBeat
	for _, b := range q.beats {
		if b.has {
			out = append(out, b)
		}
	}
	return out
}

// prgSvc — только очередь (для ReportStep вне пресета); остальное yue.Service не реализовано.
type prgSvc struct {
	yue.Service
	*prgQueue
}

// prgPresetFake — воркер пресета (vfFake) + очередь + переименование и смена родителя версий.
type prgPresetFake struct {
	*vfFake
	*prgQueue
	renames []prgRename
	parents map[int64]int64
}

type prgRename struct {
	id    int64
	title string
}

func (f *prgPresetFake) UpdateJob(_ context.Context, id int64, title, folder *string) (*yue.Job, error) {
	if folder != nil {
		f.t.Errorf("UpdateJob(%d): папка меняться не должна (%q)", id, *folder)
	}
	if title == nil {
		f.t.Errorf("UpdateJob(%d) без названия", id)
		return &yue.Job{ID: id}, nil
	}
	f.renames = append(f.renames, prgRename{id, *title})
	for i := range f.jobs {
		if f.jobs[i].ID == id {
			f.jobs[i].Title = *title
		}
	}
	return &yue.Job{ID: id, Title: *title}, nil
}

func (f *prgPresetFake) SetJobParent(_ context.Context, id, parent int64) error {
	f.parents[id] = parent
	return nil
}

func prgSetup(t *testing.T, errs map[int64]float64) *prgPresetFake {
	t.Helper()
	vf, _ := vfSetup(t, errs)
	return &prgPresetFake{vfFake: vf, prgQueue: &prgQueue{}, parents: map[int64]int64{}}
}

func prgApply(t *testing.T, f *prgPresetFake, p yue.SoundPreset) (*PresetResult, time.Duration) {
	t.Helper()
	start := time.Now()
	res, err := ApplySoundPreset(withoutLiveLayer(context.Background()), f, psJobID, p)
	if err != nil {
		t.Fatalf("ApplySoundPreset: %v", err)
	}
	if res == nil {
		t.Fatal("ApplySoundPreset: nil без ошибки")
	}
	return res, time.Since(start)
}

// prgLabel — шаг в записи последовательности: номер сборки только у render/verify/fix.
func prgLabel(p yue.QueueProgress) string {
	switch p.Stage {
	case "render", "verify", "fix":
		return fmt.Sprintf("%s(%d)", p.Stage, p.Pass)
	}
	return p.Stage
}

// prgSteps — последовательность шагов без повторов подряд (ход повторяется с обычными битами).
func prgSteps(beats []prgBeat) []string {
	var out []string
	for _, b := range beats {
		l := prgLabel(b.p)
		if len(out) == 0 || out[len(out)-1] != l {
			out = append(out, l)
		}
	}
	return out
}

func prgCheckSteps(t *testing.T, f *prgPresetFake, want []string) {
	t.Helper()
	got := prgSteps(f.progress())
	if strings.Join(got, " → ") != strings.Join(want, " → ") {
		t.Fatalf("шаги пресета:\n got %s\nwant %s", strings.Join(got, " → "), strings.Join(want, " → "))
	}
}

// prgCheckEta — до первого fix (включая prepare…verify(1)) eta nil; с первого fix — число в [0, предел].
func prgCheckEta(t *testing.T, f *prgPresetFake, limit time.Duration) {
	t.Helper()
	fixed := false
	for _, b := range f.progress() {
		if b.p.Stage == "fix" {
			fixed = true
		}
		switch {
		case !fixed && b.p.EtaS != nil:
			t.Errorf("шаг %s: eta %.2f до конца первой сверки, нужно nil", prgLabel(b.p), *b.p.EtaS)
		case fixed && b.p.EtaS == nil:
			t.Errorf("шаг %s: eta nil после первой сверки, нужно число", prgLabel(b.p))
		case fixed && (*b.p.EtaS < 0 || *b.p.EtaS > limit.Seconds()):
			t.Errorf("шаг %s: eta %.3f вне [0, %.3f] (длительность первого прохода не больше всего пресета)",
				prgLabel(b.p), *b.p.EtaS, limit.Seconds())
		}
	}
}

func prgNoJobDeleted(t *testing.T, f *prgPresetFake) {
	t.Helper()
	for _, d := range f.deleted {
		if strings.HasPrefix(d, "job ") {
			t.Errorf("удалена версия (%s): без исправлений единственная версия остаётся", d)
		}
	}
}

func prgVersionTitle(f *prgPresetFake, id int64) string {
	for _, j := range f.jobs {
		if j.ID == id {
			return j.Title
		}
	}
	return ""
}

// ---------- ReportStep ----------

// Внутри операции очереди ход уходит воркеру сразу, не дожидаясь обычного бита.
func TestReportStepSendsImmediately(t *testing.T) {
	t.Parallel()
	q := &prgQueue{}
	svc := &prgSvc{prgQueue: q}
	eta := 12.0
	var sentIn time.Duration
	_, err := RunOp(context.Background(), svc, "preset", "p", 463, func(ctx context.Context) (int64, error) {
		start := time.Now()
		ReportStep(ctx, "render", 2, &eta)
		deadline := time.Now().Add(opPoll / 2)
		for time.Now().Before(deadline) {
			if len(q.progress()) > 0 {
				sentIn = time.Since(start)
				return 1, nil
			}
			time.Sleep(5 * time.Millisecond)
		}
		return 1, nil
	})
	if err != nil {
		t.Fatalf("RunOp: %v", err)
	}
	pr := q.progress()
	if len(pr) == 0 {
		t.Fatalf("ход не ушёл воркеру за %s (раньше обычного бита)", opPoll/2)
	}
	p := pr[0].p
	if p.Stage != "render" || p.Pass != 2 || p.EtaS == nil || *p.EtaS != 12 {
		t.Fatalf("ход: %+v, нужно render/2/12", p)
	}
	t.Logf("ход ушёл через %s", sentIn)
}

// Ход повторяется с обычными битами (воркер, перезапустивший отображение, видит шаг и дальше).
func TestReportStepRepeatedWithRegularBeats(t *testing.T) {
	t.Parallel()
	q := &prgQueue{}
	svc := &prgSvc{prgQueue: q}
	_, err := RunOp(context.Background(), svc, "preset", "p", 463, func(ctx context.Context) (int64, error) {
		ReportStep(ctx, "levels", 1, nil)
		deadline := time.Now().Add(waitLimit)
		for time.Now().Before(deadline) {
			if len(q.progress()) >= 3 {
				return 1, nil
			}
			time.Sleep(20 * time.Millisecond)
		}
		return 1, nil
	})
	if err != nil {
		t.Fatalf("RunOp: %v", err)
	}
	pr := q.progress()
	if len(pr) < 3 {
		t.Fatalf("битов с ходом %d за %s, нужно ≥ 3 (сразу + с обычными битами)", len(pr), waitLimit)
	}
	for i, b := range pr {
		if b.p.Stage != "levels" || b.p.Pass != 1 || b.p.EtaS != nil {
			t.Errorf("бит %d: %+v, нужно levels/1/nil", i, b.p)
		}
	}
}

// Последний ход заменяет прежний: обычные биты несут новый шаг.
func TestReportStepLatestWins(t *testing.T) {
	t.Parallel()
	q := &prgQueue{}
	svc := &prgSvc{prgQueue: q}
	_, err := RunOp(context.Background(), svc, "preset", "p", 463, func(ctx context.Context) (int64, error) {
		ReportStep(ctx, "render", 1, nil)
		ReportStep(ctx, "verify", 1, nil)
		n := len(q.progress())
		deadline := time.Now().Add(waitLimit)
		for time.Now().Before(deadline) && len(q.progress()) < n+2 {
			time.Sleep(20 * time.Millisecond)
		}
		return 1, nil
	})
	if err != nil {
		t.Fatalf("RunOp: %v", err)
	}
	pr := q.progress()
	if len(pr) < 3 {
		t.Fatalf("битов с ходом %d", len(pr))
	}
	if steps := prgSteps(pr); strings.Join(steps, " ") != "render(1) verify(1)" {
		t.Fatalf("шаги %v, нужно render(1) → verify(1) без возврата к прежнему", steps)
	}
}

// Вне операции очереди — ничего (и не паника), в том числе у сервиса без очереди.
func TestReportStepOutsideOpIsNoop(t *testing.T) {
	t.Parallel()
	ReportStep(context.Background(), "render", 1, nil)
	// сервис без очереди: RunOp вызывает fn сразу, операции нет
	svc := &psFake{}
	called := false
	if _, err := RunOp(context.Background(), svc, "preset", "p", 1, func(ctx context.Context) (int64, error) {
		called = true
		ReportStep(ctx, "prepare", 1, nil)
		return 1, nil
	}); err != nil {
		t.Fatalf("RunOp: %v", err)
	}
	if !called {
		t.Fatal("fn не вызвана")
	}
}

// После итога операции ход больше не шлётся.
func TestReportStepAfterOpEndsSendsNothing(t *testing.T) {
	t.Parallel()
	q := &prgQueue{}
	svc := &prgSvc{prgQueue: q}
	var opCtx context.Context
	if _, err := RunOp(context.Background(), svc, "preset", "p", 463, func(ctx context.Context) (int64, error) {
		opCtx = ctx
		return 1, nil
	}); err != nil {
		t.Fatalf("RunOp: %v", err)
	}
	before := len(q.progress())
	ReportStep(opCtx, "render", 1, nil)
	time.Sleep(opPoll + 200*time.Millisecond)
	if n := len(q.progress()); n != before {
		t.Fatalf("после итога ушло %d битов с ходом", n-before)
	}
}

// ---------- ApplySoundPreset ----------

// Все у цели: prepare → parts → levels → render(1) → verify(1); одна версия, без переименований и смены родителя.
func TestPresetProgressSinglePass(t *testing.T) {
	t.Parallel()
	f := prgSetup(t, map[int64]float64{vfFirst: 0.5})
	p := vfPreset(71)
	res, _ := prgApply(t, f, p)
	prgCheckSteps(t, f, []string{"prepare", "parts", "levels", "render(1)", "verify(1)"})
	prgCheckEta(t, f, time.Hour)
	if res.ChildID != vfFirst {
		t.Errorf("ChildID = %d, нужно %d", res.ChildID, vfFirst)
	}
	if len(f.promoted) != 1 {
		t.Errorf("версий %d, нужно 1", len(f.promoted))
	}
	if len(f.renames) != 0 {
		t.Errorf("переименования без исправлений: %+v", f.renames)
	}
	if len(f.parents) != 0 {
		t.Errorf("смена родителя без исправлений: %v", f.parents)
	}
	prgNoJobDeleted(t, f)
	if got, want := prgVersionTitle(f, vfFirst), psTitle+" · "+p.Name; got != want {
		t.Errorf("название версии %q, нужно %q", got, want)
	}
}

// prgOnlyFinal — промежуточные версии inter удалены, итоговая final — нет; ни подписей, ни смены родителя.
func prgOnlyFinal(t *testing.T, f *prgPresetFake, base string, final int64, inter ...int64) {
	t.Helper()
	for _, id := range inter {
		if !contains(f.deleted, fmt.Sprintf("job %d", id)) {
			t.Errorf("промежуточная версия #%d не удалена: %v", id, f.deleted)
		}
	}
	if contains(f.deleted, fmt.Sprintf("job %d", final)) {
		t.Errorf("итоговая версия #%d удалена: %v", final, f.deleted)
	}
	if len(f.renames) != 0 {
		t.Errorf("переименования %+v, want ни одного (подписей «промежуточная» нет)", f.renames)
	}
	if len(f.parents) != 0 {
		t.Errorf("смена родителя %v, want ни одной", f.parents)
	}
	if got := prgVersionTitle(f, final); got != base {
		t.Errorf("итоговая %q, нужно %q", got, base)
	}
}

// Один исправляющий проход: … verify(1) → fix(2) → verify(2); V1 удалена, итог — V2; eta nil до первой
// сверки, потом число.
func TestPresetProgressOneFix(t *testing.T) {
	t.Parallel()
	f := prgSetup(t, map[int64]float64{vfFirst: 3.6})
	p := vfPreset(72)
	res, took := prgApply(t, f, p)
	prgCheckSteps(t, f, []string{"prepare", "parts", "levels", "render(1)", "verify(1)", "fix(2)", "verify(2)"})
	prgCheckEta(t, f, took)
	final := vfFirst + 1
	if res.ChildID != final {
		t.Fatalf("ChildID = %d, нужно %d", res.ChildID, final)
	}
	prgOnlyFinal(t, f, psTitle+" · "+p.Name, final, vfFirst)
}

// Два исправляющих прохода: V1 и V2 удалены, итог — V3.
func TestPresetProgressTwoFixes(t *testing.T) {
	t.Parallel()
	f := prgSetup(t, map[int64]float64{vfFirst: 3.6, vfFirst + 1: 1.6})
	p := vfPreset(73)
	res, took := prgApply(t, f, p)
	// последняя (третья) сборка — предел исправлений: сверять её или нет, карточка не говорит — допустимы оба
	steps := strings.Join(prgSteps(f.progress()), " → ")
	base3 := "prepare → parts → levels → render(1) → verify(1) → fix(2) → verify(2) → fix(3)"
	if steps != base3 && steps != base3+" → verify(3)" {
		t.Fatalf("шаги пресета:\n got %s\nwant %s [→ verify(3)]", steps, base3)
	}
	prgCheckEta(t, f, took)
	final := vfFirst + 2
	if res.ChildID != final {
		t.Fatalf("ChildID = %d, нужно %d", res.ChildID, final)
	}
	prgOnlyFinal(t, f, psTitle+" · "+p.Name, final, vfFirst, vfFirst+1)
}

// Сбой разделения первой версии: сверки нет — исправлений нет, одна версия без переименования; ход не дальше verify(1).
func TestPresetProgressVerifyFailsNoFix(t *testing.T) {
	t.Parallel()
	f := prgSetup(t, map[int64]float64{vfFirst: 3.6})
	f.splitFail[vfFirst] = true
	res, _ := prgApply(t, f, vfPreset(74))
	prgCheckSteps(t, f, []string{"prepare", "parts", "levels", "render(1)", "verify(1)"})
	if res.ChildID != vfFirst || len(f.renames) != 0 || len(f.parents) != 0 {
		t.Errorf("ChildID %d, переименования %+v, родители %v; нужно V1 без изменений", res.ChildID, f.renames, f.parents)
	}
	prgNoJobDeleted(t, f)
}

// Оценка «осталось ≈» в отметках «жива» убывает на прошедшее с отчёта шага и не уходит ниже нуля (условие 4:
// «минус прошедшее в текущем проходе») — иначе строка очереди минутами показывала бы одно число.
func TestOpProgressEtaDecreasesBetweenBeats(t *testing.T) {
	t0 := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	now := t0
	eta := 180.0
	p := &opProgress{now: func() time.Time { return now }}
	p.cur = []yue.QueueProgress{{Stage: "fix", Pass: 2, EtaS: &eta}}
	p.at = t0
	for _, tc := range []struct {
		after time.Duration
		want  float64
	}{{0, 180}, {40 * time.Second, 140}, {3 * time.Minute, 0}, {5 * time.Minute, 0}} {
		now = t0.Add(tc.after)
		got := p.current()
		if len(got) != 1 || got[0].EtaS == nil || *got[0].EtaS != tc.want {
			t.Fatalf("через %v: eta = %v, want %v", tc.after, got, tc.want)
		}
	}
	if eta != 180 {
		t.Fatalf("сообщённая оценка изменилась: %v", eta)
	}
	// без оценки — ход как есть
	p.cur = []yue.QueueProgress{{Stage: "prepare", Pass: 1}}
	if got := p.current(); got[0].EtaS != nil {
		t.Fatalf("без оценки появилась оценка: %v", *got[0].EtaS)
	}
}
