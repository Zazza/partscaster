package studio

import (
	"context"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"partscaster/internal/dsp"
)

// Запись пересборки «на весь трек» (SectionSpec.Mix, json "mix") — карточка internal-studio-draft2,
// условие 1, тест-кейсы ТК1–ТК4.
// Mix: эффект на ВЕСЬ собранный микс — Chain+Params, либо Steps (педали по порядку), либо Envelope
// (линия громкости). Записи Mix ложатся на микс после правок дорожек и мест, до мастера, в порядке
// записей; Stems у них не учитываются. Ошибка до скачивания: Mix вместе с ChildID > 0, Engine, Add или
// Master; ни одного из Chain/Steps/Envelope; цепочка с ключом (ducking); неизвестная цепочка.
// Список только из записей Mix — пересборка работает (микс = оригинал + эффекты).
//
// Синтетика: трек 6 с — тон 440 Гц 0.3 (audio.flac); other = тот же тон, остальные дорожки — тишина.
// Цепочка «Громкость альбома» (level): сдвиг громкости gain дБ + ограничитель на потолке ceiling дБ.

const mxDur = 6.0

// mxSetup — родитель со всеми стемами: other = 440, остальное — тишина; воркер с мастером (msFake),
// чтобы случайный вызов ApplyFx не ронял тест паникой, а попадал в лог.
func mxSetup(t *testing.T) *msFake {
	t.Helper()
	needFFmpeg(t)
	dir := t.TempDir()
	f := newSecFake()
	put(f, parentID, map[string]string{
		"audio.flac":       lavfi(t, aeval(expr440, mxDur), filepath.Join(dir, "mx-audio.flac")),
		"stem-other.flac":  lavfi(t, aeval(expr440, mxDur), filepath.Join(dir, "mx-other.flac")),
		"stem-drums.flac":  lavfi(t, aeval("0", mxDur), filepath.Join(dir, "mx-drums.flac")),
		"stem-bass.flac":   lavfi(t, aeval("0", mxDur), filepath.Join(dir, "mx-bass.flac")),
		"stem-vocals.flac": lavfi(t, aeval("0", mxDur), filepath.Join(dir, "mx-vocals.flac")),
	})
	return &msFake{secFake: f}
}

// mxLevel — запись Mix «Громкость альбома»; дорожка "mix" — как её шлёт студия.
func mxLevel(gain, ceiling float64) SectionSpec {
	return SectionSpec{ChildID: 0, Mix: true, Stems: []string{"mix"}, Chain: "level",
		Params: map[string]float64{"gain": gain, "ceiling": ceiling}}
}

// mxMuteOther — правка дорожки: other заглушён в окне [from, to].
func mxMuteOther(from, to float64) SectionSpec {
	return SectionSpec{ChildID: 0, From: from, To: to, Stems: []string{"other"}, Db: -100}
}

func mxRun(t *testing.T, f *msFake, specs ...SectionSpec) []float32 {
	t.Helper()
	if _, err := RebuildSections(context.Background(), f, parentID, specs); err != nil {
		t.Fatalf("RebuildSections: %v", err)
	}
	return mxMix(t, f)
}

// mxMix — микс overdub-inst-0.flac в том виде, в каком он ушёл первой загрузкой.
func mxMix(t *testing.T, f *msFake) []float32 {
	t.Helper()
	data, ok := f.first["overdub-inst-0.flac"]
	if !ok {
		t.Fatalf("микс overdub-inst-0.flac не загружен; загружены %v", keys(f.uploads))
	}
	return decodeBytes(t, data)
}

func mxCheckLen(t *testing.T, out []float32) {
	t.Helper()
	if d := float64(len(out)) / sr; math.Abs(d-mxDur) > 0.05 {
		t.Errorf("длина микса %.3f с, want %.0f ± 0.05 (длительность прежняя)", d, mxDur)
	}
}

// ТК1. Mix с Chain — эффект на весь трек: микс ≠ микс без записи (440 тише на 12 дБ всюду, в т.ч.
// вне окна правки дорожки), длительность та же; правка дорожки при этом на месте.
func TestRebuildSectionsWholeMixChainChangesMix(t *testing.T) {
	t.Parallel()
	without := mxRun(t, mxSetup(t), mxMuteOther(1, 2))
	with := mxRun(t, mxSetup(t), mxMuteOther(1, 2), mxLevel(-12, -1.5))
	mxCheckLen(t, with)
	if d := float64(len(with)-len(without)) / sr; math.Abs(d) > 0.05 {
		t.Errorf("длина с записью Mix отличается от длины без неё на %.3f с", d)
	}
	for _, w := range [][2]float64{{0, 1}, {3, 4}, {4, 5}} {
		a, b := toneAmp(with, 440, w[0], w[1]), toneAmp(without, 440, w[0], w[1])
		if d := db(a) - db(b); math.Abs(d-(-12)) > 1 {
			t.Errorf("%.0f–%.0f с: 440 Гц с записью Mix %+.2f дБ к миксу без неё, want −12 ± 1", w[0], w[1], d)
		}
	}
	if a := toneAmp(with, 440, 1.2, 1.8); a > 0.01 {
		t.Errorf("в окне заглушения 440 Гц = %.4f, want тишина (правка дорожки потерялась)", a)
	}
}

