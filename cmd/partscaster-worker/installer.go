package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2"
	wailsopts "github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"partscaster/internal/workerexe"
)

//go:embed ui
var uiFS embed.FS

// workerStartTimeout — сколько ждать первого ответа воркера после установки (импорт пакетов на холодном диске).
const workerStartTimeout = 3 * time.Minute

// Installer — окно установки (Wails): методы зовёт страница ui/app.js, ход установки уходит событиями
// stage/line/failed/done. Логика этапов — workerexe.RunInstall; здесь только связка с окном.
type Installer struct {
	opts   options
	cfgDir string

	home      string
	workerDir string
	lock      *os.File // замок папки установки — до выхода программы
	w         *worker  // воркер этого запуска (журнал — его): окно запускает его на этапе «Запуск»

	wails context.Context // контекст окна (события, диалоги)

	mu       sync.Mutex
	starting bool // Start выполняется (повторное нажатие «Установить» не берёт папку второй раз)
	running  bool
	done     bool
	cancel   context.CancelFunc
}

func newInstaller(opts options, cfgDir string) *Installer {
	return &Installer{opts: opts, cfgDir: cfgDir}
}

// needWindow — папка взята: окно нужно, если что-то не установлено (пакеты, выбор модели, файлы модели или весов
// разделения) или флагом задана модель/режим, отличные от выбранных (--cpp-model auto — рекомендуемая).
func (i *Installer) needWindow() bool {
	req, err := os.ReadFile(filepath.Join(i.workerDir, "requirements.txt"))
	if err != nil || workerexe.NeedSetup(i.home, req) {
		return true
	}
	cur, curLow := workerexe.SettingsModel(i.home), workerexe.SettingsLowVram(i.home)
	want, wantLow := i.flagModel()
	if want != "" && (want != cur || wantLow != curLow) || want == "" && wantLow && !curLow {
		return true
	}
	// файлы могли пропасть (удалены, оборвана загрузка): проверка без сети — пара секунд на запуск Python
	return workerexe.ModelInstalledCommand(i.home, i.workerDir, cur).Run() != nil ||
		workerexe.SepReadyCommand(i.home, i.workerDir).Run() != nil
}

// flagModel — модель и экономный режим из флагов: "" — модель не задана (тогда режим — только --cpp-low-vram);
// с моделью режим — рекомендованный для карты или --cpp-low-vram; auto — рекомендуемая модель.
func (i *Installer) flagModel() (model string, low bool) {
	if i.opts.cppModel == "" {
		return "", i.opts.cppLowVram
	}
	gpu, _ := checkGPU() // видеокарта не ответила — рекомендация «неизвестно» (q8 эконом)
	rec, recLow := workerexe.RecommendFromBytes(engineSource(), gpu.MemoryMiB)
	model = i.opts.cppModel
	if model == "auto" {
		model = rec
	}
	return model, i.opts.cppLowVram || recLow
}

// runInstaller — окно до закрытия; true — установка закончена (дальше — трей).
func runInstaller(i *Installer) bool {
	assets, err := fs.Sub(uiFS, "ui")
	if err != nil {
		showError(err.Error())
		return false
	}
	err = wails.Run(&wailsopts.App{
		Title:            "Partscaster — установка воркера",
		Width:            760,
		Height:           680,
		MinWidth:         640,
		MinHeight:        520,
		BackgroundColour: &wailsopts.RGBA{R: 8, G: 8, B: 12, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        func(ctx context.Context) { i.wails = ctx },
		OnBeforeClose:    i.beforeClose,
		Bind:             []any{i},
	})
	if err != nil {
		showError("окно установки не открылось: " + err.Error())
		return false
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.done
}

// beforeClose — закрытие во время установки: спросить; «прервать» останавливает её (загрузки — вместе с программой).
func (i *Installer) beforeClose(ctx context.Context) bool {
	i.mu.Lock()
	running := i.running
	i.mu.Unlock()
	if !running {
		return false
	}
	ans, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "Прервать установку?",
		Message:       "Установка ещё идёт. Прервать её? Следующий запуск продолжит с того же места.",
		DefaultButton: "No", // Windows: только «Да/Нет»; Enter — не прерывать
	})
	if err != nil || ans != "Yes" {
		return true // окно остаётся
	}
	i.mu.Lock()
	if i.cancel != nil {
		i.cancel()
	}
	i.mu.Unlock()
	return false
}

// ModelOpt — вариант модели в окне.
type ModelOpt struct {
	Name        string  `json:"name"`
	SizeGB      float64 `json:"size_gb"` // 0 — неизвестно
	Recommended bool    `json:"recommended"`
}

