// Package welcome — клипы «было/стало» для окна знакомства. Встроены в приложение:
// окно показывается при первом запуске, когда воркера может ещё не быть.
// Список шагов и пар — frontend/src/welcomeClips.json (тест сверяет его с набором файлов).
package welcome

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed clips/*.mp3
var clips embed.FS

// Clip — байты клипа по имени файла (без каталога).
func Clip(name string) ([]byte, error) {
	if name == "" || strings.ContainsAny(name, `/\`) {
		return nil, fmt.Errorf("welcome clip %q: bad name", name)
	}
	b, err := clips.ReadFile("clips/" + name)
	if err != nil {
		return nil, fmt.Errorf("welcome clip %q: not found", name)
	}
	return b, nil
}

// Names — имена всех встроенных клипов по алфавиту.
func Names() []string {
	ents, err := fs.ReadDir(clips, "clips")
	if err != nil {
		return nil // каталог встроен при сборке: ошибки здесь не бывает
	}
	out := make([]string, 0, len(ents))
	for _, e := range ents {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}
