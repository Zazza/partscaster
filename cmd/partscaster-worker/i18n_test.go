package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"
)

// Тесты карточки internal-installer-en: окно установки и трей воркера на языке системы.

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// ТК1: PARTSCASTER_LANG главнее всего; на не-Windows — LANG/LC_ALL (ru* → ru), пусто → en.
func TestDetectLangNonWindows(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"override en beats LANG ru", map[string]string{"PARTSCASTER_LANG": "en", "LANG": "ru_RU.UTF-8"}, "en"},
		{"override ru beats LANG en", map[string]string{"PARTSCASTER_LANG": "ru", "LANG": "en_US.UTF-8"}, "ru"},
		{"override ru, no LANG", map[string]string{"PARTSCASTER_LANG": "ru"}, "ru"},
		{"LANG ru", map[string]string{"LANG": "ru_RU.UTF-8"}, "ru"},
		{"LANG en", map[string]string{"LANG": "en_US.UTF-8"}, "en"},
		{"LC_ALL ru", map[string]string{"LC_ALL": "ru_RU.UTF-8"}, "ru"},
		{"empty env", map[string]string{}, "en"},
	}
	for _, c := range cases {
		if got := detectLang(envOf(c.env), 0, false); got != c.want {
			t.Errorf("%s: detectLang = %q, want %q", c.name, got, c.want)
		}
	}
}

// ТК1: на Windows — язык интерфейса (LANGID), а не LANG; PARTSCASTER_LANG главнее.
func TestDetectLangWindows(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		id   uint16
		want string
	}{
		{"ru UI", map[string]string{}, 0x0419, "ru"},
		{"en UI", map[string]string{}, 0x0409, "en"},
		{"uk UI", map[string]string{}, 0x0422, "en"},
		{"en UI ignores LANG ru", map[string]string{"LANG": "ru_RU.UTF-8"}, 0x0409, "en"},
		{"override en beats ru UI", map[string]string{"PARTSCASTER_LANG": "en"}, 0x0419, "en"},
		{"override ru beats en UI", map[string]string{"PARTSCASTER_LANG": "ru"}, 0x0409, "ru"},
	}
	for _, c := range cases {
		if got := detectLang(envOf(c.env), c.id, true); got != c.want {
			t.Errorf("%s: detectLang = %q, want %q", c.name, got, c.want)
		}
	}
}

// ТК1: русский — основной язык 0x19 в любом варианте (младшие 10 бит LANGID).
func TestLangFromLANGID(t *testing.T) {
	cases := map[uint16]string{
		0x0419: "ru", // ru-RU
		0x0819: "ru", // ru-MD: основной язык тоже 0x19
		0x0409: "en",
		0x0422: "en", // украинский
		0x0423: "en", // белорусский
		0x0000: "en",
	}
	for id, want := range cases {
		if got := langFromLANGID(id); got != want {
			t.Errorf("langFromLANGID(%#04x) = %q, want %q", id, got, want)
		}
	}
}

func withLang(t *testing.T, l string) {
	t.Helper()
	old := lang
	lang = l
	t.Cleanup(func() { lang = old })
}

// ТК2: tr отдаёт строку текущего языка.
func TestTrFollowsCurrentLang(t *testing.T) {
	withLang(t, "ru")
	if got := tr("Выход", "Quit"); got != "Выход" {
		t.Errorf("ru: tr = %q", got)
	}
	withLang(t, "en")
	if got := tr("Выход", "Quit"); got != "Quit" {
		t.Errorf("en: tr = %q", got)
	}
}

// ТК3: Info().Lang — текущий язык (окно выбирает словарь по нему).
func TestInfoLangMatchesCurrent(t *testing.T) {
	for _, l := range []string{"ru", "en"} {
		withLang(t, l)
		inf := newInstaller(options{}, t.TempDir()).Info()
		if inf.Lang != l {
			t.Errorf("lang=%q: Info().Lang = %q", l, inf.Lang)
		}
		b, err := json.Marshal(inf)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `"lang":"`+l+`"`) {
			t.Errorf("JSON для окна без поля lang=%q: %s", l, b)
		}
	}
}