// Info — что показать на шагах окна.
type Info struct {
	Version     string     `json:"version"`
	GPU         string     `json:"gpu"`
	VRAMGB      int        `json:"vram_gb"`
	GPUError    string     `json:"gpu_error"`
	AskHome     bool       `json:"ask_home"`
	AskChoices  bool       `json:"ask_choices"` // модель и доступ (первая установка)
	Home        string     `json:"home"`
	NeedGB      int        `json:"need_gb"`
	Models      []ModelOpt `json:"models"`
	Model       string     `json:"model"`
	LowVram     bool       `json:"low_vram"`
	ModelFixed  bool       `json:"model_fixed"` // задана флагом --cpp-model
	LAN         bool       `json:"lan"`
	LANFixed    bool       `json:"lan_fixed"` // задан флагом --lan
	ConnectHint string     `json:"connect_hint"`
}

// Info — сведения для окна: видеокарта, папка, модели с рекомендацией, выбор доступа.
func (i *Installer) Info() Info {
	inf := Info{Version: version, NeedGB: needFreeGB, AskHome: i.home == "", Home: i.home, LANFixed: i.opts.lan}
	if inf.AskHome {
		inf.Home = defaultHome()
	}
	gpu, err := checkGPU()
	if err != nil {
		inf.GPUError = err.Error()
	} else {
		inf.GPU, inf.VRAMGB = gpu.Name, (gpu.MemoryMiB+512)/1024
	}
	src := engineSource()
	rec, recLow := workerexe.RecommendFromBytes(src, gpu.MemoryMiB)
	sizes := workerexe.ModelSizesFromBytes(src)
	for _, n := range workerexe.CppModels {
		inf.Models = append(inf.Models, ModelOpt{Name: n, SizeGB: float64(sizes[n]) / 1e9, Recommended: n == rec})
	}
	inf.Model, inf.LowVram = rec, recLow
	if i.home != "" {
		if cur := workerexe.SettingsModel(i.home); cur != "" {
			inf.Model, inf.LowVram = cur, workerexe.SettingsLowVram(i.home)
		}
		inf.LAN = workerexe.LoadLauncher(i.home).LAN
	}
	switch m := i.opts.cppModel; {
	case m == "auto":
		inf.Model, inf.LowVram, inf.ModelFixed = rec, recLow, true
	case m != "":
		inf.Model, inf.LowVram, inf.ModelFixed = m, recLow, true // режим — как needWindow (flagModel)
	}
	if i.opts.cppLowVram {
		inf.LowVram = true
	}
	inf.LAN = inf.LAN || i.opts.lan
	// модель и доступ спрашиваются при первой установке, если не заданы оба флагами (--cpp-model и --lan)
	first := inf.AskHome || workerexe.SettingsModel(i.home) == ""
	inf.AskChoices = first && (!inf.ModelFixed || !inf.LANFixed)
	inf.ConnectHint = workerexe.ConnectAddress(inf.LAN, lanIPs())
	return inf
}

// FreeGB — свободное место в папке (или в ближайшей существующей выше); -1 — неизвестно.
func (i *Installer) FreeGB(dir string) float64 {
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		if free, err := workerexe.FreeBytes(d); err == nil {
			return float64(free) / (1 << 30)
		}
		if filepath.Dir(d) == d {
			return -1
		}
	}
}

// ChooseFolder — диалог выбора папки; отмена — прежний путь.
func (i *Installer) ChooseFolder(current string) string {
	dir, err := runtime.OpenDirectoryDialog(i.wails, runtime.OpenDialogOptions{
		Title: "Папка установки воркера", DefaultDirectory: filepath.Dir(current), CanCreateDirectories: true,
	})
	if err != nil || dir == "" {
		return current
	}
	return dir
}

// Choice — выбор человека в окне.
type Choice struct {
	Home    string `json:"home"`
	Model   string `json:"model"`
	LowVram bool   `json:"low_vram"`
	LAN     bool   `json:"lan"`
}

