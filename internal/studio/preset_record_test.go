package studio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"partscaster/internal/dsp"
	"partscaster/internal/yue"
)

// Запись-пресет черновика и сборка (карточка internal-studio-instant-apply, условия 4, 7, 8, 9, 10;
// тест-кейсы ТК5–ТК9).
//
// Контракт:
//
//	SectionSpec.ID     string     `json:"id,omitempty"`     — id правки черновика;
//	SectionSpec.Preset *PresetRef `json:"preset,omitempty"` — запись «пресет звука»: только она (без Stems/Engine…);
//	PresetRef{Preset yue.SoundPreset; SkipSound, SkipPlace []string; SkipMaster bool};
//	RebuildResult.Warnings []string `json:"warnings,omitempty"` — предупреждения подготовки пресетов;
//	EditError{ID string; Err error}: Error() = Err.Error() + " {edit:" + ID + "}", Unwrap().
//
// Сборка (RebuildSections / ApplyEdits) раскрывает запись-пресет подготовкой — как применение пресета:
// недостающие дорожки (разделение), наборы сэмплов, партии по аккордам, замер уровней, финал как запись на весь
// микс, мастер как запись мастера. Записи пресета со звуком (Chain/Steps/Engine, не Add) на дорожках SkipSound
// не идут, записи «только место» на дорожках SkipPlace не идут, мастер пресета не идёт при SkipMaster.
// Сбой движка/эффекта/мастера записи — *EditError с ID этой записи (у раскрытых записей пресета — ID пресета).
// Сборка ставит недостающие наборы сэмплов цепочек движка до расчёта.
//
// Воркер — фейки соседних тестов (psFake, ppFake, gpFake, msPresetFake, pdVf/aeFake, msFake, engFake):
// только внешние границы.
//
// Толкования:
//   - слой «живой звук» выключен (withoutLiveLayer), как во всех тестах механики пресета: его записи
//     проверяются своими тестами;
//   - «пропущена запись пресета» видно по вызовам движка (на дорожке нет цепочки пресета) и по правилам
//     пересборки, которые не терпят двух записей «место» на одной дорожке и двух мастеров: если пропуск не
//     сработал, сборка падает;
//   - ошибка правки отдаётся указателем *EditError (errors.As с *EditError).

// prRecord — запись-пресет черновика с id правки.
func prRecord(id string, p yue.SoundPreset) SectionSpec {
	return SectionSpec{ID: id, Preset: &PresetRef{Preset: p}}
}

func prCtx() context.Context { return withoutLiveLayer(context.Background()) }

func prRebuild(t *testing.T, svc yue.Service, specs ...SectionSpec) *RebuildResult {
	t.Helper()
	res, err := RebuildSections(prCtx(), svc, psJobID, specs)
	if err != nil {
		t.Fatalf("RebuildSections: %v", err)
	}
	if res == nil {
		t.Fatal("RebuildSections: nil без ошибки")
	}
	return res
}

// prEditID — id правки из ошибки сборки (*EditError в цепочке); нет — тест падает.
func prEditID(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("ошибки нет, want сбой правки")
	}
	var ee *EditError
	if !errors.As(err, &ee) {
		t.Fatalf("ошибка %q без *EditError (какая правка — не отмечено)", err)
	}
	return ee.ID
}

// prSourceCalls — цепочки движка по дорожке (source) в порядке вызовов.
func prSourceCalls(calls []engCall, source string) [][]map[string]any {
	var out [][]map[string]any
	for _, c := range calls {
		if c.req.Source == source {
			out = append(out, c.req.Chain)
		}
	}
	return out
}

// prExpanded — запись-пресет раскрыта: движок пресета на басе посчитан (у тестов с записью bass+psAmpEngine).
func prExpanded(t *testing.T, f *psFake) {
	t.Helper()
	if bass := prSourceCalls(f.calls, "bass"); len(bass) != 1 || !reflect.DeepEqual(bass[0], psAmpEngine()) {
		t.Fatalf("движок пресета на басе %v, want один вызов (запись-пресет раскрыта)", bass)
	}
}

// ---------- ТК5: запись-пресет раскрывается подготовкой при сборке ----------

