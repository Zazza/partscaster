package workerexe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Спецификация выбора движка partscaster-worker.exe (карточка internal-partscaster-worker-exe, ТК9–ТК12):
//   - настройки воркера пишутся атомарно, существующие ключи сохраняются (TК9);
//   - settings.json в папке установки — признак «выбор уже сделан» (ТК10);
//   - бинарник yue2.cpp ищется в каталоге кода воркера: yue2cpp/yue-server(.exe на Windows) (ТК11);
//   - модель по умолчанию и экономный режим — по объёму карты, пороги как у install.sh:
//     ≥ 11776 МиБ — q8 обычный, 7680…11775 — q8 экономный, меньше — q5 экономный (ТК12, internal-fix-found п.2);
//   - имя модели — из манифеста bf16/q8/q6/q5.

// --- ТК9: WriteSettings ---

func TestWriteSettingsCreatesAndMerges(t *testing.T) {
	home := t.TempDir()
	if err := WriteSettings(home, map[string]any{"engine": "cpp", "cpp_model": "q6", "cpp_low_vram": true}); err != nil {
		t.Fatal(err)
	}
	if !SettingsExist(home) {
		t.Fatal("settings.json не появился")
	}
	// второй запуск перекрывает переданное, сохраняя остальное
	if err := WriteSettings(home, map[string]any{"engine": "torch"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(SettingsFile(home))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{`"engine": "torch"`, `"cpp_model": "q6"`, `"cpp_low_vram": true`} {
		if !strings.Contains(got, want) {
			t.Fatalf("settings.json = %s, без %s", got, want)
		}
	}
}

func TestWriteSettingsCreatesDataDir(t *testing.T) {
	home := filepath.Join(t.TempDir(), "nested", "home")
	if err := WriteSettings(home, map[string]any{"engine": "torch"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "data", "settings.json")); err != nil {
		t.Fatal(err)
	}
}

// --- ТК10: SettingsExist ---

func TestSettingsExistFalseOnFreshHome(t *testing.T) {
	if SettingsExist(t.TempDir()) {
		t.Fatal("в чистой папке настройки уже есть")
	}
}

// --- ТК11: CppBinary ---

func TestCppBinary(t *testing.T) {
	for _, tc := range []struct{ goos, name string }{
		{"windows", "yue-server.exe"},
		{"linux", "yue-server"},
	} {
		dir := t.TempDir()
		if got := CppBinary(dir, tc.goos); got != "" {
			t.Fatalf("%s: в пустом каталоге нашёлся %s", tc.goos, got)
		}
		p := filepath.Join(dir, "yue2cpp", tc.name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := CppBinary(dir, tc.goos); got == "" {
			t.Fatalf("%s: бинарник не найден", tc.goos)
		}
		// имя другой ОС не находится
		other := "linux"
		if tc.goos == "linux" {
			other = "windows"
		}
		if got := CppBinary(dir, other); got != "" {
			t.Fatalf("%s: найден бинарник чужой ОС", other)
		}
	}
}

// --- ТК12: дефолты по видеокарте ---

// Пороги — как у install.sh (карточка internal-fix-found, условие 2):
// ≥ 11776 МиБ — q8 обычный; 7680…11775 — q8 экономный; меньше — q5 экономный.
var vramCases = []struct {
	mib  int
	want string
	low  bool
}{
	{6 * 1024, "q5", true},
	{7679, "q5", true},
	{7680, "q8", true},
	{11775, "q8", true},
	{11776, "q8", false},
	{16 * 1024, "q8", false},
	{24 * 1024, "q8", false},
}

func TestDefaultCppModelByVRAM(t *testing.T) {
	for _, tc := range vramCases {
		if got := DefaultCppModel(tc.mib); got != tc.want {
			t.Errorf("DefaultCppModel(%d МиБ) = %s, want %s", tc.mib, got, tc.want)
		}
	}
}

func TestLowVramDefault(t *testing.T) {
	for _, tc := range vramCases {
		if got := LowVramDefault(tc.mib); got != tc.low {
			t.Errorf("LowVramDefault(%d МиБ) = %v, want %v", tc.mib, got, tc.low)
		}
	}
}

func TestValidCppModel(t *testing.T) {
	for _, m := range CppModels {
		if !ValidCppModel(m) {
			t.Errorf("%s из манифеста не валиден", m)
		}
	}
	for _, m := range []string{"", "q7", "Q8", "int8"} {
		if ValidCppModel(m) {
			t.Errorf("%q не должен быть валиден", m)
		}
	}
}
