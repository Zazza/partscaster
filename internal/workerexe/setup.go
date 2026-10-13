package workerexe

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var goosHere = runtime.GOOS

type uvAsset struct{ url, sha256 string }

// сборки uv с GitHub; Linux — чтобы программу можно было прогнать и там, без GPU-машины с Windows
var uvAssets = map[string]uvAsset{
	"windows/amd64": {"https://github.com/astral-sh/uv/releases/download/" + UvVersion + "/uv-x86_64-pc-windows-msvc.zip",
		"6dbb02d79e419522f1c500f0adb1cddcff0cda7d59b0d66ea7f5e3b4a1b2f5f0"},
	"linux/amd64": {"https://github.com/astral-sh/uv/releases/download/" + UvVersion + "/uv-x86_64-unknown-linux-gnu.tar.gz",
		"23bf5552d220e0842b65c862097b2ebaeba0064b74eda5e565e77fd25969d8c8"},
}

func exeName(name string) string {
	if goosHere == "windows" {
		return name + ".exe"
	}
	return name
}

// ErrHomeBusy — папкой уже занят другой запуск yue-worker.
var ErrHomeBusy = errors.New("в этой папке уже работает yue-worker")

// LockHome занимает папку установки на всё время работы программы: второй запуск (двойной щелчок во время
// долгой установки) иначе ставил бы пакеты в то же окружение одновременно с первым. Отпускается закрытием
// файла — или системой при выходе процесса.
func LockHome(home string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(home, "yue-worker.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		f.Close()
		if lockBusy(err) {
			return nil, ErrHomeBusy
		}
		return nil, fmt.Errorf("блокировка папки %s: %w", home, err)
	}
	return f, nil
}

// UvEnv — окружение для uv: Python, кеш пакетов — в папке установки; сертификаты — из хранилища системы.
func UvEnv(home string) []string {
	return append(os.Environ(),
		"UV_PYTHON_INSTALL_DIR="+filepath.Join(home, "python"),
		"UV_CACHE_DIR="+filepath.Join(home, "uv-cache"),
		"UV_SYSTEM_CERTS=1",
	)
}

// EnsureUv скачивает uv закреплённой версии в home/uv (с проверкой sha256), если его там нет.
func EnsureUv(home string) (string, error) {
	uv := filepath.Join(home, "uv", exeName("uv"))
	if _, err := os.Stat(uv); err == nil {
		return uv, nil
	}
	a, ok := uvAssets[goosHere+"/"+runtime.GOARCH]
	if !ok {
		return "", fmt.Errorf("нет сборки uv для %s/%s", goosHere, runtime.GOARCH)
	}
	if err := os.MkdirAll(filepath.Join(home, "uv"), 0o755); err != nil {
		return "", err
	}
	tmp := filepath.Join(home, "uv", "download.part")
	defer os.Remove(tmp)
	if err := download(a.url, tmp); err != nil {
		return "", err
	}
	if err := VerifySHA256(tmp, a.sha256); err != nil {
		return "", err
	}
	// через .part: оборванная распаковка (закрыли окно) не оставит обрезанный uv, который потом считался бы готовым
	if err := extractUv(tmp, strings.HasSuffix(a.url, ".zip"), uv+".part"); err != nil {
		return "", err
	}
	return uv, os.Rename(uv+".part", uv)
}

func download(url, dst string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// extractUv достаёт из архива сборки только сам uv (рядом лежит uvx — он не нужен).
func extractUv(archive string, isZip bool, dst string) error {
	want := exeName("uv")
	if isZip {
		zr, err := zip.OpenReader(archive)
		if err != nil {
			return err
		}
		defer func() { _ = zr.Close() }() // всё здесь — чтение: ошибки закрытия не важны
		for _, f := range zr.File {
			if filepath.Base(f.Name) != want {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer func() { _ = rc.Close() }()
			return writeFile(dst, rc, 0o755)
		}
		return fmt.Errorf("в архиве uv нет %s", want)
	}
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("в архиве uv нет %s", want)
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag == tar.TypeReg && filepath.Base(hdr.Name) == want {
			return writeFile(dst, tr, 0o755)
		}
	}
}

// InstallPackages ставит Python 3.12 и пакеты воркера в home/venv и запоминает отпечаток.
func InstallPackages(home, uv, requirementsPath string, out io.Writer) error {
	req, err := os.ReadFile(requirementsPath)
	if err != nil {
		return err
	}
	py := venvPythonHere(home)
	if _, err := os.Stat(py); err != nil {
		if err := run(out, UvEnv(home), "", uv, "venv", filepath.Join(home, "venv"), "--python", "3.12"); err != nil {
			return err
		}
	}
	if err := run(out, UvEnv(home), "", uv, PipInstallArgs(py, requirementsPath)...); err != nil {
		return err
	}
	return MarkInstalled(home, req)
}

func run(out io.Writer, env []string, dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env, cmd.Dir, cmd.Stdout, cmd.Stderr = env, dir, out, out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", filepath.Base(name), args[0], err)
	}
	return nil
}

// ServeCommand — процесс воркера: python из venv в каталоге кода воркера, с его окружением.
func ServeCommand(home, workerDir string, lan bool) *exec.Cmd {
	cmd := exec.Command(venvPythonHere(home), ServeArgs(lan)...)
	cmd.Dir = workerDir
	cmd.Env = os.Environ()
	for k, v := range WorkerEnv(home, goosHere) {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	return cmd
}
