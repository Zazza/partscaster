package studio

import (
	"context"
	"reflect"
	"testing"

	"partscaster/internal/yue"
)

// Спецификация (internal-guitar-notes, контракт «Докачка наборов … Go ensurePresetKits учитывает mute_kit»):
// наборы блока gtrnotes — kit и mute_kit («<набор>/<часть>»); которых нет на воркере (FxAssets().kits) — ставятся
// InstallFxKit(<набор>) по разу на набор; пустой mute_kit — не набор; всё стоит — ничего не ставится.

// gnKitFake — воркер на границе: список наборов и журнал установок; прочие методы не нужны.
type gnKitFake struct {
	yue.Service
	kits     []any
	installs []string
}

func (f *gnKitFake) FxAssets(context.Context) (map[string]any, error) {
	return map[string]any{"kits": f.kits, "captures": []any{}, "irs": []any{}}, nil
}

func (f *gnKitFake) InstallFxKit(_ context.Context, name string) (map[string]any, error) {
	f.installs = append(f.installs, name)
	return map[string]any{"name": name, "downloaded": true}, nil
}

func gnKits(names ...string) []any {
	out := make([]any, 0, len(names))
	for _, n := range names {
		out = append(out, map[string]any{"name": n, "samples": 3.0})
	}
	return out
}

func gnSpec(b map[string]any) []yue.PresetSpec {
	return []yue.PresetSpec{{Stems: []string{"guitar"}, Engine: []map[string]any{b, {"type": "drive", "gain_db": 20.0}}}}
}

func TestEnsurePresetKitsGtrnotesMuteKit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		have  []any
		block map[string]any
		want  []string
	}{
		{"mute_kit другого набора нет → ставится он",
			gnKits("fsbsguitar/guitar"),
			map[string]any{"type": "gtrnotes", "kit": "fsbsguitar/guitar", "mute_kit": "emilyguitar/mute"},
			[]string{"emilyguitar"}},
		{"части глушения того же набора нет → набор ставится",
			gnKits("emilyguitar/notes"),
			map[string]any{"type": "gtrnotes", "kit": "emilyguitar/notes", "mute_kit": "emilyguitar/mute"},
			[]string{"emilyguitar"}},
		{"нет ни kit, ни mute_kit одного набора → один раз",
			nil,
			map[string]any{"type": "gtrnotes", "kit": "shinyguitar/acoustic", "mute_kit": "shinyguitar/mute"},
			[]string{"shinyguitar"}},
		{"kit нет, mute_kit пустой → только kit",
			gnKits("osdk/kick"),
			map[string]any{"type": "gtrnotes", "kit": "nylonguitar/guitar", "mute_kit": ""},
			[]string{"nylonguitar"}},
		{"всё стоит → ничего",
			gnKits("emilyguitar/notes", "emilyguitar/mute"),
			map[string]any{"type": "gtrnotes", "kit": "emilyguitar/notes", "mute_kit": "emilyguitar/mute"},
			nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := &gnKitFake{kits: tc.have}
			if err := ensurePresetKits(context.Background(), f, gnSpec(tc.block), nil); err != nil {
				t.Fatalf("ensurePresetKits: %v", err)
			}
			if !reflect.DeepEqual(f.installs, tc.want) {
				t.Errorf("InstallFxKit %q, want %q", f.installs, tc.want)
			}
		})
	}
}
