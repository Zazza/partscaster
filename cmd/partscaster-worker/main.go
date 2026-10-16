// partscaster-worker — воркер Partscaster одной программой для Windows (без WSL): при первом запуске ставит Python,
// пакеты и код воркера в папку пользователя, дальше просто запускает воркер. Окно открыто — воркер работает,
// закрыли — остановлен. Код воркера вшит в программу при сборке (tools/release.sh, пакет Windows).
//
//	partscaster-worker [--home ПАПКА] [--lan] [--engine torch|cpp] [--cpp-model q8] [--cpp-low-vram]
package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"partscaster/internal/workerexe"
)

//go:embed payload
var payload embed.FS

var version = "dev" // git describe (tools/release.sh)

const needFreeGB = 25

func main() {
	homeFlag := flag.String("home", "", "папка установки (Python, пакеты, модели, треки)")
	lan := flag.Bool("lan", false, "открыть воркер для других компьютеров сети (по умолчанию — только этот)")
	engineFlag := flag.String("engine", "", "движок генерации: torch (основной) или cpp (yue2.cpp, эксперимент)")
	cppModelFlag := flag.String("cpp-model", "", "модель yue2.cpp: bf16 | q8 | q6 | q5")
	cppLowVram := flag.Bool("cpp-low-vram", false, "yue2.cpp в экономном режиме (контекст 8192 — меньше видеопамяти)")
	flag.Parse()

	fmt.Printf("Partscaster — воркер %s\n\n", version)
	opts, err := parseEngineFlags(*engineFlag, *cppModelFlag, *cppLowVram)
	if err != nil {
		fmt.Printf("\nОшибка: %v\n", err)
		pause()
		os.Exit(1)
	}
	if err := start(*homeFlag, *lan, opts); err != nil {
		fmt.Printf("\nОшибка: %v\n", err)
		pause()
		os.Exit(1)
	}
}

// engineOptions — выбор движка из флагов (всё пустое — интерактив при первой установке).
type engineOptions struct {
	engine     string // "torch" | "cpp" | "" (не задан флагом)
	cppModel   string // "" — по умолчанию (по видеокарте)
	cppLowVram bool
}

func parseEngineFlags(engine, cppModel string, lowVram bool) (engineOptions, error) {
	o := engineOptions{engine: engine, cppModel: cppModel, cppLowVram: lowVram}
	switch o.engine {
	case "", "torch", "cpp":
	default:
		return o, fmt.Errorf("--engine: %q (допустимо torch или cpp)", o.engine)
	}
	if o.cppModel != "" && !workerexe.ValidCppModel(o.cppModel) {
		return o, fmt.Errorf("--cpp-model: %q (допустимо %s)", o.cppModel, strings.Join(workerexe.CppModels, ", "))
	}
	if o.engine != "cpp" && o.cppModel != "" {
		return o, errors.New("--cpp-model имеет смысл только с --engine cpp")
	}
	return o, nil
}

func start(homeFlag string, lan bool, opts engineOptions) error {
	gpu, err := checkGPU()
	if err != nil {
		return err
	}
	fmt.Printf("Видеокарта: %s, %s, драйвер %s\n", gpu.Name, workerexe.FormatGB(gpu.MemoryMiB), gpu.Driver)

	home, err := chooseHome(homeFlag)
	if err != nil {
		return err
	}
	lock, err := workerexe.LockHome(home)
	if err != nil {
		return fmt.Errorf("%w — закройте окно другого запуска", err)
	}
	defer lock.Close()
	// воркер из другой папки (или другая программа) на том же порту
	if c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", workerexe.Port), time.Second); err == nil {
		_ = c.Close() // пробное соединение: ошибка закрытия не важна
		return fmt.Errorf("порт %d занят — воркер уже запущен (его окно открыто) или порт занят другой программой",
			workerexe.Port)
	}
	workerDir, err := unpackWorker(home)
	if err != nil {
		return err
	}
	req, err := os.ReadFile(filepath.Join(workerDir, "requirements.txt"))
	if err != nil {
		return err
	}
	if workerexe.NeedInstall(home, req) {
		fmt.Println("\nУстановка Python и пакетов (первый раз — долго: скачивается несколько гигабайт)…")
		uv, err := workerexe.EnsureUv(home)
		if err != nil {
			return fmt.Errorf("uv: %w", err)
		}
		if err := workerexe.InstallPackages(home, uv, filepath.Join(workerDir, "requirements.txt"), os.Stdout); err != nil {
			return fmt.Errorf("пакеты не встали: %w (повторный запуск продолжит)", err)
		}
	}
	if err := setupEngine(home, workerDir, gpu, opts); err != nil {
		return err
	}
	return serve(home, workerDir, lan)
}

