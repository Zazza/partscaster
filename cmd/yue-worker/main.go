// yue-worker — воркер Yue Studio одной программой для Windows (без WSL): при первом запуске ставит Python,
// пакеты и код воркера в папку пользователя, дальше просто запускает воркер. Окно открыто — воркер работает,
// закрыли — остановлен. Код воркера вшит в программу при сборке (tools/release.sh, пакет Windows).
//
//	yue-worker [--home ПАПКА] [--lan]
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

	"yue-studio/internal/workerexe"
)

//go:embed payload
var payload embed.FS

var version = "dev" // git describe (tools/release.sh)

const needFreeGB = 25

func main() {
	homeFlag := flag.String("home", "", "папка установки (Python, пакеты, модели, треки)")
	lan := flag.Bool("lan", false, "открыть воркер для других компьютеров сети (по умолчанию — только этот)")
	flag.Parse()

	fmt.Printf("Yue Studio — воркер %s\n\n", version)
	if err := start(*homeFlag, *lan); err != nil {
		fmt.Printf("\nОшибка: %v\n", err)
		pause()
		os.Exit(1)
	}
}

func start(homeFlag string, lan bool) error {
	gpu, err := checkGPU()
	if err != nil {
		return err
	}
	fmt.Printf("Видеокарта: %s, %.1f ГБ, драйвер %s\n", gpu.Name, float64(gpu.MemoryMiB)/1024, gpu.Driver)
	fmt.Println("Движок: основной (PyTorch). yue2.cpp на Windows пока недоступен.")

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
	return serve(home, workerDir, lan)
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
		return filepath.Join(d, "yue-worker")
	}
	if d, err := os.UserHomeDir(); err == nil {
		return filepath.Join(d, "yue-worker")
	}
	return "yue-worker"
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
	fmt.Printf("\nВоркер: http://127.0.0.1:%d — в Yue Studio укажите адрес 127.0.0.1.\n", workerexe.Port)
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