// ТК1. Mix ложится ПОСЛЕ правок дорожек, даже если запись стоит в списке раньше. Если бы −6 дБ
// легло на трек до заглушения, вычитание исходной дорожки (0.3) из ослабленного трека (0.15)
// оставило бы в окне 440 Гц с амплитудой ≈ 0.15.
func TestRebuildSectionsWholeMixAfterStemEdits(t *testing.T) {
	t.Parallel()
	out := mxRun(t, mxSetup(t), mxLevel(-6, -1.5), mxMuteOther(1, 3))
	mxCheckLen(t, out)
	if a := toneAmp(out, 440, 1.5, 2.5); a > 0.02 {
		t.Errorf("в окне заглушения 440 Гц = %.4f, want < 0.02 (Mix должен лечь после правки дорожки)", a)
	}
	if d := db(toneAmp(out, 440, 4, 5)) - db(amp440); math.Abs(d-(-6)) > 1 {
		t.Errorf("вне окна 440 Гц %+.2f дБ к исходному, want −6 ± 1", d)
	}
}

// ТК1. Две записи Mix — по порядку записей. A = +12 дБ с потолком −3 дБ, B = −12 дБ:
// A→B — тон 0.3 упирается в потолок (≈ 0.71), затем ×0.25 → ≈ 0.18;
// B→A — 0.075, затем ×4 → 0.3, до потолка не доходит. Порядок различим на ≈ 4,5 дБ.
func TestRebuildSectionsWholeMixOrder(t *testing.T) {
	t.Parallel()
	a, b := mxLevel(12, -3), mxLevel(-12, -0.1)
	ab := toneAmp(mxRun(t, mxSetup(t), a, b), 440, 1, 5)
	ba := toneAmp(mxRun(t, mxSetup(t), b, a), 440, 1, 5)
	if ab > 0.23 {
		t.Errorf("A→B: 440 Гц = %.4f, want ≤ 0.23 (сначала +12 в потолок, потом −12)", ab)
	}
	if d := db(ba) - db(amp440); math.Abs(d) > 1 {
		t.Errorf("B→A: 440 Гц %+.2f дБ к исходному, want 0 ± 1 (−12, потом +12 без упора в потолок)", d)
	}
	if db(ba)-db(ab) < 3 {
		t.Errorf("порядок записей не влияет: A→B %.4f, B→A %.4f", ab, ba)
	}
}

// ТК1. Mix со Steps — педали по порядку на весь трек: два шага по −6 дБ = −12 дБ.
func TestRebuildSectionsWholeMixSteps(t *testing.T) {
	t.Parallel()
	steps := []dsp.Step{
		{Chain: "level", Params: map[string]float64{"gain": -6, "ceiling": -1.5}},
		{Chain: "level", Params: map[string]float64{"gain": -6, "ceiling": -1.5}},
	}
	out := mxRun(t, mxSetup(t), SectionSpec{Mix: true, Stems: []string{"mix"}, Steps: steps})
	mxCheckLen(t, out)
	for _, w := range [][2]float64{{0, 1}, {4, 5}} {
		if d := db(toneAmp(out, 440, w[0], w[1])) - db(amp440); math.Abs(d-(-12)) > 1 {
			t.Errorf("%.0f–%.0f с: 440 Гц %+.2f дБ, want −12 ± 1 (шаги педалей на весь трек)", w[0], w[1], d)
		}
	}
}

// ТК1. Mix с Envelope — линия громкости по всему треку (rampPts: 0 дБ до 1 с, −12 после 3 с).
func TestRebuildSectionsWholeMixEnvelope(t *testing.T) {
	t.Parallel()
	out := mxRun(t, mxSetup(t), SectionSpec{Mix: true, Stems: []string{"mix"}, Envelope: rampPts})
	assertRamp440(t, out)
}