// setupEngine — выбор движка (первая установка — вопрос, флаги — молча) и доустановка yue2.cpp:
// скачивание весов и запись настроек. Ошибка скачивания не отменяет установку — откат на основной.
func setupEngine(home, workerDir string, gpu workerexe.GPU, opts engineOptions) error {
	cppBin := workerexe.CppBinary(workerDir, "windows")
	settings := map[string]any{}
	switch {
	case opts.engine == "cpp":
		if cppBin == "" {
			return errors.New("движок cpp выбран флагом, но в этой сборке нет yue2.cpp (пакет собран без него)")
		}
	case opts.engine == "torch":
		settings["engine"] = "torch"
	case workerexe.SettingsExist(home):
		printEngine(home)
		return nil // выбор уже сделан при установке
	default:
		choice, err := askEngine(cppBin != "", gpu)
		if err != nil {
			return err
		}
		if choice == "" {
			settings["engine"] = "torch"
		} else {
			settings["engine"] = "cpp"
			settings["cpp_model"] = choice
			settings["cpp_low_vram"] = workerexe.LowVramDefault(gpu.MemoryMiB)
		}
	}
	if settings["engine"] == "cpp" {
		model, _ := settings["cpp_model"].(string)
		if model == "" {
			model = opts.cppModel
			if model == "" {
				model = workerexe.DefaultCppModel(gpu.MemoryMiB)
			}
			settings["cpp_model"] = model
		}
		if !workerexe.ValidCppModel(model) {
			return fmt.Errorf("модель yue2.cpp: %q (допустимо %s)", model, strings.Join(workerexe.CppModels, ", "))
		}
		fmt.Printf("\nСкачивание модели yue2.cpp (%s, несколько гигабайт — зависит от сети)…\n", model)
		cmd := workerexe.DownloadEngineModel(home, workerDir, model)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("!! модель yue2.cpp не скачалась: %v — воркер работает на основном движке\n", err)
			settings["engine"] = "torch"
			delete(settings, "cpp_model")
			delete(settings, "cpp_low_vram")
		}
	}
	if err := workerexe.WriteSettings(home, settings); err != nil {
		return err
	}
	printEngine(home)
	return nil
}

// askEngine — вопрос о движке; ответ "" — основной (PyTorch), иначе имя модели yue2.cpp.
func askEngine(cppAvailable bool, gpu workerexe.GPU) (string, error) {
	fmt.Println("\nДвижок генерации:")
	fmt.Println("  [Enter] основной (PyTorch) — лучшее качество, нужно ~11 ГБ видеопамяти")
	if !cppAvailable {
		fmt.Println("         yue2.cpp — в этой сборке отсутствует (пакет собран без него)")
		fmt.Print("Выбор: ")
		return "", nil
	}
	def := workerexe.DefaultCppModel(gpu.MemoryMiB)
	low := ""
	if workerexe.LowVramDefault(gpu.MemoryMiB) {
		low = ", экономный режим"
	}
	fmt.Printf("  2       yue2.cpp — эксперимент: меньше видеопамяти (модель %s%s)\n", def, low)
	fmt.Print("Выбор: ")
	if readLine() != "2" {
		return "", nil
	}
	fmt.Printf("\nМодель yue2.cpp ([Enter] — %s, рекомендовано для %s):\n", def, workerexe.FormatGB(gpu.MemoryMiB))
	fmt.Println("  q8   — почти без потерь, ~8 ГБ видеопамяти (в экономном режиме ~4,5)")
	fmt.Println("  q6   — файл меньше (2,9 ГБ), видеопамяти почти как q8 (~7,7 ГБ)")
	fmt.Println("  q5   — самый маленький файл (2,6 ГБ), видеопамяти ~7,7 ГБ")
	fmt.Println("  bf16 — без сжатия, ~10 ГБ видеопамяти, файл 7,2 ГБ")
	fmt.Print("Выбор: ")
	m := readLine()
	switch {
	case m == "":
		return def, nil
	case workerexe.ValidCppModel(m):
		return m, nil
	}
	fmt.Printf("«%s» не понял — берём %s\n", m, def)
	return def, nil
}

// printEngine — какой движок активен (из настроек воркера).
func printEngine(home string) {
	st := map[string]any{}
	if b, err := os.ReadFile(workerexe.SettingsFile(home)); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	if st["engine"] == "cpp" {
		fmt.Printf("Движок: yue2.cpp (%v)\n", st["cpp_model"])
		return
	}
	fmt.Println("Движок: основной (PyTorch).")
}

