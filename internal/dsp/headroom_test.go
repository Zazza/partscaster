package dsp

import (
	"math"
	"path/filepath"
	"strconv"
	"testing"
)

// Пик файла и «опустить до потолка» — карточка internal-mix-headroom, условия 2–3, ТК1–ТК3.
//
// PeakDb(path) (float64, error): пик сэмплов в дБFS по всем каналам; у float WAV значения выше 0
// возвращаются как есть; тишина — очень малое число или −Inf, без ошибки.
// FitPeak(in, out, ceilingDb) (gainDb, err): пик входа выше потолка — весь файл одной громкостью
// опускается до пика ceilingDb (без ограничителя: форма волны та же), gainDb < 0; пик не выше —
// громкость не меняется, gainDb == 0. Выход — формат по расширению out.
//
// Хелперы hrGen/hrRaw/hrPeakDb/hrClipRuns — в headroom_run_test.go.
//
// Толкования (не записаны в карточке явно):
//   - нет файла — PeakDb возвращает ошибку (сигнатура с error, тишина — единственный оговорённый
//     случай «без ошибки»);
//   - «отношение выход/вход постоянно» проверяется на сэмплах с |вход| > 0.05 (ниже — шум квантования
//     FLAC даёт большую относительную ошибку), допуск 0,1 %.

// hrAmp — амплитуда синуса с пиком db дБFS (выражение для aevalsrc).
func hrAmp(db float64) float64 { return math.Pow(10, db/20) }

func hrSine(t *testing.T, name string, peakDb float64) string {
	t.Helper()
	expr := ffNumT(hrAmp(peakDb)) + "*sin(2*PI*440*t)"
	return hrGen(t, filepath.Join(t.TempDir(), name), expr, 2, "pcm_f32le")
}

func ffNumT(x float64) string { return strconv.FormatFloat(x, 'f', 6, 64) }

func hrPeak(t *testing.T, path string) float64 {
	t.Helper()
	p, err := PeakDb(path)
	if err != nil {
		t.Fatalf("PeakDb(%s): %v", filepath.Base(path), err)
	}
	return p
}

// ТК1. Float WAV синус с пиком +6 дБFS → PeakDb ≈ +6 (±0,1); с пиком −12 → ≈ −12.
func TestPeakDbFloatWav(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	for _, want := range []float64{6, -12} {
		in := hrSine(t, "in.wav", want)
		if got := hrPeak(t, in); math.Abs(got-want) > 0.1 {
			t.Errorf("пик синуса %+.0f дБFS: PeakDb = %.3f, want %+.0f ± 0.1", want, got, want)
		}
	}
}

// Условие 2, «по всем каналам»: левый −12, правый −3 → пик −3 (громкий канал не теряется,
// каналы не усредняются).
func TestPeakDbAllChannels(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	expr := ffNumT(hrAmp(-12)) + "*sin(2*PI*440*t)|" + ffNumT(hrAmp(-3)) + "*sin(2*PI*330*t)"
	in := hrGen(t, filepath.Join(t.TempDir(), "st.wav"), expr, 2, "pcm_f32le")
	if got := hrPeak(t, in); math.Abs(got-(-3)) > 0.1 {
		t.Errorf("стерео −12/−3: PeakDb = %.3f, want −3 ± 0.1", got)
	}
}

// Условие 2, тишина: без ошибки, очень малое число или −Inf (не NaN).
func TestPeakDbSilence(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	in := hrGen(t, filepath.Join(t.TempDir(), "zero.wav"), "0", 1, "pcm_f32le")
	got, err := PeakDb(in)
	if err != nil {
		t.Fatalf("PeakDb(тишина): %v, want без ошибки", err)
	}
	if math.IsNaN(got) || got > -90 {
		t.Errorf("PeakDb(тишина) = %v, want −Inf или очень малое число (≤ −90)", got)
	}
}

// Нет файла — ошибка (толкование, см. шапку).
func TestPeakDbMissingFile(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	if _, err := PeakDb(filepath.Join(t.TempDir(), "nope.wav")); err == nil {
		t.Error("PeakDb(нет файла): ошибка nil, want ошибка")
	}
}

