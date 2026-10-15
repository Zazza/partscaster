package studio

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"partscaster/internal/yue"
)

// Живой слой пресета: «воздух» (верх выше 16 кГц) на голос/прочее или на весь микс
// и живые тарелки (крэш на смене части и на каждом такте припева, ride в припеве)
// у пресетов с живым набором osdk.
//
// Сигнатуры Go:
//
//	presetLiveKit(p yue.SoundPreset) (live, hh bool)
//	liveCymbals(bars []yue.ChordBar, dur float64, ride bool) []SectionSpec
//	liveLayer(ctx, svc yue.Service, job *yue.Job, p yue.SoundPreset) ([]SectionSpec, error)
//	withoutLiveLayer(ctx) context.Context
//
// Внешняя граница — воркер (yue.Service.ChordGrid), подменяется фейком.

// liveFake — фейковый воркер: только ChordGrid со счётчиком вызовов.
// Остальные методы yue.Service не реализованы: их вызов — паника (нарушение контракта).
type liveFake struct {
	yue.Service
	grid  *yue.ChordGrid
	err   error
	calls int
	ids   []int64
}

func (f *liveFake) ChordGrid(_ context.Context, id int64) (*yue.ChordGrid, error) {
	f.calls++
	f.ids = append(f.ids, id)
	if f.err != nil {
		return nil, f.err
	}
	return f.grid, nil
}

// num — число из JSON-блока движка (int или float64)
func num(t *testing.T, v any) float64 {
	t.Helper()
	switch x := v.(type) {
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case float64:
		return x
	case float32:
		return float64(x)
	}
	t.Fatalf("ожидалось число, получено %T (%v)", v, v)
	return 0
}

func bar(start, end float64, section string) yue.ChordBar {
	return yue.ChordBar{Start: start, End: end, Chord: "Am", Section: section}
}

// testBars — такты по 2 с: вступление, куплет×2, припев×2, куплет, такт без части, припев×2.
// Смены части (крэш громче): 2 (verse), 6 (chorus), 10 (verse), 14 (chorus после пустого такта);
// такты припева без смены (крэш тише): 8, 16; пустой такт 12 — без удара.
// Припевы (ride): [6, 10] и [14, 18].
func testBars() []yue.ChordBar {
	return []yue.ChordBar{
		bar(0, 2, "intro"),
		bar(2, 4, "verse"),
		bar(4, 6, "verse"),
		bar(6, 8, "chorus"),
		bar(8, 10, "chorus"),
		bar(10, 12, "verse"),
		bar(12, 14, ""),
		bar(14, 16, "chorus"),
		bar(16, 18, "chorus"),
	}
}

func engineKit(s SectionSpec) (typ, kit string) {
	if len(s.Engine) == 0 {
		return "", ""
	}
	typ, _ = s.Engine[0]["type"].(string)
	kit, _ = s.Engine[0]["kit"].(string)
	return typ, kit
}

func crashSpecs(specs []SectionSpec) []SectionSpec {
	var out []SectionSpec
	for _, s := range specs {
		if typ, kit := engineKit(s); typ == "perc" && kit == "osdk/crash" {
			out = append(out, s)
		}
	}
	return out
}

func rideSpecs(specs []SectionSpec) []SectionSpec {
	var out []SectionSpec
	for _, s := range specs {
		if typ, kit := engineKit(s); typ == "sampler" && kit == "osdk/ride" {
			out = append(out, s)
		}
	}
	return out
}

// checkTopEQ — второй блок цепочки тарелки: {"type":"eq","lowpass_hz":14000}
func checkTopEQ(t *testing.T, what string, eng []map[string]any) {
	t.Helper()
	if len(eng) != 2 {
		t.Errorf("%s: ожидалась цепочка из двух блоков, получено %+v", what, eng)
		return
	}
	if eng[1]["type"] != "eq" {
		t.Errorf("%s: второй блок %v, ожидался eq", what, eng[1]["type"])
		return
	}
	if v, ok := eng[1]["lowpass_hz"]; !ok || num(t, v) != 14000 {
		t.Errorf("%s: lowpass_hz = %v, ожидалось 14000", what, eng[1]["lowpass_hz"])
	}
}

