// partscaster-worker — воркер Partscaster одной программой для Windows (без WSL): при первом запуске ставит Python,
// пакеты и код воркера в папку пользователя, дальше просто запускает воркер. Окно открыто — воркер работает,
// закрыли — остановлен. Код воркера вшит в программу при сборке (tools/release.sh, пакет Windows).
//
//	partscaster-worker [--home ПАПКА] [--lan] [--cpp-model auto|bf16|q8|q6|q5] [--cpp-low-vram]
package main

import (
	"bufio"
	"embed"
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

const needFreeGB = 30 // ~ воркер 17,8 + разделение 7,3 + наборы 1,2 + запас 2 (как MCP install_worker), с округлением вверх

func main() {
	homeFlag := flag.String("home", "", "папка установки (Python, пакеты, модели, треки)")
	lan := flag.Bool("lan", false, "открыть воркер для других компьютеров сети (по умолчанию — только этот)")
	cppModelFlag := flag.String("cpp-model", "", "модель генерации: auto (рекомендуемая) | bf16 | q8 | q6 | q5")
	cppLowVram := flag.Bool("cpp-low-vram", false, "экономный режим (контекст 8192 — меньше видеопамяти)")
	// ошибка флага — с подсказкой и паузой: окно с ярлыка иначе закрылось бы сразу (например, ярлык прежних
	// версий с --engine cpp)
	flag.CommandLine.Init(os.Args[0], flag.ContinueOnError)
	if err := flag.CommandLine.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		if strings.Contains(err.Error(), "-engine") {
			fmt.Println("\nДвижок генерации один (yue2.cpp): флага --engine больше нет, модель — --cpp-model auto|bf16|q8|q6|q5.")
		}
		pause()
		os.Exit(2)
	}

	fmt.Printf("Partscaster — воркер %s\n\n", version)
	opts, err := parseEngineFlags(*cppModelFlag, *cppLowVram)
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

// engineOptions — модель генерации из флагов (пусто — выбор пользователя, без него — вопрос при установке).
type engineOptions struct {
	cppModel   string // "" — не задана флагом; "auto" — рекомендуемая
	cppLowVram bool
}

func parseEngineFlags(cppModel string, lowVram bool) (engineOptions, error) {
	o := engineOptions{cppModel: cppModel, cppLowVram: lowVram}
	if o.cppModel != "" && o.cppModel != "auto" && !workerexe.ValidCppModel(o.cppModel) {
		return o, fmt.Errorf("--cpp-model: %q (допустимо auto, %s)", o.cppModel, strings.Join(workerexe.CppModels, ", "))
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
	if err := setupSep(home, workerDir); err != nil {
		return err
	}
	if err := setupEngine(home, workerDir, gpu, opts); err != nil {
		return err
	}
	return serve(home, workerDir, lan)
}

// setupSep — разделение на дорожки (RoFormer): своё окружение (первый раз или сменились пакеты) и веса обеих
// моделей (есть — не качаются). Обязательно: без него у треков нет дорожек.
func setupSep(home, workerDir string) error {
	if workerexe.NeedSepInstall(home) {
		fmt.Println("\nУстановка разделения на дорожки (RoFormer, первый раз — несколько гигабайт)…")
		uv, err := workerexe.EnsureUv(home)
		if err != nil {
			return fmt.Errorf("uv: %w", err)
		}
		if err := workerexe.InstallSep(home, uv, os.Stdout); err != nil {
			return fmt.Errorf("разделение на дорожки не встало: %w (повторный запуск продолжит)", err)
		}
	}
	cmd := workerexe.SepDownloadCommand(home, workerDir)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("веса разделения на дорожки не скачались: %w (повторный запуск докачает)", err)
	}
	return nil
}

// setupEngine — модель генерации yue2.cpp. Скачанный выбор пользователя без флагов не трогается; иначе — модель
// из флага или ответ на вопрос (Enter — рекомендуемая по видеопамяти). Программа yue2.cpp и модель обязательны:
// без них воркер не запускается (повторный запуск докачает).
func setupEngine(home, workerDir string, gpu workerexe.GPU, opts engineOptions) error {
	if workerexe.CppBinary(workerDir, "windows") == "" {
		return errors.New("в этом пакете нет программы yue2.cpp (движок генерации) — скачайте пакет Windows заново")
	}
	recModel, recLow, err := recommend(home, workerDir, gpu.MemoryMiB)
	if err != nil {
		return fmt.Errorf("рекомендуемая модель: %w", err)
	}
	current := workerexe.SettingsModel(home)
	if opts.cppModel == "" && current != "" && workerexe.ModelInstalledCommand(home, workerDir, current).Run() == nil {
		// выбор сохраняется; ключ engine прежних версий убирается, экономный режим — по флагу
		values := map[string]any{"engine": nil}
		if opts.cppLowVram {
			values["cpp_low_vram"] = true
		}
		if err := workerexe.WriteSettings(home, values); err != nil {
			return err
		}
		printEngine(home)
		return nil
	}
	model, low := opts.cppModel, recLow
	switch model {
	case "":
		model = askModel(recModel, recLow, gpu)
	case "auto":
		model = recModel
	}
	if opts.cppLowVram {
		low = true
	}
	fmt.Printf("\nСкачивание модели %s (несколько гигабайт — зависит от сети)…\n", strings.ToUpper(model))
	cmd := workerexe.DownloadEngineModel(home, workerDir, model)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("модель %s не скачалась: %w (повторный запуск докачает)", model, err)
	}
	// engine: nil — выбор движка из прежних версий убирается: движок теперь один
	if err := workerexe.WriteSettings(home, map[string]any{"cpp_model": model, "cpp_low_vram": low, "engine": nil}); err != nil {
		return err
	}
	printEngine(home)
	return nil
}

