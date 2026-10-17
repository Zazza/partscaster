package studio

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"partscaster/internal/dsp"
)

// Сумма дорожек без среза при пересборке — карточка internal-mix-headroom, условия 4–5, ТК5–ТК6.
// rebuildSections: сумма вставок и каждый шаг «эффекты на весь микс» — во float WAV; итоговый FLAC
// (вариант, загружаемый на воркер) — через dsp.FitPeak с потолком −0,1 дБFS: громче потолка — весь
// микс одной громкостью опускается до потолка, тише — звук прежний. Микс без мастера
// (overdub-premaster-*) — те же данные, что ушли вариантом до мастера.
//
// Синтетика — как в sections_mix_test.go (mxSetup): трек 6 с — тон 440 Гц 0.3, other = тот же тон.
// «Громче» дорожки other на +12 дБ в окне 1–5 с даёт сумму 0.3 + 0.3·(4 − 1) ≈ 1.19 (+1,5 дБFS).
//
// Пик и срез считаются по сэмплам загруженного файла как есть (родная частота, все каналы), не
// через dsp.PeakDb: проверка не зависит от новой функции.

// hmCeilingDb — потолок микса из карточки (mixCeilingDb пакета studio = −0,1 дБFS); здесь числом,
// чтобы тест собирался и до появления константы.
const hmCeilingDb = mixCeilingDb

// hmLoudOther — правка дорожки other: +db в окне [1, 5] с.
func hmLoudOther(db float64) SectionSpec {
	return SectionSpec{ChildID: 0, From: 1, To: 5, Stems: []string{"other"}, Db: db}
}

// hmRaw — сэмплы загруженных байтов как есть (каналы чередуются) и число каналов.
func hmRaw(t *testing.T, data []byte) ([]float32, int) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "uploaded.flac")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-i", p, "-f", "f32le", "-").Output()
	if err != nil {
		t.Fatalf("decode upload: %v", err)
	}
	s := make([]float32, len(raw)/4)
	for i := range s {
		s[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "a:0",
		"-show_entries", "stream=channels", "-of", "csv=p=0", p).Output()
	if err != nil {
		t.Fatalf("ffprobe upload: %v", err)
	}
	ch, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || ch < 1 {
		t.Fatalf("ffprobe channels %q: %v", out, err)
	}
	return s, ch
}

func hmPeakDb(s []float32) float64 {
	var m float64
	for _, v := range s {
		m = math.Max(m, math.Abs(float64(v)))
	}
	return 20 * math.Log10(m)
}

