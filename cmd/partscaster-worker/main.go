// partscaster-worker — воркер Partscaster одной программой для Windows (без WSL и без консоли). Первый запуск —
// окно установки (папка, модель генерации, доступ из сети; Python, пакеты, разделение на дорожки и модель ставятся
// в папку пользователя), дальше — значок в трее: воркер работает, пока программа не закрыта из меню значка.
// Код воркера вшит в программу при сборке (tools/release.sh, пакет Windows).
//
//	partscaster-worker [--home ПАПКА] [--lan] [--cpp-model auto|bf16|q8|q6|q5] [--cpp-low-vram]
package main

import (
	"bytes"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"partscaster/internal/workerexe"
)

//go:embed payload
var payload embed.FS

var version = "dev" // git describe (tools/release.sh)

const needFreeGB = 30 // ~ воркер 17,8 + разделение 7,3 + наборы 1,2 + запас 2 (как MCP install_worker), с округлением вверх

// options — флаги запуска; заданный флаг заполняет свой шаг окна установки.
type options struct {
	home       string
	lan        bool
	cppModel   string // "" — не задана флагом; "auto" — рекомендуемая
	cppLowVram bool
}

func main() {
	workerexe.Lang = lang // строки состояния трея — на языке окна
	opts, err := parseFlags(os.Args[1:])
	if errors.Is(err, flag.ErrHelp) {
		showInfo(err.Error())
		return
	}
	if err != nil {
		showError(err.Error())
		os.Exit(2)
	}
	// потомки (uv, Python, yue-server) — в задании программы: вышли — они завершаются вместе с ней
	if err := workerexe.BindSelfToJob(); err != nil {
		note("задание Windows не создано (%v): после аварийного выхода процессы воркера могут остаться", err)
	}
	if err := run(opts); err != nil {
		showError(err.Error())
		os.Exit(1)
	}
}

// helpError — справка по флагам (её текст — в сообщении: консоли нет).
type helpError struct{ usage string }

func (e helpError) Error() string   { return e.usage }
func (e helpError) Is(t error) bool { return t == flag.ErrHelp }

func parseFlags(args []string) (options, error) {
	var o options
	fs := flag.NewFlagSet("partscaster-worker", flag.ContinueOnError)
	var out bytes.Buffer
	fs.SetOutput(&out)
	fs.StringVar(&o.home, "home", "", tr("папка установки (Python, пакеты, модели, треки)", "install folder (Python, packages, models, tracks)"))
	fs.BoolVar(&o.lan, "lan", false, tr("открыть воркер для других компьютеров сети (по умолчанию — только этот)", "open the worker to other computers on the network (default — this one only)"))
	fs.StringVar(&o.cppModel, "cpp-model", "", tr("модель генерации: auto (рекомендуемая) | bf16 | q8 | q6 | q5", "generation model: auto (recommended) | bf16 | q8 | q6 | q5"))
	fs.BoolVar(&o.cppLowVram, "cpp-low-vram", false, tr("экономный режим (контекст по видеопамяти карты — для карт до 10 ГБ)", "economy mode (context sized to the card’s video memory — for cards up to 10 GB)"))
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return o, helpError{out.String()}
		}
		msg := err.Error()
		if strings.Contains(msg, "-engine") {
			msg += tr("\n\nДвижок генерации один (yue2.cpp): флага --engine больше нет, модель — --cpp-model auto|bf16|q8|q6|q5.", "\n\nThere is one generation engine (yue2.cpp): the --engine flag is gone, the model is --cpp-model auto|bf16|q8|q6|q5.")
		}
		return o, errors.New(msg)
	}
	if o.cppModel != "" && o.cppModel != "auto" && !workerexe.ValidCppModel(o.cppModel) {
		return o, fmt.Errorf(tr("--cpp-model: %q (допустимо auto, %s)", "--cpp-model: %q (allowed: auto, %s)"), o.cppModel, strings.Join(workerexe.CppModels, ", "))
	}
	return o, nil
}

