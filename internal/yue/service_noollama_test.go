package yue

import (
	"reflect"
	"testing"
)

// spec internal-drop-ollama, условия 1 и 3: Ollama и «из трека» удалены из yue.Service —
// методов Copilot, Translate, AdaptLyrics, OllamaModels, RecognizeLyrics нет;
// JobLyrics (распознать, что поёт готовый трек) остаётся.
func TestServiceHasNoOllamaMethods(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf((*Service)(nil)).Elem()
	if typ.NumMethod() == 0 {
		t.Fatal("интерфейс Service без методов — тест ничего не проверяет")
	}
	for _, name := range []string{"Copilot", "Translate", "AdaptLyrics", "OllamaModels", "RecognizeLyrics"} {
		if _, ok := typ.MethodByName(name); ok {
			t.Errorf("метод %s всё ещё в yue.Service", name)
		}
	}
	if _, ok := typ.MethodByName("JobLyrics"); !ok {
		t.Error("JobLyrics пропал из yue.Service — он должен остаться")
	}
}
