package ffmpegbin

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
)

// Спецификация (карточка internal-ffmpeg-pin, условие 4 / ТК4): приложение на macOS, запущенное
// из Finder, получает PATH без каталогов Homebrew. Поиск ffmpeg на darwin, если в PATH нет,
// смотрит ещё /opt/homebrew/bin и /usr/local/bin. Порядок: переменная → PATH → Homebrew
// (только darwin) → своя папка. Каталоги подменяемы (brewDirs), ОС — тоже (goos).

func TestKnownDirsDarwinHasHomebrew(t *testing.T) {
	d := knownDirs("darwin")
	for _, want := range []string{"/opt/homebrew/bin", "/usr/local/bin"} {
		if !slices.Contains(d, want) {
			t.Fatalf("knownDirs(darwin) = %v, нет %s", d, want)
		}
	}
}

func TestKnownDirsOtherOSEmpty(t *testing.T) {
	for _, os := range []string{"linux", "windows", "freebsd", ""} {
		if d := knownDirs(os); len(d) != 0 {
			t.Fatalf("knownDirs(%q) = %v, ожидалось пусто", os, d)
		}
	}
}

// brewSetup: PATH пуст (execLookPath → ошибка), переменной нет, своей папки нет;
// в подменённом каталоге «Homebrew» лежит ffmpeg. Возвращает путь к нему.
func brewSetup(t *testing.T, osName string) string {
	t.Helper()
	isolate(t)
	t.Setenv("HOME", t.TempDir()) // os.UserConfigDir на darwin — от HOME
	oldGoos, oldDirs, oldLook := goos, brewDirs, execLookPath
	t.Cleanup(func() { goos, brewDirs, execLookPath = oldGoos, oldDirs, oldLook })
	execLookPath = func() (string, error) { return "", errors.New("not in PATH") }
	dir := t.TempDir()
	p := filepath.Join(dir, "ffmpeg")
	writeExe(t, p)
	goos = osName
	brewDirs = []string{dir}
	return p
}

func TestGetBrewOnDarwin(t *testing.T) {
	p := brewSetup(t, "darwin")
	s := Get()
	if !s.Ok || s.Source != "brew" {
		t.Fatalf("darwin, ffmpeg только в каталоге Homebrew: %+v, ожидалось Ok и Source brew", s)
	}
	if s.Path != p {
		t.Fatalf("путь %q, ожидался %q", s.Path, p)
	}
}

func TestGetBrewSecondDir(t *testing.T) {
	p := brewSetup(t, "darwin")
	brewDirs = []string{t.TempDir(), filepath.Dir(p)} // в первом каталоге пусто
	if s := Get(); !s.Ok || s.Source != "brew" || s.Path != p {
		t.Fatalf("ffmpeg во втором каталоге Homebrew: %+v", s)
	}
}

func TestGetBrewEmptyDirsNotFound(t *testing.T) {
	brewSetup(t, "darwin")
	brewDirs = []string{t.TempDir()} // каталог есть, ffmpeg в нём нет
	if s := Get(); s.Ok {
		t.Fatalf("Homebrew без ffmpeg, PATH и своей папки нет: %+v, ожидалось Ok=false", s)
	}
}

func TestGetBrewNotUsedOnLinux(t *testing.T) {
	brewSetup(t, "linux")
	if s := Get(); s.Ok {
		t.Fatalf("linux: каталоги Homebrew не смотрятся, а Get = %+v", s)
	}
}

func TestGetPathBeatsBrew(t *testing.T) {
	brewSetup(t, "darwin")
	sys := filepath.Join(t.TempDir(), "ffmpeg")
	writeExe(t, sys)
	execLookPath = func() (string, error) { return sys, nil }
	if s := Get(); !s.Ok || s.Source != "path" || s.Path != sys {
		t.Fatalf("PATH должен быть прежде Homebrew: %+v", s)
	}
}

func TestGetBrewBeatsOwn(t *testing.T) {
	p := brewSetup(t, "darwin")
	withOwn(t)
	if s := Get(); s.Source != "brew" || s.Path != p {
		t.Fatalf("Homebrew должен быть прежде своей папки: %+v", s)
	}
}

func TestGetEnvBeatsBrew(t *testing.T) {
	brewSetup(t, "darwin")
	env := filepath.Join(t.TempDir(), "ffmpeg")
	writeExe(t, env)
	t.Setenv("PARTSCASTER_FFMPEG", env)
	if s := Get(); s.Source != "env" || s.Path != env {
		t.Fatalf("переменная должна быть прежде Homebrew: %+v", s)
	}
}
