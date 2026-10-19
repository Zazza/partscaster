// Модель генерации (yue2.cpp) при установке partscaster-worker. Бинарник yue-server приходит из архива воркера
// (собирает релиз), веса скачивает сам воркер (worker/engine_cpp.py download), пороги рекомендуемой модели —
// тоже в engine_cpp.py (их читает launcher.go); здесь — пути, команды и запись настроек.
package workerexe

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// CppModels — модели yue2.cpp (файл-манифест весов — worker/engine_cpp.py MODELS; порядок = меню).
var CppModels = []string{"bf16", "q8", "q6", "q5"}

// SettingsFile — настройки воркера (cpp_model/cpp_low_vram читает yue_worker._load_settings).
func SettingsFile(home string) string { return filepath.Join(home, "data", "settings.json") }

// WriteSettings атомарно пишет settings.json: существующие ключи сохраняются, переданные — перекрывают,
// переданные со значением nil — удаляются.
func WriteSettings(home string, values map[string]any) error {
	p := SettingsFile(home)
	current := readSettings(home)
	for k, v := range values {
		if v == nil {
			delete(current, k)
			continue
		}
		current[k] = v
	}
	b, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func readSettings(home string) map[string]any {
	current := map[string]any{}
	if b, err := os.ReadFile(SettingsFile(home)); err == nil {
		_ = json.Unmarshal(b, &current) // битый файл — начинаем с пустого
	}
	return current
}

// SettingsModel — выбранная модель генерации ("" — не выбрана: новая установка или установка прежних версий
// с движком PyTorch).
func SettingsModel(home string) string {
	m, _ := readSettings(home)["cpp_model"].(string)
	return m
}

// SettingsLowVram — выбран экономный режим.
func SettingsLowVram(home string) bool {
	v, _ := readSettings(home)["cpp_low_vram"].(bool)
	return v
}

// CppBinary — бинарник yue2.cpp из архива воркера, "" — в этом пакете его нет (пакет собран неправильно).
func CppBinary(workerDir, goos string) string {
	exe := ""
	if goos == "windows" {
		exe = ".exe"
	}
	p := filepath.Join(workerDir, "yue2cpp", "yue-server"+exe)
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p
	}
	return ""
}

// EngineCppCommand — `python engine_cpp.py <args>` из venv в каталоге кода воркера, с окружением воркера.
func EngineCppCommand(home, workerDir string, args ...string) *exec.Cmd {
	cmd := exec.Command(VenvPython(home, goosHere), append([]string{"engine_cpp.py"}, args...)...)
	cmd.Dir = workerDir
	HideConsole(cmd)
	cmd.Env = os.Environ()
	for k, v := range WorkerEnv(home, goosHere) {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	return cmd
}

// ModelInstalledCommand — код выхода 0, если модель и VAE уже скачаны.
func ModelInstalledCommand(home, workerDir, model string) *exec.Cmd {
	return EngineCppCommand(home, workerDir, "installed", model)
}

// DownloadEngineModel — скачать веса модели+VAE (engine_cpp.py download) в консоль out.
// pythonFromVenv — готовая команда питона из venv (VenvPython); WorkerEnv задаёт PARTSCASTER_DATA_DIR/HF_HOME.
func DownloadEngineModel(home, workerDir, model string) *exec.Cmd {
	code := "import truststore; truststore.inject_into_ssl(); import sys, engine_cpp; " + // сертификаты системы — до сети
		"engine_cpp.download(sys.argv[1], lambda f, d, t: print(f'{f}: {d}/{t}', flush=True))"
	cmd := exec.Command(VenvPython(home, goosHere), "-c", code, model)
	cmd.Dir = workerDir
	HideConsole(cmd)
	cmd.Env = os.Environ()
	for k, v := range WorkerEnv(home, goosHere) {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	return cmd
}

// ValidCppModel — модель из манифеста.
func ValidCppModel(m string) bool {
	for _, c := range CppModels {
		if c == m {
			return true
		}
	}
	return false
}

// FormatGB — мегабайты в «N ГБ» для подсказок.
func FormatGB(mib int) string { return fmt.Sprintf("%d ГБ", mib/1024) }
