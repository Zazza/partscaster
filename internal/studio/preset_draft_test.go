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

// Пресет звука в черновик (карточка internal-studio-preset-draft, условия 4–8; тест-кейсы ТК8–ТК11).
//
// PresetDraft(ctx, svc, jobID, p) → {sections, final, master, warnings}: готовит всё, что готовит применение
// (дорожки, план для гитариста, части барабанов только движком, партии, наборы, замер уровней), но трек не
// меняет: итоговой версии нет, журнала нет, пресет не меняется. Сверка баланса — как у ApplySoundPreset, на
// временных версиях, которые к концу (успех, ошибка, отмена) удалены. Нет целей level_db, которые можно
// сверить, — без сборки вообще.
//
// Воркер — фейки соседних тестов пресета (psFake, ppFake, gpFake, vfFake, opFake): только внешние границы.
// Толкования и допущения:
//   - «временная версия» — версия-трек на воркере (VariantToTrack); «удалена» — DeleteJob с её id (так же
//     удаляет версии отмена ApplySoundPreset). Сверке нужно разделение версии на дорожки, а оно бывает только
//     у джобы воркера — поэтому временная версия и есть версия-трек;
//   - «без рендера» — ни одного вызова движка (ApplyFx), ни одной загрузки варианта (UploadDsp);
//   - «пресет на воркере не меняется» — методов записи пресета у фейков нет (вызов уронил бы тест паникой),
//     плюс сам аргумент p после вызова тот же;
//   - слой «живой звук» выключен (withoutLiveLayer), как в прочих тестах механики пресета: его записи
//     проверяются своими тестами; поэтому sections = записи пресета + партии;
//   - при ошибке исправляющего прохода (ТК10) ответ (ошибка или прежние поправки) карточкой не задан — тест
//     проверяет только уборку временных версий.

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

// ---------- ТК9: сверка с промахом — поправки в Db ответа, временные версии удалены, журнала нет ----------

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

// pdAllDeleted — каждая созданная версия удалена; сам трек — нет.
func pdAllDeleted(t *testing.T, ids []int64, deleted []string) {
	t.Helper()
	for _, id := range ids {
		if !contains(deleted, fmt.Sprintf("job %d", id)) {
			t.Errorf("временная версия #%d не удалена; созданы %v, удаления %v", id, ids, deleted)
		}
	}
	if contains(deleted, fmt.Sprintf("job %d", psJobID)) {
		t.Errorf("удалён сам трек #%d: %v", psJobID, deleted)
	}
}

// pdNear — |got − want| ≤ tol.
func pdNear(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %.3f, want %.3f ±%g", what, got, want, tol)
	}
}

// Гитара (движок, цель −10) промахивается +3,6, после поправки +1,6, затем у цели; бас (без движка, цель −10)
// у цели. Как у применения: два исправляющих прохода, поправка гитары −3,6 − 1,6 = −5,2 — в Db ответа;
// гитара — level_target −10 и level_total (громкость трека); бас — Db = цель − уровень дорожки по замеру трека.
// Все временные версии удалены, итоговой нет, журнал пуст.
func TestPresetDraftVerifyCorrectsAndCleansUp(t *testing.T) {
	t.Parallel()
	f := pdVfSetup(t, map[int64]float64{vfFirst: 3.6, vfFirst + 1: 1.6})
	rec := &jrRec{Service: f}
	p := vfPreset(81)
	before := pdJSON(t, p)

	res := pdMustDraft(t, yue.Journaled(rec), p)

	if !vfAsked(f.splitAsks, vfFirst) || !vfAsked(f.splitAsks, vfFirst+1) {
		t.Errorf("сверка не дошла до второй сборки: RoFormer просили у %v, want #%d и #%d", f.splitAsks, vfFirst, vfFirst+1)
	}
	if len(f.ids) == 0 {
		t.Fatal("временных версий нет — сверке нечего было разделять")
	}
	pdAllDeleted(t, f.ids, f.deleted)
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
	pdNear(t, "гитара Db (поправки сверки)", g.Db, -5.2, 0.01)
	b := res.Sections[1]
	if !reflect.DeepEqual(b.Stems, []string{"bass"}) || b.LevelTarget != nil {
		t.Fatalf("sections[1] = %+v, want бас без level_target (цель без движка — в Db)", b)
	}
	pdNear(t, "бас Db", b.Db, vfTarget-(prlSineDb(0.05)-prlTrackSum()), 0.01)
}