func liveKitPreset() yue.SoundPreset {
	return yue.SoundPreset{
		Name: "живой",
		Specs: []yue.PresetSpec{
			{Stems: []string{"drums"}, Engine: []map[string]any{{"type": "comp"}}},
			{Stems: []string{"hh"}, Engine: []map[string]any{{"type": "sampler", "kit": "osdk/hh"}}},
		},
	}
}

// liveKickPreset — живой набор только на бочке и малом, без hh
func liveKickPreset() yue.SoundPreset {
	return yue.SoundPreset{
		Name: "живая бочка",
		Specs: []yue.PresetSpec{
			{Stems: []string{"kick"}, Engine: []map[string]any{{"type": "sampler", "kit": "osdk/kick"}}},
			{Stems: []string{"snare"}, Engine: []map[string]any{{"type": "sampler", "kit": "osdk/snare"}}},
		},
	}
}

func machinePreset() yue.SoundPreset {
	return yue.SoundPreset{
		Name: "машина",
		Specs: []yue.PresetSpec{
			{Stems: []string{"hh"}, Engine: []map[string]any{{"type": "sampler", "kit": "tr808/hh"}}},
		},
	}
}

// --- presetLiveKit ---

func checkLiveKit(t *testing.T, p yue.SoundPreset, wantLive, wantHH bool) {
	t.Helper()
	live, hh := presetLiveKit(p)
	if live != wantLive || hh != wantHH {
		t.Errorf("presetLiveKit = (live %v, hh %v), ожидалось (%v, %v)", live, hh, wantLive, wantHH)
	}
}

func TestPresetLiveKit_OsdkOnHH(t *testing.T) {
	checkLiveKit(t, liveKitPreset(), true, true)
}

func TestPresetLiveKit_SamplerNotFirstBlock(t *testing.T) {
	p := yue.SoundPreset{Specs: []yue.PresetSpec{{
		Stems: []string{"drums", "hh"},
		Engine: []map[string]any{
			{"type": "gate"},
			{"type": "sampler", "kit": "osdk/hh_open"},
		},
	}}}
	checkLiveKit(t, p, true, true)
}

func TestPresetLiveKit_OsdkOnKickSnareOnly(t *testing.T) {
	checkLiveKit(t, liveKickPreset(), true, false)
}

// машинный набор на hh не делает hh «живым», даже если бочка живая
func TestPresetLiveKit_MachineHHLiveKick(t *testing.T) {
	p := yue.SoundPreset{Specs: []yue.PresetSpec{
		{Stems: []string{"hh"}, Engine: []map[string]any{{"type": "sampler", "kit": "tr909/hh"}}},
		{Stems: []string{"kick"}, Engine: []map[string]any{{"type": "sampler", "kit": "osdk/kick"}}},
	}}
	checkLiveKit(t, p, true, false)
}

func TestPresetLiveKit_MachineKits(t *testing.T) {
	for _, kit := range []string{"tr808/hh", "tr909/hh", "linn/hh", "cr78/hh", "simmons/hh"} {
		p := yue.SoundPreset{Specs: []yue.PresetSpec{{
			Stems:  []string{"hh"},
			Engine: []map[string]any{{"type": "sampler", "kit": kit}},
		}}}
		if live, _ := presetLiveKit(p); live {
			t.Errorf("машинный набор %q — ожидалось live=false", kit)
		}
	}
}

func TestPresetLiveKit_NoSpecs(t *testing.T) {
	checkLiveKit(t, yue.SoundPreset{Final: []yue.PresetStep{{Chain: "x"}}}, false, false)
}

// --- liveCymbals ---

