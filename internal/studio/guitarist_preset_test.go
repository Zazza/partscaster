package studio

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"partscaster/internal/yue"
)

// Гитарист «по аккордам плана» в пресетах (карточка internal-guitarist-presets).
//
// Условие 4: крэш слоя «живой звук» — rel_db −18 (во всех пресетах с живым набором).
// Условие 6: трек без плана (ChordGrid 404) и запись пресета с блоком gtrnotes → применение не падает:
// gtrnotes снимается из записи, гитара YuE2 идёт через остальную обработку записи; название версии — с
// пометкой «гитара YuE2 — у трека нет плана»; в результате — предупреждение. С планом — как раньше
// (gtrnotes на месте, без пометки). Без gtrnotes в пресете — поведение прежнее (план не обязателен).
//
// Толкования:
//   - «Записи, отданные на рендер» — цепочки движка в запросах ApplyFx (воркер — внешняя граница).
//   - «Предупреждение в результате» — непустой текст в результате ApplySoundPreset помимо File
//     (строковое поле или список строк): имя поля карточкой не задано, тест его не фиксирует.
//   - Без gtrnotes план не нужен: воркер отвечает на ChordGrid ошибкой 500 — применение всё равно проходит.

const gpNoPlanMark = "гитара YuE2 — у трека нет плана"

// gpFake — psFake + ChordGrid (план трека) со счётчиком.
type gpFake struct {
	*psFake
	grid      *yue.ChordGrid
	gridErr   error
	gridCalls int
}

func (f *gpFake) ChordGrid(_ context.Context, _ int64) (*yue.ChordGrid, error) {
	f.gridCalls++
	if f.gridErr != nil {
		return nil, f.gridErr
	}
	return f.grid, nil
}

func gpSetup(t *testing.T) *gpFake {
	t.Helper()
	return &gpFake{psFake: psSetup(t, map[string]string{"guitar": "0.3*sin(2*PI*800*t)"})}
}

func gpGtrnotes() map[string]any {
	return map[string]any{"type": "gtrnotes", "kit": "fsbsguitar/guitar", "style": 4.0}
}

func gpRest() []map[string]any {
	return []map[string]any{
		{"type": "eq", "highpass_hz": 100.0},
		{"type": "drive", "gain_db": 34.0, "mix": 1.0},
	}
}

// gpPreset — пресет с записью гитары [gtrnotes, eq, drive] (withNotes=false — без gtrnotes) и записью баса.
func gpPreset(withNotes bool) yue.SoundPreset {
	eng := gpRest()
	if withNotes {
		eng = append([]map[string]any{gpGtrnotes()}, eng...)
	}
	return psPreset(7, []yue.PresetSpec{
		{Stems: []string{"guitar"}, Engine: eng},
		{Stems: []string{"bass"}, Engine: psAmpEngine()},
	}, nil)
}

func gpApply(t *testing.T, f *gpFake, p yue.SoundPreset) *PresetResult {
	t.Helper()
	res, err := ApplySoundPreset(withoutLiveLayer(context.Background()), f, psJobID, p)
	if err != nil {
		t.Fatalf("ApplySoundPreset: %v", err)
	}
	if res == nil {
		t.Fatal("ApplySoundPreset: nil результат без ошибки")
	}
	return res
}

// gpChain — цепочка, отданная на рендер дорожке stem (ровно один запрос).
func gpChain(t *testing.T, f *gpFake, stem string) []map[string]any {
	t.Helper()
	var out [][]map[string]any
	for _, c := range f.calls {
		if c.req.Source == stem {
			out = append(out, c.req.Chain)
		}
	}
	if len(out) != 1 {
		t.Fatalf("запросов ApplyFx на %s: %d, want 1 (все: %+v)", stem, len(out), f.calls)
	}
	return out[0]
}

func gpHasType(chain []map[string]any, typ string) bool {
	for _, b := range chain {
		if b["type"] == typ {
			return true
		}
	}
	return false
}

// gpWarnings — тексты результата помимо File: строковые поля и списки строк.
func gpWarnings(res *PresetResult) []string {
	var out []string
	v := reflect.ValueOf(*res)
	for i := 0; i < v.NumField(); i++ {
		if v.Type().Field(i).Name == "File" {
			continue
		}
		fv := v.Field(i)
		switch {
		case fv.Kind() == reflect.String && fv.String() != "":
			out = append(out, fv.String())
		case fv.Kind() == reflect.Slice && fv.Type().Elem().Kind() == reflect.String:
			for j := 0; j < fv.Len(); j++ {
				if s := fv.Index(j).String(); s != "" {
					out = append(out, s)
				}
			}
		}
	}
	return out
}

func gpTitle(t *testing.T, f *gpFake) string {
	t.Helper()
	if len(f.promoted) != 1 {
		t.Fatalf("VariantToTrack вызван %d раз, want 1", len(f.promoted))
	}
	return f.promoted[0].title
}