// Громкость «на этот раз» db +2 у гитары: цель сверки −10 + 2 = −8; гитара первой сборки на −7,5 (в пределах
// 1 дБ) — поправок нет, Db гитары как в записи (+2); временная версия удалена.
func TestPresetDraftVerifyOnTarget(t *testing.T) {
	t.Parallel()
	f := pdVfSetup(t, map[int64]float64{vfFirst: 2.5})
	res := pdMustDraft(t, f, vfPresetDb(82, 2))
	if !vfAsked(f.splitAsks, vfFirst) {
		t.Errorf("первая сборка не сверена: RoFormer просили у %v", f.splitAsks)
	}
	if len(f.ids) == 0 {
		t.Fatal("временных версий нет")
	}
	pdAllDeleted(t, f.ids, f.deleted)
	pdNear(t, "гитара Db", res.Sections[0].Db, 2, 0.01)
}

// Сбой разделения временной версии — без поправок (как у применения), ответ без ошибки, версия удалена.
func TestPresetDraftVerifySplitErrorNoCorrection(t *testing.T) {
	t.Parallel()
	f := pdVfSetup(t, map[int64]float64{vfFirst: 3.6})
	f.splitFail[vfFirst] = true
	res := pdMustDraft(t, f, vfPreset(83))
	if len(f.ids) == 0 {
		t.Fatal("временных версий нет")
	}
	pdAllDeleted(t, f.ids, f.deleted)
	if len(res.Sections) == 0 {
		t.Fatal("sections пусто")
	}
	pdNear(t, "гитара Db (поправок нет)", res.Sections[0].Db, 0, 0.01)
	if res.Sections[0].LevelTarget == nil {
		t.Errorf("гитара без level_target: %+v", res.Sections[0])
	}
}

// ---------- ТК10: ошибка / отмена после временной версии — версия удалена ----------

func TestPresetDraftFailureAfterTempVersionCleansUp(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		guitar, trk int
	}{
		{"пересборка исправляющего прохода упала", 2, 0},
		{"версия исправляющего прохода не создана", 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := pdVfSetup(t, map[int64]float64{vfFirst: 3.6})
			f.failGuitarAt, f.failTrackAt = tc.guitar, tc.trk
			res, err := pdDraft(f, vfPreset(84))
			if err == nil && res == nil {
				t.Fatal("PresetDraft: nil без ошибки")
			}
			if len(f.ids) == 0 {
				t.Fatal("сценарий: первая временная версия не создана")
			}
			if tc.guitar > 0 && f.guitarN < tc.guitar {
				t.Fatalf("сценарий: исправляющей пересборки не было (движок на гитару %d раз)", f.guitarN)
			}
			if tc.trk > 0 && f.trackN < tc.trk {
				t.Fatalf("сценарий: второй версии не просили (VariantToTrack %d раз)", f.trackN)
			}
			pdAllDeleted(t, f.ids, f.deleted)
		})
	}
}

// pdVfQueue — pdVf + очередь воркера: метка отмены появляется, как только создана временная версия.
type pdVfQueue struct {
	*pdVf
	q *opFake
}

func (f *pdVfQueue) QueueAdd(ctx context.Context, in yue.QueueOpIn) (*yue.QueueOp, error) {
	op, err := f.q.QueueAdd(ctx, in)
	if op != nil {
		op.State = "running"
	}
	return op, err
}

func (f *pdVfQueue) QueueBeat(ctx context.Context, id int64, progress ...yue.QueueProgress) (*yue.QueueOp, error) {
	return f.q.QueueBeat(ctx, id, progress...)
}

func (f *pdVfQueue) QueueFinish(ctx context.Context, id int64, in yue.QueueFinishIn) (*yue.QueueOp, error) {
	return f.q.QueueFinish(ctx, id, in)
}

func TestPresetDraftCanceledAfterTempVersionCleansUp(t *testing.T) {
	t.Parallel()
	f := &pdVfQueue{pdVf: pdVfSetup(t, map[int64]float64{vfFirst: 3.6}), q: newOpFake()}
	f.q.beat = func(int) (*yue.QueueOp, error) {
		return &yue.QueueOp{ID: fakeOpID, State: "running", CancelRequested: f.made.Load()}, nil
	}
	_, err := pdDraft(f, vfPreset(85))
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("PresetDraft err = %v, want ErrCanceled", err)
	}
	if len(f.ids) == 0 {
		t.Fatal("сценарий: временная версия не создана до отмены")
	}
	pdAllDeleted(t, f.ids, f.deleted)
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