// Пресет с записью на дорожку, которой у трека нет (гитара), движком на бас, эффектом на голос и партиями:
// сборка разделяет трек, ставит наборы партий, считает партии по аккордам (ноты), движок баса — цепочкой пресета;
// микс собран.
func TestRebuildPresetRecordPrepares(t *testing.T) {
	t.Parallel()
	f := ppSetup(t, 8, ppBars())
	f.afterSplit = map[string]string{"stem-guitar.flac": lavfi(t, aeval("0.2*sin(2*PI*700*t)", 8),
		filepath.Join(t.TempDir(), "pr-guitar.flac"))}
	p := ppPreset(t, 71, ppPartsPadPerc)
	p.Specs = []yue.PresetSpec{
		psVocalCut(),
		{Stems: []string{"bass"}, Engine: psAmpEngine(), Db: -2},
		{Stems: []string{"guitar"}, Chain: "eq", Params: map[string]float64{"low": 2}},
	}

	res := prRebuild(t, f, prRecord("-900", p))

	if len(f.splits) != 1 {
		t.Errorf("разделения %v, want одно (у трека нет гитары)", f.splits)
	}
	for _, kit := range []string{"pianokit", "clapkit"} {
		if !contains(f.installs, kit) {
			t.Errorf("набор %s не поставлен (installs %v)", kit, f.installs)
		}
	}
	if bass := prSourceCalls(f.calls, "bass"); len(bass) != 1 || !reflect.DeepEqual(bass[0], psAmpEngine()) {
		t.Errorf("движок на бас %v, want один вызов с цепочкой пресета %v", bass, psAmpEngine())
	}
	for _, kind := range []string{"synth", "perc"} {
		c := ppCall(t, f, kind)
		if c == nil {
			t.Errorf("партия %s не посчитана движком", kind)
			continue
		}
		if n := reflect.ValueOf(c.Chain[0]["notes"]); !n.IsValid() || n.Kind() != reflect.Slice || n.Len() == 0 {
			t.Errorf("партия %s без нот (по аккордам не построена): %v", kind, c.Chain[0]["notes"])
		}
	}
	// наборы ставятся до расчёта: ни одного вызова движка раньше установки
	first := -1
	for i, v := range f.log {
		if strings.HasPrefix(v, "applyfx:") {
			first = i
			break
		}
	}
	for _, kit := range []string{"pianokit", "clapkit"} {
		if i := psIndex(f.log, "install:"+kit); i < 0 || first < 0 || i > first {
			t.Errorf("порядок %v, want набор %s поставлен до расчёта движком", f.log, kit)
		}
	}
	if res.Variant == nil || res.Variant.File == "" {
		t.Errorf("результат %+v, want файл микса", res)
	}
	if len(f.promoted) != 0 {
		t.Errorf("созданы версии %+v, want ни одной (пересборка для прослушивания)", f.promoted)
	}
}

// Финал пресета (громкость −6) — на весь микс; мастер пресета (glue + цель −13) — мастером на воркере.
func TestRebuildPresetRecordFinalAndMaster(t *testing.T) {
	t.Parallel()
	f := msPresetSetup(t)
	p := psPreset(72, []yue.PresetSpec{psVocalCut()},
		[]yue.PresetStep{{Chain: "level", Params: map[string]float64{"gain": -6, "ceiling": -1}}})
	p.Master = []map[string]any{msGlue()}
	p.TargetLUFS = psTarget(-13)

	prRebuild(t, f, prRecord("-900", p))

	m := msOneMaster(t, f)
	if m.Source != "mix" {
		t.Errorf("мастер source %q, want mix", m.Source)
	}
	if len(m.Chain) != 2 || !reflect.DeepEqual(m.Chain[0], msGlue()) {
		t.Fatalf("цепочка мастера %v, want [glue, limiter −13]", m.Chain)
	}
	msCheckLimiter(t, m.Chain[1], -13)
	data, ok := f.uploads["overdub-inst-0.flac"]
	if !ok {
		t.Fatalf("микс не загружен: %v", keys(f.uploads))
	}
	out := decodeBytes(t, data)
	// «прочее» 500 Гц эффект на голос не трогает: в миксе оно ниже исходного на гейн финала
	if d := psDb(toneAmp(out, 500, 2, 8), 0.3); math.Abs(d-(-6)) > 1 {
		t.Errorf("500 Гц в миксе %+.2f дБ к исходному, want −6 ±1 (финал пресета на весь микс)", d)
	}
}