// Start — начать (или повторить) установку: "" — пошла, иначе текст, почему нет (окно его показывает).
func (i *Installer) Start(c Choice) string {
	i.mu.Lock()
	if i.running || i.starting {
		i.mu.Unlock()
		return ""
	}
	i.starting = true
	i.mu.Unlock()
	defer func() {
		i.mu.Lock()
		i.starting = false
		i.mu.Unlock()
	}()
	if !workerexe.ValidCppModel(c.Model) {
		return fmt.Sprintf("модель %q неизвестна", c.Model)
	}
	if i.home == "" {
		if strings.TrimSpace(c.Home) == "" {
			return "выберите папку установки"
		}
		if err := i.takeHome(c.Home); err != nil {
			if errors.Is(err, errAlreadyRunning) {
				return "в этой папке уже работает воркер Partscaster (его значок в трее)"
			}
			return err.Error()
		}
	}
	if err := workerexe.SaveLauncher(i.home, workerexe.LauncherSettings{LAN: c.LAN}); err != nil {
		return err.Error()
	}
	log := i.w.log
	ctx, cancel := context.WithCancel(context.Background())
	i.mu.Lock()
	i.running, i.cancel = true, cancel
	i.mu.Unlock()
	steps := &realSteps{home: i.home, workerDir: i.workerDir, model: c.Model, lowVram: c.LowVram}
	go func() {
		err := workerexe.RunInstall(ctx, steps, &windowReporter{i: i, log: log})
		if err == nil {
			err = i.startWorker()
		}
		i.mu.Lock()
		i.running, i.cancel = false, nil
		i.done = err == nil
		i.mu.Unlock()
		if err != nil {
			say(log, "!! %v\n", err)
			i.emit("failed", err.Error())
			return
		}
		i.emit("done", workerexe.ConnectAddress(c.LAN, lanIPs()))
	}()
	return ""
}

// startWorker — этап «Запуск»: воркер стартует и отвечает на /health (трей его потом принимает как есть).
func (i *Installer) startWorker() error {
	i.emit("stage", int(workerexe.StageStart))
	say(i.w.log, "== этап %d ==\n", int(workerexe.StageStart))
	if !i.w.isRunning() {
		if err := i.w.start(); err != nil {
			return fmt.Errorf("воркер не запустился: %w", err)
		}
	}
	i.emit("line", "Запуск воркера…")
	if !i.w.waitHealthy(workerStartTimeout) {
		i.w.stop()
		return errors.New("воркер не ответил после запуска — подробности в журнале")
	}
	return nil
}

// Copy — текст в буфер обмена (адрес для подключения).
func (i *Installer) Copy(text string) {
	_ = runtime.ClipboardSetText(i.wails, text) // не скопировалось — адрес и так на экране
}

// OpenLog — журнал установки в Блокноте (из окна ошибки).
func (i *Installer) OpenLog() {
	if i.home != "" {
		openFile(filepath.Join(i.home, "logs", "worker.log"))
	}
}

// Close — закрыть окно (после установки — дальше трей; без установки — выход).
func (i *Installer) Close() { runtime.Quit(i.wails) }

func (i *Installer) emit(event string, data any) {
	if i.wails != nil {
		runtime.EventsEmit(i.wails, event, data)
	}
}

// windowReporter — этапы и строки установки: в окно и в журнал. Скачивание — событием progress
// не чаще раза в 300 мс (engine_cpp.py печатает ход каждым мегабайтом — тысячи строк за модель);
// в журнал — каждым изменением десятка процентов и в конце файла.
type windowReporter struct {
	i   *Installer
	log *os.File

	lastEmit time.Time // троттлинг событий в окно
	lastPct  int       // последняя записанная в журнал десятка процентов
}

func (r *windowReporter) Stage(s workerexe.Stage) {
	say(r.log, "== этап %d ==\n", int(s))
	r.i.emit("stage", int(s))
}

func (r *windowReporter) Line(text string) {
	sayln(r.log, text)
	r.i.emit("line", text)
}

func (r *windowReporter) Progress(file string, done, total int64) {
	now := time.Now()
	if done < total && now.Sub(r.lastEmit) < 300*time.Millisecond {
		return
	}
	r.lastEmit = now
	r.i.emit("progress", map[string]any{"file": file, "done": done, "total": total})
	if pct := int(100 * done / total); pct/10 != r.lastPct/10 || done == total {
		r.lastPct = pct
		say(r.log, "  %s: %s из %s (%d%%)\n", file, humanBytes(done), humanBytes(total), pct)
	}
}

// humanBytes — «3,8 ГБ» / «530 МБ»: байты в лоб не читаются.
func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f ГБ", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%d МБ", n/(1<<20))
	default:
		return fmt.Sprintf("%d КБ", n/(1<<10))
	}
}

func checkGPU() (workerexe.GPU, error) {
	cmd := exec.Command("nvidia-smi", "--query-gpu=name,memory.total,driver_version",
		"--format=csv,noheader,nounits")
	workerexe.HideConsole(cmd)
	out, err := cmd.Output()
	if err != nil {
		return workerexe.GPU{}, fmt.Errorf("нужна видеокарта NVIDIA с драйвером (nvidia-smi не отвечает: %v)", err)
	}
	return workerexe.ParseGPU(string(out))
}