// ---------- ТК13: пробные варианты у трека удалены (условие 20) ----------
//
// Толкование: «файл, созданный пробной сборкой» — файл, который воркер вернул на запрос сборки у самого трека
// psJobID: загрузка (микс пересборки, финал) или вариант/мастер движка; превью кусков движка (preview-*) —
// вход пересборки, а не итог, в счёт не идут. «Удалён» — DspVariantDelete(psJobID, file), фейк пишет
// "id/file" в deleted. Файлы временных версий уходят вместе с версиями (DeleteJob) — их проверяет ТК9/ТК10.

// pdVfPresetFull — пресет сверки (гитара с движком и бас, цели −10) с финалом и мастером: пробная сборка
// даёт микс пересборки, финал и мастер.
func pdVfPresetFull(id int64) yue.SoundPreset {
	p := vfPreset(id)
	p.Final = psFinal()
	p.Master = []map[string]any{msGlue()}
	p.TargetLUFS = psTarget(-13)
	return p
}

// pdVariantsDeleted — каждый файл, созданный сборками у трека, удалён; файл самого трека — нет.
func pdVariantsDeleted(t *testing.T, f *pdVf) {
	t.Helper()
	if len(f.created) == 0 {
		t.Fatal("сценарий: пробные сборки не создали у трека ни одного файла")
	}
	for _, k := range f.created {
		if !contains(f.deleted, k) {
			t.Errorf("пробный файл %s не удалён; созданы %v, удаления %v", k, f.created, f.deleted)
		}
	}
	if k := key(psJobID, "audio.flac"); contains(f.deleted, k) {
		t.Errorf("удалён файл самого трека %s: %v", k, f.deleted)
	}
}

func TestPresetDraftTrialVariantsDeletedOnSuccess(t *testing.T) {
	t.Parallel()
	f := pdVfSetup(t, map[int64]float64{vfFirst: 3.6, vfFirst + 1: 1.6})
	if _, err := pdDraft(f, pdVfPresetFull(91)); err != nil {
		t.Fatalf("PresetDraft: %v", err)
	}
	if !vfAsked(f.splitAsks, vfFirst+1) {
		t.Fatalf("сценарий: второй сборки не было (RoFormer просили у %v)", f.splitAsks)
	}
	if f.masterN == 0 {
		t.Errorf("сценарий: мастер пресета в пробной сборке не вызван")
	}
	pdVariantsDeleted(t, f)
}

// Сбой второй сборки: пересборка исправляющего прохода упала / версия не создана.
func TestPresetDraftTrialVariantsDeletedOnSecondBuildFailure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		guitar, trk int
	}{
		{"пересборка второй сборки упала", 2, 0},
		{"версия второй сборки не создана", 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := pdVfSetup(t, map[int64]float64{vfFirst: 3.6})
			f.failGuitarAt, f.failTrackAt = tc.guitar, tc.trk
			res, err := pdDraft(f, pdVfPresetFull(92))
			if err == nil && res == nil {
				t.Fatal("PresetDraft: nil без ошибки")
			}
			if tc.guitar > 0 && f.guitarN < tc.guitar {
				t.Fatalf("сценарий: второй пересборки не было (движок на гитару %d раз)", f.guitarN)
			}
			if tc.trk > 0 && f.trackN < tc.trk {
				t.Fatalf("сценарий: второй версии не просили (VariantToTrack %d раз)", f.trackN)
			}
			pdVariantsDeleted(t, f)
		})
	}
}

func TestPresetDraftTrialVariantsDeletedOnCancel(t *testing.T) {
	t.Parallel()
	f := &pdVfQueue{pdVf: pdVfSetup(t, map[int64]float64{vfFirst: 3.6}), q: newOpFake()}
	f.q.beat = func(int) (*yue.QueueOp, error) {
		return &yue.QueueOp{ID: fakeOpID, State: "running", CancelRequested: f.made.Load()}, nil
	}
	_, err := pdDraft(f, pdVfPresetFull(93))
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("PresetDraft err = %v, want ErrCanceled", err)
	}
	pdVariantsDeleted(t, f.pdVf)
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
