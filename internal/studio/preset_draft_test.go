package studio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"partscaster/internal/dsp"
	"partscaster/internal/yue"
)

// Пресет звука в черновик (карточка internal-studio-preset-draft, условия 4–8; тест-кейсы ТК8–ТК11;
// internal-preset-verify-on-apply, условия 1–2, ТК1).
//
// PresetDraft(ctx, svc, jobID, p) → {sections, final, master, warnings}: готовит всё, что готовит применение
// (дорожки, план для гитариста, части барабанов только движком, партии, наборы, замер уровней), но трек не
// меняет: итоговой версии нет, журнала нет, пресет не меняется. Сборок нет вовсе — ни пересборки, ни версии,
// ни разделения версий, ни пробных вариантов: сверка баланса — при итоге черновика (ApplyEdits). Записи — как
// первый проход (громкости по замеру дорожек, без поправок сверки); у записи с level_db на одну дорожку с
// замером — verify_db = level_db + db записи.
//
// Воркер — фейки соседних тестов пресета (psFake, ppFake, gpFake, vfFake, opFake): только внешние границы.
// Толкования и допущения:
//   - «без сборки» — ни одного вызова движка (ApplyFx), ни одной загрузки варианта (UploadDsp), ни одной
//     версии-трека (VariantToTrack), ни разделения версии, ни удалений;
//   - «пресет на воркере не меняется» — методов записи пресета у фейков нет (вызов уронил бы тест паникой),
//     плюс сам аргумент p после вызова тот же;
//   - слой «живой звук» выключен (withoutLiveLayer), как в прочих тестах механики пресета: его записи
//     проверяются своими тестами; поэтому sections = записи пресета + партии.

// pdDraft — PresetDraft на трек psJobID без слоя «живой звук».
func pdDraft(svc yue.Service, p yue.SoundPreset) (*PresetDraftResult, error) {
	return PresetDraft(withoutLiveLayer(context.Background()), svc, psJobID, p)
}

func pdMustDraft(t *testing.T, svc yue.Service, p yue.SoundPreset) *PresetDraftResult {
	t.Helper()
	res, err := pdDraft(svc, p)
	if err != nil {
		t.Fatalf("PresetDraft: %v", err)
	}
	if res == nil {
		t.Fatal("PresetDraft: nil без ошибки")
	}
	return res
}

// pdJSON — пресет как JSON: снимок до вызова, чтобы увидеть, что аргумент не изменён.
func pdJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	return string(b)
}

// pdNoRender — ни версии, ни удалений, ни движка, ни загрузок.
func pdNoRender(t *testing.T, f *psFake) {
	t.Helper()
	if len(f.promoted) != 0 {
		t.Errorf("созданы версии %+v, want ни одной (сборки нет)", f.promoted)
	}
	if len(f.deleted) != 0 {
		t.Errorf("удаления %v, want ни одного (удалять нечего)", f.deleted)
	}
	if n := psCount(f.log, "applyfx:"); n != 0 || len(f.calls) != 0 {
		t.Errorf("движок (ApplyFx) вызван %d раз, want 0 (сборки нет): %v", max(n, len(f.calls)), f.log)
	}
	if len(f.uploads) != 0 {
		t.Errorf("загружены варианты %v, want ни одного (сборки нет)", keys(f.uploads))
	}
}

// ---------- ТК8: без целей level_db — без сборки; sections/final/master в ответе ----------

