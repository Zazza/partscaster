// Запуск воркера на Windows без консоли: этапы установки для окна, настройки запуска (доступ из сети), журнал,
// состояние для значка в трее. Всё — без окон и без Windows: окно установки (Wails) и трей — тонкий слой в
// cmd/partscaster-worker.
package workerexe

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// --- рекомендуемая модель без Python ---

var reRecommend = regexp.MustCompile(`(?m)^(RECOMMEND_BF16_MIB|RECOMMEND_Q8_MIB)\s*=\s*(\d+)`)

// RecommendFromSource — рекомендуемая модель для карты vramMiB по порогам из engine_cpp.py вшитого кода воркера:
// окно установки выбирает модель до установки Python, а пороги живут в одном месте (engine_cpp.recommend).
// Порогов не нашлось — как «видеопамять неизвестна»: Q8 в экономном режиме.
func RecommendFromSource(workerDir string, vramMiB int) (model string, low bool) {
	b, err := os.ReadFile(filepath.Join(workerDir, "engine_cpp.py"))
	if err != nil {
		return "q8", true
	}
	return RecommendFromBytes(b, vramMiB)
}

// RecommendFromBytes — то же по тексту engine_cpp.py (окно читает его из вшитого архива, ещё до распаковки).
func RecommendFromBytes(b []byte, vramMiB int) (model string, low bool) {
	th := map[string]int{}
	for _, m := range reRecommend.FindAllStringSubmatch(string(b), -1) {
		th[m[1]], _ = strconv.Atoi(m[2])
	}
	bf16, okB := th["RECOMMEND_BF16_MIB"]
	q8, okQ := th["RECOMMEND_Q8_MIB"]
	switch {
	case !okB || !okQ:
		return "q8", true
	case vramMiB >= bf16:
		return "bf16", false
	case vramMiB >= q8:
		return "q8", false
	}
	return "q8", true
}

var reModelSize = regexp.MustCompile(`"(bf16|q8|q6|q5)":\s*\{"file":\s*"[^"]+",\s*"size":\s*(\d+)`)

// ModelSizesFromBytes — размеры файлов моделей (байты) из MODELS в engine_cpp.py: подписи вариантов в окне.
func ModelSizesFromBytes(b []byte) map[string]int64 {
	out := map[string]int64{}
	for _, m := range reModelSize.FindAllStringSubmatch(string(b), -1) {
		out[m[1]], _ = strconv.ParseInt(m[2], 10, 64)
	}
	return out
}

// ReadArchiveFile — файл name (по имени, без каталога) из архива воркера worker-<версия>.tar.gz.
func ReadArchiveFile(r io.Reader, name string) ([]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer func() { _ = gz.Close() }() // чтение архива: ошибка закрытия не важна
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("в архиве воркера нет %s", name)
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag == tar.TypeReg && path.Base(hdr.Name) == name {
			return io.ReadAll(tr)
		}
	}
}

// --- этапы установки ---

// Stage — этап установки в окне.
type Stage int

const (
	StagePackages Stage = iota // Python и пакеты воркера
	StageSep                   // разделение на дорожки (RoFormer)
	StageModel                 // модель генерации
	StageStart                 // запуск воркера (его сообщает сам запуск, не RunInstall)
)

// Reporter — куда установка сообщает этап и строки вывода (окно установки и журнал).
type Reporter interface {
	Stage(s Stage)
	Line(text string)
}

// InstallSteps — шаги установки; каждый сам пропускает уже сделанное, поэтому «Повторить» безопасно.
type InstallSteps interface {
	Packages(ctx context.Context, out io.Writer) error
	Sep(ctx context.Context, out io.Writer) error
	Model(ctx context.Context, out io.Writer) error
}

// RunInstall — шаги по порядку: перед каждым — его этап, вывод — построчно в rep.Line. Сбой шага или отмена —
// ошибка, следующие шаги не идут.
func RunInstall(ctx context.Context, steps InstallSteps, rep Reporter) error {
	for _, s := range []struct {
		stage Stage
		run   func(context.Context, io.Writer) error
	}{{StagePackages, steps.Packages}, {StageSep, steps.Sep}, {StageModel, steps.Model}} {
		if err := ctx.Err(); err != nil {
			return err
		}
		rep.Stage(s.stage)
		if err := runLines(ctx, s.run, rep); err != nil {
			return err
		}
	}
	return nil
}