// hmClipRuns — мест, где в одном канале ≥ 3 сэмпла подряд на |x| ≥ 0.999.
func hmClipRuns(s []float32, ch int) int {
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

// hmUploaded — первая загрузка микса overdub-inst-0.flac.
func hmUploaded(t *testing.T, f *msFake) []byte {
	t.Helper()
	data, ok := f.first["overdub-inst-0.flac"]
	if !ok {
		t.Fatalf("микс overdub-inst-0.flac не загружен; загружены %v", keys(f.uploads))
	}
	return data
}

// hmCheckNoClip — пик варианта ≤ −0,05 дБFS и не ниже потолка больше чем на 0,15 дБ (опущен до
// потолка, а не глубже), нет срезанных вершин.
func hmCheckNoClip(t *testing.T, data []byte) {
	t.Helper()
	s, ch := hmRaw(t, data)
	p := hmPeakDb(s)
	if p > -0.05 {
		t.Errorf("пик варианта %.3f дБFS, want ≤ −0.05 (сумма выше 0 должна опуститься до потолка)", p)
	}
	if p < hmCeilingDb-0.15 {
		t.Errorf("пик варианта %.3f дБFS, want ≈ потолок %.1f (опущен до потолка, не глубже)", p, hmCeilingDb)
	}
	if n := hmClipRuns(s, ch); n > 0 {
		t.Errorf("в варианте %d мест с ≥ 3 сэмплами подряд на полной шкале, want 0", n)
	}
}

// ТК5. Сумма вставок выше 0 дБFS (other +12 → ≈ +1,5 дБFS) → вариант: пик ≤ −0,05, срезанных
// вершин нет; форма волны та же — одна громкость на весь микс: тон в окне громче тона вне окна
// на те же +12 дБ (±0,5), что и в сумме.
func TestRebuildSectionsLoudSumFitsCeiling(t *testing.T) {
	t.Parallel()
	f := mxSetup(t)
	if _, err := RebuildSections(context.Background(), f, parentID, []SectionSpec{hmLoudOther(12)}); err != nil {
		t.Fatalf("RebuildSections: %v", err)
	}
	data := hmUploaded(t, f)
	hmCheckNoClip(t, data)
	out := decodeBytes(t, data)
	in, outside := toneAmp(out, 440, 2, 4), toneAmp(out, 440, 5.5, 6)
	if d := db(in) - db(outside); math.Abs(d-12) > 0.5 {
		t.Errorf("тон в окне %+.2f дБ к тону вне окна, want +12 ± 0.5 (без ограничителя, одна громкость)", d)
	}
}

// Условие 4, «каждый шаг эффекты на весь микс — во float»: линия громкости +12 дБ на весь микс
// (0.3 → ≈ 1.19), затем «Громкость альбома» −12 дБ → тон ≈ 0.3 без искажений. Срез между шагами
// дал бы гармоники среза: 3-я гармоника (1320 Гц) должна быть ниже тона на 40 дБ и более.
func TestRebuildSectionsWholeMixStepsNoIntermediateClip(t *testing.T) {
	t.Parallel()
	f := mxSetup(t)
	up := SectionSpec{Mix: true, Stems: []string{"mix"},
		Envelope: []dsp.EnvPoint{{T: 0, Db: 12}, {T: mxDur, Db: 12}}}
	down := mxLevel(-12, -0.1)
	if _, err := RebuildSections(context.Background(), f, parentID, []SectionSpec{up, down}); err != nil {
		t.Fatalf("RebuildSections: %v", err)
	}
	data := hmUploaded(t, f)
	out := decodeBytes(t, data)
	fund, h3 := toneAmp(out, 440, 1, 5), toneAmp(out, 1320, 1, 5)
	if d := db(fund) - db(amp440); math.Abs(d) > 0.5 {
		t.Errorf("тон после +12 и −12 %+.2f дБ к исходному, want 0 ± 0.5", d)
	}
	if r := db(h3) - db(fund); r > -40 {
		t.Errorf("3-я гармоника %.1f дБ к тону, want ≤ −40 (промежуточный шаг срезан на 0 дБFS)", r)
	}
	s, ch := hmRaw(t, data)
	if n := hmClipRuns(s, ch); n > 0 {
		t.Errorf("в варианте %d мест с ≥ 3 сэмплами подряд на полной шкале, want 0", n)
	}
}

// Условие 4, мастер: громкая сумма + мастер → вариант до мастера без среза, а микс без мастера
// (overdub-premaster-0) — те же данные, что ушли этим вариантом.
func TestRebuildSectionsLoudSumPremasterSame(t *testing.T) {
	t.Parallel()
	f := mxSetup(t)
	if _, err := RebuildSections(context.Background(), f, parentID,
		[]SectionSpec{hmLoudOther(12), msMaster()}); err != nil {
		t.Fatalf("RebuildSections: %v", err)
	}
	hmCheckNoClip(t, hmUploaded(t, f))
	msCheckPremaster(t, f, "0")
}

// ТК6 / условие 5. Сумма ниже потолка (other +6 → 0.6, ≈ −4,4 дБFS) → громкость варианта как у
// суммы: пик совпадает с ожидаемым ±0,1 дБ, тон в окне и вне окна — как в сумме.
func TestRebuildSectionsQuietSumUnchanged(t *testing.T) {
	t.Parallel()
	f := mxSetup(t)
	if _, err := RebuildSections(context.Background(), f, parentID, []SectionSpec{hmLoudOther(6)}); err != nil {
		t.Fatalf("RebuildSections: %v", err)
	}
	data := hmUploaded(t, f)
	s, _ := hmRaw(t, data)
	want := db(2 * amp440)
	if p := hmPeakDb(s); math.Abs(p-want) > 0.1 {
		t.Errorf("пик варианта %.3f дБFS, want %.2f ± 0.1 (сумма ниже потолка — громкость прежняя)", p, want)
	}
	out := decodeBytes(t, data)
	if d := db(toneAmp(out, 440, 5.5, 6)) - db(amp440); math.Abs(d) > 0.2 {
		t.Errorf("тон вне окна %+.2f дБ к исходному, want 0 ± 0.2", d)
	}
}
