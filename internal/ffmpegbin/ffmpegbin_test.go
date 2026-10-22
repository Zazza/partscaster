package ffmpegbin

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Спецификация (карточка ffmpeg-локально):
//   - Path/Get: PARTSCASTER_FFMPEG → PATH → Homebrew (macOS) → своя папка; ничего нет — «ffmpeg» (внятная ошибка запуска);
//   - Install: качает zip закреплённой сборки, сверяет sha256, распаковывает только bin/
//     (без ffplay/ffprobe), ставит в свою папку; не совпала сумма — ошибка, папка не меняется;
//     уже установлено — без сети;
//   - прогресс установки виден через Snapshot (полоса окна).

func writeExe(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("PARTSCASTER_FFMPEG", "")
	t.Setenv("PATH", t.TempDir()) // своего ffmpeg в PATH нет
	cfg := t.TempDir()
	t.Setenv("AppData", cfg) // своя папка (UserConfigDir) — тоже пустая
	t.Setenv("XDG_CONFIG_HOME", cfg)
}

func withOwn(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AppData", dir) // os.UserConfigDir на windows
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeExe(t, filepath.Join(dir, "partscaster", "tools", "ffmpeg", "bin", "ffmpeg.exe"))
	writeExe(t, filepath.Join(dir, "partscaster", "tools", "ffmpeg", "bin", "ffmpeg"))
}

func TestGetEnvWins(t *testing.T) {
	isolate(t)
	p := filepath.Join(t.TempDir(), "ffmpeg")
	writeExe(t, p)
	t.Setenv("PARTSCASTER_FFMPEG", p)
	s := Get()
	if !s.Ok || s.Source != "env" || s.Path != p {
		t.Fatalf("env: %+v", s)
	}
}

func TestGetPathBeforeOwn(t *testing.T) {
	isolate(t)
	withOwn(t)
	// в PATH тоже есть — системный приоритетнее: у кого стоит свой (часто полнее) — не подменяем.
	// Оба имени: на unix LookPath ищет «ffmpeg», на windows — «ffmpeg.exe»
	dir := t.TempDir()
	writeExe(t, filepath.Join(dir, "ffmpeg.exe"))
	writeExe(t, filepath.Join(dir, "ffmpeg"))
	t.Setenv("PATH", dir)
	s := Get()
	if !s.Ok || s.Source != "path" {
		t.Fatalf("path перед own: %+v", s)
	}
}

func TestGetOwnWhenNoPath(t *testing.T) {
	isolate(t) // PATH пуст, своего в нём нет
	withOwn(t)
	s := Get()
	if !s.Ok || s.Source != "own" {
		t.Fatalf("own как дополнение машины без PATH: %+v", s)
	}
}

func TestGetPathFallback(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name = "ffmpeg.exe"
	}
	writeExe(t, filepath.Join(dir, name))
	t.Setenv("PATH", dir)
	s := Get()
	if !s.Ok || s.Source != "path" {
		t.Fatalf("path: %+v", s)
	}
}

func TestGetNothing(t *testing.T) {
	isolate(t)
	t.Setenv("AppData", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if s := Get(); s.Ok {
		t.Fatalf("в пустоте нашёлся ffmpeg: %+v", s)
	}
	if got := Path(); got != "ffmpeg" {
		t.Fatalf("Path без находок = %q, ждали имя для внятной ошибки", got)
	}
}

// fakeZip — сборка «как BtbN»: <корень>/bin/{ffmpeg.exe, dll, ffplay.exe, ffprobe.exe} + мусор.
func fakeZip(t *testing.T, ffmpegBody []byte) (zipBytes []byte, sum string) {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	files := map[string][]byte{
		"ffmpeg-n9.0.2-win64/bin/ffmpeg.exe":  ffmpegBody,
		"ffmpeg-n9.0.2-win64/bin/ffmpeg":      ffmpegBody, // unix-имя: Install сверяется с own() той же ОС
		"ffmpeg-n9.0.2-win64/bin/avcodec.dll": []byte("dll"),
		"ffmpeg-n9.0.2-win64/bin/ffplay.exe":  []byte("play"),
		"ffmpeg-n9.0.2-win64/bin/ffprobe.exe": []byte("probe"),
		"ffmpeg-n9.0.2-win64/LICENSE.txt":     []byte("license"),
	}
	for name, body := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	h := sha256.Sum256(b)
	return b, hex.EncodeToString(h[:])
}

func TestInstallDownloadsExtractsAndSkipsWhenInstalled(t *testing.T) {
	isolate(t)
	cfg := t.TempDir()
	t.Setenv("AppData", cfg)
	t.Setenv("XDG_CONFIG_HOME", cfg)

	body := []byte("fake ffmpeg exe")
	zipBytes, sum := fakeZip(t, body)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write(zipBytes)
	}))
	defer srv.Close()
	url, sha := URL, SHA256
	URL, SHA256 = srv.URL, sum
	defer func() { URL, SHA256 = url, sha }()

	if err := Install(); err != nil {
		t.Fatal(err)
	}
	s := Get()
	if !s.Ok || s.Source != "own" {
		t.Fatalf("после установки: %+v", s)
	}
	got, err := os.ReadFile(s.Path)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("ffmpeg.exe не тот: %v", err)
	}
	dir := filepath.Dir(s.Path)
	for _, absent := range []string{"ffplay.exe", "ffprobe.exe", "LICENSE.txt"} {
		if _, err := os.Stat(filepath.Join(dir, absent)); err == nil {
			t.Errorf("в bin прошёл лишний файл %s", absent)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "avcodec.dll")); err != nil {
		t.Error("avcodec.dll не распакован")
	}
	// повторная установка — без сети (счётчик загрузок не растёт)
	if err := Install(); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("скачиваний %d, ждали 1 (повтор — без сети)", hits)
	}
}

func TestInstallBadChecksum(t *testing.T) {
	isolate(t)
	cfg := t.TempDir()
	t.Setenv("AppData", cfg)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	zipBytes, _ := fakeZip(t, []byte("x"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(zipBytes)
	}))
	defer srv.Close()
	url, sha := URL, SHA256
	URL, SHA256 = srv.URL, "deadbeef"
	defer func() { URL, SHA256 = url, sha }()

	if err := Install(); err == nil {
		t.Fatal("чужая сумма прошла")
	}
	if s := Get(); s.Ok {
		t.Fatalf("при ошибке папка изменилась: %+v", s)
	}
	if p := Snapshot(); p.Downloading {
		t.Fatal("прогресс не сброшен после ошибки")
	}
}
