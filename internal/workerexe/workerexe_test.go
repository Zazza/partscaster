package workerexe

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Спецификация partscaster-worker.exe (карточка internal-partscaster-worker-exe, ТК1–ТК8):
//   - uv скачивается с проверкой sha256;
//   - пакеты ставятся один раз: отпечаток (requirements + закреплённые версии)
//     решает, нужна ли переустановка;
//   - uv pip: torch/torchaudio 2.10.0+cu128, truststore; индекс cu128 основной, PyPI дополнительный
//     (пакета модели yue2-infer нет — генерация на yue2.cpp, internal-cpp-only);
//   - nvidia-smi разбирается в имя/память/драйвер, мусор — ошибка;
//   - архив воркера распаковывается в каталог версии, выход за каталог — отказ;
//   - окружение воркера живёт в папке установки;
//   - запуск: truststore до uvicorn, 127.0.0.1:8091 (--lan → 0.0.0.0);
//   - выбранная папка запоминается.

// --- ТК1: sha256 ---

func testWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestVerifySHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "uv.zip")
	data := []byte("содержимое архива uv")
	testWriteFile(t, path, data)
	sum := sha256.Sum256(data)
	good := hex.EncodeToString(sum[:])

	if err := VerifySHA256(path, good); err != nil {
		t.Errorf("верная сумма: ожидали nil, получили %v", err)
	}

	other := sha256.Sum256([]byte("другое"))
	if err := VerifySHA256(path, hex.EncodeToString(other[:])); err == nil {
		t.Error("неверная сумма: ожидали ошибку")
	}
	if err := VerifySHA256(path, ""); err == nil {
		t.Error("пустая ожидаемая сумма: ожидали ошибку")
	}
	if err := VerifySHA256(filepath.Join(dir, "нет-такого"), good); err == nil {
		t.Error("нет файла: ожидали ошибку")
	}
}

// --- ТК2: отпечаток установки ---

func testMakeVenv(t *testing.T, home string) {
	t.Helper()
	testWriteFile(t, VenvPython(home, runtime.GOOS), []byte("python"))
}

func TestPackagesFingerprint(t *testing.T) {
	a := []byte("fastapi==0.115.0\nnumpy\n")
	b := []byte("fastapi==0.116.0\nnumpy\n")
	if PackagesFingerprint(a) == "" {
		t.Fatal("отпечаток пустой")
	}
	if PackagesFingerprint(a) != PackagesFingerprint(append([]byte(nil), a...)) {
		t.Error("одинаковые requirements дают разный отпечаток")
	}
	if PackagesFingerprint(a) == PackagesFingerprint(b) {
		t.Error("разные requirements дают одинаковый отпечаток")
	}
}

func TestNeedInstall_NoVenv(t *testing.T) {
	home := t.TempDir()
	req := []byte("fastapi\n")
	if !NeedInstall(home, req) {
		t.Error("нет venv: нужно ставить")
	}
	// Отпечаток есть, а venv нет (папку venv удалили) — всё равно ставить.
	if err := MarkInstalled(home, req); err != nil {
		t.Fatal(err)
	}
	if !NeedInstall(home, req) {
		t.Error("есть отпечаток, но нет python в venv: нужно ставить")
	}
}

func TestNeedInstall_VenvWithoutMark(t *testing.T) {
	home := t.TempDir()
	testMakeVenv(t, home)
	if !NeedInstall(home, []byte("fastapi\n")) {
		t.Error("venv есть, отпечатка нет (установка прервана): нужно ставить")
	}
}

func TestNeedInstall_MatchedThenChanged(t *testing.T) {
	home := t.TempDir()
	testMakeVenv(t, home)
	req := []byte("fastapi\nnumpy\n")
	if err := MarkInstalled(home, req); err != nil {
		t.Fatal(err)
	}
	if NeedInstall(home, req) {
		t.Error("venv есть, отпечаток совпал: переставлять не нужно")
	}
	if !NeedInstall(home, []byte("fastapi\nnumpy\nscipy\n")) {
		t.Error("requirements изменён: нужно ставить")
	}
	// После установки нового набора — снова не ставить.
	changed := []byte("fastapi\nnumpy\nscipy\n")
	if err := MarkInstalled(home, changed); err != nil {
		t.Fatal(err)
	}
	if NeedInstall(home, changed) {
		t.Error("после MarkInstalled нового набора переставлять не нужно")
	}
}