// ТК2. FitPeak(вход +6, out.flac, −0,1) → gainDb ≈ −6,1; пик выхода ≈ −0,1; нет 3 сэмплов подряд
// на |x| ≥ 0,999; выход/вход по сэмплам постоянно ≈ 10^(gainDb/20); выход — FLAC.
func TestFitPeakLowersLoudFile(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	in := hrSine(t, "in.wav", 6)
	out := filepath.Join(t.TempDir(), "out.flac")
	gain, err := FitPeak(in, out, -0.1)
	if err != nil {
		t.Fatalf("FitPeak: %v", err)
	}
	if math.Abs(gain-(-6.1)) > 0.1 {
		t.Errorf("gainDb = %.3f, want −6.1 ± 0.1", gain)
	}
	if c := hrProbeStr(t, out, "codec_name"); c != "flac" {
		t.Errorf("кодек выхода out.flac = %q, want flac (формат по расширению)", c)
	}
	if p := hrPeak(t, out); math.Abs(p-(-0.1)) > 0.1 {
		t.Errorf("PeakDb(выход) = %.3f, want −0.1 ± 0.1", p)
	}
	a, _ := hrRaw(t, in)
	b, ch := hrRaw(t, out)
	if n := hrClipRuns(b, ch); n > 0 {
		t.Errorf("в выходе %d мест с ≥ 3 сэмплами подряд на |x| ≥ 0.999, want 0", n)
	}
	if len(a) != len(b) {
		t.Fatalf("длина выхода %d сэмплов, вход %d (форма волны должна сохраниться)", len(b), len(a))
	}
	k := math.Pow(10, gain/20)
	bad, worst := 0, 0.0
	for i := range a {
		if math.Abs(float64(a[i])) <= 0.05 {
			continue
		}
		r := float64(b[i]) / float64(a[i])
		if e := math.Abs(r/k - 1); e > 1e-3 {
			bad++
			worst = math.Max(worst, e)
		}
	}
	if bad > 0 {
		t.Errorf("отношение выход/вход не постоянно: %d сэмплов отличаются от 10^(gainDb/20)=%.4f, "+
			"худшее на %.2f %% (ограничитель/компрессор вместо одной громкости?)", bad, k, worst*100)
	}
}

// ТК3. FitPeak(вход −12, …, −0,1) → gainDb == 0, пик выхода ≈ −12, сэмплы те же.
func TestFitPeakKeepsQuietFile(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	in := hrSine(t, "in.wav", -12)
	out := filepath.Join(t.TempDir(), "out.flac")
	gain, err := FitPeak(in, out, -0.1)
	if err != nil {
		t.Fatalf("FitPeak: %v", err)
	}
	if gain != 0 {
		t.Errorf("gainDb = %v, want 0 (пик ниже потолка — громкость не меняется)", gain)
	}
	if p := hrPeak(t, out); math.Abs(p-(-12)) > 0.1 {
		t.Errorf("PeakDb(выход) = %.3f, want −12 ± 0.1", p)
	}
	a, _ := hrRaw(t, in)
	b, _ := hrRaw(t, out)
	if len(a) != len(b) {
		t.Fatalf("длина выхода %d сэмплов, вход %d", len(b), len(a))
	}
	for i := range a {
		if d := math.Abs(float64(b[i] - a[i])); d > 1e-5 {
			t.Fatalf("сэмпл %d: выход %.6f, вход %.6f — звук изменился", i, b[i], a[i])
		}
	}
}

// Условие 3, «формат по расширению outPath»: выход .wav — float; потолок выше 0 сохраняется
// (вход +6, потолок +3 → пик +3, gainDb ≈ −3).
func TestFitPeakWavOutKeepsFloat(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	in := hrSine(t, "in.wav", 6)
	out := filepath.Join(t.TempDir(), "out.wav")
	gain, err := FitPeak(in, out, 3)
	if err != nil {
		t.Fatalf("FitPeak: %v", err)
	}
	if math.Abs(gain-(-3)) > 0.1 {
		t.Errorf("gainDb = %.3f, want −3 ± 0.1", gain)
	}
	if p := hrPeak(t, out); math.Abs(p-3) > 0.1 {
		t.Errorf("PeakDb(выход .wav) = %.3f, want +3 ± 0.1 (float, без среза на 0)", p)
	}
}

// FitPeak на несуществующем входе — ошибка (толкование: тихо вернуть 0 нельзя — пересборка
// загрузила бы пустой или старый файл).
func TestFitPeakMissingInput(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "out.flac")
	if _, err := FitPeak(filepath.Join(dir, "nope.wav"), out, -0.1); err == nil {
		t.Error("FitPeak(нет входа): ошибка nil, want ошибка")
	}
}
