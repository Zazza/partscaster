package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"partscaster/internal/yue"
)

// Барабанщик в слое «живой звук» (карточка internal-live-drummer).
//
// Условие 1 — СБИВКИ перед каждой сменой части (кроме начала трека): в припев (chorus) — 1 такт, в последний
// припев трека — 2 такта, в прочие части — полтакта; шестнадцатые (такт/16) от (начало части − длина) до начала
// части; первая треть — osdk/snare, дальше osdk/tom-small до 5/9, osdk/tom-medium до 7/9, osdk/tom-large — остальное;
// сила 0,6 → 1,0; блоки perc (voice 0, kit набора, rel_db −7, humanize_ms 6), как крэш слоя.
// Условие 2 — БАРАБАНЫ ВПЕРЁД: план с припевом → у записей пресета на kick/snare/toms/hh/ride/crash последний блок
// цепочки — level: припев +4,5, pre-chorus +1, verse −1, прочее 0; на сбивке — линейно от прежней части к новой.
// Условие 3 — level_db этих записей +4,5. Условие 5 — без плана / машина — как раньше. Условие 6 — не в журнале,
// пресет не меняется.
//
// Толкования (не заданы карточкой, проверяются так):
//   - Сигнатура liveFills карточкой не зафиксирована — сбивки проверяются через liveLayer: записи perc с kit
//     osdk/snare или osdk/tom-*; ноты собираются по наборам из всех таких записей (одна запись на набор или на
//     сбивку — неважно).
//   - Доля удара в сбивке (граница наборов 1/3, 5/9, 7/9) — карточка не уточняет, от начала удара, его середины
//     или хода 0 → 1; тест требует порядок наборов и границы с точностью до одного удара.
//   - Смена части — как у крэша: непустой Section, не как у предыдущего такта; первый такт трека — не смена.
//   - «Сила нарастает 0,6 → 1,0»: первый удар сбивки 0,6, не убывает, последний ≥ 0,95, все ≤ 1.
//   - Линия «громкость по линии» проверяется значениями в моменты трека (линейная интерполяция по её точкам,
//     до первой/после последней — их дБ), а не набором точек.
//   - Барабаны вперёд и level_db проверяются через ApplySoundPreset со слоем (воркер — фейк): цепочки — в запросах
//     ApplyFx (запись находится по метке: блок eq с уникальным highpass_hz); level_db — по громкости рендера записи
//     в пересборке: тот же пресет на плане с припевом громче, чем на плане без припева, на +4,5 дБ.

// --- сбивки через liveLayer ---

func isFillKit(kit string) bool {
	return kit == "osdk/snare" || strings.HasPrefix(kit, "osdk/tom-")
}

// fillSpecs — записи сбивок: perc с набором малого/томов.
func fillSpecs(specs []SectionSpec) []SectionSpec {
	var out []SectionSpec
	for _, s := range specs {
		if typ, kit := engineKit(s); typ == "perc" && isFillKit(kit) {
			out = append(out, s)
		}
	}
	return out
}

// withoutFills — записи слоя без сбивок (для сверки тарелок с liveCymbals).
func withoutFills(specs []SectionSpec) []SectionSpec {
	var out []SectionSpec
	for _, s := range specs {
		if typ, kit := engineKit(s); typ == "perc" && isFillKit(kit) {
			continue
		}
		out = append(out, s)
	}
	return out
}

type fillHit struct {
	T, Vel float64
	Kit    string
}

