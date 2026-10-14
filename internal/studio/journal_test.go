package studio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"partscaster/internal/dsp"
	"partscaster/internal/yue"
)

// Составные операции
// studio после УСПЕХА пишут ровно одну запись журнала через OpRecorder сервиса:
//   RebuildSections → rebuild_sections (result — файл микса, refs — child_id),
//   VolumeEnvelope → envelope, Splice → splice (refs — job_id кусков),
//   ApplyChain → dsp_apply, ApplySteps → pedals,
//   ApplySoundPreset → sound_preset (params — пресет целиком, result job:<версия>).
// Внутренние вызовы сервиса при этом не журналируются (проверка — через yue.Journaled:
// без «контекста без журнала» внутренний VariantToTrack/ApplyFx дал бы лишние записи).
// Ошибка правки — записи нет; сбой записи — правка успешна.
//
// jrRec добавляет к фейкам secFake/psFake
// внешнюю границу «журнал воркера» (RecordOp).

type jrEntry struct {
	jobID int64
	op    yue.JobOp
}

type jrRec struct {
	yue.Service
	recs   []jrEntry
	recErr error
}

func (r *jrRec) RecordOp(_ context.Context, jobID int64, op yue.JobOp) error {
	r.recs = append(r.recs, jrEntry{jobID, op})
	return r.recErr
}

// jrOne — ровно одна запись с этим op у трека jobID.
func jrOne(t *testing.T, r *jrRec, op string, jobID int64) yue.JobOp {
	t.Helper()
	if len(r.recs) != 1 {
		var ops []string
		for _, e := range r.recs {
			ops = append(ops, fmt.Sprintf("%d:%s", e.jobID, e.op.Op))
		}
		t.Fatalf("записей журнала %d %v, want 1 (%s)", len(r.recs), ops, op)
	}
	e := r.recs[0]
	if e.op.Op != op || e.jobID != jobID {
		t.Fatalf("запись %d:%s, want %d:%s", e.jobID, e.op.Op, jobID, op)
	}
	var v any
	if err := json.Unmarshal(e.op.Params, &v); err != nil {
		t.Fatalf("params %s — не JSON: %v", e.op.Params, err)
	}
	return e.op
}

// jrHasNum — в JSON на любой глубине есть ключ k со значением n.
func jrHasNum(v any, k string, n float64) bool {
	switch x := v.(type) {
	case map[string]any:
		for kk, vv := range x {
			if kk == k {
				if f, ok := vv.(float64); ok && f == n {
					return true
				}
			}
			if jrHasNum(vv, k, n) {
				return true
			}
		}
	case []any:
		for _, vv := range x {
			if jrHasNum(vv, k, n) {
				return true
			}
		}
	}
	return false
}

// jrHasStr — в JSON на любой глубине есть строка s (значение).
func jrHasStr(v any, s string) bool {
	switch x := v.(type) {
	case string:
		return x == s
	case map[string]any:
		for _, vv := range x {
			if jrHasStr(vv, s) {
				return true
			}
		}
	case []any:
		for _, vv := range x {
			if jrHasStr(vv, s) {
				return true
			}
		}
	}
	return false
}

func jrDecode(t *testing.T, raw json.RawMessage) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("params %s: %v", raw, err)
	}
	return v
}