// Без плана: gtrnotes снят, остальная обработка гитары — на месте, пометка в названии, предупреждение.
func TestApplySoundPresetGuitaristNoPlan(t *testing.T) {
	t.Parallel()
	f := gpSetup(t)
	f.gridErr = &yue.StatusError{Code: 404, Msg: "нет плана"}
	p := gpPreset(true)

	res := gpApply(t, f, p)

	chain := gpChain(t, f, "guitar")
	if gpHasType(chain, "gtrnotes") {
		t.Errorf("на рендер ушёл gtrnotes у трека без плана: %v", chain)
	}
	if !reflect.DeepEqual(chain, gpRest()) {
		t.Errorf("цепочка гитары %v, want остальная обработка записи %v", chain, gpRest())
	}
	// прочие записи — как в пресете
	if got := gpChain(t, f, "bass"); !reflect.DeepEqual(got, psAmpEngine()) {
		t.Errorf("цепочка баса %v, want %v", got, psAmpEngine())
	}
	title := gpTitle(t, f)
	if !strings.HasPrefix(title, psTrackTitle(p)) || !strings.Contains(title, gpNoPlanMark) {
		t.Errorf("название версии %q, want «%s» с пометкой «%s»", title, psTrackTitle(p), gpNoPlanMark)
	}
	if w := gpWarnings(res); len(w) == 0 {
		t.Errorf("в результате нет предупреждения: %+v", *res)
	}
	if res.ChildID != psChild {
		t.Errorf("ChildID %d, want %d", res.ChildID, psChild)
	}
}

// С планом: gtrnotes на месте первым блоком, название без пометки, предупреждений нет.
func TestApplySoundPresetGuitaristWithPlan(t *testing.T) {
	t.Parallel()
	f := gpSetup(t)
	f.grid = &yue.ChordGrid{BPM: 120, Bars: testBars()}
	p := gpPreset(true)

	res := gpApply(t, f, p)

	chain := gpChain(t, f, "guitar")
	if len(chain) == 0 || chain[0]["type"] != "gtrnotes" {
		t.Fatalf("цепочка гитары %v, want gtrnotes первым блоком", chain)
	}
	if !reflect.DeepEqual(chain[1:], gpRest()) {
		t.Errorf("обработка после gtrnotes %v, want %v", chain[1:], gpRest())
	}
	if title := gpTitle(t, f); title != psTrackTitle(p) {
		t.Errorf("название версии %q, want %q (без пометки)", title, psTrackTitle(p))
	}
	if w := gpWarnings(res); len(w) != 0 {
		t.Errorf("предупреждения у трека с планом: %v", w)
	}
}

// Без gtrnotes в пресете план не нужен: даже ошибка воркера на ChordGrid применению не мешает.
func TestApplySoundPresetNoGuitaristPlanNotRequired(t *testing.T) {
	t.Parallel()
	f := gpSetup(t)
	f.gridErr = &yue.StatusError{Code: 500, Msg: "сломалось"}
	p := gpPreset(false)

	res := gpApply(t, f, p)

	if chain := gpChain(t, f, "guitar"); !reflect.DeepEqual(chain, gpRest()) {
		t.Errorf("цепочка гитары %v, want как в пресете %v", chain, gpRest())
	}
	if title := gpTitle(t, f); title != psTrackTitle(p) {
		t.Errorf("название версии %q, want %q", title, psTrackTitle(p))
	}
	if w := gpWarnings(res); len(w) != 0 {
		t.Errorf("предупреждения без gtrnotes: %v", w)
	}
}

// Условие 4: крэш слоя «живой звук» — rel_db −18.
func TestLiveCymbals_CrashRelDbMinus18(t *testing.T) {
	crashHits(t, liveCymbals(testBars(), 0, true)) // форма записи
	c := crashSpecs(liveCymbals(testBars(), 0, true))[0]
	if got := num(t, c.Engine[0]["rel_db"]); got != -18 {
		t.Errorf("rel_db крэша %v, want −18", got)
	}
}

// Условие 4 через слой целиком (пресет с живым набором).
func TestLiveLayer_CrashRelDbMinus18(t *testing.T) {
	f := &liveFake{grid: &yue.ChordGrid{BPM: 120, Bars: testBars()}}
	job := &yue.Job{ID: 42, DurationSec: 0}
	specs, err := liveLayer(context.Background(), f, job, liveKitPreset())
	if err != nil {
		t.Fatalf("liveLayer: %v", err)
	}
	crash := crashSpecs(specs)
	if len(crash) != 1 {
		t.Fatalf("записей крэша %d, want 1", len(crash))
	}
	if got := num(t, crash[0].Engine[0]["rel_db"]); got != -18 {
		t.Errorf("rel_db крэша %v, want −18", got)
	}
}
