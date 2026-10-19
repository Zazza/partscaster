package studio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"partscaster/internal/yue"
)

// Сверка баланса при итоге черновика (internal-preset-verify-on-apply, условия 3–10, 12; ТК2–ТК10, ТК12).
//
// Контракт:
//
//	SectionSpec.VerifyDb *float64 `json:"verify_db,omitempty"` — цель уровня дорожки записи (Stems[0]) к треку, дБ;
//	ApplyEdits(ctx, svc, parentID, specs, title, mixFile) (int64, error) — mixFile "" — собрать;
//	  иначе первой сборки нет, первая версия — из этого файла варианта трека.
//
// Поведение по карточке: нет ни одной цели — одна сборка, одна версия, без разделения. Есть цели — сборка →
// версия → разделение версии RoFormer → замер дорожек к треку; промах > 1 дБ → у записей с целью поправка Db
// −ошибка (≤ ±6 за проход, сумма ≤ ±9) → пересборка → новая версия, прежняя удаляется; исправляющих проходов
// ≤ 2. Итог — одна новая версия с названием из вызова. Сбой сверки — не ошибка (остаётся последняя версия);
// сбой исправляющей пересборки — остаётся прежняя версия; сбой первой сборки — ошибка, версий нет, черновик
// цел. Отмена — ни одной новой версии.
//
// Воркер — pdVf (preset_draft_test.go) поверх vfFake/prlFake: замеры дорожек трека, движок отдаёт кусок
// 1000 Гц, версии получают id 900, 901, 902…, замеры версии — только после её разделения, гитара версии к
// треку = цель + guitarErr[версия], бас — ровно у цели. Добавлены внешние границы «черновик»
// (DraftDelete) и «бас версии сдвинут» (bassErr — для ТК6).
//
// Записи итога — такие, какие давала подготовка пресета vfPreset: гитара (движок, level_target −10 +
// level_total, verify_db −10) и бас (Db по замеру трека, verify_db −10).
//
// Как видно: каждая загрузка — пересборка (записи без финала и мастера); гейн гитары — тон 1000 Гц в загрузке
// к амплитуде куска; бас — тон 200 Гц к исходному треку.

// aeFake — pdVf + черновик + сдвиг баса в замерах версий.
type aeFake struct {
	*pdVf
	draftDel []int64
	bassErr  map[int64]float64 // версия → бас к треку = цель + bassErr
}

func (f *aeFake) DraftDelete(_ context.Context, jobID int64) (bool, error) {
	f.draftDel = append(f.draftDel, jobID)
	return true, nil
}

func (f *aeFake) Drafts(context.Context) ([]yue.Draft, error) { return nil, nil }

func (f *aeFake) DraftPut(context.Context, int64, json.RawMessage) error { return nil }

// JobStems — как у vfFake; у версии с bassErr бас громче на bassErr (rms_p95_db).
func (f *aeFake) JobStems(ctx context.Context, id int64) ([]map[string]any, error) {
	list, err := f.pdVf.JobStems(ctx, id)
	if err != nil || f.bassErr[id] == 0 {
		return list, err
	}
	for _, s := range list {
		if s["name"] != "bass" {
			continue
		}
		outer, _ := s["metrics"].(map[string]any)
		inner, _ := outer["metrics"].(map[string]any)
		if v, ok := inner["rms_p95_db"].(float64); ok {
			inner["rms_p95_db"] = v + f.bassErr[id]
		}
	}
	return list, nil
}

func aeSetup(t *testing.T, errs map[int64]float64) (*aeFake, []float32) {
	t.Helper()
	vf, orig := vfSetup(t, errs)
	return &aeFake{pdVf: &pdVf{vfFake: vf}, bassErr: map[int64]float64{}}, orig
}

// aeBassDb — Db баса первого прохода: цель − уровень баса по замеру трека.
func aeBassDb() float64 { return vfTarget - (prlSineDb(0.05) - prlTrackSum()) }

// aeGuitar — запись гитары, как её отдаёт черновик: движок, цель по обработанному куску, Db «на этот раз».
func aeGuitar(db float64, verify bool) SectionSpec {
	s := SectionSpec{Stems: []string{"guitar"}, Engine: engChain(), Db: db,
		LevelTarget: lvF(vfTarget), LevelTotal: prlTrackSum()}
	if verify {
		s.VerifyDb = lvF(vfTarget + db)
	}
	return s
}