// Без финала и мастера у пресета — мастера нет вовсе.
func TestRebuildPresetRecordNoMasterNoMasterCall(t *testing.T) {
	t.Parallel()
	f := msPresetSetup(t)
	prRebuild(t, f, prRecord("-900", psPreset(73, []yue.PresetSpec{{Stems: []string{"bass"}, Engine: psAmpEngine()}}, nil)))
	prExpanded(t, f.psFake)
	if len(f.master) != 0 {
		t.Errorf("мастер вызван %d раз у пресета без мастера, want 0", len(f.master))
	}
}

// Звук человека на басе перекрывает звук пресета на басе: движок пресета на басе не идёт, движок пресета на
// «прочем» идёт, движок человека на басе — свой.
func TestRebuildPresetRecordSkipSound(t *testing.T) {
	t.Parallel()
	f := psSetup(t, nil)
	p := psPreset(74, []yue.PresetSpec{
		{Stems: []string{"bass"}, Engine: psAmpEngine()},
		{Stems: []string{"other"}, Engine: psAmpEngine()},
	}, nil)
	rec := prRecord("-900", p)
	rec.Preset.SkipSound = []string{"bass"}
	own := SectionSpec{ID: "-5", Stems: []string{"bass"}, Engine: engChain()}

	prRebuild(t, f, rec, own)

	if bass := prSourceCalls(f.calls, "bass"); len(bass) != 1 || !reflect.DeepEqual(bass[0], engChain()) {
		t.Errorf("движок на бас %v, want один вызов — цепочка человека %v", bass, engChain())
	}
	if other := prSourceCalls(f.calls, "other"); len(other) != 1 || !reflect.DeepEqual(other[0], psAmpEngine()) {
		t.Errorf("движок на «прочее» %v, want цепочка пресета (не перекрыта)", other)
	}
}

// Без перекрытия — оба движка на басе (контроль: SkipSound на другой дорожке бас не трогает).
func TestRebuildPresetRecordSkipSoundOtherStemKeepsPreset(t *testing.T) {
	t.Parallel()
	f := psSetup(t, nil)
	rec := prRecord("-900", psPreset(75, []yue.PresetSpec{{Stems: []string{"bass"}, Engine: psAmpEngine()}}, nil))
	rec.Preset.SkipSound = []string{"vocals"}
	prRebuild(t, f, rec)
	if bass := prSourceCalls(f.calls, "bass"); len(bass) != 1 || !reflect.DeepEqual(bass[0], psAmpEngine()) {
		t.Errorf("движок на бас %v, want цепочка пресета", bass)
	}
}

// Место человека на «прочем» перекрывает «место» пресета на «прочем» (иначе — две записи места на одной дорожке,
// сборка падает); звук пресета на той же дорожке остаётся.
func TestRebuildPresetRecordSkipPlace(t *testing.T) {
	t.Parallel()
	f := psSetup(t, nil)
	p := psPreset(76, []yue.PresetSpec{
		{Stems: []string{"other"}, Place: &yue.Place{Pan: 0.4, Width: 1}},
		{Stems: []string{"other"}, Engine: psAmpEngine()},
	}, nil)
	rec := prRecord("-900", p)
	rec.Preset.SkipPlace = []string{"other"}
	own := SectionSpec{ID: "-6", Stems: []string{"other"}, Place: &yue.Place{Pan: -0.5, Width: 1}}

	prRebuild(t, f, rec, own)

	if other := prSourceCalls(f.calls, "other"); len(other) != 1 || !reflect.DeepEqual(other[0], psAmpEngine()) {
		t.Errorf("движок пресета на «прочем» %v, want идёт (место звук не перекрывает)", other)
	}
}

// Контроль к SkipPlace: без перекрытия две записи «место» на одной дорожке — ошибка сборки.
func TestRebuildPresetRecordPlaceNotSkippedConflicts(t *testing.T) {
	t.Parallel()
	f := psSetup(t, nil)
	rec := prRecord("-900", psPreset(77, []yue.PresetSpec{{Stems: []string{"other"}, Place: &yue.Place{Pan: 0.4, Width: 1}}}, nil))
	own := SectionSpec{ID: "-6", Stems: []string{"other"}, Place: &yue.Place{Pan: -0.5, Width: 1}}
	if _, err := RebuildSections(prCtx(), f, psJobID, []SectionSpec{rec, own}); err == nil {
		t.Fatal("два «места» на «прочем» без перекрытия: want ошибку (контроль теста)")
	}
}

