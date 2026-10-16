package studio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"partscaster/internal/yue"
)

// «Применить всё» (internal-studio-draft, условие 3, ТК3):
// ApplyEdits(ctx, svc, parentID, specs, title) → id нового трека:
//   RebuildSections (как сейчас, с записью rebuild_sections в журнал)
//   → VariantToTrack(parentID, файл микса, title)
//   → DraftDelete(parentID).
// Пустые specs — ошибка без вызовов. Ошибка любого шага — следующие не вызываются.
//
// apFake — фейковый воркер поверх secFake (скачивание/стемы/загрузка) + внешние
// границы «новый трек из варианта», «черновик» и «журнал».

type apFake struct {
	*secFake
	steps      []string
	recs       []jrEntry
	newID      int64
	vtErr      error
	vtArgs     []string // "job/file/title"
	delCalls   []int64
	delErr     error
	otherCalls []string
}

func newApFake(f *secFake) *apFake { return &apFake{secFake: f, newID: 501} }

func (a *apFake) UploadDsp(ctx context.Context, id int64, fname, label string, data []byte) (*yue.DspVariant, error) {
	a.steps = append(a.steps, "upload:"+fname)
	return a.secFake.UploadDsp(ctx, id, fname, label, data)
}

func (a *apFake) RecordOp(_ context.Context, jobID int64, op yue.JobOp) error {
	a.steps = append(a.steps, "journal:"+op.Op)
	a.recs = append(a.recs, jrEntry{jobID, op})
	return nil
}

func (a *apFake) VariantToTrack(_ context.Context, jobID int64, file, title string, _ int64) (int64, error) {
	a.steps = append(a.steps, "variant_track")
	a.vtArgs = append(a.vtArgs, fmt.Sprintf("%d/%s/%s", jobID, file, title))
	if a.vtErr != nil {
		return 0, a.vtErr
	}
	return a.newID, nil
}

func (a *apFake) DraftDelete(_ context.Context, jobID int64) (bool, error) {
	a.steps = append(a.steps, "draft_delete")
	a.delCalls = append(a.delCalls, jobID)
	return a.delErr == nil, a.delErr
}

func (a *apFake) Drafts(context.Context) ([]yue.Draft, error) {
	a.otherCalls = append(a.otherCalls, "Drafts")
	return nil, nil
}

func (a *apFake) DraftPut(context.Context, int64, json.RawMessage) error {
	a.otherCalls = append(a.otherCalls, "DraftPut")
	return nil
}

func stepIndex(steps []string, prefix string) int {
	for i, s := range steps {
		if strings.HasPrefix(s, prefix) {
			return i
		}
	}
	return -1
}

func TestApplyEditsRebuildThenTrackThenClearDraft(t *testing.T) {
	t.Parallel()
	f, _ := secSetup(t)
	a := newApFake(f)
	id, err := ApplyEdits(context.Background(), a, parentID,
		[]SectionSpec{spec7([]string{"other"}, 0)}, "Песня · итог")
	if err != nil {
		t.Fatalf("ApplyEdits: %v", err)
	}
	if id != a.newID {
		t.Errorf("id %d, want %d (id трека от VariantToTrack)", id, a.newID)
	}
	// пересборка — как сейчас: микс загружен родителю
	file := onlyUpload(t, f)
	want := fmt.Sprintf("%d/%s/%s", parentID, file, "Песня · итог")
	if len(a.vtArgs) != 1 || a.vtArgs[0] != want {
		t.Errorf("VariantToTrack %v, want один вызов %s (файл результата пересборки и title)", a.vtArgs, want)
	}
	if len(a.delCalls) != 1 || a.delCalls[0] != parentID {
		t.Errorf("DraftDelete %v, want один вызов для %d", a.delCalls, parentID)
	}
	up, vt, del := stepIndex(a.steps, "upload:"), stepIndex(a.steps, "variant_track"), stepIndex(a.steps, "draft_delete")
	if up < 0 || vt < 0 || del < 0 || up >= vt || vt >= del {
		t.Errorf("порядок шагов %v, want пересборка → variant_track → draft_delete", a.steps)
	}
	if len(a.otherCalls) != 0 {
		t.Errorf("лишние вызовы черновика: %v", a.otherCalls)
	}
}

// Пересборка пишется в журнал, как сейчас (rebuild_sections у родителя с файлом микса).
func TestApplyEditsJournalsRebuild(t *testing.T) {
	t.Parallel()
	f, _ := secSetup(t)
	a := newApFake(f)
	if _, err := ApplyEdits(context.Background(), a, parentID,
		[]SectionSpec{spec7([]string{"other"}, 0)}, "t"); err != nil {
		t.Fatalf("ApplyEdits: %v", err)
	}
	n := 0
	for _, e := range a.recs {
		if e.op.Op == "rebuild_sections" {
			n++
			if e.jobID != parentID {
				t.Errorf("rebuild_sections у %d, want %d", e.jobID, parentID)
			}
			if e.op.Result != onlyUpload(t, f) {
				t.Errorf("result %q, want файл микса", e.op.Result)
			}
		}
	}
	if n != 1 {
		t.Errorf("записей rebuild_sections %d, want 1; журнал: %+v", n, a.recs)
	}
}

// Ошибка пересборки → VariantToTrack и DraftDelete не вызываются, черновик цел.
func TestApplyEditsRebuildErrorStops(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	dir := t.TempDir()
	f := newSecFake()
	f.stemsFailed = true
	put(f, parentID, parentFiles(t, dir))
	f.files[key(7, "audio.flac")] = childFiles(t, dir, 7, 8, exprClicks)["audio.flac"]
	a := newApFake(f)
	id, err := ApplyEdits(context.Background(), a, parentID,
		[]SectionSpec{spec7([]string{"other"}, 0)}, "t")
	if err == nil {
		t.Fatal("ошибки нет, want ошибка пересборки")
	}
	if id != 0 {
		t.Errorf("id %d при ошибке, want 0", id)
	}
	if len(a.vtArgs) != 0 || len(a.delCalls) != 0 {
		t.Errorf("после ошибки пересборки вызваны VariantToTrack %v / DraftDelete %v", a.vtArgs, a.delCalls)
	}
}

// Ошибка VariantToTrack → черновик не удаляется (правки не применены).
func TestApplyEditsVariantErrorKeepsDraft(t *testing.T) {
	t.Parallel()
	f, _ := secSetup(t)
	a := newApFake(f)
	a.vtErr = errors.New("воркер недоступен")
	if _, err := ApplyEdits(context.Background(), a, parentID,
		[]SectionSpec{spec7([]string{"other"}, 0)}, "t"); err == nil {
		t.Fatal("ошибки нет, want ошибка VariantToTrack")
	}
	if len(a.delCalls) != 0 {
		t.Errorf("DraftDelete вызван после ошибки VariantToTrack: %v", a.delCalls)
	}
}

// Пустые specs — ошибка без вызовов сервиса.
func TestApplyEditsEmptySpecs(t *testing.T) {
	t.Parallel()
	for _, specs := range [][]SectionSpec{nil, {}} {
		f := newSecFake()
		a := newApFake(f)
		if _, err := ApplyEdits(context.Background(), a, parentID, specs, "t"); err == nil {
			t.Errorf("specs=%v: ошибки нет, want ошибка", specs)
		}
		if len(a.steps) != 0 || len(f.fetched) != 0 || len(f.stemsCalls) != 0 {
			t.Errorf("specs=%v: были вызовы %v / fetched %v / stems %v", specs, a.steps, f.fetched, f.stemsCalls)
		}
	}
}