func aeBass(verify bool) SectionSpec {
	s := SectionSpec{Stems: []string{"bass"}, Db: aeBassDb()}
	if verify {
		s.VerifyDb = lvF(vfTarget)
	}
	return s
}

func aeSpecs() []SectionSpec { return []SectionSpec{aeGuitar(0, true), aeBass(true)} }

const aeTitle = "Тёплый провод · итог"

func aeApply(t *testing.T, svc yue.Service, specs []SectionSpec, mixFile string) int64 {
	t.Helper()
	id, err := ApplyEdits(context.Background(), svc, psJobID, specs, aeTitle, mixFile)
	if err != nil {
		t.Fatalf("ApplyEdits: %v", err)
	}
	return id
}

// aePass — гейны пересборки: гитара (вставка куска к его амплитуде) и бас (к исходному треку), дБ.
type aePass struct{ guitar, bass float64 }

func (f *aeFake) passes(t *testing.T, orig []float32) []aePass {
	t.Helper()
	var out []aePass
	for _, u := range f.ups {
		s := decodeBytes(t, u.data)
		out = append(out, aePass{
			guitar: 20 * math.Log10(toneAmp(s, prlChunkHz, 2, 8)/prlChunkAmp),
			bass:   20 * math.Log10(toneAmp(s, 200, 2, 8)/toneAmp(orig, 200, 2, 8)),
		})
	}
	return out
}

// aeBaseGain — гейн вставки гитары без поправок: цель − уровень куска к треку (−6,02).
func aeBaseGain() float64 { return vfTarget - (prlSineDb(prlChunkAmp) - prlTrackSum()) }

// aeAlive — созданные версии, которые не удалены.
func aeAlive(f *aeFake) []int64 {
	var out []int64
	for _, id := range f.ids {
		if !contains(f.deleted, fmt.Sprintf("job %d", id)) {
			out = append(out, id)
		}
	}
	return out
}

// aeOneVersion — у трека осталась ровно одна новая версия want (с названием из вызова), прочие удалены.
func aeOneVersion(t *testing.T, f *aeFake, want int64) {
	t.Helper()
	alive := aeAlive(f)
	if len(alive) != 1 || alive[0] != want {
		t.Errorf("оставшиеся версии %v, want только #%d (созданы %v, удаления %v)", alive, want, f.ids, f.deleted)
	}
	title := ""
	for _, j := range f.jobs {
		if j.ID == want {
			title = j.Title
		}
	}
	if got := title; got != aeTitle {
		t.Errorf("название итоговой версии %q, want %q (название из вызова)", got, aeTitle)
	}
	if contains(f.deleted, fmt.Sprintf("job %d", psJobID)) {
		t.Errorf("удалён сам трек #%d: %v", psJobID, f.deleted)
	}
}

// aeShifts — гитара прохода k (k ≥ 2) к первому = shifts[k−2] ±0,1; бас во всех проходах как в первом.
func aeShifts(t *testing.T, f *aeFake, orig []float32, shifts []float64) {
	t.Helper()
	ps := f.passes(t, orig)
	if len(ps) != len(shifts)+1 {
		t.Fatalf("пересборок %d, want %d (загрузки %d, движок на гитару %d)", len(ps), len(shifts)+1, len(f.ups), f.guitarRuns())
	}
	for k, sh := range shifts {
		if d := ps[k+1].guitar - ps[0].guitar; math.Abs(d-sh) > 0.1 {
			t.Errorf("гитара в проходе %d: %+.2f дБ к первому, want %+.2f ±0,1", k+2, d, sh)
		}
		if d := ps[k+1].bass - ps[0].bass; math.Abs(d) > 0.1 {
			t.Errorf("бас в проходе %d: %+.2f дБ к первому, want 0 ±0,1 (бас у цели)", k+2, d)
		}
	}
}

// ---------- ТК2: без verify_db — одна сборка, одна версия, без разделения ----------

