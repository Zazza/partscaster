// ffmpeg для приложения и MCP: где взять и при необходимости скачать. Перемотка плеера и эффекты
// студии (internal/dsp) запускают ffmpeg локально — на чистой машине его нет, и всё молча не работало.
// Приложение показывает окно «скачать и установить»: закреплённая сборка качается в папку данных
// пользователя (в систему ничего не прописывается), при наличии в PATH берётся системный.
package ffmpegbin

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// закреплённая сборка (BtbN, GPL-вариант: в LGPL нет фильтра rubberband — октавер и высота тона
// в студии на ней падают); скачивает пользователь (приложение не распространяет бинарь), ассеты
// GitHub постоянны, версия фиксирована — обновление осознанной сменой этих строк. var — для подмены в тестах
var (
	URL    = "https://github.com/BtbN/FFmpeg-Builds/releases/download/autobuild-2026-10-21-13-06/ffmpeg-n9.0.2-31-gc89d6ee1ca-win64-gpl-shared-9.0.zip"
	SHA256 = "242f3b24335b04511de006594b685d19bc3a46538e086322beb5797f91554413"
	Size   = 86585710 // приблизительно: только для полосы прогресса до заголовка ответа
)

var (
	once sync.Once
	mu   sync.Mutex // установка одна; Path читает кэш без блокировки (файл не двигается)

	resolved string
)

// Status — где приложение возьмёт ffmpeg: source «own» (своя папка), «path» (PATH системы),
// ок=false — нет нигде (Windows: предложить установить; unix: системный пакет).
type Status struct {
	Ok     bool   `json:"ok"`
	Path   string `json:"path"`
	Source string `json:"source"` // own | path | env
}

func Get() Status {
	if p := os.Getenv("PARTSCASTER_FFMPEG"); p != "" {
		if fileExists(p) {
			return Status{Ok: true, Path: p, Source: "env"}
		}
	}
	// PATH прежде своей папки: у кого стоит свой ffmpeg (часто сборка полнее) — не подменяем;
	// своя папка — дополнение для машин без ffmpeg
	if p, err := execLookPath(); err == nil {
		return Status{Ok: true, Path: p, Source: "path"}
	}
	if p := own(); fileExists(p) {
		return Status{Ok: true, Path: p, Source: "own"}
	}
	return Status{}
}

// Path — путь к ffmpeg для запуска (player.go, internal/dsp): PATH → своя папка → имя «ffmpeg»
// (запуск упадёт с внятной ошибкой exec, которую вызывающий покажет).
func Path() string {
	if s := Get(); s.Ok {
		return s.Path
	}
	return "ffmpeg"
}

// Dir — своя папка ffmpeg: <данные пользователя>/partscaster/tools/ffmpeg/bin.
func Dir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "partscaster", "tools", "ffmpeg", "bin")
}

func own() string {
	d := Dir()
	if d == "" {
		return ""
	}
	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name = "ffmpeg.exe"
	}
	return filepath.Join(d, name)
}

// Progress — ход установки для окна приложения (полоса «из N байт»).
type Progress struct {
	Downloading bool  `json:"downloading"`
	Done        int64 `json:"done"`
	Total       int64 `json:"total"`
}

var (
	progressMu sync.Mutex
	progress   Progress
)

func Snapshot() Progress {
	progressMu.Lock()
	defer progressMu.Unlock()
	return progress
}

func setProgress(p Progress) {
	progressMu.Lock()
	progress = p
	progressMu.Unlock()
}

// Install — скачать закреплённую сборку и положить bin/ (без ffplay/ffprobe) в свою папку.
// Уже установлено (файл с верной суммой) — без сети. Ошибка — целиком: папка не меняется.
func Install() error {
	mu.Lock()
	defer mu.Unlock()
	if s := Get(); s.Ok { // уже есть (PATH/своё/переменная) — без сети
		return nil
	}
	setProgress(Progress{Downloading: true})
	defer setProgress(Progress{})
	dir := Dir()
	if dir == "" {
		return errors.New("не определить папку данных пользователя")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := dir + ".part"
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	zipPath := filepath.Join(tmp, "ffmpeg.zip")
	if err := download(zipPath); err != nil {
		return err
	}
	if err := extractBin(zipPath, tmp); err != nil {
		return err
	}
	bin := filepath.Join(tmp, "bin")
	if !fileExists(filepath.Join(bin, filepath.Base(own()))) {
		return errors.New("в сборке нет ffmpeg")
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.Rename(bin, dir)
}

func download(dst string) error {
	req, err := http.NewRequest(http.MethodGet, URL, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 30 * time.Minute}).Do(req)
	if err != nil {
		return fmt.Errorf("скачивание ffmpeg: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("скачивание ffmpeg: код %d", resp.StatusCode)
	}
	total := int64(Size)
	if l := resp.ContentLength; l > 0 {
		total = l
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	h := sha256.New()
	var done int64
	buf := make([]byte, 1<<20)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				return werr
			}
			h.Write(buf[:n])
			done += int64(n)
			setProgress(Progress{Downloading: true, Done: done, Total: total})
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return fmt.Errorf("скачивание ffmpeg: %w", rerr)
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != SHA256 {
		return fmt.Errorf("ffmpeg: контрольная сумма не совпала (%s)", got[:16])
	}
	return nil
}

// extractBin — из архива только bin/ и без плееров: остальное (доки, исходники include) не нужно.
func extractBin(zipPath, dst string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		name := filepath.ToSlash(f.Name)
		i := strings.Index(name, "/bin/")
		if i < 0 || strings.HasSuffix(name, "/") {
			continue
		}
		base := name[i+len("/bin/"):]
		if base != filepath.ToSlash(filepath.Base(base)) || base == "ffplay.exe" || base == "ffprobe.exe" {
			continue
		}
		out := filepath.Join(dst, "bin", filepath.FromSlash(base))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		w, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(w, rc)
		rc.Close()
		w.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
