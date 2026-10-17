package dsp

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// outCodec — кодек выхода по расширению: .wav — 32-бит float, сумма дорожек выше 0 дБFS не срезается
// (по умолчанию ffmpeg пишет WAV 16-бит и режет её, как FLAC); прочие — как выбирает ffmpeg.
func outCodec(outPath string) []string {
	if strings.EqualFold(filepath.Ext(outPath), ".wav") {
		return []string{"-c:a", "pcm_f32le"}
	}
	return nil
}

var peakRe = regexp.MustCompile(`Peak level dB:\s*(-?inf|[-+0-9.]+)`)

// PeakDb — пик сэмплов файла в дБFS по всем каналам (ffmpeg astats); у float WAV выше 0 — как есть,
// тишина — -Inf.
func PeakDb(path string) (float64, error) {
	if _, err := os.Stat(path); err != nil {
		return 0, err
	}
	cmd := ffmpegCmd([]string{"-hide_banner", "-nostats", "-i", path,
		"-af", "astats=measure_perchannel=none:measure_overall=Peak_level", "-f", "null", "-"})
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("ffmpeg: %w: %s", err, stderr.String())
	}
	m := peakRe.FindStringSubmatch(stderr.String())
	if m == nil {
		return 0, fmt.Errorf("ffmpeg astats: нет пика в выводе")
	}
	if strings.HasSuffix(m[1], "inf") {
		return math.Inf(-1), nil
	}
	return strconv.ParseFloat(m[1], 64)
}

// FitPeak — файл в outPath (формат по расширению) с пиком не выше ceilingDb: громче — весь файл одной
// громкостью опускается до потолка (форма волны та же, без ограничителя); gainDb — на сколько опущено, 0 — не
// менялся.
func FitPeak(inPath, outPath string, ceilingDb float64) (gainDb float64, err error) {
	peak, err := PeakDb(inPath)
	if err != nil {
		return 0, err
	}
	if peak > ceilingDb {
		gainDb = ceilingDb - peak
	}
	graph := fmt.Sprintf("[0:a]volume=%.6fdB:precision=double[out]", gainDb)
	if err := Run(inPath, outPath, graph, nil); err != nil {
		return 0, err
	}
	return gainDb, nil
}
