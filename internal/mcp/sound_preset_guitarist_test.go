package mcp

import (
	"context"
	"strings"
	"sync"
	"testing"

	"partscaster/internal/yue"
)

// sound_preset_apply и гитарист «по аккордам плана» (карточка internal-guitarist-presets, условие 6):
// трек без плана (ChordGrid 404) и пресет с блоком gtrnotes → применение не падает, версия создаётся,
// в ответе — предупреждение; с планом — без предупреждения.
//
// Толкование: предупреждение узнаётся по словам «нет плана» (пометка карточки «гитара YuE2 — у трека нет
// плана»); точная формулировка ответа карточкой не задана.

const gtPresetID = 60

// gtFake — presetFake с дорожкой гитары у трека 466, планом трека (ChordGrid) и установкой наборов.
type gtFake struct {
	*presetFake
	grid    *yue.ChordGrid
	gridErr error
	mu      sync.Mutex
	chains  map[string][]map[string]any // цепочки ApplyFx по дорожке (без «воздуха» поверх микса)
}

func (f *gtFake) JobStems(context.Context, int64) ([]map[string]any, error) {
	var out []map[string]any
	for _, n := range []string{"drums", "bass", "other", "vocals", "guitar"} {
		out = append(out, map[string]any{"file": "stem-" + n + ".flac", "name": n})
	}
	return out, nil
}

func (f *gtFake) ChordGrid(context.Context, int64) (*yue.ChordGrid, error) {
	if f.gridErr != nil {
		return nil, f.gridErr
	}
	return f.grid, nil
}

func (f *gtFake) InstallFxKit(_ context.Context, name string) (map[string]any, error) {
	return map[string]any{"name": name, "downloaded": true}, nil
}

func (f *gtFake) ApplyFx(ctx context.Context, id int64, req yue.FxRequest) (*yue.DspVariant, error) {
	if !req.Add {
		f.mu.Lock()
		f.chains[req.Source] = req.Chain
		f.mu.Unlock()
	}
	return f.presetFake.ApplyFx(ctx, id, req)
}

func newGuitaristPresetServer(t *testing.T) (*Server, *gtFake) {
	t.Helper()
	needFF(t)
	s, pf := newPresetServer(t)
	f := &gtFake{presetFake: pf, chains: map[string][]map[string]any{}}
	s.client = f
	pf.presets = append(pf.presets, yue.SoundPreset{ID: gtPresetID, Name: "Панк · гитарист",
		Specs: []yue.PresetSpec{{Stems: []string{"guitar"}, Engine: []map[string]any{
			{"type": "gtrnotes", "kit": "fsbsguitar/guitar", "style": 4.0},
			{"type": "eq", "highpass_hz": 100.0},
		}}}})
	const dur = 4
	pf.jobs = []yue.Job{{ID: 466, Title: "Warm Wire", Status: "done", AudioFile: "audio.flac", DurationSec: dur}}
	pf.fetch = map[string]string{
		"audio.flac":       fxLavfi(t, "0.2*sin(2*PI*3000*t)+0.2*sin(2*PI*800*t)", dur),
		"stem-vocals.flac": fxLavfi(t, "0.2*sin(2*PI*3000*t)", dur),
		"stem-guitar.flac": fxLavfi(t, "0.2*sin(2*PI*800*t)", dur),
		"stem-bass.flac":   fxLavfi(t, "0", dur),
		"stem-other.flac":  fxLavfi(t, "0", dur),
		"stem-drums.flac":  fxLavfi(t, "0", dur),
		presetAirChunk:     fxLavfi(t, "0", dur),
	}
	return s, f
}

func gtHasGtrnotes(chain []map[string]any) bool {
	for _, b := range chain {
		if b["type"] == "gtrnotes" {
			return true
		}
	}
	return false
}

func TestSoundPresetApplyGuitaristNoPlanWarns(t *testing.T) {
	t.Parallel()
	s, f := newGuitaristPresetServer(t)
	f.gridErr = &yue.StatusError{Code: 404, Msg: "нет плана"}

	out, ok := call(t, s, "sound_preset_apply", map[string]any{"job_id": 466, "preset_id": gtPresetID})
	if !ok {
		t.Fatalf("sound_preset_apply у трека без плана упал: %s", out)
	}
	if len(f.promoted) != 1 {
		t.Fatalf("VariantToTrack вызван %d раз, want 1", len(f.promoted))
	}
	if !strings.Contains(f.promoted[0].title, "у трека нет плана") {
		t.Errorf("название версии %q без пометки «гитара YuE2 — у трека нет плана»", f.promoted[0].title)
	}
	if !strings.Contains(strings.ToLower(out), "нет плана") {
		t.Errorf("в ответе нет предупреждения про план: %s", out)
	}
	if gtHasGtrnotes(f.chains["guitar"]) {
		t.Errorf("на рендер ушёл gtrnotes у трека без плана: %v", f.chains["guitar"])
	}
}

func TestSoundPresetApplyGuitaristWithPlanNoWarning(t *testing.T) {
	t.Parallel()
	s, f := newGuitaristPresetServer(t)
	f.grid = &yue.ChordGrid{BPM: 120, Bars: []yue.ChordBar{
		{Start: 0, End: 2, Chord: "Am", Section: "verse"}, {Start: 2, End: 4, Chord: "C", Section: "verse"}}}

	out, ok := call(t, s, "sound_preset_apply", map[string]any{"job_id": 466, "preset_id": gtPresetID})
	if !ok {
		t.Fatalf("sound_preset_apply: %s", out)
	}
	if strings.Contains(strings.ToLower(out), "нет плана") {
		t.Errorf("предупреждение про план у трека с планом: %s", out)
	}
	if len(f.promoted) != 1 || f.promoted[0].title != "Warm Wire · Панк · гитарист" {
		t.Errorf("версия %+v, want одна «Warm Wire · Панк · гитарист»", f.promoted)
	}
	if !gtHasGtrnotes(f.chains["guitar"]) {
		t.Errorf("gtrnotes пропал у трека с планом: %v", f.chains["guitar"])
	}
}
