package dsp

import (
	"encoding/binary"
	"math"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Запас над 0 дБFS при записи — карточка internal-mix-headroom, условие 1, ТК4.
// Run и RunInputs: выход .wav — 32-бит float (pcm_f32le), значения выше 0 дБFS не срезаются;
// прочие расширения — как раньше (FLAC режет на полной шкале).
//
// Пик здесь считается напрямую по сэмплам (ffmpeg → f32le без пересэмплирования), а не через
// dsp.PeakDb: проверка не зависит от новой функции и показывает сам факт среза.

const hrSR = 48000

// hrGen — файл из выражений aevalsrc (одно — моно, «a|b» — стерео) с кодеком codec
// (pcm_f32le для float WAV; "" — кодек по расширению).
func hrGen(t *testing.T, path, exprs string, dur float64, codec string) string {
	t.Helper()
	src := "aevalsrc=exprs='" + exprs + "':d=" + strconv.FormatFloat(dur, 'f', 3, 64) + ":s=" + strconv.Itoa(hrSR)
	args := []string{"-y", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", src}
	if codec != "" {
		args = append(args, "-c:a", codec)
	}
	out, err := exec.Command("ffmpeg", append(args, path)...).CombinedOutput()
	if err != nil {
		t.Fatalf("gen %q: %v %s", src, err, out)
	}
	return path
}

// hrRaw — сэмплы файла как есть: родная частота, каналы чередуются; ch — число каналов.
func hrRaw(t *testing.T, path string) (s []float32, ch int) {
	t.Helper()
	raw, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-i", path, "-f", "f32le", "-").Output()
	if err != nil {
		t.Fatalf("decode %s: %v", filepath.Base(path), err)
	}
	s = make([]float32, len(raw)/4)
	for i := range s {
		s[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	return s, hrProbe(t, path, "channels")
}

// hrProbeStr — поле первого аудиопотока по ffprobe (codec_name, channels, …).
func hrProbeStr(t *testing.T, path, field string) string {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "a:0",
		"-show_entries", "stream="+field, "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", filepath.Base(path), err)
	}
	return strings.TrimSpace(string(out))
}

func hrProbe(t *testing.T, path, field string) int {
	t.Helper()
	n, err := strconv.Atoi(hrProbeStr(t, path, field))
	if err != nil {
		t.Fatalf("ffprobe %s %s: %v", filepath.Base(path), field, err)
	}
	return n
}

// hrPeakDb — пик |сэмпла| по всем каналам, дБFS.
func hrPeakDb(s []float32) float64 {
	var m float64
	for _, v := range s {
		m = math.Max(m, math.Abs(float64(v)))
	}
	return 20 * math.Log10(m)
}

// hrClipRuns — сколько мест, где в одном канале ≥ 3 сэмпла подряд с |x| ≥ 0.999 (срезанные вершины).
func hrClipRuns(s []float32, ch int) int {
	runs := 0
	for c := 0; c < ch; c++ {
		n := 0
		for i := c; i < len(s); i += ch {
			if math.Abs(float64(s[i])) >= 0.999 {
				n++
				if n == 3 {
					runs++
				}
			} else {
				n = 0
			}
		}
	}
	return runs
}

// ТК4. RunInputs с выходом .wav и графом volume=+24 дБ: синус −6 дБFS → пик ≈ +18 дБFS, не 0;
// выход — pcm_f32le.
func TestRunInputsWavKeepsAboveFullScale(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	dir := t.TempDir()
	in := hrGen(t, filepath.Join(dir, "in.flac"), "0.5*sin(2*PI*440*t)", 1, "")
	out := filepath.Join(dir, "out.wav")
	if err := RunInputs([]string{in}, out, "[0:a]volume=24dB[out]"); err != nil {
		t.Fatalf("RunInputs: %v", err)
	}
	if c := hrProbeStr(t, out, "codec_name"); c != "pcm_f32le" {
		t.Errorf("кодек выхода .wav = %q, want pcm_f32le", c)
	}
	s, _ := hrRaw(t, out)
	if p := hrPeakDb(s); p <= 0 {
		t.Errorf("пик выхода %.2f дБFS, want > 0 (≈ +18: значения выше полной шкалы не срезаются)", p)
	} else if math.Abs(p-18) > 0.2 {
		t.Errorf("пик выхода %.2f дБFS, want ≈ +18 ± 0.2 (−6 + 24)", p)
	}
}

// Условие 1. То же для Run: выход .wav — float, +24 дБ на синусе −6 дБFS → ≈ +18 дБFS.
func TestRunWavKeepsAboveFullScale(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	dir := t.TempDir()
	in := hrGen(t, filepath.Join(dir, "in.flac"), "0.5*sin(2*PI*440*t)", 1, "")
	out := filepath.Join(dir, "out.wav")
	if err := Run(in, out, "[0:a]volume=24dB[out]", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if c := hrProbeStr(t, out, "codec_name"); c != "pcm_f32le" {
		t.Errorf("кодек выхода .wav = %q, want pcm_f32le", c)
	}
	s, _ := hrRaw(t, out)
	if p := hrPeakDb(s); math.Abs(p-18) > 0.2 {
		t.Errorf("пик выхода %.2f дБFS, want ≈ +18 ± 0.2 (не срезано)", p)
	}
}

// Условие 1, «прочие расширения — как раньше»: выход .flac остаётся FLAC и, как раньше,
// не выше полной шкалы.
func TestRunFlacUnchanged(t *testing.T) {
	t.Parallel()
	needFFmpeg(t)
	dir := t.TempDir()
	in := hrGen(t, filepath.Join(dir, "in.flac"), "0.5*sin(2*PI*440*t)", 1, "")
	for _, run := range []struct {
		name string
		do   func(out string) error
	}{
		{"Run", func(out string) error { return Run(in, out, "[0:a]volume=24dB[out]", nil) }},
		{"RunInputs", func(out string) error { return RunInputs([]string{in}, out, "[0:a]volume=24dB[out]") }},
	} {
		out := filepath.Join(dir, run.name+".flac")
		if err := run.do(out); err != nil {
			t.Fatalf("%s: %v", run.name, err)
		}
		if c := hrProbeStr(t, out, "codec_name"); c != "flac" {
			t.Errorf("%s: кодек выхода .flac = %q, want flac", run.name, c)
		}
		s, _ := hrRaw(t, out)
		if p := hrPeakDb(s); p > 0.001 {
			t.Errorf("%s: пик .flac %.3f дБFS, want ≤ 0 (как раньше)", run.name, p)
		}
	}
}
