package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// recipe_export с path — только *.yue-track.json;
// иной суффикс → ошибка инструмента, файл не создан. Корректный path → файл с рецептом.

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
	fake := &recipeFake{fakeService: &fakeService{url: "http://w:8091"}}
	s := NewServer(fake, t.TempDir())
	RegisterWorkflowTools(s)
	RegisterStudioTools(s)
	RegisterLibraryTools(s)
	RegisterInstallTools(s)
	return s, fake
}

func TestRecipeExportPathWithoutSuffixIsError(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"song.json", "song", "song.yue-track.json.txt", "song.yue-track"} {
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
	s, fake := newRecipeServer(t)
	p := filepath.Join(t.TempDir(), "song.yue-track.json")
	out, ok := call(t, s, "recipe_export", map[string]any{"job_id": float64(5), "path": p})
	if !ok {
		t.Fatalf("recipe_export: %s", out)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("файл не создан: %v", err)
	}
	var m map[string]any
	if json.Unmarshal(data, &m) != nil || m["format"] != "yue-track-recipe" {
		t.Errorf("в файле не рецепт: %s", data)
	}
	if len(fake.asked) != 1 || fake.asked[0] != 5 {
		t.Errorf("Recipe вызван %v, want [5]", fake.asked)
	}
}
