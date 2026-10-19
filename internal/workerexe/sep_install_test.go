package workerexe

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Карточка internal-no-demucs, усл. 6 — окружение разделения RoFormer на Windows.
//
// Тест-кейсы (ТК → условие карточки):
//   ТК30 (усл. 5, 6) SepPackages закреплены: audio-separator[gpu]==0.47.0 и audioread;
//   ТК31 (усл. 6)  InstallSep(home, uv, out): uv venv в <home>/sep-venv; uv pip install --python <python
//                  sep-venv> с SepPackages и индексом cu128 (--torch-backend cu128, как у основного окружения);
//                  после успеха NeedSepInstall=false;
//   ТК32 (усл. 6)  NeedSepInstall: свежая папка → true; отпечаток свой (основной MarkInstalled его не
//                  ставит, InstallSep не ставит основной); набор пакетов изменился → true; сбой uv pip →
//                  ошибка, NeedSepInstall остаётся true (повторный запуск докачает);
//   ТК33 (усл. 5, 6) SepDownloadCommand(home, workerDir): <python sep-venv> sep_run.py download <home>/sep-models,
//                  Dir = каталог кода воркера.
// Внешняя граница — программа uv: вместо неё скрипт-заглушка (пишет аргументы в журнал, `venv` создаёт
// python), без сети. Отдельный файл: в красной фазе функций ещё нет.

// sepPython — python окружения разделения на этой ОС (как WorkerEnv/VenvPython).
func sepPython(home string) string {
	if goosHere == "windows" {
		return filepath.Join(home, "sep-venv", "Scripts", "python.exe")
	}
	return filepath.Join(home, "sep-venv", "bin", "python")
}

// fakeUv — заглушка uv: каждый вызов — строки «==», затем аргументы по одному; `venv <каталог>` создаёт
// <каталог>/bin/python; `pip` падает, если рядом есть файл fail-pip. Только встроенные команды sh и /bin/mkdir:
// окружение запуска задаёт установщик.
func fakeUv(t *testing.T) (uv, log string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("заглушка uv — sh-скрипт")
	}
	dir := t.TempDir()
	log = filepath.Join(dir, "uv.log")
	uv = filepath.Join(dir, "uv")
	script := `#!/bin/sh
{ echo "=="; for a in "$@"; do printf '%s\n' "$a"; done; } >> '` + log + `'
if [ "$1" = venv ]; then
  shift; d=""; skip=0
  for a in "$@"; do
    if [ "$skip" = 1 ]; then skip=0; continue; fi
    case "$a" in --python|-p) skip=1;; -*) ;; *) [ -z "$d" ] && d="$a";; esac
  done
  /bin/mkdir -p "$d/bin" "$d/Scripts"
  : > "$d/bin/python"; : > "$d/Scripts/python.exe"
fi
if [ "$1" = pip ] && [ -f '` + filepath.Join(dir, "fail-pip") + `' ]; then exit 1; fi
exit 0
`
	if err := os.WriteFile(uv, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return uv, log
}

func uvCalls(t *testing.T, log string) [][]string {
	t.Helper()
	b, err := os.ReadFile(log)
	if err != nil {
		return nil
	}
	var out [][]string
	for _, ln := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if ln == "==" {
			out = append(out, []string{})
		} else if len(out) > 0 {
			out[len(out)-1] = append(out[len(out)-1], ln)
		}
	}
	return out
}