// crashHits — запись крэша (ровно одна) и её удары; проверяет форму записи.
func crashHits(t *testing.T, specs []SectionSpec) []PercHit {
	t.Helper()
	crash := crashSpecs(specs)
	if len(crash) != 1 {
		t.Fatalf("ожидалась одна запись крэша, получено %d (%+v)", len(crash), specs)
	}
	c := crash[0]
	if !slices.Equal(c.Stems, []string{"mix"}) || !c.Add {
		t.Errorf("крэш: ожидались Stems [mix] и Add, получено Stems %v Add %v", c.Stems, c.Add)
	}
	checkTopEQ(t, "крэш", c.Engine)
	b := c.Engine[0]
	if num(t, b["voice"]) != 0 {
		t.Errorf("крэш: voice = %v, ожидалось 0", b["voice"])
	}
	if _, ok := b["rel_db"]; !ok {
		t.Error("крэш: нет rel_db")
	} else {
		num(t, b["rel_db"])
	}
	notes, ok := b["notes"].([]PercHit)
	if !ok {
		t.Fatalf("крэш: notes — %T, ожидалось []PercHit", b["notes"])
	}
	for _, n := range notes {
		if n.D <= 0 || n.Vel <= 0 {
			t.Errorf("удар %+v: длина и сила должны быть > 0", n)
		}
	}
	return notes
}

func hitTimes(notes []PercHit) []float64 {
	var out []float64
	for _, n := range notes {
		out = append(out, n.T)
	}
	slices.Sort(out)
	return out
}

func sortedRides(t *testing.T, specs []SectionSpec) []SectionSpec {
	t.Helper()
	ride := rideSpecs(specs)
	slices.SortFunc(ride, func(a, b SectionSpec) int {
		if a.From < b.From {
			return -1
		}
		if a.From > b.From {
			return 1
		}
		return 0
	})
	for i, r := range ride {
		if !slices.Equal(r.Stems, []string{"hh"}) || !r.Add {
			t.Errorf("ride %d: ожидались Stems [hh] и Add, получено %v Add %v", i, r.Stems, r.Add)
		}
		checkTopEQ(t, "ride", r.Engine)
	}
	return ride
}

func checkRideWindows(t *testing.T, ride []SectionSpec, wins [][2]float64) {
	t.Helper()
	if len(ride) != len(wins) {
		t.Fatalf("записей ride %d, ожидалось %d (%+v)", len(ride), len(wins), ride)
	}
	for i, r := range ride {
		if r.From != wins[i][0] || r.To != wins[i][1] {
			t.Errorf("ride %d: окно [%v, %v], ожидалось %v", i, r.From, r.To, wins[i])
		}
	}
}

// Крэш: смена части (непустой Section, не как у предыдущего такта; первый такт трека — нет)
// и каждый такт припева; первый такт припева — один удар, не два. dur 0 — без ограничения.
func TestLiveCymbals_CrashOnSectionChange(t *testing.T) {
	got := hitTimes(crashHits(t, liveCymbals(testBars(), 0, true)))
	if want := []float64{2, 6, 8, 10, 14, 16}; !slices.Equal(got, want) {
		t.Errorf("удары крэша в %v, ожидалось %v", got, want)
	}
}

func TestLiveCymbals_NoDoubleHitOnChorusStart(t *testing.T) {
	notes := crashHits(t, liveCymbals(testBars(), 0, true))
	count := map[float64]int{}
	for _, n := range notes {
		count[n.T]++
	}
	for _, at := range []float64{6, 14} {
		if count[at] != 1 {
			t.Errorf("первый такт припева в %v: ударов %d, ожидался один", at, count[at])
		}
	}
}

func TestLiveCymbals_ChangeLouderThanChorusBar(t *testing.T) {
	notes := crashHits(t, liveCymbals(testBars(), 0, true))
	vel := map[float64]float64{}
	for _, n := range notes {
		vel[n.T] = n.Vel
	}
	for _, change := range []float64{2, 6, 10, 14} {
		for _, chorus := range []float64{8, 16} {
			if !(vel[change] > vel[chorus]) {
				t.Errorf("сила смены части в %v (%v) должна быть больше силы такта припева в %v (%v)",
					change, vel[change], chorus, vel[chorus])
			}
		}
	}
}

func TestLiveCymbals_NoChangeNoCrash(t *testing.T) {
	bars := []yue.ChordBar{bar(0, 2, "verse"), bar(2, 4, "verse"), bar(4, 6, "verse")}
	if crash := crashSpecs(liveCymbals(bars, 0, true)); len(crash) != 0 {
		t.Fatalf("смен части и припева нет — записи крэша быть не должно, получено %+v", crash)
	}
}