// ТК5 (Go): русские варианты строк, которые видит человек, — дословно прежние, и идут через tr.
func TestRussianGoStringsUnchanged(t *testing.T) {
	src := map[string]string{}
	for _, f := range []string{"installer.go", "steps.go", "tray.go", "main.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src[f] = string(b)
	}
	want := []struct{ file, ru string }{
		{"installer.go", "Прервать установку?"},
		{"tray.go", "Выход"},
		{"tray.go", "Журнал"},
	}
	for _, w := range want {
		re := regexp.MustCompile(`tr\(\s*"` + regexp.QuoteMeta(w.ru) + `"\s*,\s*"([^"]+)"`)
		m := re.FindStringSubmatch(src[w.file])
		if m == nil {
			t.Errorf("%s: нет tr(%q, …)", w.file, w.ru)
			continue
		}
		if hasCyrillic(m[1]) || strings.TrimSpace(m[1]) == "" {
			t.Errorf("%s: английский вариант для %q — %q", w.file, w.ru, m[1])
		}
		withLang(t, "ru")
		if got := tr(w.ru, m[1]); got != w.ru {
			t.Errorf("ru: tr(%q) = %q", w.ru, got)
		}
	}
}

func hasCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

// extractI18N — текст объектного литерала `const I18N = {...}` из app.js (скобки внутри строк не считаются).
func extractI18N(t *testing.T, js string) (obj string, rest string) {
	t.Helper()
	loc := regexp.MustCompile(`const\s+I18N\s*=\s*\{`).FindStringIndex(js)
	if loc == nil {
		t.Fatal("ui/app.js: нет const I18N = {...}")
	}
	start := loc[1] - 1
	depth, quote := 0, rune(0)
	esc := false
	for i, r := range js[start:] {
		if quote != 0 {
			switch {
			case esc:
				esc = false
			case r == '\\':
				esc = true
			case r == quote:
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"', '`':
			quote = r
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end := start + i + 1
				return js[start:end], js[:loc[0]] + js[end:]
			}
		}
	}
	t.Fatal("ui/app.js: I18N не закрыт")
	return "", ""
}

func loadI18N(t *testing.T) (map[string]map[string]string, string) {
	t.Helper()
	b, err := os.ReadFile("ui/app.js")
	if err != nil {
		t.Fatal(err)
	}
	obj, rest := extractI18N(t, string(b))
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("нет node — словарь не разобрать")
	}
	out, err := exec.Command(node, "-e", "process.stdout.write(JSON.stringify("+obj+"))").Output()
	if err != nil {
		t.Fatalf("node не разобрал I18N: %v", err)
	}
	var d map[string]map[string]any
	if err := json.Unmarshal(out, &d); err != nil {
		t.Fatalf("I18N не JSON-совместим: %v", err)
	}
	res := map[string]map[string]string{}
	for l, m := range d {
		res[l] = map[string]string{}
		for k, v := range m {
			s, ok := v.(string)
			if !ok {
				// функции-шаблоны node в JSON не отдаёт; значение не строка — тоже считаем ключом
				b, _ := json.Marshal(v)
				s = string(b)
			}
			res[l][k] = s
		}
	}
	return res, rest
}