func checkGPU() (workerexe.GPU, error) {
	out, err := exec.Command("nvidia-smi", "--query-gpu=name,memory.total,driver_version",
		"--format=csv,noheader,nounits").Output()
	if err != nil {
		return workerexe.GPU{}, fmt.Errorf("нужна видеокарта NVIDIA с драйвером (nvidia-smi: %v)", err)
	}
	return workerexe.ParseGPU(string(out))
}

func chooseHome(homeFlag string) (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	def := defaultHome()
	home, saved := workerexe.ResolveHome(cfg, def)
	switch {
	case homeFlag != "":
		home = homeFlag
	case !saved:
		fmt.Printf("\nПапка установки (Python, пакеты, модели и треки — около %d ГБ)\n", needFreeGB)
		fmt.Printf("Enter — %s, или введите другую: ", def)
		if line := readLine(); line != "" {
			home = line
		}
	}
	home, err = filepath.Abs(home)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return "", err
	}
	if free, err := workerexe.FreeBytes(home); err == nil {
		fmt.Printf("Папка: %s (свободно %.0f ГБ)\n", home, float64(free)/(1<<30))
		if free < needFreeGB<<30 {
			fmt.Printf("Внимание: нужно около %d ГБ — установка может не поместиться.\n", needFreeGB)
		}
	} else {
		fmt.Printf("Папка: %s (свободное место узнать не удалось: %v)\n", home, err)
	}
	return home, workerexe.SaveHome(cfg, home)
}

func defaultHome() string {
	if d := os.Getenv("LOCALAPPDATA"); d != "" {
		return filepath.Join(d, "partscaster-worker")
	}
	if d, err := os.UserHomeDir(); err == nil {
		return filepath.Join(d, "partscaster-worker")
	}
	return "partscaster-worker"
}

// unpackWorker кладёт вшитую версию кода воркера в home/app и убирает прежние: данные, пакеты и модели
// лежат отдельно и не трогаются.
func unpackWorker(home string) (string, error) {
	archives, err := fs.Glob(payload, "payload/worker-*.tar.gz")
	if err != nil || len(archives) != 1 {
		return "", errors.New("программа собрана без кода воркера — соберите её через make dist-windows")
	}
	f, err := payload.Open(archives[0])
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }() // файл на чтение: ошибка закрытия не важна
	appDir := filepath.Join(home, "app")
	dir, err := workerexe.ExtractWorker(f, appDir)
	if err != nil {
		return "", err
	}
	old, _ := filepath.Glob(filepath.Join(appDir, "worker-*"))
	for _, o := range old {
		if o != dir {
			if err := os.RemoveAll(o); err != nil {
				fmt.Printf("прежняя версия кода не удалена: %v\n", err)
			}
		}
	}
	return dir, nil
}

func serve(home, workerDir string, lan bool) error {
	cmd := workerexe.ServeCommand(home, workerDir, lan)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := workerexe.BindToJob(cmd.Process); err != nil {
		fmt.Printf("предупреждение: воркер может остаться после закрытия окна (%v)\n", err)
	}
	// Ctrl+C терминал или окно доставляют всем процессам сразу — здесь только ждём, пока воркер закроет базу
	// и выйдет (повторный Ctrl+C uvicorn понял бы как «выйти немедленно»). SIGTERM приходит одной этой
	// программе (kill, остановка службы) — его передаём, иначе Python остался бы работать
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	var stopping atomic.Bool
	go func() {
		for s := range sig {
			stopping.Store(true)
			if s == syscall.SIGTERM {
				// на Windows так приходит закрытие окна, но Signal там сигналы не передаёт — воркер
				// останавливает объект задания (BindToJob)
				_ = cmd.Process.Signal(s)
			}
		}
	}()
	fmt.Printf("\nВоркер: http://127.0.0.1:%d — в Partscaster укажите адрес 127.0.0.1.\n", workerexe.Port)
	fmt.Println("Модель скачивается при первой генерации. Закройте окно, чтобы остановить воркер.")
	err := cmd.Wait()
	if stopping.Load() {
		fmt.Println("Воркер остановлен.")
		return nil
	}
	return err
}

func readLine() string {
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line)
}

// pause держит окно открытым после ошибки: при запуске двойным щелчком оно иначе сразу закроется.
func pause() {
	fmt.Print("Нажмите Enter, чтобы закрыть окно.")
	readLine()
}
