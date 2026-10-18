package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"partscaster/internal/yue"
)

// rebuild_sections и запись-пресет черновика (карточка internal-studio-instant-apply, условие 15; находки
// кросс-ревью):
//   - запись {"id", "preset": {"preset": рецепт, skip_*}} принимается и тогда, когда у рецепта есть только
//     target_lufs или только parts; рецепт без specs, final, master, target_lufs и parts — ошибка «пустой рецепт»;
//   - предупреждения сборки (RebuildResult.Warnings) — каждое в тексте ответа;
//   - строка «мастер: … LUFS» — и когда мастер пришёл из записи-пресета (без skip_master), а у варианта есть lufs.
//
// Воркер — gtFake (sound_preset_guitarist_test.go: дорожки с гитарой, план трека, наборы) + мастер на воркере
// с замером громкости (rpFake). Толкование: строка мастера узнаётся по «мастер» и «LUFS» в ответе.

// rpFake — gtFake, у которого мастер на весь микс (не превью) отвечает вариантом с замером громкости.
type rpFake struct {
	*gtFake
	masters []yue.FxRequest
}

func (f *rpFake) ApplyFx(ctx context.Context, id int64, req yue.FxRequest) (*yue.DspVariant, error) {
	if !req.Preview && req.Source == "mix" {
		f.mu.Lock()
		f.masters = append(f.masters, req)
		f.mu.Unlock()
		return &yue.DspVariant{File: req.File, Metrics: map[string]any{"lufs": -13.2, "true_peak_db": -1.0}}, nil
	}
	return f.gtFake.ApplyFx(ctx, id, req)
}

func newRebuildPresetServer(t *testing.T) (*Server, *rpFake) {
	t.Helper()
	s, g := newGuitaristPresetServer(t)
	g.grid = &yue.ChordGrid{BPM: 120, Bars: []yue.ChordBar{
		{Start: 0, End: 2, Chord: "Am", Section: "verse"}, {Start: 2, End: 4, Chord: "C", Section: "verse"}}}
	f := &rpFake{gtFake: g}
	s.client = f
	return s, f
}

// rpArgs — аргументы rebuild_sections: одна запись-пресет с рецептом recipe (JSON) и перекрытиями extra.
func rpArgs(t *testing.T, recipe, extra string) map[string]any {
	t.Helper()
	ref := `{"preset":` + recipe
	if extra != "" {
		ref += "," + extra
	}
	ref += "}"
	return jsonArgs(t, `{"job_id":466,"specs":[{"from":0,"to":0,"stems":[],"id":"-900","preset":`+ref+`}]}`)
}

func TestRebuildSectionsPresetOnlyTargetLUFS(t *testing.T) {
	t.Parallel()
	s, f := newRebuildPresetServer(t)
	out, ok := call(t, s, "rebuild_sections", rpArgs(t,
		`{"id":61,"name":"Громкость","specs":[],"final":[],"master":[],"target_lufs":-13}`, ""))
	if !ok {
		t.Fatalf("рецепт только с target_lufs отклонён: %s", out)
	}
	if len(f.masters) != 1 {
		t.Errorf("мастер на воркере %d раз, want 1 (цель громкости — ограничитель мастера)", len(f.masters))
	}
}

func TestRebuildSectionsPresetOnlyParts(t *testing.T) {
	t.Parallel()
	s, _ := newRebuildPresetServer(t)
	out, ok := call(t, s, "rebuild_sections", rpArgs(t,
		`{"id":62,"name":"Синт","specs":[],"final":[],"master":[],"target_lufs":null,
		  "parts":[{"kind":"synth","engine":[{"type":"synth","osc1":4}],"style":"pad"}]}`, ""))
	if !ok {
		t.Fatalf("рецепт только с parts отклонён: %s", out)
	}
}

func TestRebuildSectionsPresetEmptyRecipeIsError(t *testing.T) {
	t.Parallel()
	for name, recipe := range map[string]string{
		"всё пусто": `{"id":63,"name":"Пусто","specs":[],"final":[],"master":[],"target_lufs":null,"parts":[]}`,
		"полей нет": `{"id":64,"name":"Пусто"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, f := newRebuildPresetServer(t)
			out, ok := call(t, s, "rebuild_sections", rpArgs(t, recipe, ""))
			if ok {
				t.Fatalf("пустой рецепт принят: %s", out)
			}
			if !strings.Contains(out, "пустой рецепт") {
				t.Errorf("ошибка %q, want «пустой рецепт»", out)
			}
			if len(f.fakeService.uploads) != 0 {
				t.Errorf("при ошибке загружено: %d", len(f.fakeService.uploads))
			}
		})
	}
}

// Предупреждение подготовки пресета (гитарист у трека без плана) — в тексте ответа.
func TestRebuildSectionsPresetWarningsInAnswer(t *testing.T) {
	t.Parallel()
	s, f := newRebuildPresetServer(t)
	f.gridErr = &yue.StatusError{Code: 404, Msg: "нет плана"}
	var recipe yue.SoundPreset
	for _, p := range f.presets {
		if p.ID == gtPresetID {
			recipe = p
		}
	}
	b, _ := json.Marshal(recipe)
	out, ok := call(t, s, "rebuild_sections", rpArgs(t, string(b), ""))
	if !ok {
		t.Fatalf("rebuild_sections: %s", out)
	}
	if !strings.Contains(strings.ToLower(out), "нет плана") {
		t.Errorf("в ответе нет предупреждения про план: %s", out)
	}
}

// Мастер пресета (без skip_master) и lufs у варианта — строка «мастер: … LUFS» в ответе.
func TestRebuildSectionsPresetMasterLine(t *testing.T) {
	t.Parallel()
	s, f := newRebuildPresetServer(t)
	out, ok := call(t, s, "rebuild_sections", rpArgs(t,
		`{"id":66,"name":"Мастер","specs":[{"stems":["vocals"],"chain":"eq","params":{"high":-1,"highf":7000}}],
		  "final":[],"master":[{"type":"glue","ratio":2}],"target_lufs":-13}`, ""))
	if !ok {
		t.Fatalf("rebuild_sections: %s", out)
	}
	if len(f.masters) != 1 {
		t.Fatalf("мастер на воркере %d раз, want 1", len(f.masters))
	}
	if !strings.Contains(strings.ToLower(out), "мастер") || !strings.Contains(out, "LUFS") {
		t.Errorf("в ответе нет строки «мастер: … LUFS»: %s", out)
	}
}

// Контроль: skip_master — мастера пресета нет, строки мастера нет.
func TestRebuildSectionsPresetSkipMasterNoLine(t *testing.T) {
	t.Parallel()
	s, f := newRebuildPresetServer(t)
	out, ok := call(t, s, "rebuild_sections", rpArgs(t,
		`{"id":67,"name":"Мастер","specs":[{"stems":["vocals"],"chain":"eq","params":{"high":-1,"highf":7000}}],
		  "final":[],"master":[{"type":"glue","ratio":2}],"target_lufs":-13}`, `"skip_master":true`))
	if !ok {
		t.Fatalf("rebuild_sections: %s", out)
	}
	if len(f.masters) != 0 {
		t.Errorf("мастер на воркере %d раз при skip_master, want 0", len(f.masters))
	}
	if strings.Contains(out, "LUFS") {
		t.Errorf("строка мастера при skip_master: %s", out)
	}
}