// ТК1. Stems у записи Mix не учитываются: Stems ["vocals"] (голос — тишина) — эффект всё равно
// на весь микс.
func TestRebuildSectionsWholeMixIgnoresStems(t *testing.T) {
	t.Parallel()
	sp := mxLevel(-12, -1.5)
	sp.Stems = []string{"vocals"}
	out := mxRun(t, mxSetup(t), sp)
	if d := db(toneAmp(out, 440, 1, 5)) - db(amp440); math.Abs(d-(-12)) > 1 {
		t.Errorf("440 Гц %+.2f дБ, want −12 ± 1 (Mix на весь микс, Stems записи не учитываются)", d)
	}
}

// ТК1. Mix + мастер: мастер (ApplyFx на воркере) — после, на файле, который уже обработан Mix;
// место записи мастера в списке не важно. «Микс без мастера» — тот же звук, что ушёл на мастер.
func TestRebuildSectionsWholeMixBeforeMaster(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		specs []SectionSpec
	}{
		{"мастер после Mix", []SectionSpec{mxLevel(-12, -1.5), msMaster()}},
		{"мастер раньше Mix", []SectionSpec{msMaster(), mxLevel(-12, -1.5)}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := mxSetup(t)
			out := mxRun(t, f, c.specs...)
			if want := msWantLog("0"); !reflect.DeepEqual(f.log, want) {
				t.Errorf("порядок вызовов %v, want %v", f.log, want)
			}
			msCheckMasterCall(t, f, "overdub-inst-0.flac")
			msCheckPremaster(t, f, "0")
			// файл, который получил мастер, уже с Mix
			if d := db(toneAmp(out, 440, 1, 5)) - db(amp440); math.Abs(d-(-12)) > 1 {
				t.Errorf("микс под мастер: 440 Гц %+.2f дБ, want −12 ± 1 (Mix до мастера)", d)
			}
		})
	}
}

// ТК2. Ошибки записи Mix — до скачивания: фейк не отдаёт ни звук трека, ни стемы, ничего не загружено,
// воркер (мастер/движок) не вызван. Рядом обычная правка дорожки — ошибку даёт именно запись Mix.
func TestRebuildSectionsWholeMixValidation(t *testing.T) {
	t.Parallel()
	level := map[string]float64{"gain": -6, "ceiling": -1.5}
	for _, c := range []struct {
		name string
		spec SectionSpec
	}{
		{"Mix с вклейкой (ChildID > 0)", SectionSpec{Mix: true, ChildID: 7, Chain: "level", Params: level}},
		{"Mix с Engine", SectionSpec{Mix: true, Engine: msChain()}},
		{"Mix с Add", SectionSpec{Mix: true, Add: true, Chain: "level", Params: level}},
		{"Mix с Master", SectionSpec{Mix: true, Master: true, Chain: "level", Params: level}},
		{"Mix без Chain/Steps/Envelope", SectionSpec{Mix: true, Stems: []string{"mix"}}},
		{"Mix без Chain/Steps/Envelope и без дорожек", SectionSpec{Mix: true}},
		{"Mix с цепочкой с ключом (ducking)", SectionSpec{Mix: true, Stems: []string{"mix"}, Chain: "ducking"}},
		{"Mix с цепочкой с ключом (ducking), дорожка other", SectionSpec{Mix: true, Stems: []string{"other"}, Chain: "ducking"}},
		{"Mix с неизвестной цепочкой", SectionSpec{Mix: true, Stems: []string{"mix"}, Chain: "no-such-chain"}},
		{"Mix с неизвестной цепочкой, дорожка other", SectionSpec{Mix: true, Stems: []string{"other"}, Chain: "no-such-chain"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := mxSetup(t)
			_, err := RebuildSections(context.Background(), f, parentID, []SectionSpec{mxMuteOther(1, 2), c.spec})
			if err == nil {
				t.Fatal("want ошибку пересборки, got nil")
			}
			if len(f.fetched) != 0 {
				t.Errorf("до ошибки скачано %v, want ничего (проверка до скачивания)", f.fetched)
			}
			if len(f.stemsCalls) != 0 {
				t.Errorf("до ошибки запрошено разделение на дорожки: %v", f.stemsCalls)
			}
			if len(f.uploads) != 0 || len(f.log) != 0 {
				t.Errorf("при ошибке загрузки %v, вызовы %v — want ничего", keys(f.uploads), f.log)
			}
		})
	}
}

