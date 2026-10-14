// Выбор движка генерации при установке partscaster-worker: yue2.cpp (меньше видеопамяти) вместо основного
// PyTorch. Бинарник yue-server приходит из архива воркера (собирает релиз), веса скачивает сам воркер
// (worker/engine_cpp.py download) — здесь только пути, дефолты по видеокарте и запись настроек.
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

// SettingsFile — настройки воркера (engine/cpp_model/cpp_low_vram читает yue_worker._load_settings).
func SettingsFile(home string) string { return filepath.Join(home, "data", "settings.json") }

// WriteSettings атомарно пишет settings.json: существующие ключи сохраняются, переданные — перекрывают.
func WriteSettings(home string, values map[string]any) error {
	p := SettingsFile(home)
	current := map[string]any{}
	if b, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(b, &current) // битый файл — начинаем с пустого
	}
	for k, v := range values {
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

// SettingsExist — установка уже была: движок выбрали, вопросов больше не задаём.
func SettingsExist(home string) bool {
	_, err := os.Stat(SettingsFile(home))
	return err == nil
}

// CppBinary — бинарник yue2.cpp из архива воркера, nil — в этом пакете движка нет.
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

// DefaultCppModel — модель по объёму карты (как install_worker auto): q8 «почти без потерь» — на
// просторных картах; тесным (8 ГБ: q8 ≈ 8 ГБ VRAM впритык) — q6.
func DefaultCppModel(vramMiB int) string {
	if vramMiB < 10*1024 {
		return "q6"
	}
	return "q8"
}

// LowVramDefault — экономный режим (контекст 8192) для тесных карт.
func LowVramDefault(vramMiB int) bool { return vramMiB < 10*1024 }

// DownloadEngineModel — скачать веса модели+VAE (engine_cpp.py download) в консоль out.
// pythonFromVenv — готовая команда питона из venv (VenvPython); WorkerEnv задаёт PARTSCASTER_DATA_DIR/HF_HOME.
func DownloadEngineModel(home, workerDir, model string) *exec.Cmd {
	code := "import truststore; truststore.inject_into_ssl(); import sys, engine_cpp; " + // сертификаты системы — до сети
		"engine_cpp.download(sys.argv[1], lambda f, d, t: print(f'{f}: {d}/{t}', flush=True))"
	cmd := exec.Command(VenvPython(home, goosHere), "-c", code, model)
	cmd.Dir = workerDir
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
