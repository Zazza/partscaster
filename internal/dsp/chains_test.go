package dsp

import (
	"fmt"
	"math"
	"path/filepath"
	"testing"
)

// Тесты цепочек по ID: breathe, gap, cresc, tempo-from, tape, warp
// и общий параметр from. Проверяется звук на синтетике через Run, а не текст графа
// (кроме регрессии «кГц → Гц» — она про граф).

const chSR = 16000

func dbfs(x float64) float64 {
	if x <= 0 {
		return -200
	}
	return 20 * math.Log10(x)
}

// genIn — вход 16 кГц моно из выражения aevalsrc (запятые экранированы).
func genIn(t *testing.T, expr string, dur float64) (in string, samples []float32) {
	t.Helper()
	in = filepath.Join(t.TempDir(), "in.flac")
	lavfi(t, fmt.Sprintf("aevalsrc=exprs='%s':d=%g:s=16000", expr, dur), in)
	return in, decode(t, in)
}

// runChain — прогнать вход через цепочку id с параметрами p, вернуть выход 16 кГц моно.
func runChain(t *testing.T, id, in string, p map[string]float64) []float32 {
	t.Helper()
	c := ByID(id)
	if c == nil {
		t.Fatalf("ByID(%q) = nil, цепочка должна существовать", id)
	}
	out := filepath.Join(t.TempDir(), "out.flac")
	if err := Run(in, out, c.FilterGraph(p), nil); err != nil {
		t.Fatalf("Run %s %v: %v", id, p, err)
	}
	return decode(t, out)
}

func secs(s []float32) float64 { return float64(len(s)) / chSR }

func diffSeg(a, b []float32, from, to float64) float64 {
	x, y := a[min(int(from*chSR), len(a)):min(int(to*chSR), len(a))],
		b[min(int(from*chSR), len(b)):min(int(to*chSR), len(b))]
	n := min(len(x), len(y))
	if n == 0 {
		return math.Inf(1)
	}
	d := make([]float32, n)
	for i := range d {
		d[i] = x[i] - y[i]
	}
	return RMS(d)
}

func hasParam(c *Chain, id string) bool {
	for _, p := range c.Params {
		if p.ID == id {
			return true
		}
	}
	return false
}

// Цепочки breathe/gap/cresc/tempo-from/tape/warp доступны по ID; чужой ID — nil.
func TestChainsByID(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"breathe", "gap", "cresc", "tempo-from", "tape", "warp"} {
		if c := ByID(id); c == nil || c.ID != id || c.Name == "" {
			t.Errorf("ByID(%q) = %+v, want цепочку с этим ID и именем", id, c)
		}
	}
	if c := ByID("no-such-chain"); c != nil {
		t.Errorf("ByID(no-such-chain) = %+v, want nil", c)
	}
}

func TestChainsDefaultsCoverParams(t *testing.T) {
	t.Parallel()
	for _, c := range All() {
		d := c.Defaults()
		if len(d) != len(c.Params) {
			t.Errorf("%s: Defaults %d значений, крутилок %d", c.ID, len(d), len(c.Params))
		}
		for _, p := range c.Params {
			if v, ok := d[p.ID]; !ok || v != p.Default {
				t.Errorf("%s: Defaults[%s]=%v (есть=%v), want %v", c.ID, p.ID, v, ok, p.Default)
			}
		}
		// без параметров граф строится по значениям по умолчанию
		if c.FilterGraph(nil) != c.FilterGraph(d) {
			t.Errorf("%s: граф без параметров ≠ граф со значениями по умолчанию", c.ID)
		}
	}
}

// Общий параметр from — у цепочек без собственного start; у cresc/tempo-from/gap
// (своя отметка start) его нет.
func TestChainsFromParamOnlyWithoutStart(t *testing.T) {
	t.Parallel()
	for _, c := range All() {
		c := c
		start, from := hasParam(&c, "start"), hasParam(&c, "from")
		if start && from {
			t.Errorf("%s: есть и start, и from — from только для цепочек без своей отметки", c.ID)
		}
		if !start && !from {
			t.Errorf("%s: нет ни start, ни from — эффект нельзя включить с отметки", c.ID)
		}
	}
	for _, id := range []string{"cresc", "tempo-from", "gap"} {
		if c := ByID(id); c == nil || !hasParam(c, "start") {
			t.Errorf("%s: нет параметра start", id)
		}
	}
	for _, id := range []string{"breathe", "tape", "warp"} {
		if c := ByID(id); c == nil || !hasParam(c, "from") {
			t.Errorf("%s: нет общего параметра from", id)
		}
	}
}

