package studio

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"

	"partscaster/internal/yue"
)

// Слой «живой звук» — общий для всех пресетов, применяется вместе с пресетом (сырой трек без пресета —
// выход YuE2 как есть). По замеру против живых записей (FLAC) у YuE2 два изъяна, которые пресет сам не лечит:
//   - верх выше ~16 кГц срезан кодеком: голос «глухой» — блок движка air достраивает его по соседней полосе
//     (тёмная по природе дорожка такой и остаётся). Только дорожкам, которые пресет сам не обрабатывает: воздух
//     считается от исходной дорожки и пробил бы срез верха, который пресет ставит гитаре намеренно;
//   - тарелок мало: crash тише живых на 6–30 дБ, ride — до 10 дБ. У пресета с живым набором крэш добавляется на
//     начало каждой части песни и на каждый такт припева; ride — в припевах по ударам хэта, если хэт пресет
//     заменяет набором (есть его дорожка). Пресет с драм-машиной (808, 909 и др.) свои барабаны сохраняет:
//     машина там — часть стиля.
// Слой не входит в рецепт пресета и в журнал правок: при импорте рецепта он добавится заново, без удвоения.

const (
	liveKitPrefix = "osdk/"
	liveCrashKit  = "osdk/crash"
	liveRideKit   = "osdk/ride"
	// громкость крэша: удары к уровню трека в момент удара (блок perc); ride — к уровню хэта, по ударам которого
	// играет (sampler). Замер на After Glow (#847: crash −27,6 дБ к треку при −27 у живых записей) и слух:
	// на 3 и 6 дБ тише (#851) — редкий громкий раскат не выпирает; ещё на 5 дБ тише (2026-10-15, After Glow
	// #979 → #984: «редкие удары по тарелке громкие» — сэмпл ярче тарелок сведённых записей)
	liveCrashRelDb = -18.0
	liveRideDb     = -18.0
	// сэмплы тарелок набора ярче тарелок в сведённых записях: без среза верх трека выше 16 кГц на 7 дБ светлее
	// живых записей (#851: −31 дБ при −40…−42), со срезом 14 кГц — −37,9, как без добавленных тарелок
	liveCymbalLowpassHz = 14000.0
	liveCrashDur        = 2.0 // с: длина удара крэша (хвост сэмпла)
	liveCrashVel        = 0.9 // удар на смене части
	liveChorusVel       = 0.7 // удар на такте припева
	liveChorus          = "chorus"

	// барабанщик (2026-10-15, After Glow #990 → #1002 «нравится, можно чуть громче»): сбивка перед сменой части —
	// дробь малого, томы сверху вниз шестнадцатыми с нарастанием; длина — полтакта, в припев — такт, в последний
	// припев — два такта
	liveFillRelDb     = -7.0
	liveFillHumanize  = 6.0
	liveFillVelFrom   = 0.6
	liveFillVelTo     = 1.0
	liveFillSnareTo   = 1.0 / 3 // доля сбивки: до неё — малый
	liveFillTomHiTo   = 5.0 / 9 // … высокий том
	liveFillTomMidTo  = 7.0 / 9 // … средний, дальше — низкий
	liveFillHitDur    = 0.3
	liveFillPerBar    = 16  // шестнадцатые
	liveDrumChorusDb  = 4.5 // барабаны вперёд: припев громче, куплет чуть глубже
	liveDrumPreDb     = 1.0
	liveDrumVerseDb   = -1.0
	liveDrumPreChorus = "pre-chorus"
	liveDrumVerse     = "verse"
)

// liveFillKits — наборы сбивки по порядку: малый, высокий, средний, низкий том
var liveFillKits = []string{"osdk/snare", "osdk/tom-small", "osdk/tom-medium", "osdk/tom-large"}

// liveDrumParts — части барабанов, которым «барабаны вперёд» ставит линию громкости
var liveDrumParts = []string{"kick", "snare", "toms", "hh", "ride", "crash"}