// Свой мастер при SkipMaster — мастер пресета не идёт: один вызов мастера, цепочка человека.
func TestRebuildPresetRecordSkipMaster(t *testing.T) {
	t.Parallel()
	f := msPresetSetup(t)
	p := psPreset(78, []yue.PresetSpec{{Stems: []string{"bass"}, Engine: psAmpEngine()}}, nil)
	p.Master = []map[string]any{msGlue()}
	p.TargetLUFS = psTarget(-13)
	rec := prRecord("-900", p)
	rec.Preset.SkipMaster = true
	own := SectionSpec{ID: "-7", Master: true, Engine: msChain()}

	prRebuild(t, f, rec, own)

	prExpanded(t, f.psFake)
	m := msOneMaster(t, f)
	if !reflect.DeepEqual(m.Chain, msChain()) {
		t.Errorf("цепочка мастера %v, want мастер человека %v", m.Chain, msChain())
	}
}

// SkipMaster без своего мастера — мастера нет вовсе.
func TestRebuildPresetRecordSkipMasterAlone(t *testing.T) {
	t.Parallel()
	f := msPresetSetup(t)
	p := psPreset(79, []yue.PresetSpec{{Stems: []string{"bass"}, Engine: psAmpEngine()}}, nil)
	p.Master = []map[string]any{msGlue()}
	rec := prRecord("-900", p)
	rec.Preset.SkipMaster = true
	prRebuild(t, f, rec)
	prExpanded(t, f.psFake)
	if len(f.master) != 0 {
		t.Errorf("мастер вызван %d раз, want 0 (мастер пресета пропущен)", len(f.master))
	}
}

// Замер: пресет с целями level_db — гейны пересборки те же, что у первого прохода применения пресета сразу.
func TestRebuildPresetRecordLevelsAsDirect(t *testing.T) {
	t.Parallel()
	direct, origD := vfSetup(t, nil)
	vfApply(t, direct, vfPreset(80))
	dp := direct.passes(t, origD)
	if len(dp) == 0 {
		t.Fatal("сценарий: применение сразу не собрало ни одного прохода")
	}

	f, orig := vfSetup(t, nil)
	prRebuild(t, f, prRecord("-900", vfPreset(80)))
	ps := prPasses(t, f, orig)
	if len(ps) != 1 {
		t.Fatalf("пересборок %d, want одна (прослушивание без сверки)", len(ps))
	}
	pdNear(t, "гитара пересборки к применению сразу", ps[0].guitar, dp[0].guitar, 0.1)
	pdNear(t, "бас пересборки к применению сразу", ps[0].bass, dp[0].bass, 0.1)
	if len(f.splitAsks) != 0 || len(f.promoted) != 0 {
		t.Errorf("разделение версий %v / версии %v, want ни одного (прослушивание не сверяет)", f.splitAsks, f.promoted)
	}
}

// prPasses — гейны сборок, как vfFake.passes, но считая и микс пересборки черновика (overdub-inst-N.flac):
// vfFake.passes видит только загрузки с «mix» в имени (сборки применения пресета).
func prPasses(t *testing.T, f *vfFake, orig []float32) []vfPass {
	t.Helper()
	var out []vfPass
	for _, u := range f.ups {
		if !strings.Contains(u.name, "mix") && !strings.Contains(u.name, "overdub-inst") {
			continue
		}
		s := decodeBytes(t, u.data)
		out = append(out, vfPass{
			guitar: 20 * math.Log10(toneAmp(s, prlChunkHz, 2, 8)/prlChunkAmp),
			bass:   20 * math.Log10(toneAmp(s, 200, 2, 8)/toneAmp(orig, 200, 2, 8)),
		})
	}
	return out
}

// ---------- ТК6: сбой правки — ошибка с id правки ----------