func TestApplyEditsNoVerifyOneBuild(t *testing.T) {
	t.Parallel()
	f, orig := aeSetup(t, map[int64]float64{vfFirst: 3.6}) // промах есть, но целей нет — не сверяется
	id := aeApply(t, f, []SectionSpec{aeGuitar(0, false), aeBass(false)}, "")
	if id != vfFirst {
		t.Errorf("id %d, want %d (единственная версия)", id, vfFirst)
	}
	aeShifts(t, f, orig, nil)
	if len(f.splitAsks) != 0 {
		t.Errorf("разделение версий %v, want ни одного (целей нет)", f.splitAsks)
	}
	if len(f.ids) != 1 {
		t.Errorf("версий создано %v, want одна", f.ids)
	}
	aeOneVersion(t, f, vfFirst)
	if len(f.draftDel) != 1 || f.draftDel[0] != psJobID {
		t.Errorf("DraftDelete %v, want один у #%d", f.draftDel, psJobID)
	}
}

// ---------- ТК3: цели есть, замер в допуске — одна сборка, одна версия, одно разделение ----------

func TestApplyEditsVerifyOnTarget(t *testing.T) {
	t.Parallel()
	f, orig := aeSetup(t, map[int64]float64{vfFirst: 0.5}) // ≤ 1 дБ — не промах
	id := aeApply(t, f, aeSpecs(), "")
	if id != vfFirst {
		t.Errorf("id %d, want %d", id, vfFirst)
	}
	aeShifts(t, f, orig, nil)
	if len(f.splitAsks) != 1 || f.splitAsks[0] != vfFirst {
		t.Errorf("разделение просили у %v, want одно разделение #%d", f.splitAsks, vfFirst)
	}
	aeOneVersion(t, f, vfFirst)
	if len(f.draftDel) != 1 {
		t.Errorf("DraftDelete %v, want один", f.draftDel)
	}
	// гитара первого прохода — у цели по куску, без поправок
	if ps := f.passes(t, orig); len(ps) == 1 {
		pdNear(t, "гейн гитары первого прохода", ps[0].guitar, aeBaseGain(), 0.3)
	}
}

// ---------- ТК4: промах 3 дБ → поправка −3, вторая сборка, первая версия удалена ----------

func TestApplyEditsVerifyOneFix(t *testing.T) {
	t.Parallel()
	f, orig := aeSetup(t, map[int64]float64{vfFirst: 3})
	id := aeApply(t, f, aeSpecs(), "")
	if id != vfFirst+1 {
		t.Errorf("id %d, want %d (версия после поправки)", id, vfFirst+1)
	}
	if !vfAsked(f.splitAsks, vfFirst) {
		t.Errorf("первая версия не разделена для сверки: %v", f.splitAsks)
	}
	aeShifts(t, f, orig, []float64{-3})
	aeOneVersion(t, f, vfFirst+1)
	if len(f.draftDel) != 1 {
		t.Errorf("DraftDelete %v, want один", f.draftDel)
	}
}

// ---------- ТК5: промах не уходит → ровно 2 исправляющих прохода, ≤ 6 за проход, сумма ≤ 9 ----------

func TestApplyEditsVerifyClampsAndStopsAtTwoFixes(t *testing.T) {
	t.Parallel()
	f, orig := aeSetup(t, map[int64]float64{vfFirst: 9, vfFirst + 1: 9, vfFirst + 2: 9, vfFirst + 3: 9})
	id := aeApply(t, f, aeSpecs(), "")
	aeShifts(t, f, orig, []float64{-6, -9}) // 2-й проход зажат 6, 3-й — сумма зажата 9
	if n := f.guitarRuns(); n != 3 {
		t.Errorf("сборок (движок на гитару) %d, want 3", n)
	}
	if len(f.ids) != 3 {
		t.Errorf("версий создано %v, want 3", f.ids)
	}
	if id != vfFirst+2 {
		t.Errorf("id %d, want %d (последняя сборка)", id, vfFirst+2)
	}
	for _, v := range []int64{vfFirst, vfFirst + 1} {
		if !contains(f.deleted, fmt.Sprintf("job %d", v)) {
			t.Errorf("версия #%d не удалена: %v", v, f.deleted)
		}
	}
	aeOneVersion(t, f, vfFirst+2)
}

// Промах 3 дБ остаётся после каждой поправки → всё равно 3 сборки, не больше.
func TestApplyEditsVerifyNoThirdFix(t *testing.T) {
	t.Parallel()
	f, _ := aeSetup(t, map[int64]float64{vfFirst: 3, vfFirst + 1: 3, vfFirst + 2: 3, vfFirst + 3: 3})
	id := aeApply(t, f, aeSpecs(), "")
	if n := f.guitarRuns(); n != 3 {
		t.Errorf("сборок %d, want ровно 3 (2 исправляющих прохода)", n)
	}
	aeOneVersion(t, f, id)
}