// liveAirStems — дорожка, которой достраивается верх, → дорожки, обработка которых пресетом его исключает
// (части гитары и клавишных — внутри other)
var liveAirStems = map[string][]string{
	"vocals": {"vocals"},
	"other":  {"other", "guitar", "piano"},
}

// liveAirOrder — порядок записей воздуха (карта не упорядочена)
var liveAirOrder = []string{"vocals", "other"}

// presetLiveKit — пресет заменяет части барабанов живым набором (а не машиной); hh — заменяет и хэт: есть его
// дорожка и удары, по которым играет ride.
func presetLiveKit(p yue.SoundPreset) (live, hh bool) {
	for _, s := range p.Specs {
		for _, b := range s.Engine {
			if kit, _ := b["kit"].(string); b["type"] == "sampler" && strings.HasPrefix(kit, liveKitPrefix) {
				live = true
				hh = hh || slices.Contains(s.Stems, "hh")
			}
		}
	}
	return live, hh
}

// presetTouches — пресет обрабатывает одну из дорожек stems
func presetTouches(p yue.SoundPreset, stems []string) bool {
	for _, s := range p.Specs {
		if slices.ContainsFunc(s.Stems, func(n string) bool { return slices.Contains(stems, n) }) {
			return true
		}
	}
	return false
}

type liveLayerOffKey struct{}

// withoutLiveLayer — пресет без слоя: проверки механики пресета (мастер, финал, места дорожек), где лишняя
// пересборка слоя заслоняла бы проверяемое; сам слой проверяется своими тестами.
func withoutLiveLayer(ctx context.Context) context.Context {
	return context.WithValue(ctx, liveLayerOffKey{}, true)
}

// liveLayer — записи пересборки слоя «живой звук» для пресета p на треке job.
func liveLayer(ctx context.Context, svc yue.Service, job *yue.Job, p yue.SoundPreset) ([]SectionSpec, error) {
	if off, _ := ctx.Value(liveLayerOffKey{}).(bool); off {
		return nil, nil
	}
	out := make([]SectionSpec, 0, len(liveAirOrder)+4)
	if len(p.Specs) == 0 {
		// пресет работает только с целым миксом — воздух поверх него, без разделения на дорожки (минуты GPU)
		return append(out, SectionSpec{Stems: []string{"mix"}, Add: true,
			Engine: []map[string]any{{"type": "air", "only": 1}}}), nil
	}
	for _, st := range liveAirOrder {
		if !presetTouches(p, liveAirStems[st]) {
			out = append(out, SectionSpec{Stems: []string{st}, Engine: []map[string]any{{"type": "air"}}})
		}
	}
	live, hh := presetLiveKit(p)
	if !live {
		return out, nil
	}
	grid, err := svc.ChordGrid(ctx, job.ID)
	var se *yue.StatusError
	if errors.As(err, &se) && se.Code == http.StatusNotFound {
		return out, nil // у трека нет плана — нет частей песни: тарелкам не к чему привязаться
	}
	if err != nil {
		return nil, fmt.Errorf("части песни: %w", err)
	}
	out = append(out, liveCymbals(grid.Bars, job.DurationSec, hh)...)
	return append(out, liveFills(grid.Bars, job.DurationSec)...), nil
}

// fillSpan — сбивка перед частью, которая начинается тактом i: [from, to) и число шестнадцатых.
type fillSpan struct {
	i        int
	from, to float64
	n        int
}

// liveFillSpans — сбивки перед каждой сменой части песни (кроме начала трека): полтакта, в припев — такт, в
// последний припев трека — два такта (по длине такта перед частью).
func liveFillSpans(bars []yue.ChordBar, dur float64) []fillSpan {
	last := -1
	for i := 1; i < len(bars); i++ {
		if bars[i].Section == liveChorus && bars[i-1].Section != liveChorus {
			last = i
		}
	}
	var out []fillSpan
	for i := 1; i < len(bars); i++ {
		if bars[i].Section == "" || bars[i].Section == bars[i-1].Section {
			continue
		}
		if dur > 0 && bars[i].Start >= dur {
			break
		}
		nb := 0.5
		switch {
		case i == last:
			nb = 2
		case bars[i].Section == liveChorus:
			nb = 1
		}
		bar := bars[i-1].End - bars[i-1].Start
		if bar <= 0 {
			continue
		}
		n := int(math.Round(nb * liveFillPerBar))
		from := bars[i].Start - float64(n)*bar/liveFillPerBar
		if from < 0 {
			continue
		}
		out = append(out, fillSpan{i: i, from: from, to: bars[i].Start, n: n})
	}
	return out
}