// fillHits — все удары сбивок по времени; проверяет форму записей (как крэш: [mix], Add; perc voice 0,
// rel_db −7, humanize_ms 6).
func fillHits(t *testing.T, specs []SectionSpec) []fillHit {
	t.Helper()
	var out []fillHit
	for _, s := range fillSpecs(specs) {
		if !slices.Equal(s.Stems, []string{"mix"}) || !s.Add {
			t.Errorf("сбивка: ожидались Stems [mix] и Add, получено Stems %v Add %v", s.Stems, s.Add)
		}
		b := s.Engine[0]
		_, kit := engineKit(s)
		if num(t, b["voice"]) != 0 {
			t.Errorf("сбивка %s: voice = %v, ожидалось 0", kit, b["voice"])
		}
		if v, ok := b["rel_db"]; !ok || num(t, v) != -7 {
			t.Errorf("сбивка %s: rel_db = %v, ожидалось −7", kit, b["rel_db"])
		}
		if v, ok := b["humanize_ms"]; !ok || num(t, v) != 6 {
			t.Errorf("сбивка %s: humanize_ms = %v, ожидалось 6", kit, b["humanize_ms"])
		}
		notes, ok := b["notes"].([]PercHit)
		if !ok {
			t.Fatalf("сбивка %s: notes — %T, ожидалось []PercHit", kit, b["notes"])
		}
		for _, n := range notes {
			if n.D <= 0 {
				t.Errorf("сбивка %s: удар %+v — длина должна быть > 0", kit, n)
			}
			out = append(out, fillHit{T: n.T, Vel: n.Vel, Kit: kit})
		}
	}
	slices.SortFunc(out, func(a, b fillHit) int {
		switch {
		case a.T < b.T:
			return -1
		case a.T > b.T:
			return 1
		}
		return 0
	})
	return out
}

// fillWant — ожидаемые удары одной сбивки: к началу части start длиной bars тактов (такт barLen).
func fillWant(start, bars, barLen float64) []fillHit {
	step := barLen / 16
	n := int(math.Round(bars * 16))
	from := start - bars*barLen
	var out []fillHit
	for i := range n {
		frac := float64(i) / float64(n)
		kit := "osdk/tom-large"
		switch {
		case frac < 1.0/3:
			kit = "osdk/snare"
		case frac < 5.0/9:
			kit = "osdk/tom-small"
		case frac < 7.0/9:
			kit = "osdk/tom-medium"
		}
		out = append(out, fillHit{T: from + float64(i)*step, Kit: kit})
	}
	return out
}

var fillKitOrder = []string{"osdk/snare", "osdk/tom-small", "osdk/tom-medium", "osdk/tom-large"}

// checkFillHits — моменты ударов точно; наборы — по порядку малый → томы сверху вниз, а граница смены набора
// в каждой сбивке — у доли 1/3, 5/9, 7/9 с точностью до одного удара (карточка не уточняет, считается доля по
// началу удара, его середине или по ходу 0 → 1, как сила; расхождение этих прочтений — не больше удара).
func checkFillHits(t *testing.T, got []fillHit, fills ...[]fillHit) {
	t.Helper()
	var want []fillHit
	for _, f := range fills {
		want = append(want, f...)
	}
	if len(got) != len(want) {
		t.Fatalf("ударов сбивок %d, ожидалось %d\nполучено %+v", len(got), len(want), got)
	}
	for i := range want {
		if math.Abs(got[i].T-want[i].T) > 1e-6 {
			t.Errorf("удар %d: в %.4f, ожидалось %.4f", i, got[i].T, want[i].T)
		}
	}
	off := 0
	for fi, f := range fills {
		seg := got[off : off+len(f)]
		off += len(f)
		n := len(f)
		prev := 0
		for j, h := range seg {
			k := slices.Index(fillKitOrder, h.Kit)
			if k < prev {
				t.Errorf("сбивка %d: удар %d набор %s после %s — томы должны идти сверху вниз", fi, j, h.Kit, fillKitOrder[prev])
			}
			prev = max(prev, k)
		}
		for bi, b := range []float64{1.0 / 3, 5.0 / 9, 7.0 / 9} {
			first := -1 // первый удар после границы (набор старше bi)
			for j, h := range seg {
				if slices.Index(fillKitOrder, h.Kit) > bi {
					first = j
					break
				}
			}
			wantFirst := int(math.Ceil(float64(n) * b))
			if first < 0 || first < wantFirst-1 || first > wantFirst+1 {
				t.Errorf("сбивка %d (%d ударов): смена после %s на ударе %d, ожидалось у доли %.3f (удар %d ±1)",
					fi, n, fillKitOrder[bi], first, b, wantFirst)
			}
		}
	}
}