// Промах −3 (тише цели) → поправка +3: знак поправки — против ошибки.
func TestApplyEditsVerifyRaisesQuietStem(t *testing.T) {
	t.Parallel()
	f, orig := aeSetup(t, map[int64]float64{vfFirst: -3})
	aeApply(t, f, aeSpecs(), "")
	aeShifts(t, f, orig, []float64{3})
}

// Цель verify_db = level_db + db: гитара с db +3 → цель −7; версия на −7 — в допуске, поправки нет.
func TestApplyEditsVerifyTargetIncludesDb(t *testing.T) {
	t.Parallel()
	f, orig := aeSetup(t, map[int64]float64{vfFirst: 3}) // гитара −10 + 3 = −7 = цель
	aeApply(t, f, []SectionSpec{aeGuitar(3, true), aeBass(true)}, "")
	aeShifts(t, f, orig, nil)
	if ps := f.passes(t, orig); len(ps) == 1 {
		pdNear(t, "гейн гитары (db +3 сверх цели куска)", ps[0].guitar, aeBaseGain()+3, 0.3)
	}
}

// ---------- ТК6: записи без verify_db не поправляются ----------

// Гитара (цель) промахивается +3, бас без цели тоже «промахивается» +3 в замере версии, плюс правка человека
// «other −2» — после исправляющего прохода бас и other как в первом проходе, гитара поправлена.
func TestApplyEditsVerifyOnlyTargetsCorrected(t *testing.T) {
	t.Parallel()
	f, orig := aeSetup(t, map[int64]float64{vfFirst: 3, vfFirst + 1: 0})
	f.bassErr[vfFirst] = 3
	human := SectionSpec{Stems: []string{"other"}, Db: -2}
	aeApply(t, f, []SectionSpec{aeGuitar(0, true), aeBass(false), human}, "")
	ps := f.passes(t, orig)
	if len(ps) < 2 {
		t.Fatalf("сценарий: исправляющего прохода не было (пересборок %d)", len(ps))
	}
	other := func(u vfUpload) float64 {
		return 20 * math.Log10(toneAmp(decodeBytes(t, u.data), 500, 2, 8)/toneAmp(orig, 500, 2, 8))
	}
	for k := 1; k < len(ps); k++ {
		if d := ps[k].bass - ps[0].bass; math.Abs(d) > 0.1 {
			t.Errorf("бас без verify_db в проходе %d сдвинут на %+.2f дБ, want 0", k+1, d)
		}
		if d := other(f.ups[k]) - other(f.ups[0]); math.Abs(d) > 0.1 {
			t.Errorf("правка человека other в проходе %d сдвинута на %+.2f дБ, want 0", k+1, d)
		}
		if d := ps[k].guitar - ps[0].guitar; d > -1 {
			t.Errorf("гитара (цель) в проходе %d: %+.2f дБ к первому, want поправка вниз", k+1, d)
		}
	}
	pdNear(t, "other первого прохода (правка человека)", other(f.ups[0]), -2, 0.2)
}

// ---------- ТК7: сбои ----------

// Сбой разделения при сверке → итог — первая версия, без ошибки, без второй сборки.
func TestApplyEditsVerifySplitFailKeepsVersion(t *testing.T) {
	t.Parallel()
	f, orig := aeSetup(t, map[int64]float64{vfFirst: 3.6})
	f.splitFail[vfFirst] = true
	id := aeApply(t, f, aeSpecs(), "")
	if id != vfFirst {
		t.Errorf("id %d, want %d (первая версия)", id, vfFirst)
	}
	aeShifts(t, f, orig, nil)
	aeOneVersion(t, f, vfFirst)
}

// Сбой разделения второй версии → итог вторая версия, первая удалена.
func TestApplyEditsVerifySecondSplitFail(t *testing.T) {
	t.Parallel()
	f, orig := aeSetup(t, map[int64]float64{vfFirst: 3.6, vfFirst + 1: 1.6})
	f.splitFail[vfFirst+1] = true
	id := aeApply(t, f, aeSpecs(), "")
	if id != vfFirst+1 {
		t.Errorf("id %d, want %d", id, vfFirst+1)
	}
	aeShifts(t, f, orig, []float64{-3.6})
	aeOneVersion(t, f, vfFirst+1)
}