func jrRefs(refs []int64, want ...int64) bool {
	for _, w := range want {
		found := false
		for _, r := range refs {
			if r == w {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// onlyUpload — имя единственного загруженного варианта.
func onlyUpload(t *testing.T, f *secFake) string {
	t.Helper()
	if len(f.uploads) != 1 {
		t.Fatalf("загрузок %d, want 1: %v", len(f.uploads), keys(f.uploads))
	}
	for n := range f.uploads {
		return n
	}
	return ""
}

// Пересборка → rebuild_sections у родителя, result — файл микса, refs — child_id, params — спеки.
// Через Journaled — одна запись (внутренние вызовы не пишутся).
func TestJournalRebuildSections(t *testing.T) {
	t.Parallel()
	for _, wrap := range []bool{false, true} {
		t.Run(fmt.Sprintf("journaled=%v", wrap), func(t *testing.T) {
			t.Parallel()
			f, _ := secSetup(t)
			rec := &jrRec{Service: f}
			var svc yue.Service = rec
			if wrap {
				svc = yue.Journaled(rec)
			}
			if _, err := RebuildSections(context.Background(), svc, parentID,
				[]SectionSpec{spec7([]string{"other"}, 0)}); err != nil {
				t.Fatalf("RebuildSections: %v", err)
			}
			op := jrOne(t, rec, "rebuild_sections", parentID)
			if op.Result != "overdub-inst-7.flac" {
				t.Errorf("result %q, want overdub-inst-7.flac (файл микса)", op.Result)
			}
			if !jrRefs(op.Refs, 7) {
				t.Errorf("refs %v, want с child_id 7", op.Refs)
			}
			if !jrHasNum(jrDecode(t, op.Params), "child_id", 7) {
				t.Errorf("params %s без спеки child_id 7", op.Params)
			}
		})
	}
}

// Ошибка пересборки → записи нет.
func TestJournalRebuildSectionsErrorNoRecord(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	dir := t.TempDir()
	f := newSecFake()
	f.stemsFailed = true
	put(f, parentID, parentFiles(t, dir))
	f.files[key(7, "audio.flac")] = childFiles(t, dir, 7, 8, exprClicks)["audio.flac"]
	rec := &jrRec{Service: f}
	if _, err := RebuildSections(context.Background(), yue.Journaled(rec), parentID,
		[]SectionSpec{spec7([]string{"other"}, 0)}); err == nil {
		t.Fatal("ошибки нет, want ошибка: стемов ребёнка нет")
	}
	if len(rec.recs) != 0 {
		t.Errorf("записи при ошибке: %+v", rec.recs)
	}
}

// Сбой записи журнала — правка успешна, вариант загружен.
func TestJournalRebuildSectionsRecordFailureKeepsEdit(t *testing.T) {
	t.Parallel()
	f, _ := secSetup(t)
	rec := &jrRec{Service: f, recErr: errors.New("журнал недоступен")}
	res, err := RebuildSections(context.Background(), rec, parentID, []SectionSpec{spec7([]string{"other"}, 0)})
	if err != nil || res == nil {
		t.Fatalf("RebuildSections = %v, %v; want успех при сбое журнала", res, err)
	}
	if _, ok := f.uploads["overdub-inst-7.flac"]; !ok {
		t.Errorf("микс не загружен: %v", keys(f.uploads))
	}
}

func TestJournalVolumeEnvelope(t *testing.T) {
	t.Parallel()
	f := envTrack(t)
	rec := &jrRec{Service: f}
	if _, err := VolumeEnvelope(context.Background(), yue.Journaled(rec), parentID, "", rampPts); err != nil {
		t.Fatalf("VolumeEnvelope: %v", err)
	}
	op := jrOne(t, rec, "envelope", parentID)
	if op.Result != onlyUpload(t, f) {
		t.Errorf("result %q, want загруженный файл %q", op.Result, onlyUpload(t, f))
	}
	if !jrHasNum(jrDecode(t, op.Params), "db", -12) && !jrHasNum(jrDecode(t, op.Params), "Db", -12) {
		t.Errorf("params %s без точек огибающей (−12 дБ)", op.Params)
	}
}

func TestJournalVolumeEnvelopeErrorNoRecord(t *testing.T) {
	t.Parallel()
	f := newSecFake() // у трека нет звука
	rec := &jrRec{Service: f}
	if _, err := VolumeEnvelope(context.Background(), rec, parentID, "", rampPts); err == nil {
		t.Fatal("ошибки нет, want ошибка: звука трека нет")
	}
	if len(rec.recs) != 0 {
		t.Errorf("записи при ошибке: %+v", rec.recs)
	}
}

// Склейка → splice у базы, refs — job_id кусков, result — файл склейки.
func TestJournalSplice(t *testing.T) {
	t.Parallel()
	f := spliceSetup(t)
	rec := &jrRec{Service: f}
	parts := []SplicePart{{JobID: spliceA, From: 0, To: 3}, {JobID: spliceB, From: 3, To: 6}}
	if _, err := Splice(context.Background(), yue.Journaled(rec), spliceBase, parts, 0.05); err != nil {
		t.Fatalf("Splice: %v", err)
	}
	op := jrOne(t, rec, "splice", spliceBase)
	if op.Result != onlyUpload(t, f) {
		t.Errorf("result %q, want %q", op.Result, onlyUpload(t, f))
	}
	if !jrRefs(op.Refs, spliceA, spliceB) {
		t.Errorf("refs %v, want job_id кусков %d, %d", op.Refs, spliceA, spliceB)
	}
	p := jrDecode(t, op.Params)
	if !jrHasNum(p, "job_id", float64(spliceA)) || !jrHasNum(p, "job_id", float64(spliceB)) {
		t.Errorf("params %s без кусков (job_id)", op.Params)
	}
}

func TestJournalSpliceErrorNoRecord(t *testing.T) {
	t.Parallel()
	f := spliceSetup(t)
	rec := &jrRec{Service: f}
	parts := []SplicePart{{JobID: spliceA, From: 0, To: 3}, {JobID: 999, From: 0, To: 3}} // 999 — нет звука
	if _, err := Splice(context.Background(), rec, spliceBase, parts, 0.05); err == nil {
		t.Fatal("ошибки нет, want ошибка: звука куска нет")
	}
	if len(rec.recs) != 0 {
		t.Errorf("записи при ошибке: %+v", rec.recs)
	}
}

// jrTrack — трек envTrack (secFake) со списком треков воркера: конвейер DSP берёт имя звука из Jobs.
type jrTrack struct{ *secFake }

func (jrTrack) Jobs(context.Context) ([]yue.Job, error) {
	return []yue.Job{{ID: parentID, Status: "done", AudioFile: "audio.flac"}}, nil
}

func TestJournalApplyChain(t *testing.T) {
	t.Parallel()
	f := envTrack(t)
	rec := &jrRec{Service: jrTrack{f}}
	if _, err := ApplyChain(context.Background(), yue.Journaled(rec), parentID, "wall",
		map[string]float64{"wall": 0.4}); err != nil {
		t.Fatalf("ApplyChain: %v", err)
	}
	op := jrOne(t, rec, "dsp_apply", parentID)
	if op.Result != onlyUpload(t, f) {
		t.Errorf("result %q, want %q", op.Result, onlyUpload(t, f))
	}
	p := jrDecode(t, op.Params)
	if !jrHasStr(p, "wall") || !jrHasNum(p, "wall", 0.4) {
		t.Errorf("params %s: want цепочка wall и её крутилки", op.Params)
	}
}

func TestJournalApplyChainErrorNoRecord(t *testing.T) {
	t.Parallel()
	f := envTrack(t)
	rec := &jrRec{Service: jrTrack{f}}
	if _, err := ApplyChain(context.Background(), rec, parentID, "no-such-chain", nil); err == nil {
		t.Fatal("ошибки нет для неизвестной цепочки")
	}
	if len(rec.recs) != 0 || len(f.uploads) != 0 {
		t.Errorf("записи %+v / загрузки %v при ошибке", rec.recs, keys(f.uploads))
	}
}

func TestJournalApplySteps(t *testing.T) {
	t.Parallel()
	f := envTrack(t)
	rec := &jrRec{Service: jrTrack{f}}
	steps := []dsp.Step{{Chain: "level", Params: map[string]float64{"gain": -6, "ceiling": -0.5}}}
	if _, err := ApplySteps(context.Background(), yue.Journaled(rec), parentID, steps, ""); err != nil {
		t.Fatalf("ApplySteps: %v", err)
	}
	op := jrOne(t, rec, "pedals", parentID)
	if op.Result != onlyUpload(t, f) {
		t.Errorf("result %q, want %q", op.Result, onlyUpload(t, f))
	}
	p := jrDecode(t, op.Params)
	if !jrHasStr(p, "level") || !jrHasNum(p, "gain", -6) {
		t.Errorf("params %s: want шаги (level, gain −6)", op.Params)
	}
}

func TestJournalApplyStepsErrorNoRecord(t *testing.T) {
	t.Parallel()
	f := envTrack(t)
	rec := &jrRec{Service: jrTrack{f}}
	if _, err := ApplySteps(context.Background(), rec, parentID, []dsp.Step{{Chain: "no-such-chain"}}, ""); err == nil {
		t.Fatal("ошибки нет для неизвестной педали")
	}
	if len(rec.recs) != 0 {
		t.Errorf("записи при ошибке: %+v", rec.recs)
	}
}

// Пресет → одна запись sound_preset у трека, params — пресет ЦЕЛИКОМ,
// result job:<версия>; внутренний VariantToTrack через Journaled не пишется.
func TestJournalApplySoundPreset(t *testing.T) {
	t.Parallel()
	f := psSetup(t, nil)
	rec := &jrRec{Service: f}
	p := psPreset(9, nil, []yue.PresetStep{{Chain: "level", Params: map[string]float64{"gain": -6, "ceiling": -0.5}}})
	res, err := ApplySoundPreset(withoutLiveLayer(context.Background()), yue.Journaled(rec), psJobID, p)
	if err != nil || res == nil || res.ChildID != psChild {
		t.Fatalf("ApplySoundPreset = %+v, %v", res, err)
	}
	op := jrOne(t, rec, "sound_preset", psJobID)
	if op.Result != fmt.Sprintf("job:%d", psChild) {
		t.Errorf("result %q, want job:%d", op.Result, psChild)
	}
	// пресет целиком — в params (сам объект или внутри обёртки): имя, финал, крутилки
	pv := jrDecode(t, op.Params)
	if !jrHasStr(pv, p.Name) || !jrHasStr(pv, "level") || !jrHasNum(pv, "gain", -6) || !jrHasNum(pv, "ceiling", -0.5) {
		t.Errorf("params %s, want пресет целиком %+v", op.Params, p)
	}
}

func TestJournalApplySoundPresetErrorNoRecord(t *testing.T) {
	t.Parallel()
	f := psSetup(t, nil)
	f.promoteErr = errors.New("версия не создана")
	rec := &jrRec{Service: f}
	p := psPreset(9, nil, []yue.PresetStep{{Chain: "level", Params: map[string]float64{"gain": -6}}})
	if _, err := ApplySoundPreset(withoutLiveLayer(context.Background()), yue.Journaled(rec), psJobID, p); err == nil {
		t.Fatal("ошибки нет, want ошибка версии")
	}
	if len(rec.recs) != 0 {
		t.Errorf("записи при ошибке: %+v", rec.recs)
	}
}

// Пересборка с мастером — внутренний ApplyFx мастера (не превью) через Journaled
// не даёт своей записи fx: в журнале одна rebuild_sections.
func TestJournalRebuildSectionsMasterInnerFxNotRecorded(t *testing.T) {
	t.Parallel()
	f, _ := msSetup(t)
	rec := &jrRec{Service: f}
	if _, err := RebuildSections(context.Background(), yue.Journaled(rec), parentID,
		[]SectionSpec{msMute(), msMaster()}); err != nil {
		t.Fatalf("RebuildSections: %v", err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("ApplyFx мастера вызван %d раз, want 1", len(f.calls))
	}
	if op := jrOne(t, rec, "rebuild_sections", parentID); op.Result != "overdub-inst-0.flac" {
		t.Errorf("result %q, want overdub-inst-0.flac", op.Result)
	}
}