func TestEditErrorFormat(t *testing.T) {
	t.Parallel()
	inner := errors.New("движок: нет памяти")
	var err error = &EditError{ID: "-12", Err: inner}
	if got, want := err.Error(), "движок: нет памяти {edit:-12}"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(err, inner) {
		t.Error("errors.Is(EditError, причина) = false, want true (Unwrap)")
	}
	wrapped := fmt.Errorf("пересборка: %w", err)
	if id := prEditID(t, wrapped); id != "-12" {
		t.Errorf("id из обёрнутой ошибки %q, want -12", id)
	}
}

// Движок упал на гитаре — ошибка несёт id записи гитары, не баса; причина в тексте.
func TestRebuildEditErrorEngine(t *testing.T) {
	t.Parallel()
	f := pdVfSetup(t, nil)
	f.failGuitarAt = 1
	bass, guitar := aeBass(false), aeGuitar(0, false)
	bass.ID, guitar.ID = "-1", "-2"
	_, err := RebuildSections(context.Background(), f, psJobID, []SectionSpec{bass, guitar})
	if id := prEditID(t, err); id != "-2" {
		t.Errorf("id правки %q, want -2 (движок гитары)", id)
	}
	if !strings.Contains(err.Error(), "нет памяти") || !strings.HasSuffix(err.Error(), "{edit:-2}") {
		t.Errorf("текст %q, want причина и отметка {edit:-2} в конце", err)
	}
}

// Движок упал на записи пресета — id записи-пресета.
func TestRebuildEditErrorPresetRecord(t *testing.T) {
	t.Parallel()
	f := pdVfSetup(t, nil)
	f.failGuitarAt = 1
	mute := SectionSpec{ID: "-1", Stems: []string{"drums"}, Db: -100}
	_, err := RebuildSections(prCtx(), f, psJobID, []SectionSpec{mute, prRecord("-900", vfPreset(81))})
	if id := prEditID(t, err); id != "-900" {
		t.Errorf("id правки %q, want -900 (запись-пресет)", id)
	}
	if !strings.Contains(err.Error(), "нет памяти") {
		t.Errorf("текст %q, want причина «нет памяти»", err)
	}
}

// Неизвестная цепочка эффекта дорожки — id этой записи; загрузок нет.
func TestRebuildEditErrorStemFx(t *testing.T) {
	t.Parallel()
	f, _ := fxSetup(t)
	mute := SectionSpec{ID: "-4", From: 5, To: 7, Stems: []string{"other"}, Db: -100}
	bad := fxSpec(2, 4)
	bad.Chain, bad.ID = "no-such-chain", "-3"
	_, err := RebuildSections(context.Background(), f, parentID, []SectionSpec{mute, bad})
	if id := prEditID(t, err); id != "-3" {
		t.Errorf("id правки %q, want -3", id)
	}
	if len(f.uploads) != 0 {
		t.Errorf("при ошибке загружено %v, want ничего (итога нет)", keys(f.uploads))
	}
}

// Неизвестная цепочка в педалях дорожки — id этой записи.
func TestRebuildEditErrorStemSteps(t *testing.T) {
	t.Parallel()
	f, _ := fxSetup(t)
	bad := SectionSpec{ID: "-8", From: 2, To: 6, Stems: []string{"other"}, Steps: []dsp.Step{{Chain: "no-such-chain"}}}
	_, err := RebuildSections(context.Background(), f, parentID, []SectionSpec{bad})
	if id := prEditID(t, err); id != "-8" {
		t.Errorf("id правки %q, want -8", id)
	}
}

// Эффект на весь трек с неизвестной цепочкой — id этой записи.
func TestRebuildEditErrorMixFx(t *testing.T) {
	t.Parallel()
	f := mxSetup(t)
	mute := mxMuteOther(1, 2)
	mute.ID = "-1"
	bad := SectionSpec{ID: "-2", Mix: true, Stems: []string{"mix"}, Chain: "no-such-chain"}
	_, err := RebuildSections(context.Background(), f, parentID, []SectionSpec{mute, bad})
	if id := prEditID(t, err); id != "-2" {
		t.Errorf("id правки %q, want -2", id)
	}
}

// Мастер упал на воркере — id записи мастера.
func TestRebuildEditErrorMaster(t *testing.T) {
	t.Parallel()
	f, _ := msSetup(t)
	f.err = errors.New("limiter: target_lufs вне пределов")
	mute, master := msMute(), msMaster()
	mute.ID, master.ID = "-1", "-2"
	_, err := RebuildSections(context.Background(), f, parentID, []SectionSpec{mute, master})
	if id := prEditID(t, err); id != "-2" {
		t.Errorf("id правки %q, want -2 (мастер)", id)
	}
	if !strings.Contains(err.Error(), "target_lufs") {
		t.Errorf("текст %q, want причина воркера", err)
	}
}