// Нет замера у дорожки версии (сверить нечем) → без поправок, без ошибки.
func TestApplyEditsVerifyNoMetricKeepsVersion(t *testing.T) {
	t.Parallel()
	f, orig := aeSetup(t, map[int64]float64{vfFirst: 3.6})
	f.noMetric["guitar"] = true
	id := aeApply(t, f, aeSpecs(), "")
	if id != vfFirst {
		t.Errorf("id %d, want %d", id, vfFirst)
	}
	aeShifts(t, f, orig, nil)
	aeOneVersion(t, f, vfFirst)
}

// Сбой исправляющей пересборки / её версии → остаётся прежняя версия, без ошибки (сбой не первой сборки).
func TestApplyEditsVerifyFixFailKeepsPrevious(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		guitar, trk int
	}{
		{"исправляющая пересборка упала", 2, 0},
		{"версия исправляющего прохода не создана", 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, _ := aeSetup(t, map[int64]float64{vfFirst: 3.6})
			f.failGuitarAt, f.failTrackAt = tc.guitar, tc.trk
			id, err := ApplyEdits(context.Background(), f, psJobID, aeSpecs(), aeTitle, "")
			if err != nil {
				t.Fatalf("ApplyEdits: %v, want без ошибки (остаётся прежняя версия)", err)
			}
			if tc.guitar > 0 && f.guitarN < tc.guitar {
				t.Fatalf("сценарий: исправляющей пересборки не было (движок на гитару %d раз)", f.guitarN)
			}
			if tc.trk > 0 && f.trackN < tc.trk {
				t.Fatalf("сценарий: вторую версию не просили (VariantToTrack %d раз)", f.trackN)
			}
			if id != vfFirst {
				t.Errorf("id %d, want %d (прежняя версия)", id, vfFirst)
			}
			aeOneVersion(t, f, vfFirst)
		})
	}
}

// Сбой первой сборки → ошибка, версий нет, черновик не удалён.
func TestApplyEditsFirstBuildFailIsError(t *testing.T) {
	t.Parallel()
	f, _ := aeSetup(t, map[int64]float64{vfFirst: 3.6})
	f.failGuitarAt = 1
	id, err := ApplyEdits(context.Background(), f, psJobID, aeSpecs(), aeTitle, "")
	if err == nil {
		t.Fatal("ошибки нет, want ошибка первой сборки")
	}
	if id != 0 {
		t.Errorf("id %d при ошибке, want 0", id)
	}
	if len(f.ids) != 0 || len(f.promoted) != 0 {
		t.Errorf("созданы версии %v, want ни одной", f.ids)
	}
	if len(f.draftDel) != 0 {
		t.Errorf("черновик удалён при ошибке: %v", f.draftDel)
	}
}

// ---------- ТК8: отмена во время сверки — ни одной новой версии ----------

// aeQueue — aeFake + очередь воркера: метка отмены — когда cancelAt() истинно; учёт операций и хода.
type aeQueue struct {
	*aeFake
	mu       sync.Mutex
	adds     []yue.QueueOpIn
	beats    []prgBeat
	finished []yue.QueueFinishIn
	cancelAt func() bool
}

func (q *aeQueue) QueueAdd(_ context.Context, in yue.QueueOpIn) (*yue.QueueOp, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.adds = append(q.adds, in)
	return &yue.QueueOp{ID: fakeOpID, Kind: in.Kind, Title: in.Title, JobID: in.JobID, State: "running"}, nil
}

func (q *aeQueue) QueueBeat(_ context.Context, id int64, progress ...yue.QueueProgress) (*yue.QueueOp, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	b := prgBeat{}
	if len(progress) > 0 {
		b.has, b.p = true, progress[0]
	}
	q.beats = append(q.beats, b)
	cancel := q.cancelAt != nil && q.cancelAt()
	return &yue.QueueOp{ID: id, State: "running", CancelRequested: cancel}, nil
}

func (q *aeQueue) QueueFinish(_ context.Context, id int64, in yue.QueueFinishIn) (*yue.QueueOp, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.finished = append(q.finished, in)
	return &yue.QueueOp{ID: id, State: in.State, ResultJobID: in.ResultJobID}, nil
}