func TestPresetDraftNoTargetsNoRender(t *testing.T) {
	t.Parallel()
	f := ppSetup(t, 8, ppBars())
	p := ppPreset(t, 71, ppPartsPadPerc)
	p.Name = "Пост-панк"
	place := &yue.Place{Pan: 0.3, Width: 1}
	p.Specs = []yue.PresetSpec{
		psVocalCut(),
		{Stems: []string{"bass"}, Engine: psAmpEngine(), Db: -2},
		{Stems: []string{"other"}, Db: -3, Place: place},
	}
	p.Final = append(psFinal(), yue.PresetStep{Chain: "eq", Params: map[string]float64{"high": -6}, Off: true})
	p.Master = []map[string]any{msGlue()}
	p.TargetLUFS = psTarget(-13)
	before := pdJSON(t, p)

	res := pdMustDraft(t, f, p)

	pdNoRender(t, f.psFake)
	if after := pdJSON(t, p); after != before {
		t.Errorf("пресет изменён вызовом:\n было %s\nстало %s", before, after)
	}
	// наборы сэмплов партий подготовлены, как при применении
	for _, kit := range []string{"pianokit", "clapkit"} {
		if !contains(f.installs, kit) {
			t.Errorf("набор %s не установлен (installs %v)", kit, f.installs)
		}
	}
	if len(res.Sections) != 5 {
		t.Fatalf("sections %d, want 5 (3 записи пресета + 2 партии): %+v", len(res.Sections), res.Sections)
	}
	for i, s := range res.Sections[:3] {
		if s.ChildID != 0 || s.Add || s.LevelTarget != nil || s.Master || s.Mix {
			t.Errorf("sections[%d] = %+v, want правка дорожки на весь трек (child 0, без add/master/mix/level_target)", i, s)
		}
	}
	s0 := res.Sections[0]
	if !reflect.DeepEqual(s0.Stems, []string{"vocals"}) || s0.Chain != "eq" ||
		!reflect.DeepEqual(s0.Params, psVocalCut().Params) || s0.Db != 0 {
		t.Errorf("sections[0] = %+v, want эффект eq на vocals как в записи, db 0", s0)
	}
	s1 := res.Sections[1]
	if !reflect.DeepEqual(s1.Stems, []string{"bass"}) || !reflect.DeepEqual(s1.Engine, psAmpEngine()) || s1.Db != -2 {
		t.Errorf("sections[1] = %+v, want движок на bass, db −2", s1)
	}
	s2 := res.Sections[2]
	if !reflect.DeepEqual(s2.Stems, []string{"other"}) || s2.Db != -3 || s2.Place == nil || *s2.Place != *place {
		t.Errorf("sections[2] = %+v, want other db −3, место %+v", s2, *place)
	}
	for i, kind := range []string{"synth", "perc"} {
		s := res.Sections[3+i]
		if !s.Add || !reflect.DeepEqual(s.Stems, []string{"mix"}) || len(s.Engine) == 0 || s.Engine[0]["type"] != kind {
			t.Errorf("sections[%d] = %+v, want партия-добавление %s поверх mix", 3+i, s, kind)
			continue
		}
		if n := reflect.ValueOf(s.Engine[0]["notes"]); !n.IsValid() || n.Kind() != reflect.Slice || n.Len() == 0 {
			t.Errorf("партия %s без нот: %v", kind, s.Engine[0]["notes"])
		}
	}
	wantFinal := []dsp.Step{
		{Chain: "width", Params: map[string]float64{"width": 1.1, "bass": 120}},
		{Chain: "level", Params: map[string]float64{"gain": 5.5, "ceiling": -1}},
	}
	if !reflect.DeepEqual(res.Final, wantFinal) {
		t.Errorf("final %+v, want активные шаги %+v (выключенный не идёт)", res.Final, wantFinal)
	}
	if len(res.Master) != 2 || !reflect.DeepEqual(res.Master[0], msGlue()) {
		t.Fatalf("master %v, want [glue, limiter target −13]", res.Master)
	}
	msCheckLimiter(t, res.Master[1], -13)
	if len(res.Warnings) != 0 {
		t.Errorf("warnings %v, want пусто", res.Warnings)
	}
}

// Пустые final/master/warnings — полей в JSON нет; sections — есть. Финал из одних выключенных шагов — пуст.
func TestPresetDraftJSONOmitsEmpty(t *testing.T) {
	t.Parallel()
	f := psSetup(t, nil)
	p := psPreset(72, []yue.PresetSpec{psVocalCut()},
		[]yue.PresetStep{{Chain: "width", Params: map[string]float64{"width": 1.2}, Off: true}})
	res := pdMustDraft(t, f, p)
	pdNoRender(t, f)
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(pdJSON(t, res)), &m); err != nil {
		t.Fatalf("ответ не объект JSON: %v", err)
	}
	if _, ok := m["sections"]; !ok {
		t.Errorf("в ответе нет sections: %v", m)
	}
	for _, k := range []string{"final", "master", "warnings"} {
		if v, ok := m[k]; ok {
			t.Errorf("поле %s = %s, want нет поля (пусто)", k, v)
		}
	}
}

