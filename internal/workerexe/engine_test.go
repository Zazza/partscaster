package workerexe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Спецификация модели генерации partscaster-worker.exe (карточки internal-partscaster-worker-exe, ТК9–ТК11,
// и internal-cpp-only):
//   - настройки воркера пишутся атомарно, существующие ключи сохраняются, ключ со значением nil удаляется (TК9);
//   - выбранная модель и экономный режим читаются из settings.json; нет файла/ключа — не выбрана (ТК10);
//   - бинарник yue2.cpp ищется в каталоге кода воркера: yue2cpp/yue-server(.exe на Windows) (ТК11);
//   - рекомендуемая модель — у воркера (engine_cpp.py recommend, recommend_test.go); своих порогов здесь нет;
//   - имя модели — из манифеста bf16/q8/q6/q5.

// --- ТК9: WriteSettings ---

func TestWriteSettingsCreatesAndMerges(t *testing.T) {
	home := t.TempDir()
	if err := WriteSettings(home, map[string]any{"engine": "torch", "stems_model": "roformer", "cpp_model": "q6"}); err != nil {
		t.Fatal(err)
	}
	if SettingsModel(home) != "q6" || SettingsLowVram(home) {
		t.Fatalf("модель/режим = %q/%v, want q6/false", SettingsModel(home), SettingsLowVram(home))
	}
	// второй запуск перекрывает переданное, удаляет nil и сохраняет остальное
	if err := WriteSettings(home, map[string]any{"engine": nil, "cpp_model": "bf16", "cpp_low_vram": true}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(SettingsFile(home))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{`"stems_model": "roformer"`, `"cpp_model": "bf16"`, `"cpp_low_vram": true`} {
		if !strings.Contains(got, want) {
			t.Fatalf("settings.json = %s, без %s", got, want)
		}
	}
	if strings.Contains(got, `"engine"`) {
		t.Fatalf("settings.json = %s: ключ engine не удалён", got)
	}
}

func TestWriteSettingsCreatesDataDir(t *testing.T) {
	home := filepath.Join(t.TempDir(), "nested", "home")
	if err := WriteSettings(home, map[string]any{"cpp_model": "q8"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "data", "settings.json")); err != nil {
		t.Fatal(err)
	}
}

// --- ТК10: выбранная модель ---

func TestSettingsModelEmptyOnFreshHome(t *testing.T) {
	home := t.TempDir()
	if m := SettingsModel(home); m != "" {
		t.Fatalf("в чистой папке выбрана модель %q", m)
	}
	// установка прежних версий: движок PyTorch, модели нет
	if err := WriteSettings(home, map[string]any{"engine": "torch"}); err != nil {
		t.Fatal(err)
	}
	if m := SettingsModel(home); m != "" {
		t.Fatalf("у установки на PyTorch выбрана модель %q", m)
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