// steps — шаги render/verify/fix по порядку, без повторов подряд.
func (q *aeQueue) steps() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out []string
	for _, b := range q.beats {
		if !b.has {
			continue
		}
		switch b.p.Stage {
		case "render", "verify", "fix":
		default:
			continue
		}
		l := prgLabel(b.p)
		if len(out) == 0 || out[len(out)-1] != l {
			out = append(out, l)
		}
	}
	return out
}

func TestApplyEditsCanceledDuringVerifyLeavesNoVersion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		after int // отмена, как только создано столько версий
	}{
		{"после первой версии (сверка 1)", 1},
		{"после второй версии (сверка 2)", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, _ := aeSetup(t, map[int64]float64{vfFirst: 3.6, vfFirst + 1: 1.6})
			q := &aeQueue{aeFake: f}
			q.cancelAt = func() bool { return len(f.ids) >= tc.after }
			_, err := ApplyEdits(context.Background(), q, psJobID, aeSpecs(), aeTitle, "")
			if !errors.Is(err, ErrCanceled) {
				t.Fatalf("err = %v, want ErrCanceled", err)
			}
			if len(f.ids) < tc.after {
				t.Fatalf("сценарий: до отмены создано версий %v, want ≥ %d", f.ids, tc.after)
			}
			if alive := aeAlive(f); len(alive) != 0 {
				t.Errorf("после отмены остались версии %v (созданы %v, удаления %v), want ни одной", alive, f.ids, f.deleted)
			}
		})
	}
}

// aeTrackCancel — отмену принимают, пока воркер создаёт версию: версия уже создана, затем метка отмены
// появляется в ответе beat, и фейк ждёт, пока она дойдёт (или нет) до ctx вызова. Как настоящий HTTP-клиент:
// ctx вызова отменён — ошибка (id ответа теряется), иначе — id.
type aeTrackCancel struct {
	*aeQueue
	at     int // отмена при создании версии с этим номером (1 — первая)
	n      atomic.Int32
	cancel atomic.Bool
	seen   chan struct{} // beat отдал метку отмены
	once   sync.Once
}

func (c *aeTrackCancel) VariantToTrack(ctx context.Context, jobID int64, file, title string, src int64) (int64, error) {
	id, err := c.aeQueue.VariantToTrack(ctx, jobID, file, title, src)
	if err != nil || int(c.n.Add(1)) != c.at {
		return id, err
	}
	c.cancel.Store(true)
	select {
	case <-c.seen: // метка отдана — дать ей дойти до ctx
		select {
		case <-ctx.Done():
		case <-time.After(200 * time.Millisecond):
		}
	case <-ctx.Done():
	case <-time.After(waitLimit):
	}
	if ctx.Err() != nil {
		return 0, fmt.Errorf("воркер: %w", ctx.Err())
	}
	return id, nil
}

// Условие 7: отмена принята во время создания версии (первой и исправляющего прохода) — созданная версия
// не теряется и удаляется, ApplyEdits → ErrCanceled, у трека ни одной новой версии.
func TestApplyEditsCanceledDuringVariantToTrackLeavesNoVersion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		at   int
	}{
		{"первая версия", 1},
		{"версия исправляющего прохода", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, _ := aeSetup(t, map[int64]float64{vfFirst: 3.6, vfFirst + 1: 1.6})
			c := &aeTrackCancel{aeQueue: &aeQueue{aeFake: f}, at: tc.at, seen: make(chan struct{})}
			c.cancelAt = func() bool {
				if !c.cancel.Load() {
					return false
				}
				c.once.Do(func() { close(c.seen) })
				return true
			}
			_, err := ApplyEdits(context.Background(), c, psJobID, aeSpecs(), aeTitle, "")
			if !errors.Is(err, ErrCanceled) {
				t.Fatalf("err = %v, want ErrCanceled", err)
			}
			if int(c.n.Load()) < tc.at {
				t.Fatalf("сценарий: версий создано %d, отмена ждала версию %d", c.n.Load(), tc.at)
			}
			if alive := aeAlive(f); len(alive) != 0 {
				t.Errorf("после отмены остались версии %v (созданы %v, удаления %v), want ни одной", alive, f.ids, f.deleted)
			}
		})
	}
}

// ---------- ТК9: передан файл прослушивания — первой сборки нет ----------