func keysOf(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// ТК4: словари ru и en — одинаковые ключи; в en нет кириллицы.
func TestUIDictionariesMatch(t *testing.T) {
	d, _ := loadI18N(t)
	ru, en := d["ru"], d["en"]
	if len(ru) == 0 || len(en) == 0 {
		t.Fatalf("словари пусты: ru=%d en=%d", len(ru), len(en))
	}
	for _, k := range keysOf(ru) {
		if _, ok := en[k]; !ok {
			t.Errorf("ключ %q есть в ru, нет в en", k)
		}
	}
	for _, k := range keysOf(en) {
		if _, ok := ru[k]; !ok {
			t.Errorf("ключ %q есть в en, нет в ru", k)
		}
		if hasCyrillic(en[k]) {
			t.Errorf("en[%q] с кириллицей: %q", k, en[k])
		}
	}
}

// ТК4 + условие 2: каждый data-i есть в словаре; русский текст разметки — только в элементах с data-i.
func TestIndexHTMLTextsFromDictionary(t *testing.T) {
	d, _ := loadI18N(t)
	b, err := os.ReadFile("ui/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(b)
	keys := regexp.MustCompile(`data-i="([^"]+)"`).FindAllStringSubmatch(html, -1)
	if len(keys) == 0 {
		t.Fatal("ui/index.html: нет ни одного data-i")
	}
	for _, m := range keys {
		if _, ok := d["ru"][m[1]]; !ok {
			t.Errorf("data-i=%q нет в I18N.ru", m[1])
		}
		if _, ok := d["en"][m[1]]; !ok {
			t.Errorf("data-i=%q нет в I18N.en", m[1])
		}
	}
	for _, s := range cyrillicTextOutsideDataI(html) {
		t.Errorf("index.html: русский текст без data-i: %q", s)
	}
}

// cyrillicTextOutsideDataI — куски текста с кириллицей, у которых ни сам элемент, ни предки не несут data-i.
// Комментарии, <script>, <style> и <title> (заголовок окна задаёт программа) не смотрятся.
func cyrillicTextOutsideDataI(html string) []string {
	html = regexp.MustCompile(`(?s)<!--.*?-->`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?is)<(script|style|title)\b.*?</(script|style|title)>`).ReplaceAllString(html, "")
	void := map[string]bool{"img": true, "input": true, "meta": true, "link": true, "br": true, "hr": true}
	tagRe := regexp.MustCompile(`<(/?)([a-zA-Z0-9]+)([^>]*)>`)
	var stack []bool // у элемента (или предка) есть data-i
	var bad []string
	pos := 0
	check := func(text string) {
		if !hasCyrillic(text) {
			return
		}
		if len(stack) > 0 && stack[len(stack)-1] {
			return
		}
		bad = append(bad, strings.Join(strings.Fields(text), " "))
	}
	for _, m := range tagRe.FindAllStringSubmatchIndex(html, -1) {
		check(html[pos:m[0]])
		pos = m[1]
		closing := html[m[2]:m[3]] == "/"
		name := strings.ToLower(html[m[4]:m[5]])
		attrs := html[m[6]:m[7]]
		// атрибуты, видимые человеку, тоже должны переводиться
		for _, a := range regexp.MustCompile(`\b(placeholder|title|alt|aria-label)="([^"]*)"`).FindAllStringSubmatch(attrs, -1) {
			if hasCyrillic(a[2]) {
				bad = append(bad, name+" "+a[1]+"="+a[2])
			}
		}
		if void[name] || strings.HasSuffix(attrs, "/") {
			continue
		}
		if closing {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			continue
		}
		parent := len(stack) > 0 && stack[len(stack)-1]
		stack = append(stack, parent || strings.Contains(attrs, "data-i="))
	}
	check(html[pos:])
	return bad
}

// Условие 2: в app.js вне I18N нет русских строковых литералов (комментарии не считаются).
func TestAppJSNoRussianOutsideDictionary(t *testing.T) {
	_, rest := loadI18N(t)
	for _, s := range cyrillicStringLiterals(rest) {
		t.Errorf("app.js: русская строка вне I18N: %q", s)
	}
}

func cyrillicStringLiterals(js string) []string {
	var out []string
	rs := []rune(js)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == '/' && i+1 < len(rs) && rs[i+1] == '/' {
			for i < len(rs) && rs[i] != '\n' {
				i++
			}
			continue
		}
		if r == '/' && i+1 < len(rs) && rs[i+1] == '*' {
			i += 2
			for i+1 < len(rs) && (rs[i] != '*' || rs[i+1] != '/') {
				i++
			}
			i++
			continue
		}
		if r == '\'' || r == '"' || r == '`' {
			j := i + 1
			for j < len(rs) && rs[j] != r {
				if rs[j] == '\\' {
					j++
				}
				j++
			}
			if j > len(rs) {
				j = len(rs)
			}
			lit := string(rs[i+1 : min(j, len(rs))])
			if hasCyrillic(lit) {
				out = append(out, lit)
			}
			i = j
		}
	}
	return out
}

// ТК5 (UI): русский словарь содержит прежние тексты кнопок дословно.
func TestRussianUIStringsUnchanged(t *testing.T) {
	d, _ := loadI18N(t)
	have := map[string]bool{}
	for _, v := range d["ru"] {
		have[v] = true
	}
	for _, s := range []string{"Далее", "Установить", "Закрыть"} {
		if !have[s] {
			t.Errorf("в I18N.ru нет значения %q", s)
		}
	}
}
