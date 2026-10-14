package studio

import (
	"context"
	"errors"
	"fmt"
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
	// на 3 и 6 дБ тише (#851) — редкий громкий раскат не выпирает
	liveCrashRelDb = -13.0
	liveRideDb     = -18.0
	// сэмплы тарелок набора ярче тарелок в сведённых записях: без среза верх трека выше 16 кГц на 7 дБ светлее
	// живых записей (#851: −31 дБ при −40…−42), со срезом 14 кГц — −37,9, как без добавленных тарелок
	liveCymbalLowpassHz = 14000.0
	liveCrashDur        = 2.0 // с: длина удара крэша (хвост сэмпла)
	liveCrashVel        = 0.9 // удар на смене части
	liveChorusVel       = 0.7 // удар на такте припева
	liveChorus          = "chorus"
)

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
	return append(out, liveCymbals(grid.Bars, job.DurationSec, hh)...), nil
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