// checkFillVel — сила внутри каждой сбивки [from, to): 0,6 → 1,0, не убывает.
func checkFillVel(t *testing.T, got []fillHit, windows ...[2]float64) {
	t.Helper()
	for _, w := range windows {
		var vel []float64
		for _, h := range got {
			if h.T >= w[0]-1e-9 && h.T < w[1]-1e-9 {
				vel = append(vel, h.Vel)
			}
		}
		if len(vel) == 0 {
			t.Errorf("сбивка %v: ударов нет", w)
			continue
		}
		if math.Abs(vel[0]-0.6) > 0.01 {
			t.Errorf("сбивка %v: первый удар сила %.3f, ожидалось 0,6", w, vel[0])
		}
		if last := vel[len(vel)-1]; last < 0.95 || last > 1+1e-9 {
			t.Errorf("сбивка %v: последний удар сила %.3f, ожидалось ≈ 1,0", w, last)
		}
		for i := 1; i < len(vel); i++ {
			if vel[i] < vel[i-1]-1e-9 || vel[i] > 1+1e-9 {
				t.Errorf("сбивка %v: сила %v — должна нарастать от 0,6 до 1,0", w, vel)
				break
			}
		}
	}
}

// fillBars — такты по 2 с: вступление [0,4), куплет [4,8), припев [8,12), куплет [12,16), припев [16,20) (последний),
// концовка [20,22). Сбивки: [3,4) ½ такта в куплет, [6,8) 1 такт в припев, [11,12) ½ в куплет,
// [12,16) 2 такта в последний припев, [19,20) ½ в концовку.
func fillBars() []yue.ChordBar {
	var out []yue.ChordBar
	for _, p := range []struct {
		from, to float64
		sec      string
	}{{0, 4, "intro"}, {4, 8, "verse"}, {8, 12, "chorus"}, {12, 16, "verse"}, {16, 20, "chorus"}, {20, 22, "outro"}} {
		for s := p.from; s < p.to; s += 2 {
			out = append(out, bar(s, s+2, p.sec))
		}
	}
	return out
}

func liveFillsVia(t *testing.T, p yue.SoundPreset, bars []yue.ChordBar, dur float64) []SectionSpec {
	t.Helper()
	f := &liveFake{grid: &yue.ChordGrid{BPM: 120, Bars: bars}}
	specs, err := liveLayer(context.Background(), f, &yue.Job{ID: 11, DurationSec: dur}, p)
	if err != nil {
		t.Fatalf("liveLayer: %v", err)
	}
	return specs
}

func TestLiveFills_LengthsKitsTimes(t *testing.T) {
	got := fillHits(t, liveFillsVia(t, liveKitPreset(), fillBars(), 22))
	checkFillHits(t, got,
		fillWant(4, 0.5, 2),  // в куплет — полтакта
		fillWant(8, 1, 2),    // в припев — такт
		fillWant(12, 0.5, 2), // в куплет — полтакта
		fillWant(16, 2, 2),   // в последний припев — 2 такта
		fillWant(20, 0.5, 2), // в концовку — полтакта
	)
}

func TestLiveFills_Velocity(t *testing.T) {
	got := fillHits(t, liveFillsVia(t, liveKitPreset(), fillBars(), 22))
	checkFillVel(t, got, [2]float64{3, 4}, [2]float64{6, 8}, [2]float64{11, 12}, [2]float64{12, 16}, [2]float64{19, 20})
}

// Начало трека — без сбивки: ни одного удара раньше первой сбивки (3 с).
func TestLiveFills_NoFillAtTrackStart(t *testing.T) {
	for _, h := range fillHits(t, liveFillsVia(t, liveKitPreset(), fillBars(), 22)) {
		if h.T < 3-1e-9 {
			t.Errorf("удар сбивки в %.3f — до первой смены части сбивок нет", h.T)
		}
	}
}

// Пред-припев (pre-chorus) — прочая часть: полтакта, не такт как в припев. Единственный припев — он же последний: 2 такта.
func TestLiveFills_PreChorusHalfBar_SingleChorusTwoBars(t *testing.T) {
	var bars []yue.ChordBar
	for s := 0.0; s < 14; s += 2 {
		sec := "intro"
		switch {
		case s >= 10:
			sec = "chorus"
		case s >= 4:
			sec = "pre-chorus"
		}
		bars = append(bars, bar(s, s+2, sec))
	}
	got := fillHits(t, liveFillsVia(t, liveKitPreset(), bars, 14))
	checkFillHits(t, got, fillWant(4, 0.5, 2), fillWant(10, 2, 2))
}