// run — папка известна (выбрана раньше или --home): установлено и флаг модели ничего не меняет — сразу трей,
// иначе окно установки (без вопросов, если модель уже выбиралась). Папки нет — окно со всеми шагами.
func run(opts options) error {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	home, saved := workerexe.ResolveHome(cfgDir, defaultHome())
	if opts.home != "" {
		home, saved = opts.home, true
	}
	inst := newInstaller(opts, cfgDir)
	if saved {
		if err := inst.takeHome(home); err != nil {
			if errors.Is(err, errAlreadyRunning) {
				showInfo(tr("Воркер Partscaster уже запущен — его значок в трее (у часов).", "The Partscaster worker is already running — its icon is in the tray (by the clock)."))
				return nil
			}
			return err
		}
		if !inst.needWindow() {
			return runTray(inst.w, inst.lock)
		}
	}
	if !runInstaller(inst) {
		if inst.lock != nil {
			_ = inst.lock.Close() // выход без трея: папка свободна
		}
		return nil // окно закрыто до конца установки: следующий запуск продолжит
	}
	return runTray(inst.w, inst.lock)
}

var errAlreadyRunning = errors.New(tr("воркер этой папки уже запущен", "the worker of this folder is already running"))

// takeHome — папка установки занята этим запуском (второй запуск той же папки — errAlreadyRunning), порт свободен,
// код воркера распакован, журнал запуска открыт. Замок папки держится до выхода программы (его закрывает runTray).
func (i *Installer) takeHome(home string) error {
	home, err := filepath.Abs(home)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return err
	}
	lock, err := workerexe.LockHome(home)
	if errors.Is(err, workerexe.ErrHomeBusy) {
		return errAlreadyRunning
	}
	if err != nil {
		return err
	}
	release := func() { _ = lock.Close() } // папку не держим: запуска не будет
	if c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", workerexe.Port), time.Second); err == nil {
		_ = c.Close() // пробное соединение: ошибка закрытия не важна
		release()
		return fmt.Errorf(tr("порт %d занят другой программой (или воркером из другой папки) — закройте её и запустите снова",
			"port %d is taken by another program (or by a worker from another folder) — close it and start again"), workerexe.Port)
	}
	workerDir, err := unpackWorker(home)
	if err != nil {
		release()
		return err
	}
	if err := workerexe.SaveHome(i.cfgDir, home); err != nil {
		release()
		return err
	}
	if i.opts.lan {
		st := workerexe.LoadLauncher(home)
		st.LAN = true
		if err := workerexe.SaveLauncher(home, st); err != nil {
			release()
			return err
		}
	}
	log, err := workerexe.OpenWorkerLog(home)
	if err != nil {
		release()
		return err
	}
	writeNotes(log)
	i.home, i.workerDir, i.lock = home, workerDir, lock
	i.w = &worker{home: home, workerDir: workerDir, log: log}
	return nil
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

// workerArchive — вшитый архив кода воркера.
func workerArchive() (fs.File, error) {
	archives, err := fs.Glob(payload, "payload/worker-*.tar.gz")
	if err != nil || len(archives) != 1 {
		return nil, errors.New(tr("программа собрана без кода воркера — соберите её через make dist-windows", "the program was built without the worker code — build it with make dist-windows"))
	}
	return payload.Open(archives[0])
}

// unpackWorker кладёт вшитую версию кода воркера в home/app и убирает прежние: данные, пакеты и модели
// лежат отдельно и не трогаются.
func unpackWorker(home string) (string, error) {
	f, err := workerArchive()
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
				note("прежняя версия кода не удалена: %v", err) // место, а не работа
			}
		}
	}
	return dir, nil
}

// notes — предупреждения до открытия журнала (консоли нет): пишутся в журнал, как только он откроется.
var (
	notesMu sync.Mutex
	notes   []string
)

func note(format string, args ...any) {
	notesMu.Lock()
	defer notesMu.Unlock()
	notes = append(notes, fmt.Sprintf(format, args...))
}

// writeNotes — накопленные предупреждения в журнал (один раз).
func writeNotes(w io.Writer) {
	notesMu.Lock()
	defer notesMu.Unlock()
	for _, n := range notes {
		say(w, "предупреждение: %s\n", n)
	}
	notes = nil
}

// engineSource — engine_cpp.py из вшитого архива: пороги рекомендации и размеры моделей для окна (до распаковки).
func engineSource() []byte {
	f, err := workerArchive()
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }() // файл на чтение: ошибка закрытия не важна
	b, err := workerexe.ReadArchiveFile(f, "engine_cpp.py")
	if err != nil {
		return nil // нет — рекомендация «неизвестно» (q8 эконом), размеры без подписи
	}
	return b
}
