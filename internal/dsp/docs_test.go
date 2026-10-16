package dsp

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// effectsDocs — справочник эффектов для пользователя на двух языках: каждая цепочка там описана
// строкой с её ID в виде (`id`), число цепочек — фразой своего языка.
var effectsDocs = []struct{ path, total string }{
	{"../../docs/ru/effects.md", "Всего %d цепоч"},
	{"../../docs/en/effects.md", "Total: %d chains"},
}

// Новая цепочка без описания в справочнике (любого языка) — тест падает: справочник не
// отстаёт от кода. Число цепочек в тексте — тоже.
func TestEffectsDocListsEveryChain(t *testing.T) {
	t.Parallel()
	for _, d := range effectsDocs {
		raw, err := os.ReadFile(d.path)
		if err != nil {
			t.Fatalf("справочник эффектов: %v", err)
		}
		doc := string(raw)
		for _, c := range All() {
			if !strings.Contains(doc, "(`"+c.ID+"`)") {
				t.Errorf("цепочка %s («%s») не описана в %s — добавь строку с (`%s`)", c.ID, c.Name, d.path, c.ID)
			}
		}
		if want := fmt.Sprintf(d.total, len(All())); !strings.Contains(doc, want) {
			t.Errorf("в %s нет «%s…»: число цепочек в тексте устарело", d.path, want)
		}
	}
}