func TestLiveCymbals_EmptyBars(t *testing.T) {
	if specs := liveCymbals(nil, 0, true); len(specs) != 0 {
		t.Fatalf("пустая сетка — записей нет, получено %+v", specs)
	}
}

func TestLiveCymbals_RidePerChorusRun(t *testing.T) {
	ride := sortedRides(t, liveCymbals(testBars(), 0, true))
	checkRideWindows(t, ride, [][2]float64{{6, 10}, {14, 18}})
}

func TestLiveCymbals_RideOff_CrashStays(t *testing.T) {
	specs := liveCymbals(testBars(), 0, false)
	if ride := rideSpecs(specs); len(ride) != 0 {
		t.Errorf("ride=false — записей ride нет, получено %+v", ride)
	}
	if got := hitTimes(crashHits(t, specs)); !slices.Equal(got, []float64{2, 6, 8, 10, 14, 16}) {
		t.Errorf("ride=false: удары крэша в %v, ожидались те же, что с ride", got)
	}
}

func TestLiveCymbals_NoChorusNoRide(t *testing.T) {
	bars := []yue.ChordBar{bar(0, 2, "intro"), bar(2, 4, "verse"), bar(4, 6, "bridge")}
	if ride := rideSpecs(liveCymbals(bars, 0, true)); len(ride) != 0 {
		t.Fatalf("припева нет — ride нет, получено %+v", ride)
	}
}

// Весь трек — припев: удары на всех тактах, кроме первого такта трека; ride — один на весь трек.
func TestLiveCymbals_OnlyChorus(t *testing.T) {
	bars := []yue.ChordBar{bar(0, 2, "chorus"), bar(2, 4, "chorus"), bar(4, 6, "chorus")}
	specs := liveCymbals(bars, 0, true)
	if got, want := hitTimes(crashHits(t, specs)), []float64{2, 4}; !slices.Equal(got, want) {
		t.Errorf("удары крэша в %v, ожидалось %v", got, want)
	}
	checkRideWindows(t, sortedRides(t, specs), [][2]float64{{0, 6}})
}

// Трек кончается посреди последнего такта припева (15 внутри [14, 16]): ride обрезан по dur,
// у такта 16 (Start ≥ dur) удара нет.
func TestLiveCymbals_DurInsideLastChorusBar(t *testing.T) {
	specs := liveCymbals(testBars(), 15, true)
	checkRideWindows(t, sortedRides(t, specs), [][2]float64{{6, 10}, {14, 15}})
	if got, want := hitTimes(crashHits(t, specs)), []float64{2, 6, 8, 10, 14}; !slices.Equal(got, want) {
		t.Errorf("удары крэша в %v, ожидалось %v", got, want)
	}
}

// Второй припев начинается за концом звука (dur 13 < 14): записи ride на него нет,
// ударов крэша с T ≥ dur нет.
func TestLiveCymbals_ChorusBeyondDur(t *testing.T) {
	const dur = 13
	specs := liveCymbals(testBars(), dur, true)
	checkRideWindows(t, sortedRides(t, specs), [][2]float64{{6, 10}})
	got := hitTimes(crashHits(t, specs))
	for _, at := range got {
		if at >= dur {
			t.Errorf("удар крэша в %v за концом звука %v", at, float64(dur))
		}
	}
	if want := []float64{2, 6, 8, 10}; !slices.Equal(got, want) {
		t.Errorf("удары крэша в %v, ожидалось %v", got, want)
	}
}

// --- liveLayer ---

func isAir(s SectionSpec) bool {
	typ, _ := engineKit(s)
	return typ == "air"
}

func airSpecs(specs []SectionSpec) []SectionSpec {
	var out []SectionSpec
	for _, s := range specs {
		if isAir(s) {
			out = append(out, s)
		}
	}
	return out
}

func nonAir(specs []SectionSpec) []SectionSpec {
	var out []SectionSpec
	for _, s := range specs {
		if !isAir(s) {
			out = append(out, s)
		}
	}
	return out
}

