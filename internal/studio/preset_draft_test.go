package studio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"testing"

	"partscaster/internal/yue"
)

// Общие фейки и хелперы сборки с целями уровня (бывшие тесты PresetDraft). Функция PresetDraft удалена
// (internal-studio-instant-apply, условие 15): пресет в черновике — запись с рецептом, подготовка — при сборке;
// её тесты — preset_record_test.go (ТК5–ТК9).

// pdJSON — пресет как JSON: снимок до вызова, чтобы увидеть, что аргумент не изменён.
func pdJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	return string(b)
}

// pdVf — vfFake (замеры трека, версии с id vfFirst, vfFirst+1, …) + учёт созданных версий и сбои по счёту.
type pdVf struct {
	*vfFake
	ids          []int64 // созданные версии по порядку
	trackN       int
	failTrackAt  int // n-я VariantToTrack отвечает ошибкой (0 — нет)
	guitarN      int
	failGuitarAt int // n-й вызов движка на гитару отвечает ошибкой (0 — нет)
	made         atomic.Bool
	created      []string // файлы, которые воркер создал у самого трека psJobID (загрузки, варианты, мастер), "id/file"
	masterN      int
}

// addCreated — файл у трека psJobID, созданный сборкой (без повторов).
func (f *pdVf) addCreated(id int64, file string) {
	if id != psJobID {
		return
	}
	k := key(id, file)
	if !contains(f.created, k) {
		f.created = append(f.created, k)
	}
}

func (f *pdVf) UploadDsp(ctx context.Context, id int64, fname, label string, data []byte) (*yue.DspVariant, error) {
	v, err := f.vfFake.UploadDsp(ctx, id, fname, label, data)
	if err == nil && v != nil {
		f.addCreated(id, v.File)
	}
	return v, err
}

func (f *pdVf) VariantToTrack(ctx context.Context, jobID int64, file, title string, src int64) (int64, error) {
	f.trackN++
	if f.trackN == f.failTrackAt {
		return 0, errors.New("воркер: версия не создана")
	}
	id, err := f.vfFake.VariantToTrack(ctx, jobID, file, title, src)
	if err == nil {
		f.ids = append(f.ids, id)
		f.made.Store(true)
	}
	return id, err
}

func (f *pdVf) ApplyFx(ctx context.Context, id int64, req yue.FxRequest) (*yue.DspVariant, error) {
	if req.From == nil || req.To == nil {
		// мастер на весь файл (как msFake): на месте — тот же файл, иначе новый вариант с тем же звуком
		f.masterN++
		name := req.File
		if !req.InPlace {
			name = fmt.Sprintf("dsp-master-%d.flac", f.masterN)
			f.files[key(id, name)] = f.files[key(id, req.File)]
		}
		f.addCreated(id, name)
		return &yue.DspVariant{File: name, Label: "микс · мастер",
			Metrics: map[string]any{"lufs": -14.0, "true_peak_db": -1.0}}, nil
	}
	if req.Source == "guitar" {
		f.guitarN++
		if f.guitarN == f.failGuitarAt {
			return nil, errors.New("движок: нет памяти")
		}
	}
	v, err := f.vfFake.ApplyFx(ctx, id, req)
	// превью куска движка — вход пересборки, а не её итог: в «созданные сборкой» не идёт
	if err == nil && v != nil && !strings.HasPrefix(v.File, "preview-") {
		f.addCreated(id, v.File)
	}
	return v, err
}

func pdVfSetup(t *testing.T, errs map[int64]float64) *pdVf {
	t.Helper()
	f, _ := vfSetup(t, errs)
	return &pdVf{vfFake: f}
}

// pdNear — |got − want| ≤ tol.
func pdNear(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %.3f, want %.3f ±%g", what, got, want, tol)
	}
}

// ---------- ТК11: level_target / level_total — JSON туда-обратно ----------

func TestSectionSpecLevelJSONRoundTrip(t *testing.T) {
	t.Parallel()
	target := -9.5
	s := SectionSpec{Stems: []string{"guitar"}, Engine: engChain(), Db: -1.5, LevelTarget: &target, LevelTotal: -19.25}
	raw := pdJSON(t, s)
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	if m["level_target"] != -9.5 || m["level_total"] != -19.25 {
		t.Errorf("JSON %s, want level_target −9,5 и level_total −19,25", raw)
	}
	var back SectionSpec
	if err := json.Unmarshal([]byte(raw), &back); err != nil {
		t.Fatal(err)
	}
	if back.LevelTarget == nil || *back.LevelTarget != target || back.LevelTotal != -19.25 {
		t.Errorf("обратно: level_target %v level_total %v, want −9,5 / −19,25", back.LevelTarget, back.LevelTotal)
	}
	// пусто — полей нет
	plain := pdJSON(t, SectionSpec{Stems: []string{"bass"}, Db: 2})
	if strings.Contains(plain, "level_target") || strings.Contains(plain, "level_total") {
		t.Errorf("JSON без цели %s, want без level_target/level_total", plain)
	}
}

// Запись из JSON с level_target/level_total пересборка учитывает как раньше: обработанный кусок (+6 дБ к
// исходной) ставится к цели −10 по своему замеру, а не по исходной дорожке (было бы ≈ −4).
func TestSectionSpecLevelJSONRebuild(t *testing.T) {
	t.Parallel()
	f, _ := prlSetup(t)
	const target = -10.0
	eng, _ := json.Marshal(engChain())
	src := fmt.Sprintf(`{"child_id":0,"from":0,"to":0,"stems":["bass"],"db":0,"engine":%s,"level_target":%g,"level_total":%g}`,
		eng, target, prlTrackSum())
	var s SectionSpec
	if err := json.Unmarshal([]byte(src), &s); err != nil {
		t.Fatalf("запись: %v", err)
	}
	if _, err := RebuildSections(context.Background(), f, psJobID, []SectionSpec{s}); err != nil {
		t.Fatalf("RebuildSections: %v", err)
	}
	name := onlyUpload(t, f.secFake)
	g := prlInsertGain(t, decodeBytes(t, f.uploads[name]))
	got := prlSineDb(prlChunkAmp) + g - prlTrackSum()
	if math.Abs(got-target) > 0.5 {
		t.Errorf("обработанный бас к треку %.2f дБ (гейн вставки %+.2f), want цель %.1f ±0,5", got, g, target)
	}
}

func slicesHas(list []string, ok func(string) bool) bool {
	for _, s := range list {
		if ok(s) {
			return true
		}
	}
	return false
}