// Цель level_db есть, но сверить её нечем (у трека нет замеров дорожек) — сборки нет, громкость — как в записи.
func TestPresetDraftUnmeasurableTargetNoRender(t *testing.T) {
	t.Parallel()
	f := psSetup(t, nil) // JobStems без metrics
	p := psPreset(73, []yue.PresetSpec{
		{Stems: []string{"bass"}, LevelDb: lvF(-10), Db: 1.5},
		{Stems: []string{"other"}, Engine: psAmpEngine(), LevelDb: lvF(-12)},
	}, nil)
	res := pdMustDraft(t, f, p)
	pdNoRender(t, f)
	if len(res.Sections) != 2 {
		t.Fatalf("sections %+v, want 2", res.Sections)
	}
	if s := res.Sections[0]; s.Db != 1.5 || s.LevelTarget != nil {
		t.Errorf("bass: db %v level_target %v, want db 1,5 как в записи, без level_target (замера нет)", s.Db, s.LevelTarget)
	}
	if s := res.Sections[1]; s.Db != 0 || s.LevelTarget != nil {
		t.Errorf("other: db %v level_target %v, want db 0, без level_target (замера нет)", s.Db, s.LevelTarget)
	}
	for i, s := range res.Sections {
		if s.VerifyDb != nil {
			t.Errorf("sections[%d] verify_db %v, want нет поля (замера нет — сверить нечем)", i, *s.VerifyDb)
		}
	}
}

// Гитарист у трека без плана — выкинут из записи, пометка в warnings (как у ApplySoundPreset).
func TestPresetDraftGuitaristNoPlan(t *testing.T) {
	t.Parallel()
	f := gpSetup(t)
	f.gridErr = &yue.StatusError{Code: 404, Msg: "нет плана"}
	res := pdMustDraft(t, f, gpPreset(true))
	pdNoRender(t, f.psFake)
	if !slicesHas(res.Warnings, func(w string) bool { return strings.Contains(w, "нет плана") }) {
		t.Errorf("warnings %v, want пометку «%s»", res.Warnings, gpNoPlanMark)
	}
	var guitar *SectionSpec
	for i := range res.Sections {
		if reflect.DeepEqual(res.Sections[i].Stems, []string{"guitar"}) {
			guitar = &res.Sections[i]
		}
	}
	if guitar == nil {
		t.Fatalf("записи гитары нет: %+v (без гитариста остаётся остальная обработка)", res.Sections)
	}
	for _, b := range guitar.Engine {
		if b["type"] == "gtrnotes" {
			t.Errorf("gtrnotes остался у трека без плана: %v", guitar.Engine)
		}
	}
	if !reflect.DeepEqual(guitar.Engine, gpRest()) {
		t.Errorf("движок гитары %v, want остальная обработка %v", guitar.Engine, gpRest())
	}
}

// С планом гитарист остаётся, предупреждения нет.
func TestPresetDraftGuitaristWithPlan(t *testing.T) {
	t.Parallel()
	f := gpSetup(t)
	f.grid = &yue.ChordGrid{BPM: 120, Bars: ppBars()}
	res := pdMustDraft(t, f, gpPreset(true))
	if len(res.Warnings) != 0 {
		t.Errorf("warnings %v у трека с планом, want пусто", res.Warnings)
	}
	if len(res.Sections) == 0 || len(res.Sections[0].Engine) == 0 || res.Sections[0].Engine[0]["type"] != "gtrnotes" {
		t.Errorf("гитарист пропал у трека с планом: %+v", res.Sections)
	}
}

