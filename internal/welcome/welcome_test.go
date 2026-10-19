package welcome

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// ТК3: встроенный клип отдаётся байтами по имени; имя не из набора, путь с каталогом,
// выход наверх и пустое имя — ошибка.
func TestClipKnownNameReturnsBytes(t *testing.T) {
	t.Parallel()
	names := Names()
	if len(names) == 0 {
		t.Fatal("Names() пуст: встроенных клипов нет")
	}
	for _, n := range names {
		b, err := Clip(n)
		if err != nil {
			t.Errorf("Clip(%q): %v", n, err)
			continue
		}
		if len(b) == 0 {
			t.Errorf("Clip(%q): пустые байты", n)
		}
	}
}

func TestClipBadNamesAreErrors(t *testing.T) {
	t.Parallel()
	bad := []string{"", "../x", "clips/x", "no-such-clip.mp3", "/etc/passwd", "..", "."}
	if names := Names(); len(names) > 0 {
		// настоящее имя, но с каталогом или выходом наверх — тоже ошибка
		bad = append(bad, "clips/"+names[0], "../"+names[0], "./"+names[0])
	}
	for _, n := range bad {
		if b, err := Clip(n); err == nil {
			t.Errorf("Clip(%q): want ошибку, got %d байт", n, len(b))
		}
	}
}

func TestNamesWithoutDirAndUnique(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, n := range Names() {
		if n == "" || strings.ContainsAny(n, `/\`) {
			t.Errorf("имя %q: want имя файла без каталога", n)
		}
		if seen[n] {
			t.Errorf("имя %q повторяется", n)
		}
		seen[n] = true
	}
}

// ТК4: набор встроенных файлов и список клипов в welcomeClips.json совпадают в обе стороны.
func TestNamesMatchWelcomeClipsJSON(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../frontend/src/welcomeClips.json")
	if err != nil {
		t.Fatalf("welcomeClips.json: %v", err)
	}
	var data struct {
		Steps []struct {
			ID    string `json:"id"`
			Pairs []struct {
				ID     string `json:"id"`
				Before string `json:"before"`
				After  string `json:"after"`
			} `json:"pairs"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("welcomeClips.json: %v", err)
	}
	inJSON := map[string]bool{}
	for _, s := range data.Steps {
		for _, p := range s.Pairs {
			for _, f := range []string{p.Before, p.After} {
				if f == "" {
					t.Errorf("шаг %s, пара %s: пустое имя клипа", s.ID, p.ID)
					continue
				}
				inJSON[f] = true
			}
		}
	}
	embedded := map[string]bool{}
	for _, n := range Names() {
		embedded[n] = true
	}
	for f := range inJSON {
		if !embedded[f] {
			t.Errorf("клип %q есть в welcomeClips.json, но не встроен", f)
		}
	}
	for n := range embedded {
		if !inJSON[n] {
			t.Errorf("встроенный клип %q не упомянут в welcomeClips.json", n)
		}
	}
}
