package main

import "testing"

// ТК13: имя файла экспорта по умолчанию — «<название>.<расширение файла на воркере>»,
// без названия — «Partscaster <номер>», недопустимые символы — «_», пробелы по краям убираются.
func TestExportFileName(t *testing.T) {
	cases := []struct {
		name  string
		title string
		id    int64
		file  string
		want  string
	}{
		{"flac по названию", "Warm Wire", 901, "audio.flac", "Warm Wire.flac"},
		{"mp3 — расширение от имени файла", "Warm Wire", 901, "x.mp3", "Warm Wire.mp3"},
		{"пустое название — по номеру", "", 901, "audio.flac", "Partscaster 901.flac"},
		{"название из пробелов — по номеру", "   ", 7, "a.mp3", "Partscaster 7.mp3"},
		{"недопустимые символы — «_»", "AC/DC: Back?", 5, "audio.flac", "AC_DC_ Back_.flac"},
		{"пробелы по краям убраны, ноты abc", "  Gone  ", 3, "plan.abc", "Gone.abc"},
		{"файл без расширения — без него", "Gone", 3, "audio", "Gone"},
		{"управляющий символ — «_»", "Static\tGold", 1, "a.flac", "Static_Gold.flac"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := exportFileName(c.title, c.id, c.file); got != c.want {
				t.Errorf("exportFileName(%q, %d, %q) = %q, want %q", c.title, c.id, c.file, got, c.want)
			}
		})
	}
}

// ТК14: имя рецепта после выноса общей замены символов не меняется.
func TestRecipeFileNameKeepsBehaviour(t *testing.T) {
	cases := []struct {
		title string
		id    int64
		want  string
	}{
		{"A/B", 1, "A_B.yue-track.json"},
		{"", 2, "track-2.yue-track.json"},
	}
	for _, c := range cases {
		if got := recipeFileName(c.title, c.id); got != c.want {
			t.Errorf("recipeFileName(%q, %d) = %q, want %q", c.title, c.id, got, c.want)
		}
	}
}