// Каждая цепочка с дефолтами даёт валидный граф — Run не падает, выход не пуст.
// Вход 3 с: отметки start по умолчанию (120 с) лежат за концом — граф всё равно валиден.
func TestEveryChainDefaultsRuns(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	in, _ := genIn(t, "0.5*sin(2*PI*440*t)", 3)
	// цепочки с Key (ducking) работают только с ключом — второй вход: щелчки каждые 0.5 с
	key, _ := genIn(t, "0.8*lt(mod(t\\,0.5)\\,0.02)", 3)
	for _, c := range All() {
		t.Run(c.ID, func(t *testing.T) {
			t.Parallel()
			var out []float32
			if c.Key != "" {
				p := filepath.Join(t.TempDir(), "out.flac")
				if err := RunInputs([]string{in, key}, p, c.FilterGraph(nil)); err != nil {
					t.Fatalf("RunInputs %s с ключом: %v", c.ID, err)
				}
				out = decode(t, p)
			} else {
				out = runChain(t, c.ID, in, nil)
			}
			// по умолчанию длина выхода = длине входа до 5 мс (pitch короче на ~2,6 мс —
			// окно сдвига высоты); хвост реверба/дилея сюда не дописывается (TailSec —
			// для вызывающего). tempo-from/drop меняют длину по замыслу; slapback,
			// megaphone и cab длиннее на 100/110/10 мс — хвост эха/фильтра в конце, звук не
			// сдвинут (TestCabAndEchoesDoNotShift); здесь — только «не короче».
			switch c.ID {
			case "tempo-from", "drop":
				if d := secs(out); d < 1 {
					t.Errorf("выход %.2f с — подозрительно короткий", d)
				}
				return
			case "cab", "slapback", "megaphone":
				if d := secs(out); d < 3-0.005 {
					t.Errorf("длина выхода %.4f с — короче входа (3 с)", d)
				}
				return
			}
			if d := secs(out) - 3; math.Abs(d) > 0.005 {
				t.Errorf("длина выхода %.4f с, want 3 ± 0,005 (как вход)", secs(out))
			}
		})
	}
}

// После tape/warp с дефолтами синус не заглушён (RMS > −40 дБ).
// Дополнительно без шипения: срез 5–16 кГц не должен съедать тон 440 Гц — выход
// не тише входа более чем на 12 дБ (срез «в Гц» давал −60 дБ и глубже).
func TestTapeWarpDoNotSilence(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	in, src := genIn(t, "0.5*sin(2*PI*440*t)", 4)
	inDb := dbfs(RMS(src))
	for _, tc := range []struct {
		id string
		p  map[string]float64
	}{
		{"tape", nil}, {"warp", nil}, {"tape", map[string]float64{"hiss": 0}},
	} {
		t.Run(fmt.Sprintf("%s%v", tc.id, tc.p), func(t *testing.T) {
			t.Parallel()
			out := runChain(t, tc.id, in, tc.p)
			r := dbfs(segRMS(out, 0.5, 3.5))
			if r <= -40 {
				t.Errorf("RMS выхода %.1f дБ, want > −40 (трек заглох)", r)
			}
			if r < inDb-12 {
				t.Errorf("RMS выхода %.1f дБ при входе %.1f дБ — тон 440 Гц срезан", r, inDb)
			}
		})
	}
}

// cresc — к концу громче, чем в начале; до start громкость не меняется.
func TestCrescLouderAtEnd(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	in, src := genIn(t, "0.2*sin(2*PI*440*t)", 10)
	out := runChain(t, "cresc", in, map[string]float64{"start": 3, "ramp": 3, "db": 6})
	begin, end := dbfs(segRMS(out, 0.5, 2.5)), dbfs(segRMS(out, 7, 9.5))
	if end-begin < 3 {
		t.Errorf("конец %.1f дБ, начало %.1f дБ: прирост %.1f, want ≥ 3 (задано +6)", end, begin, end-begin)
	}
	if d := begin - dbfs(segRMS(src, 0.5, 2.5)); math.Abs(d) > 1 {
		t.Errorf("до start громкость изменилась на %+.1f дБ, want ≈ 0", d)
	}
	if d := secs(out); math.Abs(d-10) > 0.1 {
		t.Errorf("длина %.2f с, want 10 (cresc длину не меняет)", d)
	}
}

