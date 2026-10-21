package workerexe

import (
	"strings"
	"testing"
	"unicode"
)

// Условие 3 карточки internal-installer-en: строки состояния трея — на языке окна установки.

func setLang(t *testing.T, l string) {
	t.Helper()
	old := Lang
	Lang = l
	t.Cleanup(func() { Lang = old })
}

func hasCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

func TestTrayStateTextEnglish(t *testing.T) {
	setLang(t, "en")
	seen := map[string]WorkerState{}
	for _, s := range []WorkerState{StateRunning, StateFailed, StateStarting} {
		txt := s.Text()
		if strings.TrimSpace(txt) == "" {
			t.Errorf("state %v: empty text", s)
		}
		if hasCyrillic(txt) {
			t.Errorf("state %v: Cyrillic in English text %q", s, txt)
		}
		if prev, ok := seen[txt]; ok {
			t.Errorf("states %v and %v share text %q", prev, s, txt)
		}
		seen[txt] = s
	}
}

func TestTrayStateTextRussianUnchanged(t *testing.T) {
	setLang(t, "ru")
	want := map[WorkerState]string{
		StateRunning:  "Воркер работает",
		StateFailed:   "Воркер остановился с ошибкой — см. журнал",
		StateStarting: "Воркер запускается…",
	}
	for s, w := range want {
		if got := s.Text(); got != w {
			t.Errorf("state %v: got %q, want %q", s, got, w)
		}
	}
}

func TestEngineCppCommandPassesLang(t *testing.T) {
	setLang(t, "en")
	t.Setenv("PARTSCASTER_LANG", "ru") // родительское окружение не должно перебить язык окна
	home, dir := t.TempDir(), t.TempDir()
	cmd := EngineCppCommand(home, dir, "download", "q8")
	last := ""
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "PARTSCASTER_LANG=") {
			last = e
		}
	}
	if last != "PARTSCASTER_LANG=en" {
		t.Errorf("last PARTSCASTER_LANG in Env = %q, want PARTSCASTER_LANG=en", last)
	}
}