func hasArgs(args []string, seq ...string) bool {
	for i := 0; i+len(seq) <= len(args); i++ {
		ok := true
		for j, s := range seq {
			if args[i+j] != s {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// --- ТК30 ---

func TestSepPackagesPinned(t *testing.T) {
	for _, want := range []string{"audio-separator[gpu]==0.47.0", "audioread"} {
		found := false
		for _, p := range SepPackages {
			if p == want {
				found = true
			}
		}
		if !found {
			t.Errorf("SepPackages = %q, нет %q", SepPackages, want)
		}
	}
}

// --- ТК31 ---

func TestInstallSepVenvAndPip(t *testing.T) {
	home := t.TempDir()
	uv, log := fakeUv(t)
	var out bytes.Buffer
	if err := InstallSep(home, uv, &out); err != nil {
		t.Fatalf("InstallSep: %v\n%s", err, out.String())
	}
	var venv, pip []string
	for _, c := range uvCalls(t, log) {
		switch {
		case len(c) > 0 && c[0] == "venv":
			venv = c
		case len(c) > 1 && c[0] == "pip" && c[1] == "install":
			pip = c
		}
	}
	if venv == nil || !hasArgs(venv, filepath.Join(home, "sep-venv")) {
		t.Errorf("uv venv в %s не вызван: %q", filepath.Join(home, "sep-venv"), uvCalls(t, log))
	}
	if pip == nil {
		t.Fatalf("uv pip install не вызван: %q", uvCalls(t, log))
	}
	if !hasArgs(pip, "--python", sepPython(home)) {
		t.Errorf("uv pip без --python %s: %q", sepPython(home), pip)
	}
	for _, p := range SepPackages {
		if !hasArgs(pip, p) {
			t.Errorf("uv pip без пакета %q: %q", p, pip)
		}
	}
	if !hasArgs(pip, "--torch-backend", "cu128") {
		t.Errorf("uv pip без --torch-backend cu128 (torch с CUDA): %q", pip)
	}
	if NeedSepInstall(home) {
		t.Error("после InstallSep NeedSepInstall=true — повторный запуск переустановит")
	}
}

// --- ТК32 ---

func TestNeedSepInstallFingerprint(t *testing.T) {
	home := t.TempDir()
	if !NeedSepInstall(home) {
		t.Fatal("свежая папка: NeedSepInstall=false")
	}
	// основной отпечаток не заменяет отпечаток разделения
	if err := MarkInstalled(home, []byte("fastapi\n")); err != nil {
		t.Fatal(err)
	}
	if !NeedSepInstall(home) {
		t.Error("отпечаток основного окружения выключил установку разделения")
	}
	uv, _ := fakeUv(t)
	if err := InstallSep(home, uv, &bytes.Buffer{}); err != nil {
		t.Fatalf("InstallSep: %v", err)
	}
	if NeedSepInstall(home) {
		t.Fatal("после InstallSep NeedSepInstall=true")
	}
	// InstallSep не ставит отпечаток основного окружения
	other := t.TempDir()
	if err := InstallSep(other, uv, &bytes.Buffer{}); err != nil {
		t.Fatalf("InstallSep: %v", err)
	}
	if !NeedInstall(other, []byte("fastapi\n")) {
		t.Error("InstallSep выключил установку основного окружения")
	}
	// набор пакетов изменился — переустановка
	saved := SepPackages
	t.Cleanup(func() { SepPackages = saved })
	SepPackages = append(append([]string{}, saved...), "extra-pkg==1.0")
	if !NeedSepInstall(home) {
		t.Error("набор пакетов разделения изменился, а NeedSepInstall=false")
	}
}

func TestInstallSepPipFailure(t *testing.T) {
	home := t.TempDir()
	uv, log := fakeUv(t)
	if err := os.WriteFile(filepath.Join(filepath.Dir(log), "fail-pip"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := InstallSep(home, uv, &bytes.Buffer{}); err == nil {
		t.Fatal("сбой uv pip, а InstallSep без ошибки")
	}
	if !NeedSepInstall(home) {
		t.Error("после сбоя NeedSepInstall=false — повторный запуск не докачает")
	}
}

// --- ТК33 ---

func TestSepDownloadCommand(t *testing.T) {
	home := t.TempDir()
	workerDir := filepath.Join(t.TempDir(), "worker")
	cmd := SepDownloadCommand(home, workerDir)
	if cmd == nil {
		t.Fatal("SepDownloadCommand вернула nil")
	}
	if cmd.Args[0] != sepPython(home) {
		t.Errorf("python = %q, ждали python окружения разделения %q", cmd.Args[0], sepPython(home))
	}
	n := len(cmd.Args)
	want := []string{"sep_run.py", "download", filepath.Join(home, "sep-models")}
	if n < 4 {
		t.Fatalf("argv = %q, мало аргументов", cmd.Args)
	}
	for i, w := range want {
		if cmd.Args[n-3+i] != w {
			t.Fatalf("argv = %q, ждали хвост %q", cmd.Args, want)
		}
	}
	if cmd.Dir != workerDir {
		t.Errorf("Dir = %q, ждали каталог кода воркера %q", cmd.Dir, workerDir)
	}
}
