package dsp

import (
	"math"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Место дорожки в стерео — матрица 2×2.
// Порядок коэффициентов: [a, b, c, d] — L' = a·L + b·R, R' = c·L + d·R.

// pmApply — применить матрицу к паре отсчётов.
func pmApply(m [4]float64, l, r float64) (float64, float64) {
	return m[0]*l + m[1]*r, m[2]*l + m[3]*r
}

// pmSpec — эталонная матрица: ширина M/S, затем панорама складыванием канала.
func pmSpec(pan, width float64) [4]float64 {
	w := [4]float64{(1 + width) / 2, (1 - width) / 2, (1 - width) / 2, (1 + width) / 2}
	th := math.Abs(pan) * math.Pi / 2
	g := 1 / math.Sqrt(1+math.Sin(th))
	var p [4]float64
	if pan >= 0 {
		p = [4]float64{g * math.Cos(th), 0, g * math.Sin(th), g}
	} else {
		p = [4]float64{g, g * math.Sin(th), 0, g * math.Cos(th)}
	}
	// P·W
	return [4]float64{
		p[0]*w[0] + p[1]*w[2], p[0]*w[1] + p[1]*w[3],
		p[2]*w[0] + p[3]*w[2], p[2]*w[1] + p[3]*w[3],
	}
}

func pmMust(t *testing.T, pan, width float64) [4]float64 {
	t.Helper()
	m, err := PlaceMatrix(pan, width)
	if err != nil {
		t.Fatalf("PlaceMatrix(%v, %v): %v", pan, width, err)
	}
	return m
}

func pmClose(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %.6f, want %.6f ± %g", what, got, want, tol)
	}
}

// pan 0, width 1 — ровно единичная матрица.
func TestPlaceMatrixIdentity(t *testing.T) {
	t.Parallel()
	m := pmMust(t, 0, 1)
	if m != [4]float64{1, 0, 0, 1} {
		t.Errorf("PlaceMatrix(0, 1) = %v, want ровно [1 0 0 1]", m)
	}
}

// (1, 1) на моно-входе L = R = s — левый 0, правый s·√2, мощность та же ±0,1 дБ.
func TestPlaceMatrixHardRightMono(t *testing.T) {
	t.Parallel()
	m := pmMust(t, 1, 1)
	s := 0.5
	l, r := pmApply(m, s, s)
	pmClose(t, "L' при pan 1", l, 0, 1e-9)
	pmClose(t, "R' при pan 1", r, s*math.Sqrt2, 1e-9)
	pIn, pOut := 2*s*s, l*l+r*r
	if d := 10 * math.Log10(pOut/pIn); math.Abs(d) > 0.1 {
		t.Errorf("мощность изменилась на %.3f дБ, want 0 ± 0,1", d)
	}
}

// (−1, 1) — зеркально: правый 0, левый s·√2.
func TestPlaceMatrixHardLeftMono(t *testing.T) {
	t.Parallel()
	m := pmMust(t, -1, 1)
	s := 0.5
	l, r := pmApply(m, s, s)
	pmClose(t, "R' при pan −1", r, 0, 1e-9)
	pmClose(t, "L' при pan −1", l, s*math.Sqrt2, 1e-9)
}

// (0, 0) — моно: L' = R' = (L+R)/2.
func TestPlaceMatrixZeroWidthIsMono(t *testing.T) {
	t.Parallel()
	m := pmMust(t, 0, 0)
	for _, in := range [][2]float64{{0.7, -0.1}, {0.3, 0.3}, {0, 1}} {
		l, r := pmApply(m, in[0], in[1])
		mid := (in[0] + in[1]) / 2
		pmClose(t, "L' при width 0", l, mid, 1e-9)
		pmClose(t, "R' при width 0", r, mid, 1e-9)
	}
}

// (0, 2) на чистом «боке» L = s, R = −s — бока ×2; середина не меняется.
func TestPlaceMatrixDoubleWidthSide(t *testing.T) {
	t.Parallel()
	m := pmMust(t, 0, 2)
	s := 0.25
	l, r := pmApply(m, s, -s)
	pmClose(t, "L' бока при width 2", l, 2*s, 1e-9)
	pmClose(t, "R' бока при width 2", r, -2*s, 1e-9)
	l, r = pmApply(m, s, s)
	pmClose(t, "L' середины при width 2", l, s, 1e-9)
	pmClose(t, "R' середины при width 2", r, s, 1e-9)
}