// checkAir — записи воздуха ровно на дорожки want, в этом порядке; не Add, один блок air без only.
func checkAir(t *testing.T, specs []SectionSpec, want ...string) {
	t.Helper()
	air := airSpecs(specs)
	var got []string
	for _, a := range air {
		if len(a.Stems) != 1 {
			t.Errorf("воздух: ожидалась одна дорожка, получено %v", a.Stems)
			continue
		}
		got = append(got, a.Stems[0])
		if a.Add {
			t.Errorf("воздух на %v: Add должен быть false", a.Stems)
		}
		if len(a.Engine) != 1 {
			t.Errorf("воздух на %v: ожидался один блок, получено %+v", a.Stems, a.Engine)
		} else if v, ok := a.Engine[0]["only"]; ok && num(t, v) != 0 {
			t.Errorf("воздух на %v: only = %v, ожидалось без only", a.Stems, v)
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("воздух на дорожках %v, ожидалось %v", got, want)
	}
}

func TestLiveLayer_NoSpecs_AirOnMixOnly(t *testing.T) {
	f := &liveFake{grid: &yue.ChordGrid{Bars: testBars()}}
	p := yue.SoundPreset{Name: "мастер", Final: []yue.PresetStep{{Chain: "x"}},
		Master: []map[string]any{{"type": "limiter"}}}
	specs, err := liveLayer(context.Background(), f, &yue.Job{ID: 7}, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 {
		t.Fatalf("ожидалась ровно одна запись, получено %d (%+v)", len(specs), specs)
	}
	s := specs[0]
	if !slices.Equal(s.Stems, []string{"mix"}) || !s.Add {
		t.Errorf("ожидались Stems [mix] и Add, получено %v Add %v", s.Stems, s.Add)
	}
	if len(s.Engine) != 1 || s.Engine[0]["type"] != "air" {
		t.Fatalf("ожидался один блок air, получено %+v", s.Engine)
	}
	if num(t, s.Engine[0]["only"]) != 1 {
		t.Errorf("only = %v, ожидалось 1", s.Engine[0]["only"])
	}
	if f.calls != 0 {
		t.Errorf("ChordGrid вызван %d раз, ожидалось 0", f.calls)
	}
}

func TestLiveLayer_MachinePreset_AirOnly(t *testing.T) {
	f := &liveFake{grid: &yue.ChordGrid{Bars: testBars()}}
	specs, err := liveLayer(context.Background(), f, &yue.Job{ID: 7}, machinePreset())
	if err != nil {
		t.Fatal(err)
	}
	checkAir(t, specs, "vocals", "other")
	if len(specs) != 2 {
		t.Errorf("машинный пресет — только воздух (2 записи), получено %d (%+v)", len(specs), specs)
	}
	if f.calls != 0 {
		t.Errorf("ChordGrid вызван %d раз, ожидалось 0", f.calls)
	}
}

func TestLiveLayer_AirSkipsProcessedStems(t *testing.T) {
	for _, tc := range []struct {
		stem string
		want []string
	}{
		{"guitar", []string{"vocals"}},
		{"piano", []string{"vocals"}},
		{"other", []string{"vocals"}},
		{"vocals", []string{"other"}},
	} {
		p := machinePreset()
		p.Specs = append(p.Specs, yue.PresetSpec{
			Stems: []string{"bass", tc.stem}, Engine: []map[string]any{{"type": "comp"}},
		})
		f := &liveFake{}
		specs, err := liveLayer(context.Background(), f, &yue.Job{ID: 7}, p)
		if err != nil {
			t.Fatalf("%s: %v", tc.stem, err)
		}
		t.Run(tc.stem, func(t *testing.T) { checkAir(t, specs, tc.want...) })
	}
}

func TestLiveLayer_AirSkipsBoth(t *testing.T) {
	p := machinePreset()
	p.Specs = append(p.Specs,
		yue.PresetSpec{Stems: []string{"vocals"}, Engine: []map[string]any{{"type": "comp"}}},
		yue.PresetSpec{Stems: []string{"guitar"}, Engine: []map[string]any{{"type": "drive"}}},
	)
	specs, err := liveLayer(context.Background(), &liveFake{}, &yue.Job{ID: 7}, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 0 {
		t.Errorf("голос и гитара уже обработаны, набор машинный — записей нет, получено %+v", specs)
	}
}

func TestLiveLayer_LiveKit_AirAndCymbals(t *testing.T) {
	bars := testBars()
	f := &liveFake{grid: &yue.ChordGrid{BPM: 120, Bars: bars}}
	job := &yue.Job{ID: 42, DurationSec: 15}
	specs, err := liveLayer(context.Background(), f, job, liveKitPreset())
	if err != nil {
		t.Fatal(err)
	}
	checkAir(t, specs, "vocals", "other")
	if f.calls != 1 || f.ids[0] != 42 {
		t.Errorf("ChordGrid: вызовов %d (id %v), ожидался один вызов по id 42", f.calls, f.ids)
	}
	want := liveCymbals(bars, 15, true)
	if len(rideSpecs(want)) == 0 || len(crashSpecs(want)) == 0 {
		t.Fatal("сетка теста должна давать и крэш, и ride")
	}
	if cym := nonAir(withoutFills(specs)); !reflect.DeepEqual(cym, want) {
		t.Errorf("тарелки: получено %+v, ожидалось liveCymbals(сетка, 15, true) %+v", cym, want)
	}
}

func TestLiveLayer_LiveKickNoHH_CrashNoRide(t *testing.T) {
	bars := testBars()
	f := &liveFake{grid: &yue.ChordGrid{BPM: 120, Bars: bars}}
	specs, err := liveLayer(context.Background(), f, &yue.Job{ID: 5, DurationSec: 18}, liveKickPreset())
	if err != nil {
		t.Fatal(err)
	}
	cym := nonAir(withoutFills(specs))
	if len(crashSpecs(cym)) != 1 {
		t.Errorf("живая бочка без hh — крэш есть, получено %+v", cym)
	}
	if len(rideSpecs(cym)) != 0 {
		t.Errorf("живая бочка без hh — ride нет, получено %+v", rideSpecs(cym))
	}
	if want := liveCymbals(bars, 18, false); !reflect.DeepEqual(cym, want) {
		t.Errorf("тарелки: получено %+v, ожидалось liveCymbals(сетка, 18, false) %+v", cym, want)
	}
}

func TestLiveLayer_LiveKit_NoGrid404(t *testing.T) {
	f := &liveFake{err: &yue.StatusError{Code: 404, Msg: "нет сетки"}}
	specs, err := liveLayer(context.Background(), f, &yue.Job{ID: 7}, liveKitPreset())
	if err != nil {
		t.Fatalf("404 сетки — не ошибка, получено %v", err)
	}
	checkAir(t, specs, "vocals", "other")
	if len(specs) != 2 {
		t.Errorf("без сетки — только воздух (2 записи), получено %+v", specs)
	}
	if f.calls != 1 {
		t.Errorf("ChordGrid вызван %d раз, ожидался 1", f.calls)
	}
}

func TestLiveLayer_LiveKit_GridErrorPropagates(t *testing.T) {
	for _, e := range []error{
		&yue.StatusError{Code: 500, Msg: "сломалось"},
		errors.New("сеть"),
	} {
		f := &liveFake{err: e}
		if _, err := liveLayer(context.Background(), f, &yue.Job{ID: 7}, liveKitPreset()); err == nil {
			t.Errorf("ошибка ChordGrid %v — ожидалась ошибка liveLayer", e)
		}
	}
}

func TestLiveLayer_WithoutLiveLayerCtx(t *testing.T) {
	ctx := withoutLiveLayer(context.Background())
	for _, p := range []yue.SoundPreset{
		liveKitPreset(),
		liveKickPreset(),
		machinePreset(),
		{Name: "мастер", Final: []yue.PresetStep{{Chain: "x"}}},
	} {
		f := &liveFake{grid: &yue.ChordGrid{Bars: testBars()}}
		specs, err := liveLayer(ctx, f, &yue.Job{ID: 7}, p)
		if err != nil {
			t.Fatalf("%s: %v", p.Name, err)
		}
		if len(specs) != 0 {
			t.Errorf("%s: с withoutLiveLayer записей быть не должно, получено %+v", p.Name, specs)
		}
		if f.calls != 0 {
			t.Errorf("%s: ChordGrid вызван %d раз, ожидалось 0", p.Name, f.calls)
		}
	}
}