// «Применить всё»: сбой первой сборки — id правки; итога нет, черновик цел.
func TestApplyEditsEditError(t *testing.T) {
	t.Parallel()
	f, _ := aeSetup(t, nil)
	f.failGuitarAt = 1
	bass, guitar := aeBass(true), aeGuitar(0, true)
	bass.ID, guitar.ID = "-1", "-2"
	id, err := ApplyEdits(context.Background(), f, psJobID, []SectionSpec{bass, guitar}, aeTitle, "")
	if eid := prEditID(t, err); eid != "-2" {
		t.Errorf("id правки %q, want -2", eid)
	}
	if id != 0 || len(f.ids) != 0 {
		t.Errorf("итог %d, версии %v, want ни одной", id, f.ids)
	}
	if len(f.draftDel) != 0 {
		t.Errorf("черновик удалён при ошибке: %v", f.draftDel)
	}
}

// ---------- ТК7: сборка ставит недостающие наборы цепочек движка ----------

// Набора growlybass нет у воркера — ставится один раз (две записи с ним), до движка на бас; osdk есть — не ставится.
func TestRebuildInstallsMissingKits(t *testing.T) {
	t.Parallel()
	f := psSetup(t, map[string]string{"kick": "0.5*lt(mod(t\\,0.5)\\,0.02)"})
	f.kits = []any{map[string]any{"name": "osdk/kick", "samples": 3.0}}
	specs := []SectionSpec{
		{ID: "-1", Stems: []string{"kick"}, Engine: []map[string]any{{"type": "sampler", "kit": "osdk/kick", "output_db": 0.0}}},
		{ID: "-2", Stems: []string{"bass"}, Engine: []map[string]any{{"type": "bass", "kit": "growlybass/bass", "division": 2.0}}},
		{ID: "-3", Stems: []string{"other"}, Engine: []map[string]any{{"type": "bass", "kit": "growlybass/bass"}}},
	}
	if _, err := RebuildSections(context.Background(), f, psJobID, specs); err != nil {
		t.Fatalf("RebuildSections: %v", err)
	}
	if !reflect.DeepEqual(f.installs, []string{"growlybass"}) {
		t.Errorf("InstallFxKit %q, want ровно [growlybass]", f.installs)
	}
	if i, j := psIndex(f.log, "install:growlybass"), psIndex(f.log, "applyfx:bass"); i < 0 || j < 0 || i > j {
		t.Errorf("порядок %v, want набор поставлен до движка на бас", f.log)
	}
}

// Все наборы есть — ничего не ставится.
func TestRebuildKitsPresentNoInstall(t *testing.T) {
	t.Parallel()
	f := psSetup(t, nil)
	specs := []SectionSpec{{ID: "-1", Stems: []string{"bass"}, Engine: []map[string]any{{"type": "bass", "kit": "growlybass/bass"}}}}
	if _, err := RebuildSections(context.Background(), f, psJobID, specs); err != nil {
		t.Fatalf("RebuildSections: %v", err)
	}
	if len(f.installs) != 0 {
		t.Errorf("InstallFxKit %q, want ни одного", f.installs)
	}
}

// ---------- ТК8: предупреждения подготовки пресета — в ответе сборки ----------

func TestRebuildPresetRecordWarnings(t *testing.T) {
	t.Parallel()
	f := gpSetup(t)
	f.gridErr = &yue.StatusError{Code: 404, Msg: "нет плана"}
	res := prRebuild(t, f, prRecord("-900", gpPreset(true)))
	if !slicesHas(res.Warnings, func(w string) bool { return strings.Contains(w, "нет плана") }) {
		t.Errorf("warnings %v, want пометку «%s»", res.Warnings, gpNoPlanMark)
	}
	for _, c := range f.calls {
		for _, b := range c.req.Chain {
			if b["type"] == "gtrnotes" {
				t.Errorf("гитарист посчитан у трека без плана: %v", c.req.Chain)
			}
		}
	}
}