// Матрица — ширина M/S, затем панорама складыванием канала —
// на промежуточных значениях обоих знаков.
func TestPlaceMatrixFormula(t *testing.T) {
	t.Parallel()
	for _, c := range [][2]float64{{0.3, 1}, {-0.3, 1}, {0.5, 1.4}, {-0.7, 0.6}, {1, 2}, {-1, 0}, {0.2, 1.5}} {
		got := pmMust(t, c[0], c[1])
		want := pmSpec(c[0], c[1])
		for i := range got {
			if math.Abs(got[i]-want[i]) > 1e-9 {
				t.Errorf("PlaceMatrix(%v, %v) = %v, want %v", c[0], c[1], got, want)
				break
			}
		}
	}
}

// Пределы pan −1…1, width 0…2 — вне ошибка, на границах — нет.
func TestPlaceMatrixLimits(t *testing.T) {
	t.Parallel()
	for _, c := range [][2]float64{{1.5, 1}, {-1.5, 1}, {0, -1}, {0, 2.5}, {math.NaN(), 1}, {0, math.NaN()}} {
		if _, err := PlaceMatrix(c[0], c[1]); err == nil {
			t.Errorf("PlaceMatrix(%v, %v): want ошибку (вне пределов)", c[0], c[1])
		}
	}
	for _, c := range [][2]float64{{-1, 0}, {1, 2}, {-1, 2}, {1, 0}} {
		if _, err := PlaceMatrix(c[0], c[1]); err != nil {
			t.Errorf("PlaceMatrix(%v, %v) на границе: %v", c[0], c[1], err)
		}
	}
}

// Вставка с Matrix (0,5 вправо) — в графе есть фильтр pan=stereo; и у обычной вставки,
// и у «на месте». Коэффициенты проверяются прогоном ffmpeg (ниже), не форматом строки.
func TestInsertsGraphMatrixPan(t *testing.T) {
	t.Parallel()
	m := pmMust(t, 0.5, 1)
	for name, in := range map[string]Insert{
		"обычная":  {AtSec: 2, SkipSec: 0.5, Gain: 0.7, Matrix: &m},
		"на месте": {AtSec: 1.5, SkipSec: 1.5, DurSec: 2, Gain: -1, Matrix: &m},
	} {
		if g := InsertsGraph([]Insert{in}); !strings.Contains(g, "pan=stereo") {
			t.Errorf("%s: нет pan=stereo в графе: %s", name, g)
		}
	}
}

const pmSR = 16000

// pmTone — амплитуда синуса hz на отсчётах [from,to) с.
func pmTone(s []float64, hz, from, to float64) float64 {
	a, b := int(from*pmSR), min(int(to*pmSR), len(s))
	if a >= b {
		return 0
	}
	w := 2 * math.Pi * hz / pmSR
	var re, im float64
	for i, v := range s[a:b] {
		re += v * math.Cos(w*float64(i))
		im -= v * math.Sin(w*float64(i))
	}
	return 2 * math.Hypot(re, im) / float64(b-a)
}

// pmGen — файл float32 wav из выражений aevalsrc (одно — моно, «a|b» — стерео).
func pmGen(t *testing.T, path, exprs string, dur float64) {
	t.Helper()
	src := "aevalsrc=exprs='" + exprs + "':d=" + strconv.FormatFloat(dur, 'f', 3, 64) + ":s=" + strconv.Itoa(pmSR)
	out, err := exec.Command("ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", src, "-c:a", "pcm_f32le", path).CombinedOutput()
	if err != nil {
		t.Fatalf("gen %q: %v %s", src, err, out)
	}
}

// pmRun — база: стерео тишина 4 с; вставка party по in; результат — два канала.
func pmRun(t *testing.T, partyExprs string, in Insert) [2][]float64 {
	t.Helper()
	needFFmpeg(t)
	dir := t.TempDir()
	base := filepath.Join(dir, "base.wav")
	party := filepath.Join(dir, "party.wav")
	out := filepath.Join(dir, "out.wav")
	pmGen(t, base, "0|0", 4)
	pmGen(t, party, partyExprs, 4)
	if err := RunInputs([]string{base, party}, out, InsertsGraph([]Insert{in})); err != nil {
		t.Fatalf("run: %v", err)
	}
	return ipDecodeStereo(t, out, pmSR)
}