// liveFills — сбивки: удары шестнадцатыми по наборам liveFillKits (первая треть — малый, дальше томы сверху вниз),
// сила нарастает liveFillVelFrom → liveFillVelTo. Партия на набор — блок perc поверх трека, как крэш.
func liveFills(bars []yue.ChordBar, dur float64) []SectionSpec {
	hits := make([][]PercHit, len(liveFillKits))
	for _, f := range liveFillSpans(bars, dur) {
		q := (f.to - f.from) / float64(f.n)
		for k := 0; k < f.n; k++ {
			frac := 0.0
			if f.n > 1 {
				frac = float64(k) / float64(f.n-1)
			}
			kit := 3
			switch {
			case frac < liveFillSnareTo:
				kit = 0
			case frac < liveFillTomHiTo:
				kit = 1
			case frac < liveFillTomMidTo:
				kit = 2
			}
			vel := liveFillVelFrom + (liveFillVelTo-liveFillVelFrom)*frac
			hits[kit] = append(hits[kit], PercHit{T: f.from + float64(k)*q, D: liveFillHitDur, Vel: vel})
		}
	}
	var out []SectionSpec
	for k, kit := range liveFillKits {
		if len(hits[k]) == 0 {
			continue
		}
		out = append(out, SectionSpec{Stems: []string{"mix"}, Add: true, Engine: []map[string]any{
			{"type": "perc", "voice": 0, "kit": kit, "rel_db": liveFillRelDb, "humanize_ms": liveFillHumanize,
				"notes": hits[k]},
		}})
	}
	return out
}

// liveDrumDb — громкость барабанов в части песни: припев вперёд, пред-припев чуть, куплет чуть глубже.
func liveDrumDb(section string) float64 {
	switch section {
	case liveChorus:
		return liveDrumChorusDb
	case liveDrumPreChorus:
		return liveDrumPreDb
	case liveDrumVerse:
		return liveDrumVerseDb
	}
	return 0
}

// liveDrumPoints — линия громкости барабанов по частям песни (точки {t, db} в секундах трека): на каждой части —
// её дБ, на сбивке перед частью — подъём (или спад) от прежней части к новой.
func liveDrumPoints(bars []yue.ChordBar, dur float64) []map[string]any {
	if len(bars) == 0 {
		return nil
	}
	fills := map[int]fillSpan{}
	for _, f := range liveFillSpans(bars, dur) {
		fills[f.i] = f
	}
	// такт без части (за концом плана) держит часть прежнего: иначе барабаны падали бы скачком там, где кончился план
	sec := make([]string, len(bars))
	for i, b := range bars {
		sec[i] = b.Section
		if sec[i] == "" && i > 0 {
			sec[i] = sec[i-1]
		}
	}
	pts := []map[string]any{{"t": max(0, bars[0].Start), "db": liveDrumDb(sec[0])}}
	for i := 1; i < len(bars); i++ {
		if sec[i] == sec[i-1] {
			continue
		}
		prev, next := liveDrumDb(sec[i-1]), liveDrumDb(sec[i])
		if f, ok := fills[i]; ok {
			// сбивка длиннее прежней части — подъём не раньше её начала: точки линии идут по возрастанию
			last, _ := pts[len(pts)-1]["t"].(float64)
			pts = append(pts, map[string]any{"t": max(f.from, last), "db": prev})
		} else {
			pts = append(pts, map[string]any{"t": bars[i].Start, "db": prev})
		}
		pts = append(pts, map[string]any{"t": bars[i].Start, "db": next})
	}
	return pts
}

