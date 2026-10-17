package mcp

import (
	"encoding/json"
	"reflect"
	"testing"
)

// sound_preset_draft {job_id, preset_id, db?} (карточка internal-studio-preset-draft, условие 9; ТК12):
// пресет в черновик — ответ {"preset": <ответ PresetDraft: sections, final?, master?, warnings?>}; трек не
// меняется (версии нет). db — как у sound_preset_apply: {"индекс записи": дБ −24…24} заменяет Db записи
// копии пресета (пресет в библиотеке не меняется), неверный индекс/значение — ошибка без работы. Нет
// пресета — ошибка.
//
// Воркер — stemPresetFake (sound_presets_test.go): пресет dbPreset — три записи eq (vocals −2, bass 0,
// other 0) без целей level_db, поэтому сборки нет вовсе и ответ — сами записи.
// Толкование: запись ответа узнаётся по дорожке и цепочке (stems + chain eq) — порядок и прочие записи
// (слой «живой звук») тест не фиксирует.

// pdSections — записи sections из ответа {"preset": {...}}.
func pdSections(t *testing.T, out string) []map[string]any {
	t.Helper()
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &top); err != nil {
		t.Fatalf("ответ не объект JSON: %v\n%s", err, out)
	}
	raw, ok := top["preset"]
	if !ok {
		t.Fatalf("в ответе нет поля preset: %s", out)
	}
	var preset struct {
		Sections []map[string]any `json:"sections"`
	}
	if err := json.Unmarshal(raw, &preset); err != nil {
		t.Fatalf("preset — не объект с sections: %v\n%s", err, raw)
	}
	if len(preset.Sections) == 0 {
		t.Fatalf("preset.sections пусто: %s", raw)
	}
	return preset.Sections
}

// pdEqDb — db записи eq на дорожке stem; нет такой записи — тест падает.
func pdEqDb(t *testing.T, secs []map[string]any, stem string) float64 {
	t.Helper()
	for _, s := range secs {
		if reflect.DeepEqual(s["stems"], []any{stem}) && s["chain"] == "eq" {
			db, _ := s["db"].(float64) // нет поля — 0
			return db
		}
	}
	t.Fatalf("нет записи eq на %s в sections: %v", stem, secs)
	return 0
}

// Нет пресета — ошибка, работы нет.
func TestSoundPresetDraftUnknownPreset(t *testing.T) {
	t.Parallel()
	s, f := newDbPresetServer(t)
	out, ok := call(t, s, "sound_preset_draft", map[string]any{"job_id": 466, "preset_id": 999})
	if ok {
		t.Fatalf("sound_preset_draft с несуществующим пресетом прошёл: %s", out)
	}
	if len(f.promoted) != 0 || len(f.uploads) != 0 {
		t.Errorf("работа при ошибке: promoted %+v, uploads %d", f.promoted, len(f.uploads))
	}
}

// Без db — записи как в пресете; ответ {"preset": {sections…}}, версии нет.
func TestSoundPresetDraftAnswer(t *testing.T) {
	t.Parallel()
	s, f := newDbPresetServer(t)
	out, ok := call(t, s, "sound_preset_draft", map[string]any{"job_id": 466, "preset_id": dbPresetID})
	if !ok {
		t.Fatalf("sound_preset_draft: %s", out)
	}
	secs := pdSections(t, out)
	for stem, want := range map[string]float64{"vocals": -2, "bass": 0, "other": 0} {
		if got := pdEqDb(t, secs, stem); got != want {
			t.Errorf("%s db %v, want %v (как в пресете)", stem, got, want)
		}
	}
	if len(f.promoted) != 0 {
		t.Errorf("создана версия %+v, want нет (черновик трек не меняет)", f.promoted)
	}
	if len(f.updated) != 0 || len(f.created) != 0 {
		t.Errorf("пресет записан на воркер: updated %v created %v", f.updated, f.created)
	}
}

// db {"2": 3} → запись 2 («прочее») в ответе с db 3, прочие — как в пресете; пресет в библиотеке не изменён.
func TestSoundPresetDraftDbOverrides(t *testing.T) {
	t.Parallel()
	s, f := newDbPresetServer(t)
	out, ok := call(t, s, "sound_preset_draft", jsonArgs(t, `{"job_id":466,"preset_id":50,"db":{"2":3}}`))
	if !ok {
		t.Fatalf("sound_preset_draft с db: %s", out)
	}
	secs := pdSections(t, out)
	for stem, want := range map[string]float64{"vocals": -2, "bass": 0, "other": 3} {
		if got := pdEqDb(t, secs, stem); got != want {
			t.Errorf("%s db %v, want %v", stem, got, want)
		}
	}
	for _, p := range f.presets {
		if p.ID == dbPresetID && !reflect.DeepEqual(p, dbPreset()) {
			t.Errorf("пресет изменён: %+v", p)
		}
	}
	if len(f.updated) != 0 || len(f.created) != 0 || len(f.promoted) != 0 {
		t.Errorf("лишняя работа: updated %v created %v promoted %+v", f.updated, f.created, f.promoted)
	}
}

// db с неверным индексом / вне −24…24 → ошибка без работы (как у sound_preset_apply).
func TestSoundPresetDraftDbInvalid(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, args string }{
		{"индекс 3 при 3 записях", `{"job_id":466,"preset_id":50,"db":{"3":3}}`},
		{"индекс не число", `{"job_id":466,"preset_id":50,"db":{"x":3}}`},
		{"db 30", `{"job_id":466,"preset_id":50,"db":{"2":30}}`},
		{"db не число", `{"job_id":466,"preset_id":50,"db":{"2":"громче"}}`},
		{"db не объект", `{"job_id":466,"preset_id":50,"db":3}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, f := newDbPresetServer(t)
			out, ok := call(t, s, "sound_preset_draft", jsonArgs(t, tc.args))
			if ok {
				t.Fatalf("want ошибку, ответ: %s", out)
			}
			if len(f.fetched) != 0 || len(f.uploads) != 0 || len(f.promoted) != 0 {
				t.Errorf("работа при ошибке: fetched %v, uploads %d, promoted %+v", f.fetched, len(f.uploads), f.promoted)
			}
		})
	}
}
