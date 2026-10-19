package main

import (
	_ "embed"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"fyne.io/systray"

	"partscaster/internal/workerexe"
)

//go:embed ui/icon.ico
var trayIcon []byte

const (
	pollEvery  = 2 * time.Second
	httpTimout = 2 * time.Second
)

// localHTTP — запросы к своему воркеру мимо прокси системы.
var localHTTP = &http.Client{Timeout: httpTimout, Transport: &http.Transport{Proxy: nil}}

// worker — процесс воркера: запускает окно установки (этап «Запуск») или трей, управляет трей (перезапуск при
// смене доступа, остановка). Журнал — один на запуск программы.
type worker struct {
	home, workerDir string
	log             *os.File

	mu      sync.Mutex
	cmd     *exec.Cmd
	kill    func() // завершить процесс со всеми потомками (yue-server); безопасно звать повторно
	exited  chan struct{}
	running bool
}

// start — воркер с выводом в журнал; доступ из сети — из настроек запуска.
func (w *worker) start() error {
	lan := workerexe.LoadLauncher(w.home).LAN
	cmd := workerexe.ServeCommand(w.home, w.workerDir, lan)
	cmd.Stdout, cmd.Stderr = w.log, w.log
	if err := cmd.Start(); err != nil {
		return err
	}
	closeJob, err := workerexe.KillTreeOnClose(cmd.Process)
	if err != nil {
		say(w.log, "предупреждение: потомки воркера могут пережить перезапуск (%v)\n", err)
	}
	var once sync.Once
	kill := func() { once.Do(closeJob) }
	exited := make(chan struct{})
	w.mu.Lock()
	w.cmd, w.kill, w.exited, w.running = cmd, kill, exited, true
	w.mu.Unlock()
	go func() {
		err := cmd.Wait()
		say(w.log, "== воркер завершился: %v ==\n", err)
		// Python упал — его yue-server не должен держать видеопамять до «Выхода»: задание закрывается сразу
		kill()
		w.mu.Lock()
		if w.cmd == cmd {
			w.running = false
		}
		w.mu.Unlock()
		close(exited)
	}()
	return nil
}

// stop — воркер со всеми потомками завершается (и уже упавший — его задание тоже закрывается); ждём выхода.
func (w *worker) stop() {
	w.mu.Lock()
	kill, exited := w.kill, w.exited
	w.mu.Unlock()
	if kill == nil {
		return
	}
	kill()
	select {
	case <-exited:
	case <-time.After(10 * time.Second):
	}
}

func (w *worker) isRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}

// waitHealthy — воркер ответил на /health (true) или процесс вышел / время истекло (false).
func (w *worker) waitHealthy(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !w.isRunning() {
			return false
		}
		if healthOK() {
			return true
		}
		time.Sleep(time.Second)
	}
	return false
}

// runTray — значок в трее до «Выхода»: состояние воркера, адрес, доступ из сети, журнал. Воркер, запущенный окном
// установки, принимается как есть. lock — замок папки установки: держится (и закрывается) здесь, до выхода.
func runTray(w *worker, lock *os.File) error {
	defer func() { _ = lock.Close() }()  // замок папки — до выхода программы
	defer func() { _ = w.log.Close() }() // журнал: записи уже на диске
	if !w.isRunning() {
		if err := w.start(); err != nil {
			return fmt.Errorf("воркер не запустился: %w", err)
		}
	}
	systray.Run(func() { trayReady(w) }, func() { w.stop() })
	return nil
}

func trayReady(w *worker) {
	systray.SetIcon(trayIcon)
	systray.SetTooltip("Partscaster — воркер")
	mState := systray.AddMenuItem(workerexe.StateStarting.Text(), "")
	mState.Disable()
	mAddr := systray.AddMenuItem("", "Скопировать адрес для подключения приложения")
	mLAN := systray.AddMenuItemCheckbox("Доступ с других компьютеров сети", "", workerexe.LoadLauncher(w.home).LAN)
	systray.AddSeparator()
	mLog := systray.AddMenuItem("Журнал", "Открыть журнал воркера")
	mQuit := systray.AddMenuItem("Выход", "Остановить воркер")

	setAddr := func() {
		mAddr.SetTitle("Адрес: " + workerexe.ConnectAddress(workerexe.LoadLauncher(w.home).LAN, lanIPs()) +
			" (скопировать)")
	}
	setAddr()
	go func() {
		t := time.NewTicker(pollEvery)
		defer t.Stop()
		for range t.C {
			st := workerexe.WorkerStateOf(w.isRunning(), healthOK())
			mState.SetTitle(st.Text())
			systray.SetTooltip("Partscaster — " + st.Text())
		}
	}()
	go func() {
		for {
			select {
			case <-mAddr.ClickedCh:
				copyText(workerexe.ConnectAddress(workerexe.LoadLauncher(w.home).LAN, lanIPs()))
			case <-mLAN.ClickedCh:
				toggleLAN(w, mLAN)
				setAddr()
			case <-mLog.ClickedCh:
				openFile(filepath.Join(w.home, "logs", "worker.log"))
			case <-mQuit.ClickedCh:
				if generationRunning() && !askYesNo("Идёт генерация — прервать её и остановить воркер?\n"+
					"Очередь сохранится и продолжится при следующем запуске.") {
					continue
				}
				systray.Quit()
				return
			}
		}
	}()
}

// toggleLAN — доступ из сети вкл/выкл: сохраняется, воркер перезапускается с новым адресом (идущая генерация —
// только с согласия).
func toggleLAN(w *worker, item *systray.MenuItem) {
	st := workerexe.LoadLauncher(w.home)
	want := !st.LAN
	if generationRunning() && !askYesNo("Идёт генерация — перезапуск воркера её прервёт. Продолжить?") {
		return
	}
	st.LAN = want
	if err := workerexe.SaveLauncher(w.home, st); err != nil {
		showError("настройка не сохранилась: " + err.Error())
		return
	}
	if want {
		item.Check()
	} else {
		item.Uncheck()
	}
	w.stop()
	if err := w.start(); err != nil {
		showError("воркер не перезапустился: " + err.Error())
	}
}

func healthOK() bool {
	resp, err := localHTTP.Get(fmt.Sprintf("http://127.0.0.1:%d/health", workerexe.Port))
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }() // ответ прочитан не будет: важен код
	return resp.StatusCode == http.StatusOK
}

// generationRunning — идёт ли генерация (по /stats); воркер не отвечает — нет.
func generationRunning() bool {
	resp, err := localHTTP.Get(fmt.Sprintf("http://127.0.0.1:%d/stats", workerexe.Port))
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }() // тело дочитано ниже
	b, err := io.ReadAll(resp.Body)
	return err == nil && workerexe.GenerationRunning(b)
}

// lanIPs — адреса этого компьютера в сети (IPv4, без петли): первый — в адрес для подключения.
func lanIPs() []string {
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil && !ipn.IP.IsLinkLocalUnicast() {
				out = append(out, ipn.IP.String())
			}
		}
	}
	return out
}
