package main

import (
	"reflect"
	"strings"
	"testing"
)

// spec internal-drop-corpus: биндингов профилей App.YueCorpus* нет
// (Wails экспортирует во фронт все экспортируемые методы *App).
func TestAppHasNoYueCorpusBindings(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf(&App{})
	if typ.NumMethod() == 0 {
		t.Fatal("у *App нет методов — тест ничего не проверяет")
	}
	for i := 0; i < typ.NumMethod(); i++ {
		if name := typ.Method(i).Name; strings.HasPrefix(name, "YueCorpus") {
			t.Errorf("биндинг профилей %s всё ещё экспортирован", name)
		}
	}
}