func TestVenvPython(t *testing.T) {
	home := filepath.Join("base", "partscaster-worker")
	if got, want := VenvPython(home, "windows"), filepath.Join(home, "venv", "Scripts", "python.exe"); got != want {
		t.Errorf("windows: %q, ждали %q", got, want)
	}
	for _, goos := range []string{"linux", "darwin"} {
		if got, want := VenvPython(home, goos), filepath.Join(home, "venv", "bin", "python"); got != want {
			t.Errorf("%s: %q, ждали %q", goos, got, want)
		}
	}
}

// --- ТК3: аргументы uv pip ---

func testIndexOf(xs []string, s string) int {
	for i, x := range xs {
		if x == s {
			return i
		}
	}
	return -1
}

func TestPipInstallArgs(t *testing.T) {
	py := filepath.Join("h", "venv", "Scripts", "python.exe")
	req := filepath.Join("h", "worker-1.0.0", "requirements.txt")
	args := PipInstallArgs(py, req)

	head := []string{"pip", "install", "--python", py, "-r", req}
	if len(args) < len(head) {
		t.Fatalf("аргументов мало: %q", args)
	}
	for i, w := range head {
		if args[i] != w {
			t.Errorf("args[%d] = %q, ждали %q (все: %q)", i, args[i], w, args)
		}
	}

	for _, pkg := range []string{"torch==2.10.0+cu128", "torchaudio==2.10.0+cu128", "truststore"} {
		if testIndexOf(args, pkg) < 0 {
			t.Errorf("нет пакета %q в %q", pkg, args)
		}
	}

	if i := testIndexOf(args, "--index-url"); i < 0 || i+1 >= len(args) || args[i+1] != TorchIndex {
		t.Errorf("нет пары --index-url %s: %q", TorchIndex, args)
	}
	if i := testIndexOf(args, "--extra-index-url"); i < 0 || i+1 >= len(args) || args[i+1] != "https://pypi.org/simple" {
		t.Errorf("нет пары --extra-index-url https://pypi.org/simple: %q", args)
	}
}

func TestPinnedConstants(t *testing.T) {
	// Закреплённые версии — часть условия 3 карточки.
	if UvVersion != "0.12.19" {
		t.Errorf("UvVersion = %q", UvVersion)
	}
	if TorchIndex != "https://download.pytorch.org/whl/cu128" {
		t.Errorf("TorchIndex = %q", TorchIndex)
	}
	if Port != 8091 {
		t.Errorf("Port = %d", Port)
	}
}

// --- ТК4: разбор nvidia-smi ---

func TestParseGPU(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want GPU
	}{
		{"одна карта", "NVIDIA GeForce RTX 4090, 24564, 576.88\n", GPU{"NVIDIA GeForce RTX 4090", 24564, "576.88"}},
		{"две карты — первая", "NVIDIA GeForce RTX 3090, 24576, 560.94\nNVIDIA GeForce RTX 3060, 12288, 560.94\n",
			GPU{"NVIDIA GeForce RTX 3090", 24576, "560.94"}},
		{"CRLF (Windows)", "NVIDIA GeForce RTX 4070, 12282, 576.02\r\n", GPU{"NVIDIA GeForce RTX 4070", 12282, "576.02"}},
	}
	for _, c := range cases {
		got, err := ParseGPU(c.out)
		if err != nil {
			t.Errorf("%s: ошибка %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: %+v, ждали %+v", c.name, got, c.want)
		}
	}
}

func TestParseGPU_Errors(t *testing.T) {
	for _, out := range []string{
		"",
		"\n",
		"No devices were found\n",
		"NVIDIA-SMI has failed because it couldn't communicate with the NVIDIA driver.",
		"NVIDIA GeForce RTX 4090, много, 576.88\n",
		"NVIDIA GeForce RTX 4090, 24564\n",
	} {
		if _, err := ParseGPU(out); err == nil {
			t.Errorf("вывод %q: ожидали ошибку", out)
		}
	}
}

