package mcp

import (
	"encoding/json"
	"math"
	"os/exec"
	"reflect"
	"testing"

	"yue-studio/internal/dsp"
	"yue-studio/internal/yue"
)

// dsp_presets отдаёт
// наборы педалей {id, name, note, steps}; dsp_apply и rebuild_sections (спека)
// принимают steps. Воркер —
// fakeService (server_test.go), звук — синтетика ffmpeg (fxLavfi/fxDecode/fxTone
// из stem_fx_test.go).

// dsp_apply со steps без stem — шаги на весь трек: два шага «−6 дБ»
// последовательно дают −12 дБ у тона; результат — один новый вариант у джобы.
func TestDspApplyStepsWholeTrack(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	s, fake := newTestServer(t)
	fake.jobs = []yue.Job{{ID: 5, Status: "done", AudioFile: "audio.flac"}}
	fake.fetch = map[string]string{"audio.flac": fxLavfi(t, "0.1*sin(2*PI*440*t)", 4)}
	out, ok := call(t, s, "dsp_apply", jsonArgs(t, `{"job_id":5,"steps":[
		{"chain":"level","params":{"gain":-6}},
		{"chain":"level","params":{"gain":-6}},
		{"chain":"level","params":{"gain":-6},"off":true}]}`))
	if !ok {
		t.Fatalf("dsp_apply со steps без stem: %s", out)
	}
	if len(fake.uploads) != 1 {
		t.Fatalf("загрузок %d (%v), want 1 — один вариант", len(fake.uploads), fake.uploads)
	}
	var res []float32
	for _, data := range fake.uploads {
		res = fxDecode(t, data)
	}
	if d := 20 * math.Log10(fxTone(res, 440, 0.5, 3.5)/0.1); math.Abs(d-(-12)) > 1 {
		t.Errorf("440 Гц: %+.1f дБ к исходному, want −12 ± 1 (два включённых шага по −6, третий выключен)", d)
	}
}

// dsp_apply со steps и неизвестной цепочкой — ошибка, ничего не загружено.
func TestDspApplyStepsUnknownChainIsError(t *testing.T) {
	t.Parallel()
	s, fake := newTestServer(t)
	fake.jobs = []yue.Job{{ID: 5, Status: "done", AudioFile: "audio.flac"}}
	out, ok := call(t, s, "dsp_apply", jsonArgs(t, `{"job_id":5,"steps":[{"chain":"no-such-chain"}]}`))
	if ok {
		t.Fatalf("неизвестная цепочка в steps: want ошибку, ответ: %s", out)
	}
	if len(fake.uploads) != 0 {
		t.Errorf("при ошибке загружено: %v", fake.uploads)
	}
}

// Спека rebuild_sections принимает steps (chain/off/params).
func TestParseSectionSpecsSteps(t *testing.T) {
	t.Parallel()
	var raw any
	if err := json.Unmarshal([]byte(`[
		{"from":2,"to":6,"stems":["guitar"],"steps":[
			{"chain":"od-ts","params":{"drive":5}},
			{"chain":"reverb-room","off":true}]}
	]`), &raw); err != nil {
		t.Fatal(err)
	}
	got, err := parseSectionSpecs(raw)
	if err != nil {
		t.Fatalf("parseSectionSpecs: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("спек %d, want 1: %+v", len(got), got)
	}
	want := []dsp.Step{{Chain: "od-ts", Params: map[string]float64{"drive": 5}}, {Chain: "reverb-room", Off: true}}
	if !reflect.DeepEqual(got[0].Steps, want) {
		t.Errorf("Steps = %+v, want %+v", got[0].Steps, want)
	}
	if got[0].Chain != "" || !reflect.DeepEqual(got[0].Stems, []string{"guitar"}) {
		t.Errorf("спека: %+v, want Chain пустой, Stems [guitar]", got[0])
	}
}
