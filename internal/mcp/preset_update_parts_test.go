package mcp

import (
	"reflect"
	"testing"

	"partscaster/internal/yue"
)

// sound_preset_update
// без parts сохраняет прежние партии пресета; с parts [] — снимает их. Проверяется тело
// SoundPresetUpdate, которое уходит воркеру (presetFake из sound_presets_test.go).

func ownPartsPreset() []yue.PresetPart {
	return []yue.PresetPart{
		{Kind: "synth", Engine: []map[string]any{{"type": "synth"}}, Style: "pad"},
		{Kind: "perc", Style: "shaker"},
	}
}

func TestSoundPresetUpdateWithoutPartsKeepsOld(t *testing.T) {
	t.Parallel()
	s, fake := newPresetServer(t)
	fake.presets[1].Parts = ownPartsPreset()
	out, ok := call(t, s, "sound_preset_update", jsonArgs(t, `{"preset_id":12,"name":"Мой тёплый","note":"новое"}`))
	if !ok {
		t.Fatalf("sound_preset_update: %s", out)
	}
	if len(fake.updated) != 1 || fake.updated[0].id != 12 {
		t.Fatalf("SoundPresetUpdate %+v, want один вызов для 12", fake.updated)
	}
	if got := fake.updated[0].p.Parts; !reflect.DeepEqual(got, ownPartsPreset()) {
		t.Errorf("update без parts: parts %+v, want прежние %+v", got, ownPartsPreset())
	}
}

func TestSoundPresetUpdateEmptyPartsClears(t *testing.T) {
	t.Parallel()
	s, fake := newPresetServer(t)
	fake.presets[1].Parts = ownPartsPreset()
	out, ok := call(t, s, "sound_preset_update", jsonArgs(t, `{"preset_id":12,"name":"Мой тёплый","parts":[]}`))
	if !ok {
		t.Fatalf("sound_preset_update: %s", out)
	}
	if len(fake.updated) != 1 {
		t.Fatalf("SoundPresetUpdate вызван %d раз, want 1", len(fake.updated))
	}
	if got := fake.updated[0].p.Parts; len(got) != 0 {
		t.Errorf("update с parts []: parts %+v, want пусто", got)
	}
}
