// Разделение на дорожки (RoFormer, worker/sep_run.py) — своё окружение в папке установки: python-audio-separator
// тянет свой torch/onnxruntime, в окружении воркера они бы конфликтовали. Ставится всегда, вместе с воркером.
package workerexe

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SepPackages — пакеты окружения разделения (0.47.0 проверен на GPU-машине; = SEP_PACKAGES в worker/install.sh);
// imageio-ffmpeg — ffmpeg внутри колеса: audio-separator без программы ffmpeg не работает, а на Windows её нет.
var SepPackages = []string{"audio-separator[gpu]==0.47.0", "audioread", "imageio-ffmpeg==0.6.0"}

func sepVenv(home string) string { return filepath.Join(home, "sep-venv") }

// SepModelsDir — веса RoFormer/DrumSep.
func SepModelsDir(home string) string { return filepath.Join(home, "sep-models") }

// SepPython — python окружения разделения на ОС goos.
func SepPython(home, goos string) string {
	if goos == "windows" {
		return filepath.Join(sepVenv(home), "Scripts", "python.exe")
	}
	return filepath.Join(sepVenv(home), "bin", "python")
}

func sepFingerprintFile(home string) string { return filepath.Join(home, "sep-packages.sha256") }

func sepFingerprint() string {
	h := sha256.Sum256([]byte(strings.Join(SepPackages, "\n") + "\n" + TorchIndex))
	return hex.EncodeToString(h[:])
}

// NeedSepInstall — окружения разделения нет или набор его пакетов изменился.
func NeedSepInstall(home string) bool {
	if _, err := os.Stat(SepPython(home, goosHere)); err != nil {
		return true
	}
	b, err := os.ReadFile(sepFingerprintFile(home))
	return err != nil || strings.TrimSpace(string(b)) != sepFingerprint()
}

// InstallSep ставит окружение разделения в home/sep-venv (torch — с CUDA, из индекса cu128, как у воркера)
// и запоминает отпечаток; сбой — отпечаток не пишется, повторный запуск поставит заново.
func InstallSep(home, uv string, out io.Writer) error {
	py := SepPython(home, goosHere)
	if _, err := os.Stat(py); err != nil {
		if err := run(out, UvEnv(home), "", uv, "venv", sepVenv(home), "--python", "3.12"); err != nil {
			return err
		}
	}
	args := append([]string{"pip", "install", "--python", py}, SepPackages...)
	args = append(args, "--index-url", TorchIndex, "--extra-index-url", PyPIIndex, "--torch-backend", "cu128")
	if err := run(out, UvEnv(home), "", uv, args...); err != nil {
		return err
	}
	return os.WriteFile(sepFingerprintFile(home), []byte(sepFingerprint()+"\n"), 0o644)
}

// SepReadyCommand — код 0, если веса разделения на месте целиком (sep_run.py ready; без сети и видеокарты).
func SepReadyCommand(home, workerDir string) *exec.Cmd {
	cmd := exec.Command(SepPython(home, goosHere), "sep_run.py", "ready", SepModelsDir(home))
	cmd.Dir = workerDir
	HideConsole(cmd)
	return cmd
}

// SepDownloadCommand — скачать веса обеих моделей (sep_run.py download) без видеокарты; есть — не качает.
func SepDownloadCommand(home, workerDir string) *exec.Cmd {
	cmd := exec.Command(SepPython(home, goosHere), "sep_run.py", "download", SepModelsDir(home))
	cmd.Dir = workerDir
	HideConsole(cmd)
	cmd.Env = os.Environ()
	for k, v := range WorkerEnv(home, goosHere) {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	return cmd
}
