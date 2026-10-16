package yue

import (
	"reflect"
	"strings"
	"testing"
)

// spec internal-drop-corpus: профили исполнителей удалены со всех слоёв —
// в интерфейсе Service нет методов Corpus*.
func TestServiceHasNoCorpusMethods(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeOf((*Service)(nil)).Elem()
	if typ.NumMethod() == 0 {
		t.Fatal("интерфейс Service без методов — тест ничего не проверяет")
	}
	for i := 0; i < typ.NumMethod(); i++ {
		if name := typ.Method(i).Name; strings.HasPrefix(name, "Corpus") {
			t.Errorf("метод профилей %s всё ещё в yue.Service", name)
		}
	}
}