// Шестнадцатые — от длины такта: такт 1 с → шаг 1/16 с.
func TestLiveFills_SixteenthsFollowBarLength(t *testing.T) {
	bars := []yue.ChordBar{bar(0, 1, "intro"), bar(1, 2, "intro"), bar(2, 3, "verse"), bar(3, 4, "verse")}
	got := fillHits(t, liveFillsVia(t, liveKitPreset(), bars, 4))
	checkFillHits(t, got, fillWant(2, 0.5, 1))
}

func TestLiveFills_NoSectionChange_NoFills(t *testing.T) {
	bars := []yue.ChordBar{bar(0, 2, "verse"), bar(2, 4, "verse"), bar(4, 6, "verse")}
	if f := fillSpecs(liveFillsVia(t, liveKitPreset(), bars, 6)); len(f) != 0 {
		t.Errorf("смен части нет — сбивок нет, получено %+v", f)
	}
	only := []yue.ChordBar{bar(0, 2, "chorus"), bar(2, 4, "chorus")}
	if f := fillSpecs(liveFillsVia(t, liveKitPreset(), only, 4)); len(f) != 0 {
		t.Errorf("весь трек — припев, смен нет — сбивок нет, получено %+v", f)
	}
}

// Живая бочка без живого хэта — тоже живой набор: сбивки есть.
func TestLiveLayer_LiveKick_HasFills(t *testing.T) {
	got := fillHits(t, liveFillsVia(t, liveKickPreset(), fillBars(), 22))
	if len(got) == 0 {
		t.Fatal("живая бочка — сбивок нет, ожидались")
	}
	kits := map[string]bool{}
	for _, h := range got {
		kits[h.Kit] = true
	}
	for _, k := range []string{"osdk/snare", "osdk/tom-small", "osdk/tom-medium", "osdk/tom-large"} {
		if !kits[k] {
			t.Errorf("среди сбивок нет набора %s (есть %v)", k, kits)
		}
	}
}

func TestLiveLayer_Machine_NoFills(t *testing.T) {
	if f := fillSpecs(liveFillsVia(t, machinePreset(), fillBars(), 22)); len(f) != 0 {
		t.Errorf("драм-машина — сбивок нет, получено %+v", f)
	}
}

func TestLiveLayer_NoPlan_NoFills(t *testing.T) {
	f := &liveFake{err: &yue.StatusError{Code: 404, Msg: "нет плана"}}
	specs, err := liveLayer(context.Background(), f, &yue.Job{ID: 7, DurationSec: 22}, liveKitPreset())
	if err != nil {
		t.Fatalf("без плана — не ошибка, получено %v", err)
	}
	if fs := fillSpecs(specs); len(fs) != 0 {
		t.Errorf("без плана — сбивок нет, получено %+v", fs)
	}
}

// --- барабаны вперёд и level_db через ApplySoundPreset ---

// «Обработанная» запись-метка звучит тоном 2500 Гц (в треке его больше нигде нет) амплитудой 0,01 (≈ −43 дБ):
// цель записи с обработкой ставится по обработанному куску, и тихий тон держит поправку внутри предела ±12 дБ
// и при level_db −28, и при −23,5.
const ldToneHz = 2500.0

// ldFake — lvFake (замеры дорожек) + план трека; запись с меткой toneMark (eq highpass_hz) рендерится тоном 2500 Гц.
type ldFake struct {
	*lvFake
	grid     *yue.ChordGrid
	gridErr  error
	toneMark float64
	chains   [][]map[string]any
	first    map[string][]byte // первая загрузка варианта по имени: первый проход пресета, до исправляющих проходов сверки
}

func (f *ldFake) UploadDsp(ctx context.Context, id int64, fname, label string, data []byte) (*yue.DspVariant, error) {
	if f.first == nil {
		f.first = map[string][]byte{}
	}
	if _, ok := f.first[fname]; !ok {
		f.first[fname] = slices.Clone(data)
	}
	return f.lvFake.UploadDsp(ctx, id, fname, label, data)
}

