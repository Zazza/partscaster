package main

import (
	"reflect"
	"strings"
	"testing"
)

// spec internal-drop-ollama, условия 1 и 3: биндингов Ollama, «из трека» и неиспользуемого
// фронтом YueJobLyrics нет (Wails экспортирует во фронт все экспортируемые методы *App).
func TestAppHasNoOllamaBindings(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(&App{})
	if typ.NumMethod() == 0 {
		t.Fatal("у *App нет методов — тест ничего не проверяет")
	}
	for _, name := range []string{
		"YueCopilot", "YueTranslate", "YueAdaptLyrics", "YueOllamaModels",
		"YueRecognizeLyrics", "YueJobLyrics",
	} {
		if _, ok := typ.MethodByName(name); ok {
			t.Errorf("биндинг %s всё ещё экспортирован", name)
		}
	}
	// «из трека» мог называться с суффиксом (YueRecognizeLyricsFile) — ловим по префиксу.
	for i := 0; i < typ.NumMethod(); i++ {
		if name := typ.Method(i).Name; strings.HasPrefix(name, "YueRecognizeLyrics") {
			t.Errorf("биндинг «из трека» %s всё ещё экспортирован", name)
		}
	}
}