// Эффект ffmpeg на части барабанов — ошибка до работы (как у применения).
func TestPresetDraftFfmpegOnDrumPartIsError(t *testing.T) {
	t.Parallel()
	f := psSetup(t, nil)
	p := psPreset(74, []yue.PresetSpec{{Stems: []string{"kick"}, Chain: "eq", Params: map[string]float64{"low": -12}}}, nil)
	_, err := pdDraft(f, p)
	if err == nil || !strings.Contains(err.Error(), "kick") {
		t.Fatalf("want ошибку про kick, got %v", err)
	}
	if len(f.promoted) != 0 || len(f.splits) != 0 {
		t.Errorf("работа началась: promoted %v splits %v", f.promoted, f.splits)
	}
}

// Нет дорожки у трека — разделение (как при применении), черновик готов.
func TestPresetDraftSplitsMissingStems(t *testing.T) {
	t.Parallel()
	f := psSetup(t, nil)
	f.afterDefault = map[string]string{"stem-guitar.flac": psStemFile(t, "guitar", "0.2*sin(2*PI*700*t)")}
	p := psPreset(75, []yue.PresetSpec{{Stems: []string{"guitar"}, Chain: "eq", Params: map[string]float64{"low": 2}}}, nil)
	res := pdMustDraft(t, f, p)
	if !reflect.DeepEqual(f.splits, []string{""}) {
		t.Errorf("разделения %q, want одно обычное", f.splits)
	}
	if len(res.Sections) != 1 || !reflect.DeepEqual(res.Sections[0].Stems, []string{"guitar"}) {
		t.Errorf("sections %+v, want запись гитары", res.Sections)
	}
	if len(f.promoted) != 0 {
		t.Errorf("версии %+v, want ни одной", f.promoted)
	}
}

// Одна операция очереди вида preset у трека; закрыта «готово».
func TestPresetDraftOneQueueOpPreset(t *testing.T) {
	t.Parallel()
	f := &psQueueFake{psFake: psSetup(t, nil), q: newOpFake()}
	if _, err := pdDraft(f, psPreset(76, []yue.PresetSpec{psVocalCut()}, psFinal())); err != nil {
		t.Fatalf("PresetDraft: %v", err)
	}
	added, _, finished := f.q.snapshot()
	if len(added) != 1 || added[0].Kind != "preset" || added[0].JobID != psJobID {
		t.Errorf("операции очереди %+v, want одна preset у #%d", added, psJobID)
	}
	if len(finished) != 1 || finished[0].State != "done" {
		t.Errorf("итоги операции %+v, want один done", finished)
	}
}

// pdVf — vfFake (замеры трека, версии с id vfFirst, vfFirst+1, …) + учёт созданных версий и сбои по счёту.
type pdVf struct {
	*vfFake
	ids          []int64 // созданные версии по порядку
	trackN       int
	failTrackAt  int // n-я VariantToTrack отвечает ошибкой (0 — нет)
	guitarN      int
	failGuitarAt int // n-й вызов движка на гитару отвечает ошибкой (0 — нет)
	made         atomic.Bool
	created      []string // файлы, которые воркер создал у самого трека psJobID (загрузки, варианты, мастер), "id/file"
	masterN      int
}

// addCreated — файл у трека psJobID, созданный сборкой (без повторов).
func (f *pdVf) addCreated(id int64, file string) {
	if id != psJobID {
		return
	}
	k := key(id, file)
	if !contains(f.created, k) {
		f.created = append(f.created, k)
	}
}

func (f *pdVf) UploadDsp(ctx context.Context, id int64, fname, label string, data []byte) (*yue.DspVariant, error) {
	v, err := f.vfFake.UploadDsp(ctx, id, fname, label, data)
	if err == nil && v != nil {
		f.addCreated(id, v.File)
	}
	return v, err
}

func (f *pdVf) VariantToTrack(ctx context.Context, jobID int64, file, title string, src int64) (int64, error) {
	f.trackN++
	if f.trackN == f.failTrackAt {
		return 0, errors.New("воркер: версия не создана")
	}
	id, err := f.vfFake.VariantToTrack(ctx, jobID, file, title, src)
	if err == nil {
		f.ids = append(f.ids, id)
		f.made.Store(true)
	}
	return id, err
}