// runLines — шаг с выводом через трубу: строки уходят в rep.Line до того, как шаг вернётся
// (и неполная последняя строка тоже).
func runLines(ctx context.Context, run func(context.Context, io.Writer) error, rep Reporter) error {
	pr, pw := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			rep.Line(strings.TrimRight(sc.Text(), "\r"))
		}
		_, _ = io.Copy(io.Discard, pr) // слишком длинная строка — дочитать, чтобы шаг не встал на записи
	}()
	err := run(ctx, pw)
	_ = pw.Close() // закрытие своей трубы: ошибки нет
	<-done
	return err
}

// --- настройки запуска ---

// LauncherSettings — выбор человека для запуска воркера (переживает перезапуск).
type LauncherSettings struct {
	LAN bool `json:"lan"` // доступ с других компьютеров сети
}

func launcherFile(home string) string { return filepath.Join(home, "launcher.json") }

// LoadLauncher — сохранённые настройки запуска; нет файла или он битый — по умолчанию.
func LoadLauncher(home string) LauncherSettings {
	var s LauncherSettings
	if b, err := os.ReadFile(launcherFile(home)); err == nil {
		if json.Unmarshal(b, &s) != nil {
			return LauncherSettings{}
		}
	}
	return s
}

// SaveLauncher — атомарно: временный файл и замена.
func SaveLauncher(home string, s LauncherSettings) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return err
	}
	tmp := launcherFile(home) + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, launcherFile(home))
}

// --- журнал ---

// OpenWorkerLog — журнал этого запуска <home>/logs/worker.log; прошлый остаётся как worker.1.log.
func OpenWorkerLog(home string) (*os.File, error) {
	dir := filepath.Join(home, "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	cur := filepath.Join(dir, "worker.log")
	if _, err := os.Stat(cur); err == nil {
		if err := os.Rename(cur, filepath.Join(dir, "worker.1.log")); err != nil {
			return nil, err
		}
	}
	return os.Create(cur)
}

// --- трей ---

// WorkerState — что показывает значок в трее.
type WorkerState int

const (
	StateStarting WorkerState = iota
	StateRunning
	StateFailed
)

// WorkerStateOf — процесс воркера жив и отвечает на /health — работает; жив, но молчит — запускается; процесса нет —
// остановился с ошибкой (сам по «Выходу» трей закрывается вместе с ним).
func WorkerStateOf(procRunning, healthOK bool) WorkerState {
	switch {
	case !procRunning:
		return StateFailed
	case healthOK:
		return StateRunning
	}
	return StateStarting
}

// Text — строка состояния для меню и подсказки значка.
func (s WorkerState) Text() string {
	switch s {
	case StateRunning:
		return "Воркер работает"
	case StateFailed:
		return "Воркер остановился с ошибкой — см. журнал"
	}
	return "Воркер запускается…"
}

// GenerationRunning — по ответу /stats: идёт ли сейчас генерация (вопрос перед «Выходом»).
func GenerationRunning(stats []byte) bool {
	var s struct {
		Running *struct {
			JobID *int64 `json:"job_id"`
		} `json:"running"`
	}
	return json.Unmarshal(stats, &s) == nil && s.Running != nil && s.Running.JobID != nil
}

// ConnectAddress — адрес, который вписать в приложение: при доступе из сети — первый адрес компьютера в сети.
func ConnectAddress(lan bool, lanIPs []string) string {
	host := "127.0.0.1"
	if lan && len(lanIPs) > 0 {
		host = lanIPs[0]
	}
	return fmt.Sprintf("http://%s:%d", host, Port)
}

// NeedSetup — нужно окно установки: пакеты воркера или разделения не стоят/устарели, модель не выбрана.
func NeedSetup(home string, requirements []byte) bool {
	return NeedInstall(home, requirements) || NeedSepInstall(home) || SettingsModel(home) == ""
}