// liveDrumSpecs — «барабаны вперёд»: у пресета с живым набором и трека с припевом — копия записей, где частям
// барабанов в конец цепочки добавлена линия громкости по частям песни (блок движка level), а цель громкости
// поднята на liveDrumChorusDb (замер «громких мест» приходится на припев: без этого сверка опустила бы барабаны во
// всём треке). Иначе — записи как есть. Пресет на воркере не меняется.
func liveDrumSpecs(ctx context.Context, svc yue.Service, job *yue.Job, p yue.SoundPreset) ([]yue.PresetSpec, error) {
	if off, _ := ctx.Value(liveLayerOffKey{}).(bool); off {
		return p.Specs, nil
	}
	if live, _ := presetLiveKit(p); !live {
		return p.Specs, nil
	}
	grid, err := svc.ChordGrid(ctx, job.ID)
	var se *yue.StatusError
	if errors.As(err, &se) && se.Code == http.StatusNotFound {
		return p.Specs, nil
	}
	if err != nil {
		return nil, fmt.Errorf("части песни: %w", err)
	}
	if !slices.ContainsFunc(grid.Bars, func(b yue.ChordBar) bool { return b.Section == liveChorus }) {
		return p.Specs, nil
	}
	pts := liveDrumPoints(grid.Bars, job.DurationSec)
	out := make([]yue.PresetSpec, len(p.Specs))
	for i, s := range p.Specs {
		out[i] = s
		drums := len(s.Stems) > 0 && !slices.ContainsFunc(s.Stems, func(n string) bool {
			return !slices.Contains(liveDrumParts, n)
		})
		if !drums || len(s.Engine) == 0 {
			continue
		}
		eng := make([]map[string]any, 0, len(s.Engine)+1)
		eng = append(eng, s.Engine...)
		out[i].Engine = append(eng, map[string]any{"type": "level", "points": pts})
		if s.LevelDb != nil {
			v := *s.LevelDb + liveDrumChorusDb
			out[i].LevelDb = &v
		}
	}
	return out, nil
}

// liveCymbals — крэш на первую долю каждой смены части песни (кроме начала трека) и каждого такта припева,
// с ride — ride по ударам хэта в каждом припеве: у живого барабанщика припев звучит «стеной» тарелок. dur — длина
// трека (0 — не известна): такт плана может выйти за конец звука, окно за ним воркер не примет.
func liveCymbals(bars []yue.ChordBar, dur float64, ride bool) []SectionSpec {
	var out []SectionSpec
	var crash []PercHit
	for i := 1; i < len(bars); i++ {
		if dur > 0 && bars[i].Start >= dur {
			break
		}
		switch {
		case bars[i].Section != "" && bars[i].Section != bars[i-1].Section:
			crash = append(crash, PercHit{T: bars[i].Start, D: liveCrashDur, Vel: liveCrashVel})
		case bars[i].Section == liveChorus:
			crash = append(crash, PercHit{T: bars[i].Start, D: liveCrashDur, Vel: liveChorusVel})
		}
	}
	if len(crash) > 0 {
		out = append(out, SectionSpec{Stems: []string{"mix"}, Add: true, Engine: []map[string]any{
			{"type": "perc", "voice": 0, "kit": liveCrashKit, "rel_db": liveCrashRelDb, "notes": crash},
			{"type": "eq", "lowpass_hz": liveCymbalLowpassHz},
		}})
	}
	if !ride {
		return out
	}
	for i := 0; i < len(bars); {
		if bars[i].Section != liveChorus {
			i++
			continue
		}
		j := i
		for j+1 < len(bars) && bars[j+1].Section == liveChorus {
			j++
		}
		from, to := bars[i].Start, bars[j].End
		if dur > 0 {
			to = min(to, dur)
		}
		if to > from {
			out = append(out, SectionSpec{Stems: []string{"hh"}, Add: true, From: from, To: to,
				Engine: []map[string]any{
					{"type": "sampler", "kit": liveRideKit, "output_db": liveRideDb},
					{"type": "eq", "lowpass_hz": liveCymbalLowpassHz},
				}})
		}
		i = j + 1
	}
	return out
}