// Краевой: cresc с db=0 — громкость не растёт.
func TestCrescZeroDbFlat(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	in, _ := genIn(t, "0.2*sin(2*PI*440*t)", 8)
	out := runChain(t, "cresc", in, map[string]float64{"start": 2, "ramp": 2, "db": 0})
	begin, end := dbfs(segRMS(out, 0.5, 1.5)), dbfs(segRMS(out, 6, 7.5))
	if math.Abs(end-begin) > 1 {
		t.Errorf("db=0: конец %.1f дБ, начало %.1f дБ, want разница ≈ 0", end, begin)
	}
}

// tempoIn — 440 Гц до 4 с, 880 Гц после (граница видна в выходе), 10 с.
const tempoExpr = "0.3*if(lt(t\\,4)\\,sin(2*PI*440*t)\\,sin(2*PI*880*t))"

// shortToneAmp — средняя амплитуда тона hz по окнам 50 мс на [from,to]: atempo
// склеивает куски с разрывом фазы, одна точка ДПФ на секунды их гасит взаимно.
func shortToneAmp(s []float32, hz, from, to float64) float64 {
	var sum float64
	n := 0
	for a := from; a+0.05 <= to; a += 0.05 {
		sum += toneAmpAt(s, chSR, hz, a, a+0.05)
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// tempo-from — выход короче при ускорении; участок до отметки своей длины не меняет.
func TestTempoFromShortensAfterMark(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	in, _ := genIn(t, tempoExpr, 10)
	out := runChain(t, "tempo-from", in, map[string]float64{"start": 4, "factor": 1.25})
	// 4 с как было + 6 с / 1.25 = 8.8 с
	if d := secs(out); math.Abs(d-8.8) > 0.2 {
		t.Errorf("длина выхода %.2f с, want 8.8 ± 0.2 (4 + 6/1.25)", d)
	}
	// до отметки — тот же 440 Гц на том же месте, смена на 880 — у отметки
	if a := toneAmpAt(out, chSR, 440, 1, 3.8); math.Abs(a-0.3) > 0.03 {
		t.Errorf("440 Гц на 1–3.8 с: %.3f, want ≈ 0.3 (до отметки трек нетронут)", a)
	}
	if a := shortToneAmp(out, 880, 4.3, 8); a < 0.2 {
		t.Errorf("880 Гц после отметки: %.3f, want ≈ 0.3 (высота не меняется, atempo)", a)
	}
	if a := shortToneAmp(out, 440, 4.3, 8); a > 0.05 {
		t.Errorf("440 Гц после 4.3 с: %.3f — участок до отметки растянулся", a)
	}
}

// Краевой: factor=1 — длина не меняется.
func TestTempoFromFactorOneKeepsLength(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	in, _ := genIn(t, tempoExpr, 10)
	out := runChain(t, "tempo-from", in, map[string]float64{"start": 4, "factor": 1})
	if d := secs(out); math.Abs(d-10) > 0.1 {
		t.Errorf("factor=1: длина %.2f с, want 10", d)
	}
}

// gap — тишина на dur секунд с отметки start; вне паузы звук на месте; длина та же.
func TestGapSilencesWindow(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	in, src := genIn(t, "0.5*sin(2*PI*440*t)", 6)
	out := runChain(t, "gap", in, map[string]float64{"start": 2, "dur": 2, "fade": 20})
	if r := dbfs(segRMS(out, 2.1, 3.9)); r > -50 {
		t.Errorf("в паузе 2.1–3.9 с RMS %.1f дБ, want тишина (< −50)", r)
	}
	for _, w := range [][2]float64{{0.2, 1.8}, {4.2, 5.8}} {
		if d := dbfs(segRMS(out, w[0], w[1])) - dbfs(segRMS(src, w[0], w[1])); math.Abs(d) > 1 {
			t.Errorf("вне паузы %.1f–%.1f с уровень изменился на %+.1f дБ", w[0], w[1], d)
		}
	}
	if d := secs(out); math.Abs(d-6) > 0.1 {
		t.Errorf("длина %.2f с, want 6 (пауза заменяет звук, не вставляется)", d)
	}
}

// breatheExpr — громко (0.5) 0–3 с, тихо (0.02) 3–6 с.
const breatheExpr = "if(lt(t\\,3)\\,0.5\\,0.02)*sin(2*PI*440*t)"

// breathe (экспандер) — тихие места тише, громкие как были.
func TestBreatheExpandsDynamics(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	in, src := genIn(t, breatheExpr, 6)
	out := runChain(t, "breathe", in, map[string]float64{"amount": 1.5})
	loud := dbfs(segRMS(out, 0.5, 2.5)) - dbfs(segRMS(src, 0.5, 2.5))
	quiet := dbfs(segRMS(out, 3.5, 5.5)) - dbfs(segRMS(src, 3.5, 5.5))
	if math.Abs(loud) > 1.5 {
		t.Errorf("громкое место изменилось на %+.1f дБ, want ≈ 0", loud)
	}
	if quiet > -0.5 {
		t.Errorf("тихое место изменилось на %+.1f дБ, want тише (≤ −0.5)", quiet)
	}
}

// Краевой: breathe amount=0 — выключено, уровни как у входа.
func TestBreatheZeroIsOff(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	in, src := genIn(t, breatheExpr, 6)
	out := runChain(t, "breathe", in, map[string]float64{"amount": 0})
	for _, w := range [][2]float64{{0.5, 2.5}, {3.5, 5.5}} {
		if d := dbfs(segRMS(out, w[0], w[1])) - dbfs(segRMS(src, w[0], w[1])); math.Abs(d) > 1 {
			t.Errorf("amount=0: на %.1f–%.1f с уровень изменился на %+.1f дБ", w[0], w[1], d)
		}
	}
}

// from — до from звук не меняется, после — эффект работает; длина сохраняется.
func TestFromKeepsAudioBeforeMark(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	in, src := genIn(t, "0.3*sin(2*PI*440*t)", 8)
	out := runChain(t, "wall", in, map[string]float64{"from": 3})
	if d := secs(out); math.Abs(d-8) > 0.1 {
		t.Errorf("длина %.2f с, want 8", d)
	}
	if r := diffSeg(out, src, 0, 2.9); r > 0.01 {
		t.Errorf("до from (0–2.9 с) выход отличается от входа: RMS разности %.4f", r)
	}
	if d := dbfs(segRMS(out, 0.5, 2.5)) - dbfs(segRMS(src, 0.5, 2.5)); math.Abs(d) > 0.5 {
		t.Errorf("до from уровень изменился на %+.1f дБ", d)
	}
	if r := diffSeg(out, src, 4, 7.5); r < 0.02 {
		t.Errorf("после from эффекта нет: RMS разности %.4f", r)
	}
}

// Краевой: from=0 — эффект на весь трек с самого начала.
func TestFromZeroWholeTrack(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	in, src := genIn(t, "0.3*sin(2*PI*440*t)", 6)
	out := runChain(t, "wall", in, map[string]float64{"from": 0})
	if d := secs(out); math.Abs(d-6) > 0.1 {
		t.Errorf("длина %.2f с, want 6", d)
	}
	if r := diffSeg(out, src, 0.2, 2); r < 0.02 {
		t.Errorf("from=0: в начале эффекта нет (RMS разности %.4f)", r)
	}
}

// Педаль/эффект не сдвигает звук во времени: щелчок на 1,0 с остаётся на 1,0 с (±1 мс). Иначе обработанная
// дорожка опаздывает относительно остальных — гребёнка с остатками той же партии в других дорожках.
// «Кабинет» опаздывал на 10 мс из-за zero_phase у firequalizer (сдвигает метки времени).
func TestCabAndEchoesDoNotShift(t *testing.T) {
	needFFmpeg(t)
	t.Parallel()
	in := filepath.Join(t.TempDir(), "click.wav")
	s := make([]float32, chSR*3)
	for i := 0; i < chSR*5/1000; i++ { // пачка 1 кГц 5 мс на 1,0 с
		s[chSR+i] = float32(0.8 * math.Sin(2*math.Pi*1000*float64(i)/chSR))
	}
	writeWav16(t, in, s)
	for _, id := range []string{"cab", "slapback", "megaphone"} {
		out := runChain(t, id, in, nil)
		peak := 0.0
		for _, v := range out {
			peak = math.Max(peak, math.Abs(float64(v)))
		}
		onset := -1.0
		for i, v := range out {
			if math.Abs(float64(v)) > 0.1*peak {
				onset = float64(i) / chSR
				break
			}
		}
		if math.Abs(onset-1) > 0.002 {
			t.Errorf("%s: звук начинается на %.4f с, want 1,0 ± 0,002 (сдвиг во времени)", id, onset)
		}
	}
	// хвост фильтра в конце допустим (≤ 15 мс), начало — на месте (проверено выше)
	if d := secs(runChain(t, "cab", in, nil)); d < 3-0.005 || d > 3.015 {
		t.Errorf("cab: длина выхода %.4f с, want 3…3,015", d)
	}
}
