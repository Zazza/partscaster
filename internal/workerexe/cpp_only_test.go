package workerexe

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Карточка internal-cpp-only — Windows-установщик без движка PyTorch (partscaster-worker.exe).
//
// Тест-кейсы (ТК → условие карточки):
//   ТК14 (усл. 5)  пакет yue2-infer не ставится: нет в extraPackages, нет в исходниках пакета
//                  workerexe и программы cmd/partscaster-worker, нет константы YueCommit;
//                  torch и torchaudio в доп. пакетах остаются;
//   ТК15 (усл. 1, 5) WorkerEnv не задаёт YUE2_BACKEND ни на Windows, ни на других ОС;
//   ТК16 (усл. 4)  своих порогов видеопамяти у установщика Windows нет: функций DefaultCppModel и
//                  LowVramDefault нет, чисел-порогов 11776/9728/7680 в исходниках нет
//                  (рекомендацию даёт engine_cpp.py recommend — см. recommend_test.go);
//   ТК17 (усл. 5)  флага/выбора «движок» у partscaster-worker.exe нет: флаг engine не объявлен,
//                  settings["engine"] не записывается.
// Проверки по исходникам — там, где условие формулируется как «этого в коде нет».

// исходники (без тестов) каталога dir относительно пакета
func sources(t *testing.T, dir string) map[string]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out[f] = string(b)
	}
	if len(out) == 0 {
		t.Fatalf("нет исходников в %s", dir)
	}
	return out
}

func installerSources(t *testing.T) map[string]string {
	all := sources(t, ".")
	for k, v := range sources(t, "../../cmd/partscaster-worker") {
		all[k] = v
	}
	return all
}

// --- ТК14 ---

func TestExtraPackagesWithoutYue2Infer(t *testing.T) {
	var torch, torchaudio bool
	for _, p := range extraPackages {
		low := strings.ToLower(p)
		if strings.Contains(low, "yue2-infer") || strings.Contains(low, "yue2_infer") ||
			strings.Contains(low, "multimodal-art-projection/yue") {
			t.Errorf("доп. пакет движка PyTorch остался: %q", p)
		}
		if strings.HasPrefix(low, "torch==") {
			torch = true
		}
		if strings.HasPrefix(low, "torchaudio==") {
			torchaudio = true
		}
	}
	if !torch || !torchaudio {
		t.Errorf("torch/torchaudio нужны дорожкам, Whisper, Seed-VC, NAM — должны остаться: %q", extraPackages)
	}
}

func TestInstallerSourcesWithoutYue2Infer(t *testing.T) {
	for f, src := range installerSources(t) {
		for _, bad := range []string{"yue2-infer", "YueCommit", "torch-eager"} {
			if strings.Contains(src, bad) {
				t.Errorf("%s: остался %q (движок PyTorch)", f, bad)
			}
		}
	}
}

// --- ТК15 ---

func TestWorkerEnvWithoutYue2Backend(t *testing.T) {
	for _, goos := range []string{"windows", "linux", "darwin"} {
		env := WorkerEnv(t.TempDir(), goos)
		if v, ok := env["YUE2_BACKEND"]; ok {
			t.Errorf("WorkerEnv(%s): YUE2_BACKEND=%q — переменная движка PyTorch", goos, v)
		}
		if env["PARTSCASTER_DATA_DIR"] == "" {
			t.Errorf("WorkerEnv(%s): потерян PARTSCASTER_DATA_DIR: %v", goos, env)
		}
	}
}

// --- ТК16 ---

func TestNoOwnVramThresholds(t *testing.T) {
	funcs := regexp.MustCompile(`func\s+(DefaultCppModel|LowVramDefault)\s*\(`)
	nums := regexp.MustCompile(`\b(11776|9728|7680)\b`)
	for f, src := range installerSources(t) {
		if m := funcs.FindString(src); m != "" {
			t.Errorf("%s: %q — свои пороги установщика, рекомендация должна идти из engine_cpp.recommend", f, m)
		}
		if m := nums.FindString(src); m != "" {
			t.Errorf("%s: порог видеопамяти %s в коде установщика", f, m)
		}
	}
}

// --- ТК17 ---

func TestWorkerExeHasNoEngineChoice(t *testing.T) {
	flagRe := regexp.MustCompile(`flag\.\w+\(\s*"engine"`)
	setRe := regexp.MustCompile(`\["engine"\]\s*=`)
	for f, src := range sources(t, "../../cmd/partscaster-worker") {
		if flagRe.MatchString(src) {
			t.Errorf("%s: флаг --engine объявлен", f)
		}
		if setRe.MatchString(src) {
			t.Errorf("%s: в настройки пишется engine", f)
		}
	}
}
