package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// Документация живёт на двух языках (docs/ru и docs/en, одинаковые имена файлов; README.md + README.ru.md;
// CONTRIBUTING.md + CONTRIBUTING.ru.md): файл без пары на другом языке — тест падает.
func TestDocsHaveBothLanguages(t *testing.T) {
	t.Parallel()
	names := func(dir string) []string {
		m, err := filepath.Glob(filepath.Join("docs", dir, "*.md"))
		if err != nil || len(m) == 0 {
			t.Fatalf("docs/%s: нет файлов (%v)", dir, err)
		}
		out := make([]string, len(m))
		for i, p := range m {
			out[i] = filepath.Base(p)
		}
		sort.Strings(out)
		return out
	}
	ru, en := names("ru"), names("en")
	set := map[string]int{}
	for _, n := range ru {
		set[n] |= 1
	}
	for _, n := range en {
		set[n] |= 2
	}
	for n, v := range set {
		switch v {
		case 1:
			t.Errorf("docs/ru/%s без пары docs/en/%s", n, n)
		case 2:
			t.Errorf("docs/en/%s без пары docs/ru/%s", n, n)
		}
	}
	for _, f := range []string{"README.md", "README.ru.md", "CONTRIBUTING.md", "CONTRIBUTING.ru.md"} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("нет %s: %v", f, err)
		}
	}
	if m, _ := filepath.Glob(filepath.Join("docs", "*.md")); len(m) > 0 {
		t.Errorf("файлы доки вне docs/ru и docs/en: %v", m)
	}
}
