package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ТК16: recipe_export с path — только *.partscaster.json или старое *.yue-track.json;
// иной суффикс → ошибка инструмента, файл не создан. Корректный path → файл с рецептом.
// Без path — файл «track-<id>.partscaster.json» в каталоге загрузок.

type recipeFake struct {
	*fakeService
	asked []int64
}

func (f *recipeFake) Recipe(_ context.Context, id int64) (json.RawMessage, error) {
	f.asked = append(f.asked, id)
	return json.RawMessage(`{"format":"yue-track-recipe","version":1,"track":{"title":"T","folder":"","head":"n1"},"root":{"id":"n1"},"ops":[],"warnings":[]}`), nil
}

func newRecipeServer(t *testing.T) (*Server, *recipeFake) {
	t.Helper()
	s, fake, _ := newRecipeServerIn(t)
	return s, fake
}

// newRecipeServerIn — то же, но с известным каталогом загрузок.
func newRecipeServerIn(t *testing.T) (*Server, *recipeFake, string) {
	t.Helper()
	fake := &recipeFake{fakeService: &fakeService{url: "http://w:8091"}}
	dl := t.TempDir()
	s := NewServer(fake, dl)
	RegisterWorkflowTools(s)
	RegisterStudioTools(s)
	RegisterLibraryTools(s)
	RegisterInstallTools(s)
	return s, fake, dl
}

func TestRecipeExportPathWithoutSuffixIsError(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"song.json", "song", "song.partscaster.json.txt", "song.yue-track.json.txt", "song.yue-track"} {
		s, _ := newRecipeServer(t)
		dir := t.TempDir()
		p := filepath.Join(dir, name)
		out, ok := call(t, s, "recipe_export", map[string]any{"job_id": float64(5), "path": p})
		if ok {
			t.Errorf("%s: want ошибка инструмента, got %s", name, out)
		}
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s: файл создан при ошибке (stat %v)", name, err)
		}
		if ents, _ := os.ReadDir(dir); len(ents) != 0 {
			t.Errorf("%s: в каталоге появились файлы %v", name, ents)
		}
	}
}

func TestRecipeExportPathWithSuffixWritesRecipe(t *testing.T) {
	t.Parallel()
	// новое расширение и старое (у людей уже лежат такие файлы) — оба принимаются
	for _, name := range []string{"song.partscaster.json", "song.yue-track.json"} {
		s, fake := newRecipeServer(t)
		p := filepath.Join(t.TempDir(), name)
		out, ok := call(t, s, "recipe_export", map[string]any{"job_id": float64(5), "path": p})
		if !ok {
			t.Fatalf("%s: recipe_export: %s", name, out)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("%s: файл не создан: %v", name, err)
		}
		var m map[string]any
		if json.Unmarshal(data, &m) != nil || m["format"] != "yue-track-recipe" {
			t.Errorf("%s: в файле не рецепт: %s", name, data)
		}
		if len(fake.asked) != 1 || fake.asked[0] != 5 {
			t.Errorf("%s: Recipe вызван %v, want [5]", name, fake.asked)
		}
	}
}

func TestRecipeExportWithoutPathWritesDefaultName(t *testing.T) {
	t.Parallel()
	s, fake, dl := newRecipeServerIn(t)
	out, ok := call(t, s, "recipe_export", map[string]any{"job_id": float64(7)})
	if !ok {
		t.Fatalf("recipe_export: %s", out)
	}
	p := filepath.Join(dl, "track-7.partscaster.json")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("нет файла %s: %v", p, err)
	}
	var m map[string]any
	if json.Unmarshal(data, &m) != nil || m["format"] != "yue-track-recipe" {
		t.Errorf("в файле не рецепт: %s", data)
	}
	if len(fake.asked) != 1 || fake.asked[0] != 7 {
		t.Errorf("Recipe вызван %v, want [7]", fake.asked)
	}
}