func (f *pdVf) ApplyFx(ctx context.Context, id int64, req yue.FxRequest) (*yue.DspVariant, error) {
	if req.From == nil || req.To == nil {
		// мастер на весь файл (как msFake): на месте — тот же файл, иначе новый вариант с тем же звуком
		f.masterN++
		name := req.File
		if !req.InPlace {
			name = fmt.Sprintf("dsp-master-%d.flac", f.masterN)
			f.files[key(id, name)] = f.files[key(id, req.File)]
		}
		f.addCreated(id, name)
		return &yue.DspVariant{File: name, Label: "микс · мастер",
			Metrics: map[string]any{"lufs": -14.0, "true_peak_db": -1.0}}, nil
	}
	if req.Source == "guitar" {
		f.guitarN++
		if f.guitarN == f.failGuitarAt {
			return nil, errors.New("движок: нет памяти")
		}
	}
	v, err := f.vfFake.ApplyFx(ctx, id, req)
	// превью куска движка — вход пересборки, а не её итог: в «созданные сборкой» не идёт
	if err == nil && v != nil && !strings.HasPrefix(v.File, "preview-") {
		f.addCreated(id, v.File)
	}
	return v, err
}

func pdVfSetup(t *testing.T, errs map[int64]float64) *pdVf {
	t.Helper()
	f, _ := vfSetup(t, errs)
	return &pdVf{vfFake: f}
}

// pdNear — |got − want| ≤ tol.
func pdNear(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %.3f, want %.3f ±%g", what, got, want, tol)
	}
}

// ---------- ТК1 (internal-preset-verify-on-apply): выбор пресета — без сборок; цели сверки в verify_db ----------

// pdNoBuild — ни сборки, ни версии, ни разделения версий, ни удалений.
func pdNoBuild(t *testing.T, f *pdVf) {
	t.Helper()
	if len(f.ids) != 0 || len(f.promoted) != 0 || f.trackN != 0 {
		t.Errorf("созданы версии %v (VariantToTrack %d раз), want ни одной", f.ids, f.trackN)
	}
	if len(f.splitAsks) != 0 {
		t.Errorf("разделение версий %v, want ни одного", f.splitAsks)
	}
	if len(f.ups) != 0 || len(f.created) != 0 {
		t.Errorf("загрузки %d / файлы сборки %v, want ни одного (пересборки нет)", len(f.ups), f.created)
	}
	if len(f.calls) != 0 || f.masterN != 0 {
		t.Errorf("движок вызван %d раз, мастер %d, want 0 (пересборки нет)", len(f.calls), f.masterN)
	}
	if len(f.deleted) != 0 {
		t.Errorf("удаления %v, want ни одного", f.deleted)
	}
}

// Пресет с целями (гитара с движком и бас, level_db −10), у версий был бы промах +3,6 — черновик не собирает:
// гитара — level_target −10, level_total, Db как в записи (0), verify_db −10; бас — Db по замеру дорожки,
// verify_db −10. Журнала нет, пресет не меняется.
func TestPresetDraftNoBuildsVerifyDbOnTargets(t *testing.T) {
	t.Parallel()
	f := pdVfSetup(t, map[int64]float64{vfFirst: 3.6, vfFirst + 1: 1.6})
	rec := &jrRec{Service: f}
	p := vfPreset(81)
	before := pdJSON(t, p)

	res := pdMustDraft(t, yue.Journaled(rec), p)

	pdNoBuild(t, f)
	if len(rec.recs) != 0 {
		t.Errorf("записи журнала %+v, want ни одной (трек не меняется)", rec.recs)
	}
	if after := pdJSON(t, p); after != before {
		t.Errorf("пресет изменён вызовом:\n было %s\nстало %s", before, after)
	}
	if len(res.Sections) != 2 {
		t.Fatalf("sections %+v, want 2 (гитара, бас)", res.Sections)
	}
	g := res.Sections[0]
	if !reflect.DeepEqual(g.Stems, []string{"guitar"}) || !reflect.DeepEqual(g.Engine, engChain()) {
		t.Fatalf("sections[0] = %+v, want движок на гитару", g)
	}
	if g.LevelTarget == nil {
		t.Fatalf("гитара без level_target: %+v (цель с движком считается по обработанному куску)", g)
	}
	pdNear(t, "гитара level_target", *g.LevelTarget, vfTarget, 1e-9)
	pdNear(t, "гитара level_total", g.LevelTotal, prlTrackSum(), 0.01)
	pdNear(t, "гитара Db (без поправок сверки)", g.Db, 0, 1e-9)
	if g.VerifyDb == nil {
		t.Errorf("гитара без verify_db, want −10 (level_db + db)")
	} else {
		pdNear(t, "гитара verify_db", *g.VerifyDb, vfTarget, 1e-9)
	}
	b := res.Sections[1]
	if !reflect.DeepEqual(b.Stems, []string{"bass"}) || b.LevelTarget != nil {
		t.Fatalf("sections[1] = %+v, want бас без level_target (цель без движка — в Db)", b)
	}
	pdNear(t, "бас Db (по замеру дорожки)", b.Db, vfTarget-(prlSineDb(0.05)-prlTrackSum()), 0.01)
	if b.VerifyDb == nil {
		t.Errorf("бас без verify_db, want −10")
	} else {
		pdNear(t, "бас verify_db", *b.VerifyDb, vfTarget, 1e-9)
	}
}