func TestRebuildPresetRecordNoWarningsWithPlan(t *testing.T) {
	t.Parallel()
	f := gpSetup(t)
	f.grid = &yue.ChordGrid{BPM: 120, Bars: ppBars()}
	res := prRebuild(t, f, prRecord("-900", gpPreset(true)))
	if len(res.Warnings) != 0 {
		t.Errorf("warnings %v у трека с планом, want пусто", res.Warnings)
	}
}

// Без пресета предупреждений нет — поля в JSON нет; с предупреждением — есть.
func TestRebuildResultWarningsJSON(t *testing.T) {
	t.Parallel()
	if raw := pdJSON(t, RebuildResult{}); strings.Contains(raw, "warnings") {
		t.Errorf("JSON без предупреждений %s, want без поля warnings", raw)
	}
	raw := pdJSON(t, RebuildResult{Warnings: []string{"нет плана"}})
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m["warnings"], []any{"нет плана"}) {
		t.Errorf("JSON %s, want warnings [нет плана]", raw)
	}
}

// ---------- запись-пресет и id в JSON (фронт шлёт так) ----------

func TestSectionSpecPresetJSON(t *testing.T) {
	t.Parallel()
	src := `{"child_id":0,"from":0,"to":0,"lead":0,"beat_sec":0,"db":0,"stems":[],"fade_in":0,"fade_out":0,
		"keep_high_hz":0,"id":"-900","preset":{"preset":{"id":71,"name":"Пост-панк","specs":[{"stems":["bass"],"db":-2}],
		"final":[],"target_lufs":null,"master":[]},"skip_sound":["drums"],"skip_place":["other"],"skip_master":true}}`
	var s SectionSpec
	if err := json.Unmarshal([]byte(src), &s); err != nil {
		t.Fatalf("запись: %v", err)
	}
	if s.ID != "-900" || s.Preset == nil {
		t.Fatalf("запись %+v, want id -900 и preset", s)
	}
	r := s.Preset
	if r.Preset.ID != 71 || r.Preset.Name != "Пост-панк" || len(r.Preset.Specs) != 1 || r.Preset.Specs[0].Db != -2 {
		t.Errorf("рецепт %+v, want пресет 71 с записью баса −2", r.Preset)
	}
	if !reflect.DeepEqual(r.SkipSound, []string{"drums"}) || !reflect.DeepEqual(r.SkipPlace, []string{"other"}) || !r.SkipMaster {
		t.Errorf("перекрытия %+v, want skip_sound [drums], skip_place [other], skip_master", *r)
	}
	plain := pdJSON(t, SectionSpec{Stems: []string{"bass"}, Db: 2})
	for _, k := range []string{`"id"`, `"preset"`} {
		if strings.Contains(plain, k) {
			t.Errorf("JSON обычной записи %s, want без %s", plain, k)
		}
	}
}

// ---------- ТК9: «применить всё» с записью-пресетом — сверка баланса по целям раскрытого пресета ----------

// Запись-пресет в черновике + итог дают те же сборки и поправки, что пресет сразу (internal-preset-verify).
func TestApplyEditsPresetRecordSameAsDirect(t *testing.T) {
	t.Parallel()
	errs := map[int64]float64{vfFirst: 3.6, vfFirst + 1: 1.6}

	direct, origD := vfSetup(t, errs)
	vfApply(t, direct, vfPreset(95))
	dp := direct.passes(t, origD)

	f, orig := aeSetup(t, errs)
	id, err := ApplyEdits(prCtx(), f, psJobID, []SectionSpec{prRecord("-900", vfPreset(95))}, aeTitle, "")
	if err != nil {
		t.Fatalf("ApplyEdits: %v", err)
	}
	ap := f.passes(t, orig)

	if len(dp) < 2 {
		t.Fatalf("сценарий: применение сразу без исправляющего прохода (%d)", len(dp))
	}
	if len(ap) != len(dp) {
		t.Fatalf("сборок: черновик+итог %d, пресет сразу %d", len(ap), len(dp))
	}
	for k := range ap {
		if d := ap[k].guitar - dp[k].guitar; math.Abs(d) > 0.1 {
			t.Errorf("гитара прохода %d: черновик+итог %+.2f, сразу %+.2f", k+1, ap[k].guitar, dp[k].guitar)
		}
		if d := ap[k].bass - dp[k].bass; math.Abs(d) > 0.1 {
			t.Errorf("бас прохода %d: черновик+итог %+.2f, сразу %+.2f", k+1, ap[k].bass, dp[k].bass)
		}
	}
	if !vfAsked(f.splitAsks, vfFirst) {
		t.Errorf("разделение версий %v, want сверка версии #%d", f.splitAsks, vfFirst)
	}
	aeOneVersion(t, f, id)
}