func (f *ldFake) ChordGrid(_ context.Context, _ int64) (*yue.ChordGrid, error) {
	if f.gridErr != nil {
		return nil, f.gridErr
	}
	return f.grid, nil
}

func ldMark(chain []map[string]any) float64 {
	for _, b := range chain {
		if b["type"] == "eq" {
			if v, ok := b["highpass_hz"].(float64); ok {
				return v
			}
		}
	}
	return 0
}

func (f *ldFake) ApplyFx(ctx context.Context, id int64, req yue.FxRequest) (*yue.DspVariant, error) {
	f.chains = append(f.chains, req.Chain)
	v, err := f.lvFake.ApplyFx(ctx, id, req)
	if err != nil || f.toneMark == 0 || ldMark(req.Chain) != f.toneMark || req.From == nil || req.To == nil {
		return v, err
	}
	pf := f.psFake
	dur := *req.To + req.Fade - *req.From + engTail
	chunk := decodeFile(pf.t, lavfi(pf.t, aeval(fmt.Sprintf("0.01*sin(2*PI*%g*t)", ldToneHz), dur),
		filepath.Join(pf.dir, "tone-"+v.File)))
	return workerFile(pf.t, pf.files, pf.dir, id, v.File, chunk, *req.From, req.Pad, sr), nil
}

// ldSetup — трек lvSetup (голос/прочее/бас/хэт) + тихие дорожки остальных частей барабанов.
func ldSetup(t *testing.T, grid *yue.ChordGrid) (*ldFake, []float32) {
	t.Helper()
	lf, orig := lvSetup(t, lvMain())
	dir := t.TempDir()
	for _, n := range []string{"kick", "snare", "toms", "ride", "crash"} {
		lf.files[key(psJobID, "stem-"+n+".flac")] = lavfi(t, aeval("0", lvDur), filepath.Join(dir, "ld-"+n+".flac"))
	}
	return &ldFake{lvFake: lf, grid: grid}, orig
}

// ldRecord — запись части барабанов: живой (osdk) или машинный набор + метка eq.
func ldRecord(stem, kit string, mark float64) yue.PresetSpec {
	return yue.PresetSpec{Stems: []string{stem}, Engine: []map[string]any{
		{"type": "sampler", "kit": kit},
		{"type": "eq", "highpass_hz": mark},
	}}
}

// ldDrumRecords — записи на все шесть частей барабанов, метки 31…36.
func ldDrumRecords(prefix string) []yue.PresetSpec {
	kits := map[string]string{"kick": "kick", "snare": "snare", "toms": "tom-medium", "hh": "hh", "ride": "ride", "crash": "crash"}
	var out []yue.PresetSpec
	for i, part := range drumPartsList {
		out = append(out, ldRecord(part, prefix+"/"+kits[part], float64(31+i)))
	}
	return out
}

var drumPartsList = []string{"kick", "snare", "toms", "hh", "ride", "crash"}

func ldBassRecord() yue.PresetSpec {
	return yue.PresetSpec{Stems: []string{"bass"}, Engine: []map[string]any{
		{"type": "comp"},
		{"type": "eq", "highpass_hz": 40.0},
	}}
}

func ldPreset(id int64, specs ...yue.PresetSpec) yue.SoundPreset {
	return yue.SoundPreset{ID: id, Name: "Живой барабанщик", Specs: specs}
}