// Громкость «на этот раз» db +2 у гитары: Db +2 как в записи, verify_db = −10 + 2 = −8; без сборок.
func TestPresetDraftVerifyDbIncludesDb(t *testing.T) {
	t.Parallel()
	f := pdVfSetup(t, map[int64]float64{vfFirst: 2.5})
	res := pdMustDraft(t, f, vfPresetDb(82, 2))
	pdNoBuild(t, f)
	g := res.Sections[0]
	pdNear(t, "гитара Db", g.Db, 2, 1e-9)
	if g.VerifyDb == nil {
		t.Fatalf("гитара без verify_db, want −8")
	}
	pdNear(t, "гитара verify_db", *g.VerifyDb, -8, 1e-9)
}

// Цель — только у записи с level_db на одну дорожку с замером: без level_db, на две дорожки, у дорожки без
// замера — поля verify_db нет.
func TestPresetDraftVerifyDbOnlyMeasurableSingleStem(t *testing.T) {
	t.Parallel()
	f := pdVfSetup(t, nil)
	delete(f.p95, "guitar") // у гитары трека замера нет
	p := lvPreset(86,
		yue.PresetSpec{Stems: []string{"bass"}, LevelDb: lvF(-10)},                       // цель
		yue.PresetSpec{Stems: []string{"vocals", "other"}, LevelDb: lvF(-12)},            // две дорожки
		yue.PresetSpec{Stems: []string{"guitar"}, Engine: engChain(), LevelDb: lvF(-10)}, // без замера
		psVocalCut(), // без level_db
		yue.PresetSpec{Stems: []string{"drums"}, Db: -2}, // громкость без цели
	)
	res := pdMustDraft(t, f, p)
	pdNoBuild(t, f)
	if len(res.Sections) != 5 {
		t.Fatalf("sections %d, want 5: %+v", len(res.Sections), res.Sections)
	}
	if v := res.Sections[0].VerifyDb; v == nil || math.Abs(*v-(-10)) > 1e-9 {
		t.Errorf("бас verify_db %v, want −10", deref(v))
	}
	for i, what := range []string{"две дорожки", "дорожка без замера", "без level_db", "громкость без цели"} {
		if v := res.Sections[i+1].VerifyDb; v != nil {
			t.Errorf("sections[%d] (%s) verify_db %v, want нет поля", i+1, what, *v)
		}
	}
}

// ---------- ТК11: level_target / level_total — JSON туда-обратно ----------

