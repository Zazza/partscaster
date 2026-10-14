package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Спецификация адреса воркера при старте приложения (config.Load):
//   - PARTSCASTER_URL задан — он главный;
//   - иначе — server_url из settings.json (каталог настроек пользователя / partscaster);
//   - иначе (нет файла, файл битый, адрес пустой) — http://localhost:8091;
//   - SaveSettings сохраняет адрес, следующий Load его видит; каталог создаётся сам.
// Каталог настроек — временный (XDG_CONFIG_HOME/HOME), настоящие настройки не трогаются.

const defaultURL = "http://localhost:8091"

func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("AppData", dir) // Windows: os.UserConfigDir читает %AppData%, а не XDG/HOME
	t.Setenv("PARTSCASTER_URL", "")
	return filepath.Join(dir, "partscaster", "settings.json")
}

func TestLoadDefaultWithoutSettings(t *testing.T) {
	isolate(t)
	if got := Load().YueURL; got != defaultURL {
		t.Fatalf("YueURL %q, want %q", got, defaultURL)
	}
}

func TestSaveThenLoad(t *testing.T) {
	p := isolate(t)
	SaveSettings(Settings{ServerURL: "http://gpu-host:8091"})
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("settings.json не создан в каталоге настроек: %v", err)
	}
	if got := Load().YueURL; got != "http://gpu-host:8091" {
		t.Fatalf("YueURL %q", got)
	}
	SaveSettings(Settings{ServerURL: "http://other:9000"})
	if got := Load().YueURL; got != "http://other:9000" {
		t.Fatalf("после перезаписи YueURL %q", got)
	}
}

func TestEnvWinsOverSettings(t *testing.T) {
	isolate(t)
	SaveSettings(Settings{ServerURL: "http://gpu-host:8091"})
	t.Setenv("PARTSCASTER_URL", "http://from-env:1234")
	if got := Load().YueURL; got != "http://from-env:1234" {
		t.Fatalf("YueURL %q, want из PARTSCASTER_URL", got)
	}
}

func TestEnvWinsWithoutSettings(t *testing.T) {
	isolate(t)
	t.Setenv("PARTSCASTER_URL", "http://from-env:1234")
	if got := Load().YueURL; got != "http://from-env:1234" {
		t.Fatalf("YueURL %q", got)
	}
}

func TestBrokenOrEmptySettingsFallBackToDefault(t *testing.T) {
	for name, body := range map[string]string{
		"битый":        "{not json",
		"пустой адрес": `{"server_url": ""}`,
		"пустой файл":  "",
		"чужие поля":   `{"theme": "dark"}`,
	} {
		t.Run(name, func(t *testing.T) {
			p := isolate(t)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := Load().YueURL; got != defaultURL {
				t.Fatalf("YueURL %q, want %q", got, defaultURL)
			}
		})
	}
}

func TestSaveSettingsUnwritableDoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	// на месте каталога настроек — файл: сохранить нельзя, но и падать нельзя
	blocker := filepath.Join(dir, "partscaster")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("AppData", dir)
	t.Setenv("PARTSCASTER_URL", "")
	SaveSettings(Settings{ServerURL: "http://gpu-host:8091"})
	if got := Load().YueURL; got != defaultURL {
		t.Fatalf("YueURL %q", got)
	}
}

// Спецификация миграции настроек со старого имени проекта:
//   - был yue-studio/settings.json, нового нет — Load видит старый адрес, файл
//     копируется в partscaster/, оригинал остаётся (откат не теряет настройки);
//   - новый уже есть — старый не трогается (последнее сохранение главнее);
//   - старых нет — чистая установка, ничего не происходит.
func TestLegacySettingsMigrate(t *testing.T) {
	p := isolate(t)
	old := filepath.Join(filepath.Dir(filepath.Dir(p)), "yue-studio", "settings.json")
	if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte(`{"server_url": "http://migrated:8091"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load().YueURL; got != "http://migrated:8091" {
		t.Fatalf("после миграции YueURL %q, want старый адрес", got)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("settings.json не скопирован в partscaster: %v", err)
	}
	if string(b) != `{"server_url": "http://migrated:8091"}` {
		t.Fatalf("скопировано не то: %s", b)
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("оригинал удалён — откат потерял бы настройки: %v", err)
	}
	// новый уже есть — миграция не перезаписывает его старым файлом
	if err := os.WriteFile(p, []byte(`{"server_url": "http://fresh:8091"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load().YueURL; got != "http://fresh:8091" {
		t.Fatalf("новый файл главнее: YueURL %q", got)
	}
}