// ldChains — цепочки запросов ApplyFx с меткой mark (хотя бы один запрос).
func ldChains(t *testing.T, f *ldFake, mark float64) [][]map[string]any {
	t.Helper()
	var out [][]map[string]any
	for _, c := range f.chains {
		if ldMark(c) == mark {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		t.Fatalf("запросов ApplyFx с меткой %v нет", mark)
	}
	return out
}

type ldPoint struct {
	T  float64 `json:"t"`
	Db float64 `json:"db"`
}

// ldLine — точки блока level (через JSON: тип поля points в Go не фиксируется).
func ldLine(t *testing.T, b map[string]any) []ldPoint {
	t.Helper()
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("блок level: %v", err)
	}
	var v struct {
		Points []ldPoint `json:"points"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("блок level %s: %v", raw, err)
	}
	return v.Points
}

// lineAt — громкость линии в момент t: линейно между точками, до первой/после последней — их дБ.
func lineAt(pts []ldPoint, t float64) float64 {
	if t <= pts[0].T {
		return pts[0].Db
	}
	for i := 1; i < len(pts); i++ {
		if t <= pts[i].T {
			a, b := pts[i-1], pts[i]
			if b.T == a.T {
				return b.Db
			}
			return a.Db + (b.Db-a.Db)*(t-a.T)/(b.T-a.T)
		}
	}
	return pts[len(pts)-1].Db
}

// checkLinePoints — точки блока по контракту движка: 1…200, t ≥ 0 по возрастанию, db −30…+12.
func checkLinePoints(t *testing.T, pts []ldPoint) {
	t.Helper()
	if len(pts) == 0 || len(pts) > 200 {
		t.Fatalf("точек линии %d, ожидалось 1…200", len(pts))
	}
	for i, p := range pts {
		if p.T < 0 || p.Db < -30 || p.Db > 12 {
			t.Errorf("точка %d %+v вне пределов (t ≥ 0, db −30…+12)", i, p)
		}
		if i > 0 && p.T < pts[i-1].T {
			t.Errorf("точки не по возрастанию t: %+v после %+v", p, pts[i-1])
		}
	}
}

// dynBars — такты по 0,5 с, 10 с: вступление [0,1), куплет [1,3), пред-припев [3,4), припев [4,6), куплет [6,8),
// припев [8,9,5) (последний), концовка [9,5,10). Сбивки: [0,75,1) ½, [2,75,3) ½, [3,5,4) 1 такт, [5,75,6) ½,
// [7,8) 2 такта, [9,25,9,5) ½.
func dynBars(chorus string) []yue.ChordBar {
	var out []yue.ChordBar
	for _, p := range []struct {
		from, to float64
		sec      string
	}{{0, 1, "intro"}, {1, 3, "verse"}, {3, 4, "pre-chorus"}, {4, 6, chorus}, {6, 8, "verse"}, {8, 9.5, chorus},
		{9.5, 10, "outro"}} {
		for s := p.from; s < p.to-1e-9; s += 0.5 {
			out = append(out, bar(s, s+0.5, p.sec))
		}
	}
	return out
}

// dynWant — ожидаемая линия по dynBars("chorus"): момент трека → дБ.
var dynWant = [][2]float64{
	{0, 0}, {0.5, 0}, {0.75, 0}, {0.875, -0.5}, {1, -1}, // вступление 0 → куплет −1 на сбивке [0,75; 1)
	{2, -1}, {2.75, -1}, {2.875, 0}, {3, 1}, // куплет −1 → пред-припев +1 на [2,75; 3)
	{3.25, 1}, {3.5, 1}, {3.75, 2.75}, {4, 4.5}, // +1 → припев +4,5 на [3,5; 4)
	{5, 4.5}, {5.75, 4.5}, {5.875, 1.75}, {6, -1}, // припев → куплет −1 на [5,75; 6)
	{6.5, -1}, {7, -1}, {7.5, 1.75}, {8, 4.5}, // куплет → последний припев на [7; 8)
	{9, 4.5}, {9.25, 4.5}, {9.375, 2.25}, {9.5, 0}, {9.9, 0}, // припев → концовка 0 на [9,25; 9,5)
}

func ldApply(t *testing.T, svc yue.Service, p yue.SoundPreset) {
	t.Helper()
	if _, err := ApplySoundPreset(context.Background(), svc, psJobID, p); err != nil {
		t.Fatalf("ApplySoundPreset: %v", err)
	}
}

// План с припевом: у всех записей частей барабанов последний блок — level с линией по частям, остальное — как в пресете;
// запись баса — без изменений.
func TestDrumDynamics_LevelLineOnAllDrumParts(t *testing.T) {
	t.Parallel()
	f, _ := ldSetup(t, &yue.ChordGrid{BPM: 120, Bars: dynBars("chorus")})
	drums := ldDrumRecords("osdk")
	p := ldPreset(51, append(slices.Clone(drums), ldBassRecord())...)
	ldApply(t, f, p)

	for i, part := range drumPartsList {
		mark := float64(31 + i)
		for _, chain := range ldChains(t, f, mark) {
			if len(chain) != len(drums[i].Engine)+1 {
				t.Errorf("%s: цепочка %v, ожидались блоки записи + level в конце", part, chain)
				continue
			}
			if !reflect.DeepEqual(chain[:len(chain)-1], drums[i].Engine) {
				t.Errorf("%s: блоки до level %v, ожидались как в записи %v", part, chain[:len(chain)-1], drums[i].Engine)
			}
			last := chain[len(chain)-1]
			if last["type"] != "level" {
				t.Errorf("%s: последний блок %v, ожидался level", part, last["type"])
				continue
			}
			pts := ldLine(t, last)
			checkLinePoints(t, pts)
			if len(pts) == 0 {
				continue
			}
			for _, w := range dynWant {
				if got := lineAt(pts, w[0]); math.Abs(got-w[1]) > 0.02 {
					t.Errorf("%s: линия в %.3f с = %+.3f дБ, ожидалось %+.2f (точки %+v)", part, w[0], got, w[1], pts)
				}
			}
		}
	}
	for _, chain := range ldChains(t, f, 40) {
		if !reflect.DeepEqual(chain, ldBassRecord().Engine) {
			t.Errorf("бас: цепочка %v, ожидалась как в записи %v", chain, ldBassRecord().Engine)
		}
	}
}

// Без припева в плане — линии нет: цепочки частей барабанов как в пресете.
func TestDrumDynamics_NoChorus_NoLevel(t *testing.T) {
	t.Parallel()
	f, _ := ldSetup(t, &yue.ChordGrid{BPM: 120, Bars: dynBars("bridge")})
	drums := ldDrumRecords("osdk")
	ldApply(t, f, ldPreset(52, drums...))
	for i, part := range drumPartsList {
		for _, chain := range ldChains(t, f, float64(31+i)) {
			if !reflect.DeepEqual(chain, drums[i].Engine) {
				t.Errorf("%s без припева: цепочка %v, ожидалась как в записи %v", part, chain, drums[i].Engine)
			}
		}
	}
}

// Драм-машина — линии нет, даже с припевом.
func TestDrumDynamics_Machine_NoLevel(t *testing.T) {
	t.Parallel()
	f, _ := ldSetup(t, &yue.ChordGrid{BPM: 120, Bars: dynBars("chorus")})
	drums := ldDrumRecords("tr808")
	ldApply(t, f, ldPreset(53, drums...))
	for i, part := range drumPartsList {
		for _, chain := range ldChains(t, f, float64(31+i)) {
			if !reflect.DeepEqual(chain, drums[i].Engine) {
				t.Errorf("%s машина: цепочка %v, ожидалась как в записи %v", part, chain, drums[i].Engine)
			}
		}
	}
}

// Без плана трека — линии нет, применение не падает.
func TestDrumDynamics_NoPlan_NoLevel(t *testing.T) {
	t.Parallel()
	f, _ := ldSetup(t, nil)
	f.gridErr = &yue.StatusError{Code: 404, Msg: "нет плана"}
	drums := ldDrumRecords("osdk")
	ldApply(t, f, ldPreset(54, drums...))
	for i, part := range drumPartsList {
		for _, chain := range ldChains(t, f, float64(31+i)) {
			if !reflect.DeepEqual(chain, drums[i].Engine) {
				t.Errorf("%s без плана: цепочка %v, ожидалась как в записи %v", part, chain, drums[i].Engine)
			}
		}
	}
}

// ldToneDb — громкость тона hz в пересборке ПЕРВОГО прохода (исправляющие проходы сверки по итогу — отдельный
// механизм, его поправки сравнение бы путали), дБ; середина трека 2–8 с.
func ldToneDb(t *testing.T, f *ldFake, id int64, hz float64) float64 {
	t.Helper()
	name := fmt.Sprintf("dsp-preset-%d-mix.flac", id)
	data, ok := f.first[name]
	if !ok {
		t.Fatalf("пересборки нет: загружено %v, ожидалось %s", keys(map[string][]byte(f.uploads)), name)
	}
	a := toneAmp(decodeBytes(t, data), hz, 2, 8)
	if a <= 0 {
		t.Fatalf("тона %.0f Гц в пересборке нет", hz)
	}
	return 20 * math.Log10(a)
}

// level_db записи части барабанов с линией +4,5: запись на плане с припевом звучит в пересборке так же, как та же
// запись с level_db на 4,5 выше на плане без припева (линии нет), — и не так, как с прежним level_db. Сравнение
// «как с другой целью», а не по абсолютной громкости: подстройка цели у записи с обработкой идёт по замеру куска,
// и сверка проходов в фейке не воспроизводится точно, а одинаковая цель даёт одинаковый звук.
// Запись баса с level_db — одинаково в обоих (не часть барабанов).
// Метка — бочка (живой набор без живого хэта: ride слоя на хэт не ложится и сравнение не путает).
func TestDrumDynamics_LevelDbPlus4_5(t *testing.T) {
	t.Parallel()
	run := func(chorus string, kickLevel float64, id int64) (kickDb, bassDb float64) {
		kick := ldRecord("kick", "osdk/kick", 31)
		kick.LevelDb = lvF(kickLevel)
		bass := yue.PresetSpec{Stems: []string{"bass"}, LevelDb: lvF(-10)}
		f, _ := ldSetup(t, &yue.ChordGrid{BPM: 120, Bars: dynBars(chorus)})
		f.p95["kick"] = -40
		f.toneMark = 31
		ldApply(t, f, ldPreset(id, kick, bass))
		return ldToneDb(t, f, id, ldToneHz), ldToneDb(t, f, id, 200)
	}
	withLine, bassA := run("chorus", -28, 55)
	same, bassB := run("bridge", -28, 56)
	raised, _ := run("bridge", -28+4.5, 57)
	if math.Abs(withLine-same) < 1 {
		t.Fatalf("фейк не различает цели −28 и −23,5 (%.2f / %.2f дБ) — проверка без смысла", withLine, raised)
	}
	if math.Abs(withLine-raised) > 0.05 {
		t.Errorf("бочка с линией: %.2f дБ; как с level_db −23,5 без линии: %.2f, как с −28: %.2f — ожидалось как −23,5 (level_db +4,5)",
			withLine, raised, same)
	}
	if d := bassA - bassB; math.Abs(d) > 0.2 {
		t.Errorf("бас с припевом и без отличается на %+.2f дБ, ожидалось 0 (не часть барабанов)", d)
	}
}

// Слой не в журнале, пресет не меняется: в записи sound_preset — пресет как прислан (без level, level_db как был),
// переданный пресет после применения тот же.
func TestDrumDynamics_JournalAndPresetUnchanged(t *testing.T) {
	t.Parallel()
	f, _ := ldSetup(t, &yue.ChordGrid{BPM: 120, Bars: dynBars("chorus")})
	hh := ldRecord("hh", "osdk/hh", 34)
	hh.LevelDb = lvF(-28)
	p := ldPreset(57, hh, ldRecord("kick", "osdk/kick", 31))
	before, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	rec := &jrRec{Service: f}
	ldApply(t, yue.Journaled(rec), p)

	after, _ := json.Marshal(p)
	if string(after) != string(before) {
		t.Errorf("пресет изменён применением:\nбыл   %s\nстал  %s", before, after)
	}
	op := jrOne(t, rec, "sound_preset", psJobID)
	if strings.Contains(string(op.Params), `"level"`) || strings.Contains(string(op.Params), `"points"`) {
		t.Errorf("в журнале пресет с линией громкости: %s", op.Params)
	}
	if !jrHasNum(jrDecode(t, op.Params), "level_db", -28) {
		t.Errorf("в журнале level_db hh не −28 (как в пресете): %s", op.Params)
	}
	// линия всё же была применена (иначе тест ничего не доказывает)
	chains := ldChains(t, f, 34)
	if last := chains[0][len(chains[0])-1]; last["type"] != "level" {
		t.Errorf("hh: линии в рендере нет (%v) — проверка журнала без смысла", chains[0])
	}
}