// aeListen — файл прослушивания у трека: заранее собранный вариант (содержимое — звук трека).
func aeListen(t *testing.T, f *aeFake) string {
	t.Helper()
	const name = "dsp-listen-mix.flac"
	f.files[key(psJobID, name)] = lavfi(t, aeval(strings.Join([]string{prlVoc, prlOther, prlDrums, prlBass}, "+"), lvDur),
		filepath.Join(t.TempDir(), "listen.flac"))
	return name
}

func TestApplyEditsMixFileSkipsFirstBuild(t *testing.T) {
	t.Parallel()
	f, _ := aeSetup(t, map[int64]float64{vfFirst: 0.5})
	file := aeListen(t, f)
	id := aeApply(t, f, aeSpecs(), file)
	if len(f.ups) != 0 || f.guitarRuns() != 0 {
		t.Errorf("сборка была (загрузок %d, движок на гитару %d), want ни одной: файл прослушивания дан", len(f.ups), f.guitarRuns())
	}
	if len(f.promoted) == 0 || f.promoted[0].jobID != psJobID || f.promoted[0].file != file {
		t.Fatalf("первая версия %+v, want из файла %s трека #%d", f.promoted, file, psJobID)
	}
	if !vfAsked(f.splitAsks, vfFirst) {
		t.Errorf("версия из файла прослушивания не сверена: разделение просили у %v", f.splitAsks)
	}
	if id != vfFirst {
		t.Errorf("id %d, want %d", id, vfFirst)
	}
	aeOneVersion(t, f, vfFirst)
	if len(f.draftDel) != 1 {
		t.Errorf("DraftDelete %v, want один", f.draftDel)
	}
}

// Файл прослушивания и промах 3 → одна исправляющая сборка (гитара на −3 к гейну без поправки), версия из файла
// удалена, итог — вторая версия.
func TestApplyEditsMixFileThenFix(t *testing.T) {
	t.Parallel()
	f, orig := aeSetup(t, map[int64]float64{vfFirst: 3, vfFirst + 1: 0})
	file := aeListen(t, f)
	id := aeApply(t, f, aeSpecs(), file)
	ps := f.passes(t, orig)
	if len(ps) != 1 {
		t.Fatalf("пересборок %d, want 1 (только исправляющая)", len(ps))
	}
	pdNear(t, "гейн гитары исправляющей сборки", ps[0].guitar, aeBaseGain()-3, 0.3)
	pdNear(t, "бас исправляющей сборки (у цели, без поправки)", ps[0].bass, aeBaseBassGain(), 0.3)
	if id != vfFirst+1 {
		t.Errorf("id %d, want %d", id, vfFirst+1)
	}
	aeOneVersion(t, f, vfFirst+1)
}

// Файл прослушивания без целей → ни сборки, ни разделения; одна версия из файла.
func TestApplyEditsMixFileNoVerify(t *testing.T) {
	t.Parallel()
	f, _ := aeSetup(t, nil)
	file := aeListen(t, f)
	id := aeApply(t, f, []SectionSpec{aeGuitar(0, false), aeBass(false)}, file)
	if len(f.ups) != 0 || len(f.splitAsks) != 0 {
		t.Errorf("загрузки %d / разделения %v, want ни одного", len(f.ups), f.splitAsks)
	}
	if len(f.promoted) != 1 || f.promoted[0].file != file {
		t.Errorf("версии %+v, want одна из %s", f.promoted, file)
	}
	aeOneVersion(t, f, id)
}

// aeBaseBassGain — бас первого прохода к исходному треку = его Db.
func aeBaseBassGain() float64 { return aeBassDb() }

// ---------- ТК10 (Go): одна операция очереди; шаги render(1) → verify(1) → fix(2) → verify(2) ----------

func TestApplyEditsOneQueueOpWithSteps(t *testing.T) {
	t.Parallel()
	f, _ := aeSetup(t, map[int64]float64{vfFirst: 3.6, vfFirst + 1: 0})
	q := &aeQueue{aeFake: f}
	if _, err := ApplyEdits(context.Background(), q, psJobID, aeSpecs(), aeTitle, ""); err != nil {
		t.Fatalf("ApplyEdits: %v", err)
	}
	if len(q.adds) != 1 {
		t.Errorf("операций очереди %d %+v, want одна на весь итог", len(q.adds), q.adds)
	}
	if len(q.finished) != 1 || q.finished[0].State != "done" {
		t.Errorf("итоги операции %+v, want один done", q.finished)
	}
	want := "render(1) → verify(1) → fix(2) → verify(2)"
	if got := strings.Join(q.steps(), " → "); got != want {
		t.Errorf("шаги итога:\n got %s\nwant %s", got, want)
	}
}