// --- ТК5: распаковка архива воркера ---

type testEntry struct {
	name string
	body string // пусто и dir=true — каталог
	dir  bool
}

func testTarGz(t *testing.T, entries []testEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.body)), Typeflag: tar.TypeReg}
		if e.dir {
			h.Typeflag = tar.TypeDir
			h.Mode = 0o755
			h.Size = 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if !e.dir {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func testReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("чтение %s: %v", path, err)
	}
	return string(b)
}

func TestExtractWorker(t *testing.T) {
	dst := t.TempDir()
	arc := testTarGz(t, []testEntry{
		{name: "worker-0.9.0/", dir: true},
		{name: "worker-0.9.0/yue_worker.py", body: "print('v1')\n"},
		{name: "worker-0.9.0/phrases/", dir: true},
		{name: "worker-0.9.0/phrases/a.txt", body: "a1"},
	})
	got, err := ExtractWorker(bytes.NewReader(arc), dst)
	if err != nil {
		t.Fatalf("распаковка: %v", err)
	}
	want := filepath.Join(dst, "worker-0.9.0")
	if got != want {
		t.Errorf("путь %q, ждали %q", got, want)
	}
	if s := testReadFile(t, filepath.Join(want, "yue_worker.py")); s != "print('v1')\n" {
		t.Errorf("yue_worker.py = %q", s)
	}
	if s := testReadFile(t, filepath.Join(want, "phrases", "a.txt")); s != "a1" {
		t.Errorf("phrases/a.txt = %q", s)
	}
}

func TestExtractWorker_NoDirEntries(t *testing.T) {
	// Архив без отдельных записей каталогов — каталоги создаются сами.
	dst := t.TempDir()
	arc := testTarGz(t, []testEntry{
		{name: "worker-1.0.0/sub/deep/x.py", body: "x"},
	})
	got, err := ExtractWorker(bytes.NewReader(arc), dst)
	if err != nil {
		t.Fatalf("распаковка: %v", err)
	}
	if s := testReadFile(t, filepath.Join(got, "sub", "deep", "x.py")); s != "x" {
		t.Errorf("x.py = %q", s)
	}
}

func TestExtractWorker_SameVersionOverwrite(t *testing.T) {
	dst := t.TempDir()
	v1 := testTarGz(t, []testEntry{{name: "worker-0.9.0/yue_worker.py", body: "old"}})
	v1b := testTarGz(t, []testEntry{{name: "worker-0.9.0/yue_worker.py", body: "new"}})
	if _, err := ExtractWorker(bytes.NewReader(v1), dst); err != nil {
		t.Fatal(err)
	}
	got, err := ExtractWorker(bytes.NewReader(v1b), dst)
	if err != nil {
		t.Fatalf("повторная распаковка той же версии: %v", err)
	}
	if s := testReadFile(t, filepath.Join(got, "yue_worker.py")); s != "new" {
		t.Errorf("файл не обновлён: %q", s)
	}
}

func TestExtractWorker_NewVersionKeepsSiblings(t *testing.T) {
	// Условие 9: новая версия exe — новый каталог воркера; данные рядом не трогаются.
	dst := t.TempDir()
	data := filepath.Join(dst, "data", "jobs.db")
	testWriteFile(t, data, []byte("db"))
	if _, err := ExtractWorker(bytes.NewReader(testTarGz(t, []testEntry{{name: "worker-0.9.0/a.py", body: "1"}})), dst); err != nil {
		t.Fatal(err)
	}
	got, err := ExtractWorker(bytes.NewReader(testTarGz(t, []testEntry{{name: "worker-0.10.0/a.py", body: "2"}})), dst)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(dst, "worker-0.10.0") {
		t.Errorf("путь новой версии %q", got)
	}
	if s := testReadFile(t, data); s != "db" {
		t.Errorf("данные рядом изменены: %q", s)
	}
}

