package dsp

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
	"unicode"
)

// ТК1 internal-dsp-en: английские словари фронтенда (frontend/src/dspI18n.js) покрывают ровно
// цепочки All(), наборы Presets() и все уникальные подписи ручек цепочек; переводы непустые и без кириллицы.
// Словари читаются через node; без node тест пропускается.

type dspEnDicts struct {
	C map[string][]string `json:"c"`
	P map[string][]string `json:"p"`
	L map[string]string   `json:"l"`
}

func loadDspEnDicts(t *testing.T) dspEnDicts {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден: словари frontend/src/dspI18n.js читаются через node")
	}
	abs, err := filepath.Abs(filepath.Join("..", "..", "frontend", "src", "dspI18n.js"))
	if err != nil {
		t.Fatal(err)
	}
	js := "import(" + jsonString(t, "file://"+filepath.ToSlash(abs)) +
		").then(m=>console.log(JSON.stringify({c:m.CHAIN_EN,p:m.DSP_PRESET_EN,l:m.LABEL_EN})))"
	out, err := exec.Command(node, "--input-type=module", "-e", js).Output()
	if err != nil {
		t.Fatalf("node не прочитал dspI18n.js: %v", err)
	}
	var d dspEnDicts
	if err := json.Unmarshal(out, &d); err != nil {
		t.Fatalf("вывод node не JSON словарей: %v\n%s", err, out)
	}
	return d
}

func jsonString(t *testing.T, s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func hasCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

// sameKeys — ключи словаря совпадают с want: печатает недостающие и лишние.
func sameKeys[V any](t *testing.T, what string, dict map[string]V, want map[string]bool) {
	t.Helper()
	var missing, extra []string
	for k := range want {
		if _, ok := dict[k]; !ok {
			missing = append(missing, k)
		}
	}
	for k := range dict {
		if !want[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 || len(extra) > 0 {
		t.Errorf("%s: нет перевода для %q; лишние ключи %q", what, missing, extra)
	}
}

func checkPair(t *testing.T, what, id string, v []string) {
	t.Helper()
	if len(v) != 2 {
		t.Errorf("%s[%q]: ожидалось [name, note], получено %q", what, id, v)
		return
	}
	for i, s := range v {
		if s == "" || hasCyrillic(s) {
			t.Errorf("%s[%q][%d] = %q: пусто или кириллица", what, id, i, s)
		}
	}
}

func TestI18nEnChainsCoverAll(t *testing.T) {
	d := loadDspEnDicts(t)
	want := map[string]bool{}
	for _, c := range All() {
		want[c.ID] = true
	}
	if len(want) == 0 {
		t.Fatal("All() пуст")
	}
	sameKeys(t, "CHAIN_EN", d.C, want)
	for id, v := range d.C {
		checkPair(t, "CHAIN_EN", id, v)
	}
}

func TestI18nEnPresetsCoverAll(t *testing.T) {
	d := loadDspEnDicts(t)
	want := map[string]bool{}
	for _, p := range Presets() {
		want[p.ID] = true
	}
	if len(want) == 0 {
		t.Fatal("Presets() пуст")
	}
	sameKeys(t, "DSP_PRESET_EN", d.P, want)
	for id, v := range d.P {
		checkPair(t, "DSP_PRESET_EN", id, v)
	}
}

func TestI18nEnLabelsCoverAllParams(t *testing.T) {
	d := loadDspEnDicts(t)
	want := map[string]bool{}
	for _, c := range All() {
		for _, p := range c.Params {
			want[p.Label] = true
		}
	}
	if len(want) == 0 {
		t.Fatal("у цепочек нет ни одной подписи ручки")
	}
	var missing []string
	for l := range want {
		if _, ok := d.L[l]; !ok {
			missing = append(missing, l)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("LABEL_EN: нет перевода для %d подписей: %q", len(missing), missing)
	}
	for ru, en := range d.L {
		if en == "" || hasCyrillic(en) {
			t.Errorf("LABEL_EN[%q] = %q: пусто или кириллица", ru, en)
		}
	}
}