// ---------- ТК12: журнал итога — одна rebuild_sections с поправленными Db и одна запись версии ----------

// aeSpecDb — db записи с дорожкой stem в params журнала (на любой глубине).
func aeSpecDb(v any, stem string) (float64, bool) {
	switch x := v.(type) {
	case map[string]any:
		if st, ok := x["stems"].([]any); ok && len(st) > 0 && st[0] == stem {
			if db, ok := x["db"].(float64); ok {
				return db, true
			}
		}
		for _, vv := range x {
			if db, ok := aeSpecDb(vv, stem); ok {
				return db, true
			}
		}
	case []any:
		for _, vv := range x {
			if db, ok := aeSpecDb(vv, stem); ok {
				return db, true
			}
		}
	}
	return 0, false
}

func TestApplyEditsJournalOneRebuildWithCorrections(t *testing.T) {
	t.Parallel()
	f, _ := aeSetup(t, map[int64]float64{vfFirst: 3.6, vfFirst + 1: 0})
	rec := &jrRec{Service: f}
	id := aeApply(t, yue.Journaled(rec), aeSpecs(), "")
	if id != vfFirst+1 {
		t.Fatalf("сценарий: id %d, want %d (один исправляющий проход)", id, vfFirst+1)
	}
	var rebuilds, versions []jrEntry
	var other []string
	for _, e := range rec.recs {
		switch e.op.Op {
		case "rebuild_sections":
			rebuilds = append(rebuilds, e)
		case "variant_track":
			versions = append(versions, e)
		default:
			other = append(other, e.op.Op)
		}
	}
	if len(rebuilds) != 1 {
		t.Fatalf("записей rebuild_sections %d, want 1: %+v", len(rebuilds), rec.recs)
	}
	r := rebuilds[0]
	if r.jobID != psJobID {
		t.Errorf("rebuild_sections у #%d, want #%d", r.jobID, psJobID)
	}
	if last := f.ups[len(f.ups)-1].name; r.op.Result != last {
		t.Errorf("rebuild_sections result %q, want файл итоговой сборки %q", r.op.Result, last)
	}
	params := jrDecode(t, r.op.Params)
	if db, ok := aeSpecDb(params, "guitar"); !ok {
		t.Errorf("в params нет записи гитары: %s", r.op.Params)
	} else {
		pdNear(t, "db гитары в журнале (с поправкой)", db, -3.6, 0.05)
	}
	if db, ok := aeSpecDb(params, "bass"); !ok {
		t.Errorf("в params нет записи баса: %s", r.op.Params)
	} else {
		pdNear(t, "db баса в журнале (без поправки)", db, aeBassDb(), 0.05)
	}
	if len(versions) != 1 || versions[0].op.Result != yue.JobResult(id) {
		t.Errorf("записи версии %+v, want одна variant_track с result %s", versions, yue.JobResult(id))
	}
	if len(other) != 0 {
		t.Errorf("лишние записи журнала %v (пробные сборки и удаления в журнал не идут)", other)
	}
}

// Условие 12 (одна реализация сверки: черновик + итог = пресет сразу) — TestApplyEditsPresetRecordSameAsDirect
// в preset_record_test.go: запись-пресет черновика раскрывается при итоге (internal-studio-instant-apply, ТК9).

// ---------- verify_db в JSON ----------

func TestSectionSpecVerifyDbJSON(t *testing.T) {
	t.Parallel()
	s := SectionSpec{Stems: []string{"guitar"}, Db: 2, VerifyDb: lvF(-8)}
	raw := pdJSON(t, s)
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	if m["verify_db"] != -8.0 {
		t.Errorf("JSON %s, want verify_db −8", raw)
	}
	var back SectionSpec
	if err := json.Unmarshal([]byte(raw), &back); err != nil {
		t.Fatal(err)
	}
	if back.VerifyDb == nil || *back.VerifyDb != -8 {
		t.Errorf("обратно verify_db %v, want −8", back.VerifyDb)
	}
	if plain := pdJSON(t, SectionSpec{Stems: []string{"bass"}, Db: 2}); strings.Contains(plain, "verify_db") {
		t.Errorf("JSON без цели %s, want без verify_db", plain)
	}
}