func TestExtractWorker_RejectsEscape(t *testing.T) {
	cases := map[string][]testEntry{
		"../ внутри":    {{name: "worker-0.9.0/../../evil.txt", body: "x"}},
		"../ в начале":  {{name: "../evil.txt", body: "x"}},
		"абсолютный":    {{name: "/evil.txt", body: "x"}},
		"после обычных": {{name: "worker-0.9.0/ok.py", body: "ok"}, {name: "worker-0.9.0/../../evil.txt", body: "x"}},
	}
	for name, entries := range cases {
		root := t.TempDir()
		dst := filepath.Join(root, "home")
		if err := os.MkdirAll(dst, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := ExtractWorker(bytes.NewReader(testTarGz(t, entries)), dst); err == nil {
			t.Errorf("%s: ожидали ошибку", name)
		}
		// Вне каталога назначения ничего не появилось.
		for _, p := range []string{filepath.Join(root, "evil.txt"), filepath.Join(filepath.Dir(root), "evil.txt")} {
			if _, err := os.Stat(p); err == nil {
				t.Errorf("%s: файл записан вне каталога: %s", name, p)
				_ = os.Remove(p)
			}
		}
	}
}

func TestExtractWorker_NotGzip(t *testing.T) {
	if _, err := ExtractWorker(strings.NewReader("это не архив"), t.TempDir()); err == nil {
		t.Error("не gzip: ожидали ошибку")
	}
}

// --- ТК6: окружение воркера ---

func TestWorkerEnv(t *testing.T) {
	home := filepath.Join("base", "partscaster-worker")
	for _, goos := range []string{"windows", "linux"} {
		env := WorkerEnv(home, goos)
		if got, want := env["PARTSCASTER_DATA_DIR"], filepath.Join(home, "data"); got != want {
			t.Errorf("%s: PARTSCASTER_DATA_DIR=%q, ждали %q", goos, got, want)
		}
		if got, want := env["HF_HOME"], filepath.Join(home, "hf-cache"); got != want {
			t.Errorf("%s: HF_HOME=%q, ждали %q", goos, got, want)
		}
		if env["PYTHONUTF8"] != "1" {
			t.Errorf("%s: PYTHONUTF8=%q", goos, env["PYTHONUTF8"])
		}
	}
}

// --- ТК7: команда запуска ---

func TestServeArgs(t *testing.T) {
	for _, lan := range []bool{false, true} {
		args := ServeArgs(lan)
		if len(args) != 2 || args[0] != "-c" {
			t.Fatalf("lan=%v: ждали [-c код], получили %q", lan, args)
		}
		code := args[1]
		inj, run := strings.Index(code, "inject_into_ssl"), strings.Index(code, "uvicorn.run")
		if inj < 0 || run < 0 || inj > run {
			t.Errorf("lan=%v: truststore.inject_into_ssl() должен идти до uvicorn.run: %q", lan, code)
		}
		if !strings.Contains(code, "truststore") {
			t.Errorf("lan=%v: нет truststore: %q", lan, code)
		}
		if !strings.Contains(code, "yue_worker:app") {
			t.Errorf("lan=%v: нет yue_worker:app: %q", lan, code)
		}
		if !strings.Contains(code, "8091") {
			t.Errorf("lan=%v: нет порта 8091: %q", lan, code)
		}
		if lan {
			if !strings.Contains(code, "0.0.0.0") {
				t.Errorf("lan: нет host 0.0.0.0: %q", code)
			}
		} else {
			if !strings.Contains(code, "127.0.0.1") {
				t.Errorf("без lan: нет host 127.0.0.1: %q", code)
			}
			if strings.Contains(code, "0.0.0.0") {
				t.Errorf("без lan: слушать все интерфейсы нельзя: %q", code)
			}
		}
	}
}

// --- ТК8: папка установки ---

func TestResolveHome_Default(t *testing.T) {
	cfg := t.TempDir()
	def := filepath.Join("C:", "Users", "u", "AppData", "Local", "partscaster-worker")
	home, saved := ResolveHome(cfg, def)
	if home != def || saved {
		t.Errorf("нет сохранённой: (%q, %v), ждали (%q, false)", home, saved, def)
	}
	// Каталога настроек ещё нет вовсе — тоже по умолчанию.
	home, saved = ResolveHome(filepath.Join(cfg, "нет"), def)
	if home != def || saved {
		t.Errorf("нет каталога настроек: (%q, %v)", home, saved)
	}
}

func TestResolveHome_Saved(t *testing.T) {
	cfg := t.TempDir()
	chosen := filepath.Join(t.TempDir(), "D диск", "partscaster-worker")
	if err := SaveHome(cfg, chosen); err != nil {
		t.Fatal(err)
	}
	home, saved := ResolveHome(cfg, filepath.Join("default"))
	if home != chosen || !saved {
		t.Errorf("сохранённая: (%q, %v), ждали (%q, true)", home, saved, chosen)
	}
	// Повторный выбор перезаписывает прежний.
	other := filepath.Join(t.TempDir(), "other")
	if err := SaveHome(cfg, other); err != nil {
		t.Fatal(err)
	}
	if home, _ := ResolveHome(cfg, "default"); home != other {
		t.Errorf("после повторного SaveHome: %q, ждали %q", home, other)
	}
}

// --- Архив воркера: разделитель Windows в имени записи ---

func TestExtractWorker_RejectsBackslashEscape(t *testing.T) {
	cases := map[string][]testEntry{
		"смешанный разделитель": {{name: "worker-1/..\\..\\evil.txt", body: "x"}},
		"только обратный слеш":  {{name: "worker-1\\..\\..\\evil.txt", body: "x"}},
		"после обычных":         {{name: "worker-1/ok.py", body: "ok"}, {name: "worker-1\\..\\..\\evil.txt", body: "x"}},
	}
	for name, entries := range cases {
		root := t.TempDir()
		dst := filepath.Join(root, "home")
		if err := os.MkdirAll(dst, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := ExtractWorker(bytes.NewReader(testTarGz(t, entries)), dst); err == nil {
			t.Errorf("%s: ожидали ошибку", name)
		}
		// Вне каталога назначения ничего не появилось: в root только home.
		items, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range items {
			if it.Name() != "home" {
				t.Errorf("%s: вне каталога назначения записано %q", name, it.Name())
			}
		}
		if p := filepath.Join(filepath.Dir(root), "evil.txt"); func() bool { _, err := os.Stat(p); return err == nil }() {
			t.Errorf("%s: файл записан вне каталога: %s", name, p)
			_ = os.Remove(p)
		}
	}

	// Контроль: обычный архив worker-1/ распаковывается.
	dst := t.TempDir()
	arc := testTarGz(t, []testEntry{
		{name: "worker-1/", dir: true},
		{name: "worker-1/yue_worker.py", body: "ok"},
	})
	got, err := ExtractWorker(bytes.NewReader(arc), dst)
	if err != nil {
		t.Fatalf("обычный архив: %v", err)
	}
	if got != filepath.Join(dst, "worker-1") {
		t.Errorf("путь %q, ждали %q", got, filepath.Join(dst, "worker-1"))
	}
	if s := testReadFile(t, filepath.Join(dst, "worker-1", "yue_worker.py")); s != "ok" {
		t.Errorf("yue_worker.py = %q", s)
	}
}

// --- Один запуск на папку установки ---

func TestLockHome_SecondRunBusy(t *testing.T) {
	home := t.TempDir()
	first, err := LockHome(home)
	if err != nil {
		t.Fatalf("первый запуск: %v", err)
	}
	if first == nil {
		t.Fatal("первый запуск: файл блокировки nil")
	}
	defer first.Close()

	second, err := LockHome(home)
	if !errors.Is(err, ErrHomeBusy) {
		t.Errorf("второй запуск при живом первом: ждали ErrHomeBusy, получили %v", err)
	}
	if second != nil {
		_ = second.Close()
	}

	if err := first.Close(); err != nil {
		t.Fatalf("закрытие первого: %v", err)
	}
	third, err := LockHome(home)
	if err != nil {
		t.Fatalf("после завершения первого: ждали успех, получили %v", err)
	}
	_ = third.Close()
}

func TestLockHome_DifferentHomesIndependent(t *testing.T) {
	a, err := LockHome(t.TempDir())
	if err != nil {
		t.Fatalf("папка A: %v", err)
	}
	defer a.Close()
	b, err := LockHome(t.TempDir())
	if err != nil {
		t.Fatalf("папка B при занятой A: ждали успех, получили %v", err)
	}
	_ = b.Close()
}
