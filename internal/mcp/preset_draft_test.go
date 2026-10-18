package mcp

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"

	"partscaster/internal/yue"
)

// sound_preset_draft {preset_id, db?} (карточка internal-studio-instant-apply, условие 15): пресет в черновик —
// одна запись с рецептом, без подготовки. Ответ {"edit": запись}: childId < 0 и уникален, instId "preset",
// step "done", label = имя пресета, stems [], from/to/lead/beat/db/fadeIn/fadeOut/keepHighHz = 0,
// preset {id, name, recipe: пресет целиком с заменой громкостей db}. db — как у sound_preset_apply:
// {"индекс записи": дБ −24…24}, неверный индекс/значение — ошибка. Нет пресета — ошибка. Воркер — только
// список пресетов; пресет на воркере не меняется.
//
// Воркер — pdListOnly: отвечает только SoundPresets; любой другой вызов yue.Service — паника (вызов
// неинициализированного встроенного интерфейса), то есть «лишняя работа» роняет тест.

type pdListOnly struct {
	yue.Service
	mu      sync.Mutex
	presets []yue.SoundPreset
	listed  int
}

func (f *pdListOnly) SoundPresets(context.Context) ([]yue.SoundPreset, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listed++
	out := make([]yue.SoundPreset, len(f.presets))
	for i, p := range f.presets {
		out[i] = pdClone(p)
	}
	return out, nil
}

func pdClone(p yue.SoundPreset) yue.SoundPreset {
	b, _ := json.Marshal(p)
	var c yue.SoundPreset
	_ = json.Unmarshal(b, &c)
	return c
}

func newPresetDraftServer(t *testing.T) (*Server, *pdListOnly) {
	t.Helper()
	s, _ := newTestServer(t)
	f := &pdListOnly{presets: []yue.SoundPreset{dbPreset()}}
	s.client = f
	return s, f
}

// pdEdit — запись из ответа {"edit": {...}}.
type pdEditRec struct {
	ChildID    float64 `json:"childId"`
	InstID     string  `json:"instId"`
	Step       string  `json:"step"`
	Label      string  `json:"label"`
	Stems      []any   `json:"stems"`
	From       float64 `json:"from"`
	To         float64 `json:"to"`
	Lead       float64 `json:"lead"`
	Beat       float64 `json:"beat"`
	Db         float64 `json:"db"`
	FadeIn     float64 `json:"fadeIn"`
	FadeOut    float64 `json:"fadeOut"`
	KeepHighHz float64 `json:"keepHighHz"`
	Preset     *struct {
		ID     int64           `json:"id"`
		Name   string          `json:"name"`
		Recipe yue.SoundPreset `json:"recipe"`
	} `json:"preset"`
}

func pdEdit(t *testing.T, out string) (pdEditRec, map[string]json.RawMessage) {
	t.Helper()
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &top); err != nil {
		t.Fatalf("ответ не объект JSON: %v\n%s", err, out)
	}
	raw, ok := top["edit"]
	if !ok {
		t.Fatalf("в ответе нет поля edit: %s", out)
	}
	var rec pdEditRec
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("edit — не запись: %v\n%s", err, raw)
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	return rec, fields
}

// Без db — запись с рецептом пресета как в библиотеке; поля записи по контракту; воркер — только список.
func TestSoundPresetDraftRecord(t *testing.T) {
	t.Parallel()
	s, f := newPresetDraftServer(t)
	out, ok := call(t, s, "sound_preset_draft", map[string]any{"preset_id": dbPresetID})
	if !ok {
		t.Fatalf("sound_preset_draft: %s", out)
	}
	rec, fields := pdEdit(t, out)
	if rec.ChildID >= 0 {
		t.Errorf("childId %v, want < 0", rec.ChildID)
	}
	if rec.InstID != "preset" || rec.Step != "done" || rec.Label != dbPreset().Name {
		t.Errorf("instId %q step %q label %q, want preset / done / %q", rec.InstID, rec.Step, rec.Label, dbPreset().Name)
	}
	if _, ok := fields["stems"]; !ok || len(rec.Stems) != 0 {
		t.Errorf("stems %v (поле есть: %v), want []", rec.Stems, ok)
	}
	for name, v := range map[string]float64{"from": rec.From, "to": rec.To, "lead": rec.Lead, "beat": rec.Beat,
		"db": rec.Db, "fadeIn": rec.FadeIn, "fadeOut": rec.FadeOut, "keepHighHz": rec.KeepHighHz} {
		if v != 0 {
			t.Errorf("%s = %v, want 0", name, v)
		}
	}
	if rec.Preset == nil {
		t.Fatalf("в записи нет preset: %s", out)
	}
	if rec.Preset.ID != dbPresetID || rec.Preset.Name != dbPreset().Name {
		t.Errorf("preset id %d name %q, want %d %q", rec.Preset.ID, rec.Preset.Name, dbPresetID, dbPreset().Name)
	}
	if !reflect.DeepEqual(rec.Preset.Recipe, pdClone(dbPreset())) {
		t.Errorf("рецепт %+v, want пресет целиком %+v", rec.Preset.Recipe, dbPreset())
	}
	if f.listed == 0 {
		t.Error("список пресетов не запрошен")
	}
	if !reflect.DeepEqual(f.presets, []yue.SoundPreset{dbPreset()}) {
		t.Errorf("пресет изменён: %+v", f.presets)
	}
}