// Без целей level_db у пресета — одна сборка, без разделения версий.
func TestApplyEditsPresetRecordNoTargetsOneBuild(t *testing.T) {
	t.Parallel()
	f, _ := aeSetup(t, map[int64]float64{vfFirst: 3.6})
	p := lvPreset(96, yue.PresetSpec{Stems: []string{"guitar"}, Engine: engChain(), Db: -2})
	id, err := ApplyEdits(prCtx(), f, psJobID, []SectionSpec{prRecord("-900", p)}, aeTitle, "")
	if err != nil {
		t.Fatalf("ApplyEdits: %v", err)
	}
	if len(f.splitAsks) != 0 {
		t.Errorf("разделение версий %v, want ни одного (целей нет)", f.splitAsks)
	}
	if f.guitarRuns() != 1 {
		t.Errorf("движок на гитару %d раз, want 1 (одна сборка)", f.guitarRuns())
	}
	aeOneVersion(t, f, id)
}

// ---------- после ревью: сбой набора и скачивания вклейки — тоже ошибка правки ----------

// prKitFailFake — psFake, у которого установка набора failKit падает.
type prKitFailFake struct {
	*psFake
	failKit string
}

func (f *prKitFailFake) InstallFxKit(ctx context.Context, name string) (map[string]any, error) {
	if name == f.failKit {
		f.installs = append(f.installs, name)
		return nil, errors.New("набор не скачан: нет сети")
	}
	return f.psFake.InstallFxKit(ctx, name)
}

// Набор не встал — *EditError с id первой записи, чья цепочка ссылается на этот набор (по любому полю набора;
// имя набора — часть до «/»).
func TestRebuildKitInstallFailEditError(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"kit", "kit_open", "kit_mid", "kit_low", "mute_kit"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			f := &prKitFailFake{psFake: psSetup(t, map[string]string{"kick": "0.5*lt(mod(t\\,0.5)\\,0.02)"}), failKit: "brokenkit"}
			f.kits = []any{map[string]any{"name": "osdk/kick", "samples": 3.0}}
			specs := []SectionSpec{
				{ID: "-1", Stems: []string{"other"}, Engine: []map[string]any{{"type": "eq", "highpass_hz": 80.0}}},
				{ID: "-2", Stems: []string{"kick"}, Engine: []map[string]any{{"type": "sampler", "kit": "osdk/kick"},
					{"type": "sampler", field: "brokenkit/part"}}},
				{ID: "-3", Stems: []string{"bass"}, Engine: []map[string]any{{"type": "bass", "kit": "brokenkit/bass"}}},
			}
			_, err := RebuildSections(context.Background(), f, psJobID, specs)
			if id := prEditID(t, err); id != "-2" {
				t.Errorf("id правки %q, want -2 (первая запись с набором brokenkit через %s)", id, field)
			}
			if !strings.Contains(err.Error(), "нет сети") {
				t.Errorf("текст %q, want причина установки", err)
			}
			if len(f.uploads) != 0 {
				t.Errorf("при ошибке загружено %v, want ничего", keys(f.uploads))
			}
		})
	}
}

// Дорожки вклейки рендера (ChildID > 0) не скачались — *EditError с id этой записи.
func TestRebuildRenderFetchFailEditError(t *testing.T) {
	t.Parallel()
	f, _ := secSetup(t)
	for k := range f.files {
		if strings.HasPrefix(k, "7/") {
			delete(f.files, k)
		}
	}
	mute := SectionSpec{ID: "-1", From: 1, To: 2, Stems: []string{"other"}, Db: -100}
	ins := spec7([]string{"drums"}, 0)
	ins.ID = "7"
	_, err := RebuildSections(context.Background(), f, parentID, []SectionSpec{mute, ins})
	if id := prEditID(t, err); id != "7" {
		t.Errorf("id правки %q, want 7 (вклейка без дорожек)", id)
	}
}
