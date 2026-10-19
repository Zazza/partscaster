package workerexe

import (
	"path/filepath"
	"strings"
	"testing"
)

// Карточка internal-no-demucs — установщик Windows (partscaster-worker.exe), окружение разделения RoFormer.
//
// Тест-кейсы (ТК → условие карточки):
//   ТК27 (усл. 6)  WorkerEnv задаёт PARTSCASTER_SEP_PY = <home>/sep-venv/Scripts/python.exe (Windows),
//                  <home>/sep-venv/bin/python (прочие ОС) и PARTSCASTER_SEP_MODELS = <home>/sep-models;
//   ТК28 (усл. 1, 6) в исходниках пакета workerexe и программы cmd/partscaster-worker нет «demucs»;
//   ТК29 (усл. 6)  запуск partscaster-worker.exe ставит разделение: в программе вызываются NeedSepInstall,
//                  InstallSep и SepDownloadCommand (проверка по исходнику — запуск main в тесте невозможен
//                  без сети и видеокарты).
// ТК30–ТК33 (новые функции SepPackages, NeedSepInstall, InstallSep, SepDownloadCommand) — в
// sep_install_test.go: в красной фазе функций нет, отдельный файл не мешает собираться остальным тестам.

// --- ТК27 ---

func TestNoDemucsWorkerEnvSepPaths(t *testing.T) {
	home := filepath.Join("base", "partscaster-worker")
	for goos, py := range map[string]string{
		"windows": filepath.Join(home, "sep-venv", "Scripts", "python.exe"),
		"linux":   filepath.Join(home, "sep-venv", "bin", "python"),
	} {
		env := WorkerEnv(home, goos)
		if got := env["PARTSCASTER_SEP_PY"]; got != py {
			t.Errorf("%s: PARTSCASTER_SEP_PY=%q, ждали %q", goos, got, py)
		}
		if got, want := env["PARTSCASTER_SEP_MODELS"], filepath.Join(home, "sep-models"); got != want {
			t.Errorf("%s: PARTSCASTER_SEP_MODELS=%q, ждали %q", goos, got, want)
		}
	}
}

// --- ТК28 ---

func TestNoDemucsInstallerSources(t *testing.T) {
	for f, src := range installerSources(t) {
		if strings.Contains(strings.ToLower(src), "demucs") {
			t.Errorf("%s: упоминается demucs", f)
		}
	}
}

// --- ТК29 ---

func TestNoDemucsWorkerMainInstallsSep(t *testing.T) {
	srcs := sources(t, "../../cmd/partscaster-worker")
	all := ""
	for _, s := range srcs {
		all += s
	}
	for _, fn := range []string{"workerexe.NeedSepInstall(", "workerexe.InstallSep(", "workerexe.SepDownloadCommand("} {
		if !strings.Contains(all, fn) {
			t.Errorf("partscaster-worker не вызывает %s…) — окружение разделения не ставится при запуске", fn)
		}
	}
}