func pmDb(x float64) float64 { return 20 * math.Log10(max(x, 1e-12)) }

// pmCheckTone — амплитуда тона в канале: want > 0 — совпадение ±0,1 дБ; want = 0 — ниже ref на 60 дБ.
func pmCheckTone(t *testing.T, what string, got, want, ref float64) {
	t.Helper()
	if want == 0 {
		if pmDb(got)-pmDb(ref) > -60 {
			t.Errorf("%s: амплитуда %.5f, want тишина (≤ −60 дБ от %.3f)", what, got, ref)
		}
		return
	}
	if d := pmDb(got) - pmDb(want); math.Abs(d) > 0.1 {
		t.Errorf("%s: амплитуда %.5f, want %.5f ± 0,1 дБ (расхождение %.3f дБ)", what, got, want, d)
	}
}

// Стерео-тон (L — 440 Гц 0,3; R — 660 Гц 0,2) через вставку с матрицей: каждый
// выходной канал = a·L + b·R / c·L + d·R ±0,1 дБ; громкость (Gain) — до матрицы.
func TestRunInputsMatrixStereoTone(t *testing.T) {
	t.Parallel()
	const aL, aR = 0.3, 0.2
	for _, c := range []struct {
		name       string
		pan, width float64
		in         Insert
		from, to   float64
	}{
		{"обычная, 0,5 вправо", 0.5, 1, Insert{AtSec: 1, SkipSec: 0, Gain: 1}, 1.5, 3.5},
		{"на месте, 0,3 влево, ширина 1,5", -0.3, 1.5, Insert{Gain: 0.5}, 0.5, 3.5},
	} {
		m := pmMust(t, c.pan, c.width)
		in := c.in
		in.Matrix = &m
		ch := pmRun(t, "0.3*sin(2*PI*440*t)|0.2*sin(2*PI*660*t)", in)
		g := math.Abs(in.Gain)
		pmCheckTone(t, c.name+": L' от L (440)", pmTone(ch[0], 440, c.from, c.to), g*math.Abs(m[0])*aL, aL)
		pmCheckTone(t, c.name+": L' от R (660)", pmTone(ch[0], 660, c.from, c.to), g*math.Abs(m[1])*aR, aR)
		pmCheckTone(t, c.name+": R' от L (440)", pmTone(ch[1], 440, c.from, c.to), g*math.Abs(m[2])*aL, aL)
		pmCheckTone(t, c.name+": R' от R (660)", pmTone(ch[1], 660, c.from, c.to), g*math.Abs(m[3])*aR, aR)
	}
}

// Моно-вход сначала в стерео копией в оба канала
// без ослабления (L = R = s), затем матрица: pan 0, width 1 — L = R = s; pan 1 — слева тишина,
// справа s·√2; pan −1 — зеркально.
func TestRunInputsMatrixMonoInput(t *testing.T) {
	t.Parallel()
	const s = 0.3
	const party = "0.3*sin(2*PI*440*t)"
	id := pmMust(t, 0, 1)
	ch := pmRun(t, party, Insert{Gain: 1, Matrix: &id})
	pmCheckTone(t, "pan 0: L", pmTone(ch[0], 440, 0.5, 3.5), s, s)
	pmCheckTone(t, "pan 0: R", pmTone(ch[1], 440, 0.5, 3.5), s, s)
	for _, c := range []struct {
		pan        float64
		silent, on int
	}{{1, 0, 1}, {-1, 1, 0}} {
		m := pmMust(t, c.pan, 1)
		ch := pmRun(t, party, Insert{Gain: 1, Matrix: &m})
		name := "pan " + strconv.FormatFloat(c.pan, 'f', -1, 64)
		pmCheckTone(t, name+": заглушённый канал", pmTone(ch[c.silent], 440, 0.5, 3.5), 0, s)
		pmCheckTone(t, name+": канал панорамы", pmTone(ch[c.on], 440, 0.5, 3.5), s*math.Sqrt2, s)
	}
}