// ТК3. Список только из записей Mix — пересборка проходит без стемов: у трека их нет вовсе (только
// audio.flac), MakeStems не зовётся, стемы не скачиваются; микс = оригинал + эффекты по порядку.
func TestRebuildSectionsWholeMixOnlyNoStems(t *testing.T) {
	t.Parallel()
	f := envTrack(t) // только audio.flac: 440 Гц 0.3, 6 с
	res, err := RebuildSections(context.Background(), f, parentID, []SectionSpec{
		{Mix: true, Stems: []string{"mix"}, Envelope: rampPts},
		mxLevel(-6, -1.5),
	})
	if err != nil {
		t.Fatalf("RebuildSections только из записей Mix: %v", err)
	}
	if res == nil || res.Variant == nil {
		t.Fatalf("результат %+v, want вариант", res)
	}
	if len(f.stemsCalls) != 0 {
		t.Errorf("MakeStems вызван %v, want ни разу", f.stemsCalls)
	}
	for _, k := range f.fetched {
		if strings.Contains(k, "stem-") {
			t.Errorf("скачан стем %s, want только звук трека", k)
		}
	}
	out := secUploaded(t, f, "overdub-inst-0.flac")
	if d := float64(len(out)) / sr; math.Abs(d-6) > 0.05 {
		t.Errorf("длина %.3f с, want 6", d)
	}
	if d := db(toneAmp(out, 440, 0, 1)) - db(envAmp); math.Abs(d-(-6)) > 1 {
		t.Errorf("0–1 с: %+.2f дБ, want −6 ± 1 (линия 0 дБ + громкость −6)", d)
	}
	if d := db(toneAmp(out, 440, 4, 5)) - db(envAmp); math.Abs(d-(-18)) > 1 {
		t.Errorf("4–5 с: %+.2f дБ, want −18 ± 1 (линия −12 + громкость −6)", d)
	}
}

// ТК4. Подпись пересборки для записей Mix: «<название цепочки> · весь трек» / «<педали> · весь трек» /
// «линия громкости · весь трек»; дорожки записи в подпись не идут; с другими записями — через « + ».
func TestRebuildSectionsWholeMixLabel(t *testing.T) {
	t.Parallel()
	pedals := []dsp.Step{{Chain: "od-ts"}, {Chain: "reverb-hall"}}
	mute := SectionSpec{ChildID: 0, From: 5, To: 7, Stems: []string{"drums"}, Db: -100}
	for _, c := range []struct {
		name  string
		specs []SectionSpec
		want  string
	}{
		{"цепочка", []SectionSpec{mxLevel(-6, -1.5)}, "Громкость альбома · весь трек"},
		{"цепочка, дорожки записи не в подписи", []SectionSpec{{Mix: true, Stems: []string{"vocals"}, Chain: "level"}},
			"Громкость альбома · весь трек"},
		{"педали", []SectionSpec{{Mix: true, Stems: []string{"mix"}, Steps: pedals}},
			"Овердрайв (Tube Screamer) → Реверб: зал · весь трек"},
		{"линия громкости", []SectionSpec{{Mix: true, Stems: []string{"mix"}, Envelope: rampPts}},
			"линия громкости · весь трек"},
		{"с правкой дорожки", []SectionSpec{mute, mxLevel(-6, -1.5)},
			"заглушить · барабаны 0:05–0:07 + Громкость альбома · весь трек"},
	} {
		if got := rebuildLabel(c.specs); got != c.want {
			t.Errorf("%s: подпись %q, want %q", c.name, got, c.want)
		}
	}
}

// Условие 1: Stems у записи Mix не учитываются — трек без стемов (только audio.flac), запись Mix с
// дорожкой vocals или guitar: пересборка проходит, дорожки не скачиваются, разделение не запускается.
func TestRebuildSectionsWholeMixStemsNotFetched(t *testing.T) {
	t.Parallel()
	for _, stem := range []string{"vocals", "guitar"} {
		t.Run(stem, func(t *testing.T) {
			t.Parallel()
			f := envTrack(t) // только audio.flac: 440 Гц 0.3, 6 с
			sp := SectionSpec{Mix: true, Chain: "level", Stems: []string{stem},
				Params: map[string]float64{"gain": -6, "ceiling": -1.5}}
			if _, err := RebuildSections(context.Background(), f, parentID, []SectionSpec{sp}); err != nil {
				t.Fatalf("RebuildSections: %v", err)
			}
			if len(f.stemsCalls) != 0 {
				t.Errorf("MakeStems вызван %v, want ни разу", f.stemsCalls)
			}
			for _, k := range f.fetched {
				if strings.Contains(k, "stem-") {
					t.Errorf("скачана дорожка %s, want только звук трека", k)
				}
			}
			out := secUploaded(t, f, "overdub-inst-0.flac")
			if d := db(toneAmp(out, 440, 1, 5)) - db(envAmp); math.Abs(d-(-6)) > 1 {
				t.Errorf("440 Гц %+.2f дБ, want −6 ± 1 (эффект на весь трек)", d)
			}
		})
	}
}