func TestSectionSpecLevelJSONRoundTrip(t *testing.T) {
	t.Parallel()
	target := -9.5
	s := SectionSpec{Stems: []string{"guitar"}, Engine: engChain(), Db: -1.5, LevelTarget: &target, LevelTotal: -19.25}
	raw := pdJSON(t, s)
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	if m["level_target"] != -9.5 || m["level_total"] != -19.25 {
		t.Errorf("JSON %s, want level_target −9,5 и level_total −19,25", raw)
	}
	var back SectionSpec
	if err := json.Unmarshal([]byte(raw), &back); err != nil {
		t.Fatal(err)
	}
	if back.LevelTarget == nil || *back.LevelTarget != target || back.LevelTotal != -19.25 {
		t.Errorf("обратно: level_target %v level_total %v, want −9,5 / −19,25", back.LevelTarget, back.LevelTotal)
	}
	// через ответ черновика — тоже
	var r PresetDraftResult
	if err := json.Unmarshal([]byte(pdJSON(t, PresetDraftResult{Sections: []SectionSpec{s}})), &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Sections) != 1 || r.Sections[0].LevelTarget == nil || *r.Sections[0].LevelTarget != target ||
		r.Sections[0].LevelTotal != -19.25 {
		t.Errorf("ответ черновика обратно: %+v", r.Sections)
	}
	// пусто — полей нет
	plain := pdJSON(t, SectionSpec{Stems: []string{"bass"}, Db: 2})
	if strings.Contains(plain, "level_target") || strings.Contains(plain, "level_total") {
		t.Errorf("JSON без цели %s, want без level_target/level_total", plain)
	}
}

// Запись из JSON с level_target/level_total пересборка учитывает как раньше: обработанный кусок (+6 дБ к
// исходной) ставится к цели −10 по своему замеру, а не по исходной дорожке (было бы ≈ −4).
func TestSectionSpecLevelJSONRebuild(t *testing.T) {
	t.Parallel()
	f, _ := prlSetup(t)
	const target = -10.0
	eng, _ := json.Marshal(engChain())
	src := fmt.Sprintf(`{"child_id":0,"from":0,"to":0,"stems":["bass"],"db":0,"engine":%s,"level_target":%g,"level_total":%g}`,
		eng, target, prlTrackSum())
	var s SectionSpec
	if err := json.Unmarshal([]byte(src), &s); err != nil {
		t.Fatalf("запись: %v", err)
	}
	if _, err := RebuildSections(context.Background(), f, psJobID, []SectionSpec{s}); err != nil {
		t.Fatalf("RebuildSections: %v", err)
	}
	name := onlyUpload(t, f.secFake)
	g := prlInsertGain(t, decodeBytes(t, f.uploads[name]))
	got := prlSineDb(prlChunkAmp) + g - prlTrackSum()
	if math.Abs(got-target) > 0.5 {
		t.Errorf("обработанный бас к треку %.2f дБ (гейн вставки %+.2f), want цель %.1f ±0,5", got, g, target)
	}
}

// ---------- ТК1: пресет с финалом и мастером — пробных вариантов нет вовсе ----------

// pdVfPresetFull — пресет сверки (гитара с движком и бас, цели −10) с финалом и мастером.
func pdVfPresetFull(id int64) yue.SoundPreset {
	p := vfPreset(id)
	p.Final = psFinal()
	p.Master = []map[string]any{msGlue()}
	p.TargetLUFS = psTarget(-13)
	return p
}

// Пресет с финалом и мастером и с целями — черновик не создаёт у трека ни одного файла и ничего не удаляет.
func TestPresetDraftFullPresetNoTrialVariants(t *testing.T) {
	t.Parallel()
	f := pdVfSetup(t, map[int64]float64{vfFirst: 3.6, vfFirst + 1: 1.6})
	if _, err := pdDraft(f, pdVfPresetFull(91)); err != nil {
		t.Fatalf("PresetDraft: %v", err)
	}
	pdNoBuild(t, f)
}

// Без целей level_db сборки нет — и удалять варианты нечего: DspVariantDelete не зовётся.
func TestPresetDraftNoTargetsNoVariantDelete(t *testing.T) {
	t.Parallel()
	f := psSetup(t, nil)
	p := psPreset(94, []yue.PresetSpec{psVocalCut(), {Stems: []string{"bass"}, Db: -2}}, psFinal())
	p.Master = []map[string]any{msGlue()}
	p.TargetLUFS = psTarget(-13)
	pdMustDraft(t, f, p)
	for _, d := range f.deleted {
		if !strings.HasPrefix(d, "job ") {
			t.Errorf("DspVariantDelete(%s) без целей level_db, want ни одного", d)
		}
	}
}

func slicesHas(list []string, ok func(string) bool) bool {
	for _, s := range list {
		if ok(s) {
			return true
		}
	}
	return false
}
