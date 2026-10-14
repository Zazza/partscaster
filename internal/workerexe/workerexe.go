// Package workerexe — установка и запуск воркера одной программой (partscaster-worker.exe): uv, Python, пакеты,
// код воркера из архива в папку пользователя, затем uvicorn. Здесь — то, что не зависит от консоли.
package workerexe

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	UvVersion = "0.12.19"
	// пакет yue2 (yue2_infer) — не на PyPI: архив коммита YuE, на котором работает воркер
	YueCommit  = "72272f907522dcca2e97c848d6c8f0d343999183"
	TorchIndex = "https://download.pytorch.org/whl/cu128"
	PyPIIndex  = "https://pypi.org/simple"
	Port       = 8091
)

// пакеты сверх requirements.txt. torch — с явной меткой +cu128: на Windows без неё uv берёт сборку
// без CUDA с PyPI, даже когда первым указан индекс cu128
var extraPackages = []string{
	"torch==2.10.0+cu128",
	"torchaudio==2.10.0+cu128",
	"yue2-infer @ https://github.com/multimodal-art-projection/YuE/archive/" + YueCommit + ".zip",
	"truststore",
}

func VerifySHA256(file, want string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != strings.ToLower(want) {
		return fmt.Errorf("%s: sha256 %s, ожидалась %s", filepath.Base(file), got, want)
	}
	return nil
}

func PackagesFingerprint(requirements []byte) string {
	h := sha256.New()
	h.Write(requirements)
	h.Write([]byte("\n" + strings.Join(extraPackages, "\n") + "\n" + TorchIndex))
	return hex.EncodeToString(h.Sum(nil))
}

func fingerprintFile(home string) string { return filepath.Join(home, "packages.sha256") }

func venvPythonHere(home string) string { return VenvPython(home, goosHere) }

func NeedInstall(home string, requirements []byte) bool {
	if _, err := os.Stat(venvPythonHere(home)); err != nil {
		return true
	}
	b, err := os.ReadFile(fingerprintFile(home))
	return err != nil || strings.TrimSpace(string(b)) != PackagesFingerprint(requirements)
}

func MarkInstalled(home string, requirements []byte) error {
	return os.WriteFile(fingerprintFile(home), []byte(PackagesFingerprint(requirements)+"\n"), 0o644)
}

func VenvPython(home, goos string) string {
	if goos == "windows" {
		return filepath.Join(home, "venv", "Scripts", "python.exe")
	}
	return filepath.Join(home, "venv", "bin", "python")
}

func PipInstallArgs(python, requirementsPath string) []string {
	args := []string{"pip", "install", "--python", python, "-r", requirementsPath}
	args = append(args, extraPackages...)
	// --torch-backend: пакеты torch — только из индекса cu128; без него uv ищет сначала в --extra-index-url
	// (PyPI), находит там torch без метки +cu128 и дальше не смотрит
	return append(args, "--index-url", TorchIndex, "--extra-index-url", PyPIIndex, "--torch-backend", "cu128")
}

type GPU struct {
	Name      string
	MemoryMiB int
	Driver    string
}

// ParseGPU разбирает первую строку `nvidia-smi --query-gpu=name,memory.total,driver_version
// --format=csv,noheader,nounits` — воркер работает на первой карте.
func ParseGPU(out string) (GPU, error) {
	line := strings.TrimSpace(strings.SplitN(strings.TrimSpace(out), "\n", 2)[0])
	parts := strings.Split(line, ",")
	if len(parts) != 3 {
		return GPU{}, fmt.Errorf("nvidia-smi: не разобрать %q", line)
	}
	mem, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	name, driver := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[2])
	if err != nil || mem <= 0 || name == "" || driver == "" {
		return GPU{}, fmt.Errorf("nvidia-smi: не разобрать %q", line)
	}
	return GPU{Name: name, MemoryMiB: mem, Driver: driver}, nil
}