// job_id не нужен — и не мешает, если передан (старые вызовы).
func TestSoundPresetDraftJobIDNotNeeded(t *testing.T) {
	t.Parallel()
	s, _ := newPresetDraftServer(t)
	if out, ok := call(t, s, "sound_preset_draft", map[string]any{"job_id": 466, "preset_id": dbPresetID}); !ok {
		t.Fatalf("с лишним job_id: %s", out)
	}
}

// Два вызова — разные childId.
func TestSoundPresetDraftUniqueChildID(t *testing.T) {
	t.Parallel()
	s, _ := newPresetDraftServer(t)
	seen := map[float64]bool{}
	for i := 0; i < 3; i++ {
		out, ok := call(t, s, "sound_preset_draft", map[string]any{"preset_id": dbPresetID})
		if !ok {
			t.Fatalf("вызов %d: %s", i+1, out)
		}
		rec, _ := pdEdit(t, out)
		if rec.ChildID >= 0 || seen[rec.ChildID] {
			t.Errorf("вызов %d: childId %v, want < 0 и не повторяется (были %v)", i+1, rec.ChildID, seen)
		}
		seen[rec.ChildID] = true
	}
}

// Нет пресета — ошибка.
func TestSoundPresetDraftUnknownPreset(t *testing.T) {
	t.Parallel()
	s, _ := newPresetDraftServer(t)
	if out, ok := call(t, s, "sound_preset_draft", map[string]any{"preset_id": 999}); ok {
		t.Fatalf("несуществующий пресет прошёл: %s", out)
	}
}

// Без preset_id — ошибка.
func TestSoundPresetDraftNoPresetID(t *testing.T) {
	t.Parallel()
	s, _ := newPresetDraftServer(t)
	if out, ok := call(t, s, "sound_preset_draft", map[string]any{}); ok {
		t.Fatalf("без preset_id прошло: %s", out)
	}
}

// db {"2": 3} → в рецепте у записи 2 («прочее») db 3, прочие — как в пресете; пресет в библиотеке не изменён.
func TestSoundPresetDraftDbOverrides(t *testing.T) {
	t.Parallel()
	s, f := newPresetDraftServer(t)
	out, ok := call(t, s, "sound_preset_draft", jsonArgs(t, `{"preset_id":50,"db":{"2":3}}`))
	if !ok {
		t.Fatalf("sound_preset_draft с db: %s", out)
	}
	rec, _ := pdEdit(t, out)
	if rec.Preset == nil || len(rec.Preset.Recipe.Specs) != 3 {
		t.Fatalf("рецепт %+v, want 3 записи", rec.Preset)
	}
	for i, want := range []float64{-2, 0, 3} {
		if got := rec.Preset.Recipe.Specs[i].Db; got != want {
			t.Errorf("рецепт: запись %d db %v, want %v", i, got, want)
		}
	}
	if rec.Db != 0 {
		t.Errorf("db самой записи %v, want 0 (громкости — в рецепте)", rec.Db)
	}
	if !reflect.DeepEqual(f.presets, []yue.SoundPreset{dbPreset()}) {
		t.Errorf("пресет в библиотеке изменён: %+v", f.presets)
	}
}

// db с неверным индексом / вне −24…24 → ошибка (как у sound_preset_apply).
func TestSoundPresetDraftDbInvalid(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, args string }{
		{"индекс 3 при 3 записях", `{"preset_id":50,"db":{"3":3}}`},
		{"индекс не число", `{"preset_id":50,"db":{"x":3}}`},
		{"db 30", `{"preset_id":50,"db":{"2":30}}`},
		{"db −25", `{"preset_id":50,"db":{"0":-25}}`},
		{"db не число", `{"preset_id":50,"db":{"2":"громче"}}`},
		{"db не объект", `{"preset_id":50,"db":3}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, _ := newPresetDraftServer(t)
			if out, ok := call(t, s, "sound_preset_draft", jsonArgs(t, tc.args)); ok {
				t.Fatalf("want ошибку, ответ: %s", out)
			}
		})
	}
}

// Границы −24 и 24 допустимы.
func TestSoundPresetDraftDbBounds(t *testing.T) {
	t.Parallel()
	s, _ := newPresetDraftServer(t)
	out, ok := call(t, s, "sound_preset_draft", jsonArgs(t, `{"preset_id":50,"db":{"0":-24,"1":24}}`))
	if !ok {
		t.Fatalf("границы −24/24: %s", out)
	}
	rec, _ := pdEdit(t, out)
	if rec.Preset == nil || rec.Preset.Recipe.Specs[0].Db != -24 || rec.Preset.Recipe.Specs[1].Db != 24 {
		t.Errorf("рецепт %+v, want db −24 и 24 у записей 0 и 1", rec.Preset)
	}
}