// recommend — рекомендуемая модель и экономный режим от воркера (engine_cpp.py recommend <МиБ>).
func recommend(home, workerDir string, vramMiB int) (string, bool, error) {
	out, err := workerexe.RecommendCommand(home, workerDir, vramMiB).Output()
	if err != nil {
		return "", false, err
	}
	f := strings.Fields(string(out))
	if len(f) != 2 || !workerexe.ValidCppModel(f[0]) {
		return "", false, fmt.Errorf("непонятный ответ %q", strings.TrimSpace(string(out)))
	}
	return f[0], f[1] == "1", nil
}

// askModel — вопрос о модели при первой установке; Enter — рекомендуемая.
func askModel(rec string, recLow bool, gpu workerexe.GPU) string {
	low := ""
	if recLow {
		low = ", экономный режим"
	}
	fmt.Printf("\nМодель генерации ([Enter] — %s%s, рекомендуется для %s):\n",
		strings.ToUpper(rec), low, workerexe.FormatGB(gpu.MemoryMiB))
	fmt.Println("  bf16 — без сжатия, ~10 ГБ видеопамяти, файл 7,2 ГБ")
	fmt.Println("  q8   — почти без потерь, ~8 ГБ видеопамяти (в экономном режиме ~4,5), файл 3,8 ГБ")
	fmt.Println("  q6   — файл меньше (2,9 ГБ), видеопамяти почти как q8 (~7,7 ГБ)")
	fmt.Println("  q5   — самый маленький файл (2,6 ГБ), видеопамяти ~7,7 ГБ")
	fmt.Print("Выбор: ")
	m := strings.ToLower(readLine())
	switch {
	case m == "":
		return rec
	case workerexe.ValidCppModel(m):
		return m
	}
	fmt.Printf("«%s» не понял — берём %s\n", m, rec)
	return rec
}

// printEngine — выбранная модель генерации (из настроек воркера).
func printEngine(home string) {
	mode := ""
	if workerexe.SettingsLowVram(home) {
		mode = ", экономный режим"
	}
	fmt.Printf("Модель генерации: %s%s (сменить — в настройках приложения).\n",
		strings.ToUpper(workerexe.SettingsModel(home)), mode)
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
