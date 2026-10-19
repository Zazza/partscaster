package workerexe

import (
	"path/filepath"
	"strconv"
	"testing"
)

// Карточка internal-cpp-only, ТК16 (усл. 4): установщик Windows берёт рекомендуемую модель из
// worker/engine_cpp.py — RecommendCommand(home, workerDir, vramMiB) запускает
// `<python venv> engine_cpp.py recommend <МиБ>` в каталоге кода воркера.
// Отдельный файл: в красной фазе функции ещё нет, и без него пакет не собирается.

func TestRecommendCommandRunsEngineCpp(t *testing.T) {
	home := t.TempDir()
	workerDir := filepath.Join(t.TempDir(), "worker")
	for _, mib := range []int{8188, 12282} {
		cmd := RecommendCommand(home, workerDir, mib)
		if cmd == nil {
			t.Fatal("RecommendCommand вернула nil")
		}
		if cmd.Args[0] != VenvPython(home, goosHere) {
			t.Errorf("python = %q, want python из venv %q", cmd.Args[0], VenvPython(home, goosHere))
		}
		n := len(cmd.Args)
		if n < 4 {
			t.Fatalf("argv = %q, мало аргументов", cmd.Args)
		}
		got := cmd.Args[n-3:]
		want := []string{"engine_cpp.py", "recommend", strconv.Itoa(mib)}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("argv = %q, want хвост %q", cmd.Args, want)
			}
		}
		if cmd.Dir != workerDir {
			t.Errorf("Dir = %q, want каталог кода воркера %q", cmd.Dir, workerDir)
		}
	}
}