// ExtractWorker распаковывает архив воркера (один верхний каталог worker-<версия>/) в dstRoot
// и возвращает путь к этому каталогу. Повторная распаковка той же версии перезаписывает файлы.
func ExtractWorker(r io.Reader, dstRoot string) (string, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return "", err
	}
	defer func() { _ = gz.Close() }() // чтение архива: ошибка закрытия не важна
	tr := tar.NewReader(gz)
	top := ""
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		name := path.Clean(strings.TrimPrefix(hdr.Name, "./"))
		// «\» — разделитель на Windows: «..\» прошёл бы мимо проверки «../» ниже
		if path.IsAbs(hdr.Name) || strings.Contains(hdr.Name, "\\") || name == ".." || strings.HasPrefix(name, "../") {
			return "", fmt.Errorf("архив воркера: путь вне каталога %q", hdr.Name)
		}
		first := strings.SplitN(name, "/", 2)[0]
		if top == "" {
			top = first
		} else if first != top {
			return "", fmt.Errorf("архив воркера: больше одного верхнего каталога (%q, %q)", top, first)
		}
		dst := filepath.Join(dstRoot, filepath.FromSlash(name))
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return "", err
			}
		case tar.TypeReg:
			if err := writeFile(dst, tr, hdr.FileInfo().Mode().Perm()); err != nil {
				return "", err
			}
		case tar.TypeSymlink:
			// ссылки — только у библиотек готовой сборки yue2.cpp под Linux; на Windows она не запускается,
			// а ссылки там требуют прав администратора
			if goosHere == "windows" {
				continue
			}
			target := path.Join(path.Dir(name), hdr.Linkname)
			if path.IsAbs(hdr.Linkname) || !strings.HasPrefix(target, top+"/") {
				return "", fmt.Errorf("архив воркера: ссылка вне каталога %q → %q", hdr.Name, hdr.Linkname)
			}
			_ = os.Remove(dst) // повторная распаковка: прежняя ссылка мешает создать новую
			if err := os.Symlink(hdr.Linkname, dst); err != nil {
				return "", err
			}
		}
	}
	if top == "" || top == "." {
		return "", errors.New("архив воркера пуст")
	}
	return filepath.Join(dstRoot, top), nil
}

func writeFile(dst string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode|0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func WorkerEnv(home, goos string) map[string]string {
	env := map[string]string{
		"PARTSCASTER_DATA_DIR": filepath.Join(home, "data"),
		"HF_HOME":      filepath.Join(home, "hf-cache"),
		"PYTHONUTF8":   "1",
	}
	if goos == "windows" {
		// PyTorch под Windows собран без flash-attention, а режим CUDA-графов YuE2 вызывает её напрямую
		env["YUE2_BACKEND"] = "torch-eager"
	}
	return env
}

// ServeArgs — аргументы python для запуска воркера. truststore — до всего сетевого: модели качаются с
// Hugging Face, и сертификаты берутся из хранилища системы (антивирус или прокси подменяют HTTPS своими).
func ServeArgs(lan bool) []string {
	host := "127.0.0.1"
	if lan {
		host = "0.0.0.0"
	}
	code := fmt.Sprintf("import truststore; truststore.inject_into_ssl(); import uvicorn; "+
		"uvicorn.run('yue_worker:app', host='%s', port=%d)", host, Port)
	return []string{"-c", code}
}

func homeFile(configDir string) string { return filepath.Join(configDir, "partscaster-worker", "home.txt") }

func ResolveHome(configDir, def string) (string, bool) {
	b, err := os.ReadFile(homeFile(configDir))
	if err != nil || strings.TrimSpace(string(b)) == "" {
		return def, false
	}
	return strings.TrimSpace(string(b)), true
}

func SaveHome(configDir, home string) error {
	f := homeFile(configDir)
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		return err
	}
	return os.WriteFile(f, []byte(home+"\n"), 0o644)
}
