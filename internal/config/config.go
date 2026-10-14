package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

type Config struct {
	YueURL string
}

func Load() *Config {
	return &Config{
		YueURL: env("PARTSCASTER_URL", loadSettings().ServerURL),
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	if def == "" {
		return "http://localhost:8091"
	}
	return def
}

// Settings — сохраняемые настройки приложения (settings.json в UserConfigDir).
type Settings struct {
	ServerURL string `json:"server_url"`
}

func settingsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "partscaster", "settings.json")
}

// migrateLegacySettings — разовый перенос настроек со старого имени проекта:
// до переименования файл лежал в «yue-studio». Копируем, если нового ещё нет;
// оригинал не удаляем — откат на старую версию не теряет настройки.
func migrateLegacySettings(p string) {
	if p == "" {
		return
	}
	if _, err := os.Stat(p); err == nil {
		return // уже есть — ничего не делаем
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	old := filepath.Join(dir, "yue-studio", "settings.json")
	b, err := os.ReadFile(old)
	if err != nil {
		return // старых настроек нет — чистая установка
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		log.Printf("settings: migrate mkdir: %v", err)
		return
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		log.Printf("settings: migrate copy: %v", err)
		return
	}
	log.Printf("settings: перенесены из %s", old)
}

func loadSettings() Settings {
	var s Settings
	p := settingsPath()
	if p == "" {
		return s
	}
	migrateLegacySettings(p)
	b, err := os.ReadFile(p)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(b, &s) // битый файл настроек — дефолты
	return s
}

func SaveSettings(s Settings) {
	p := settingsPath()
	if p == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		log.Printf("settings: mkdir: %v", err)
		return
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		log.Printf("settings: save: %v", err)
	}
}
