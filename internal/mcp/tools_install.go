package mcp

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// RegisterInstallTools — установка и диагностика. Два режима:
//   - всё на одном ПК: команды выполняются локально;
//   - GPU-машина отдельно: host="user@gpu-host" — те же шаги через ssh.
//
// Установка не выполняет деструктивных действий без явных флагов;
// каждый шаг печатает результат в ответ инструмента.
func RegisterInstallTools(s *Server) {
	s.Register(Tool{
		Name: "doctor",
		Description: "Диагностика окружения: доступность воркера, GPU/VRAM (nvidia-smi), ffmpeg, " +
			"версия Go/Node, необязательные компоненты (whisper, Seed-VC). " +
			"Для GPU-проверок на отдельной машине — host.",
		InputSchema: props(map[string]any{
			"host": prop("user@gpu-host для проверки удалённой машины (пусто = локально)", "string"),
		}),
		Handler: func(s *Server, args map[string]any) (string, error) {
			// воркер и компоненты — по его адресу (отсюда); железо — на машине с GPU
			out, _ := runSteps("", []step{
				{name: "воркер Partscaster (health)", cmd: fmt.Sprintf("curl -sf --max-time 5 %s/health", s.client.GetURL())},
				{name: "компоненты воркера (whisper, Seed-VC, модель разделения)", cmd: fmt.Sprintf(
					"curl -sf --max-time 5 %s/config | grep -o '\"\\(whisper_available\\|seedvc_available\\|roformer_available\\)\": *\"*[a-z-]*'", s.client.GetURL())},
				{name: "ffmpeg (нужен для DSP на ПК)", cmd: "ffmpeg -version | head -1"},
				{name: "Go (сборка приложения)", cmd: "go version"},
				{name: "Node (сборка фронта)", cmd: "node --version"},
			}, true)
			gpu, _ := runSteps(argString(args, "host"), []step{
				{name: "GPU/VRAM", cmd: "nvidia-smi --query-gpu=name,memory.total,memory.free --format=csv,noheader"},
				{name: "место на диске (домашний каталог)", cmd: "df -h ~ | tail -1"},
			}, true)
			return workerVersionLine(s) + out + "\n" + gpu + workerAddress(s), nil
		},
	})

	s.Register(Tool{
		Name: "install_worker",
		Description: "Установить воркер на машине с GPU (Ubuntu + NVIDIA/CUDA). Пустой host — всё на этом ПК " +
			"(частая схема: приложение и GPU на одной машине), host — отдельная машина по ssh. " +
			"Компоненты: воркер (обязательно, до ~" + gbStr(sizeWorkerGB) + " ГБ: окружение, программа yue2.cpp и модель генерации, веса разбора нот) " +
			"и разделение на дорожки RoFormer (обязательно, ~" + gbStr(sizeSepGB) + " ГБ, веса некоммерческие; каталог — sep_dir); " +
			"необязательные: whisper — распознавание текстов треков (~" + gbStr(sizeWhisperGB) + " ГБ), " +
			"Seed-VC — «голос альбома», ЭКСПЕРИМЕНТ (~" + gbStr(sizeSeedvcGB) + " ГБ после установки, больше во время). " +
			"Сначала сухой прогон (по умолчанию) — покажи пользователю план и СПРОСИ, нужны ли whisper и Seed-VC; " +
			"установка — dry_run=false, confirm=true и явные whisper/seedvc (true или false). " +
			"Перед установкой проверяются GPU и место на диске — при нехватке установка не начинается.",
		InputSchema: props(map[string]any{
			"host":    prop("user@gpu-host (пусто = локально на этой машине)", "string"),
			"dry_run": prop("только показать план, ничего не выполняя (по умолчанию true)", "boolean"),
			"confirm": prop("выполнить установку (спроси пользователя)", "boolean"),
			"whisper": prop("ставить whisper — тексты треков, ~"+gbStr(sizeWhisperGB)+" ГБ (спроси пользователя)", "boolean"),
			"seedvc":  prop("ставить Seed-VC — «голос альбома», эксперимент, ~"+gbStr(sizeSeedvcGB)+" ГБ (спроси пользователя)", "boolean"),
			"sep_dir": prop("каталог разделения на дорожки (RoFormer): окружение <dir>/venv и веса <dir>/models, можно на другом диске "+
				"(пусто — ~/sep-venv и ~/sep-models или то, что уже записано в worker.env)", "string"),
			"seedvc_dir": prop("каталог Seed-VC (по умолчанию ~/partscaster/seedvc; можно на другом диске)", "string"),
			"hf_home":    prop("каталог кеша весов HF (по умолчанию ~/yue/hf-cache)", "string"),
			"cpp_model": prop("модель генерации: auto — рекомендуемая по видеопамяти (по умолчанию) | bf16 (~10 ГБ видеопамяти) | "+
				"q8 (~8 ГБ) | q6 | q5; при переустановке без cpp_model скачанный выбор не меняется", "string"),
			"archive": prop("архив воркера worker-<версия>.tar.gz (пусто — собрать из исходников make worker-tar, "+
				"либо взять лежащий рядом с partscaster-mcp из релизного пакета)", "string"),
		}),
		Handler: func(s *Server, args map[string]any) (string, error) {
			dryRun := true
			if v, ok := args["dry_run"].(bool); ok {
				dryRun = v
			}
			if !dryRun && !argBool(args, "confirm") {
				return "", errConfirm
			}
			whisper, wOK := args["whisper"].(bool)
			seedvc, sOK := args["seedvc"].(bool)
			if !dryRun && (!wOK || !sOK) {
				return "", fmt.Errorf("перед установкой спроси пользователя, нужны ли необязательные компоненты, " +
					"и передай явно whisper=true|false и seedvc=true|false")
			}
			cppModel := argString(args, "cpp_model")
			if err := checkCppModel(cppModel); err != nil {
				return "", err
			}
			hfHome := argString(args, "hf_home")
			if hfHome == "" {
				hfHome = "~/yue/hf-cache"
			}
			seedvcDir := argString(args, "seedvc_dir")
			if seedvcDir == "" {
				seedvcDir = "~/partscaster/seedvc"
			}
			sepDir := argString(args, "sep_dir")
			// пути уходят в shell-команды экранированными (shellQuote) и в worker.env строкой:
			// перевод строки и нулевой байт туда не записать — такие значения отклоняются
			for name, v := range map[string]string{"hf_home": hfHome, "seedvc_dir": seedvcDir, "sep_dir": sepDir} {
				if strings.ContainsAny(v, "\n\r\x00") {
					return "", fmt.Errorf("%s: путь не может содержать перевод строки или нулевой байт (получено %q)", name, v)
				}
			}
			archive := argString(args, "archive")
			if archive == "" {
				if dryRun {
					archive = "<архив воркера: соберётся make worker-tar или возьмётся рядом с partscaster-mcp>"
				} else {
					a, err := s.workerArchive()
					if err != nil {
						return "", err
					}
					archive = a
				}
			}
			plan, steps := workerInstallPlanFull(argString(args, "host"), archive, whisper, seedvc, hfHome, seedvcDir, sepDir)
			plan, steps = withCppModel(plan, steps, cppModel)
			out, err := runSteps(argString(args, "host"), steps, !dryRun)
			out = plan + "\n" + out + workerAddress(s)
			if dryRun {
				out = "СУХОЙ ПРОГОН — ничего не выполнено, только план (установка: dry_run=false, confirm=true, whisper, seedvc).\n\n" + out
			}
			if err != nil {
				return "", fmt.Errorf("%w\n\n%s", err, out)
			}
			return out, nil
		},
	})

	s.Register(Tool{
		Name:        "install_app",
		Description: "Собрать desktop-приложение Partscaster на этом ПК (wails build; нужны Go, Node, wails CLI). Сухой прогон по умолчанию — только план.",
		InputSchema: props(map[string]any{
			"dry_run": prop("только показать план, ничего не выполняя (по умолчанию true)", "boolean"),
			"confirm": prop("выполнить сборку (спроси пользователя)", "boolean"),
		}),
		Handler: func(_ *Server, args map[string]any) (string, error) {
			dryRun := true
			if v, ok := args["dry_run"].(bool); ok {
				dryRun = v
			}
			if !dryRun && !argBool(args, "confirm") {
				return "", errConfirm
			}
			out, err := runSteps("", []step{
				{name: "фронтенд", cmd: "cd frontend && npm ci && npm run build", must: true},
				{name: "wails", cmd: "wails build"},
			}, !dryRun)
			if dryRun {
				out = "СУХОЙ ПРОГОН — ничего не выполнено (сборка: dry_run=false + confirm=true):\n" + out
			}
			if err != nil {
				return "", fmt.Errorf("%w\n\n%s", err, out)
			}
			return out, nil
		},
	})
}

// Размеры компонентов воркера, ГБ (замер на установленной машине, 2026-10-03; модель генерации — 2026-10-19):
// воркер — окружение 7,4 + модель генерации до 7,7 (BF16 7,2 + VAE 0,5; Q8 — 4,3), MERT 2,4 и SheetSage 0,2
// (разбор нот); whisper — окружение 2,7 + модель small 0,5; Seed-VC — 11 после
// установки и ~14 на время установки (см. seedvc_install.sh).
const (
	sizeWorkerGB     = 17.8
	sizeWhisperGB    = 3.2
	sizeSeedvcGB     = 11
	sizeSeedvcPeakGB = 14
	sizeSepGB        = 7.3 // разделение дорожек: окружение audio-separator 6,1 + веса RoFormer/DrumSep 1,1
	sizeReserveGB    = 2   // запас под данные треков на первое время
	sizeKitsGB       = 1.2 // наборы сэмплов заранее (kits_install.py): барабаны, басы, оркестр
)

// gbStr — «17.5», «11»: без лишних нулей.
func gbStr(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// shellQuote — путь как одно слово shell без подстановок: «'…'» (одинарная кавычка внутри
// — «'\”»), «~/…» → «"$HOME"/'…'». Пробелы, кириллица, $, `, кавычки проходят буквально.
// $HOME раскрывается сразу — в worker.env пишется абсолютный путь (systemd тильду не раскрывает).
func shellQuote(p string) string {
	q := func(x string) string { return "'" + strings.ReplaceAll(x, "'", `'\''`) + "'" }
	if p == "~" {
		return `"$HOME"`
	}
	if strings.HasPrefix(p, "~/") {
		return `"$HOME"/` + q(p[2:])
	}
	return q(p)
}

// workerArchive — архив воркера для установки: из исходников (MCP собран make mcp — знает каталог
// репозитория) собирается make worker-tar; релизный MCP берёт worker-*.tar.gz из своего каталога.
func (s *Server) workerArchive() (string, error) {
	if s.stale != nil && s.stale.srcDir != "" {
		out, err := exec.Command("make", "-s", "-C", s.stale.srcDir, "worker-tar").CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("сборка архива воркера (make worker-tar): %w\n%s", err, out)
		}
		// последняя строка целиком: путь репозитория может содержать пробелы
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		return strings.TrimSpace(lines[len(lines)-1]), nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	found, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), "worker-*.tar.gz"))
	if len(found) == 0 {
		return "", fmt.Errorf("архив воркера не найден рядом с %s — передайте его в archive", exe)
	}
	return found[len(found)-1], nil
}

// workerGet — команда: GET локального воркера (на машине с GPU) с выводом тела. curl там может не быть
// (поставили только драйвер и uv) — тогда python из venv воркера, он есть после установки всегда.
func workerGet(path string, timeoutSec int) string {
	url := "http://localhost:8091" + path
	return fmt.Sprintf(`{ curl -sf --max-time %[1]d %[2]s || ~/yue/.venv/bin/python -c `+
		`'import sys,urllib.request; sys.stdout.write(urllib.request.urlopen("%[2]s", timeout=%[1]d).read().decode())'; }`,
		timeoutSec, url)
}

// workerAddress — строка в конец отчёта: по какому адресу MCP ходит к воркеру.
func workerAddress(s *Server) string {
	return fmt.Sprintf("\nадрес воркера сейчас: %s (сменить: config_set)", s.client.GetURL())
}

// diskNeedGB — сколько места нужно на домашнем диске (Seed-VC — по пику установки; разделение — если не в sep_dir).
func diskNeedGB(whisper, seedvc, sepHome bool) float64 {
	need := sizeWorkerGB + sizeKitsGB + sizeReserveGB
	if sepHome {
		need += sizeSepGB
	}
	if whisper {
		need += sizeWhisperGB
	}
	if seedvc {
		need += sizeSeedvcPeakGB
	}
	return need
}

// workerInstallPlanFull — план установки: host "" — на этом ПК, иначе на машине по ssh; archive — архив воркера
// (make worker-tar), его install.sh ставит базу воркера, разделение на дорожки и модель генерации.
// sepDir — каталог разделения: "" — как в worker.env или ~/sep-venv и ~/sep-models, иначе <sepDir>/venv и
// <sepDir>/models (install.sh --sep-dir пишет их в worker.env; место на домашнем диске тогда не считается).
func workerInstallPlanFull(host, archive string, whisper, seedvc bool, hfHome, seedvcDir, sepDir string) (string, []step) {
	need := diskNeedGB(whisper, seedvc, sepDir == "")
	return planText(whisper, seedvc, need, hfHome, seedvcDir, sepDir), installSteps(host, archive, whisper, seedvc, need, hfHome, seedvcDir, sepDir)
}

// checkCppModel — модель генерации для install.sh (--cpp-model); проверка до шагов.
func checkCppModel(cppModel string) error {
	switch cppModel {
	case "", "auto", "bf16", "q8", "q6", "q5":
		return nil
	}
	return fmt.Errorf("cpp_model: auto, bf16, q8, q6 или q5 (получено %q)", cppModel)
}

// withCppModel — модель генерации в плане и в шаге install.sh; без cpp_model install.sh оставит скачанный выбор,
// а без него поставит рекомендуемую.
func withCppModel(plan string, steps []step, cppModel string) (string, []step) {
	if cppModel == "" {
		return plan + "Модель генерации (yue2.cpp): выбранная раньше, если скачана; иначе — рекомендуемая по видеопамяти.\n", steps
	}
	if cppModel == "auto" {
		plan += "Модель генерации (yue2.cpp): рекомендуемая по видеопамяти.\n"
	} else {
		plan += "Модель генерации (yue2.cpp): " + cppModel + ".\n"
	}
	for i := range steps {
		steps[i].cmd = strings.Replace(steps[i].cmd, "/install.sh --no-start", "/install.sh --no-start --cpp-model "+cppModel, 1)
	}
	return plan, steps
}

func planText(whisper, seedvc bool, need float64, hfHome, seedvcDir, sepDir string) string {
	mark := map[bool]string{true: "[x]", false: "[ ]"}
	var b strings.Builder
	b.WriteString("Компоненты:\n")
	fmt.Fprintf(&b, "  [x] воркер — генерация (YuE2 на yue2.cpp), разбор нот: до ~%s ГБ "+
		"(модель генерации — при установке, веса разбора нот — при первом запуске)\n",
		gbStr(sizeWorkerGB))
	sep := "~/sep-venv и ~/sep-models (или пути из worker.env)"
	if sepDir != "" {
		sep = fmt.Sprintf("%[1]s/venv и %[1]s/models (PARTSCASTER_SEP_PY и PARTSCASTER_SEP_MODELS в worker.env)", sepDir)
	}
	fmt.Fprintf(&b, "  [x] разделение на дорожки RoFormer (+ барабаны по частям): ~%s ГБ, веса некоммерческие (CC BY-NC-SA), "+
		"каталог %s\n", gbStr(sizeSepGB), sep)
	fmt.Fprintf(&b, "  %s whisper — распознавание текстов треков: ~%s ГБ\n", mark[whisper], gbStr(sizeWhisperGB))
	fmt.Fprintf(&b, "  %s Seed-VC — «голос альбома», ЭКСПЕРИМЕНТ (голос узнаётся, но дрожит): ~%s ГБ после установки, ~%s ГБ во время, каталог %s\n",
		mark[seedvc], gbStr(sizeSeedvcGB), gbStr(sizeSeedvcPeakGB), seedvcDir)
	fmt.Fprintf(&b, "Нужно места: ~%s ГБ (веса HF — в %s; если он на другом диске, места на домашнем нужно меньше).\n",
		gbStr(need), hfHome)
	b.WriteString("Без whisper и Seed-VC приложение работает; соответствующие функции будут неактивны с подписью «не установлен».\n")
	return b.String()
}

// Проверки до установки: их провал останавливает установку (must).
const (
	checkGPUCmd = "nvidia-smi --query-gpu=name,memory.total --format=csv,noheader || " +
		"{ echo 'нет NVIDIA GPU или драйвера: воркеру нужна видеокарта с CUDA'; exit 1; }"
	// %[1]d — сколько ГБ нужно; df -BG печатает свободное место в гигабайтах
	checkDiskCmd = `free=$(df -BG --output=avail ~ | tail -1 | tr -dc 0-9); echo "свободно $free ГБ, нужно ~%[1]d ГБ"; ` +
		`[ "$free" -ge %[1]d ] || { echo 'мало места: освободите диск или поставьте веса/Seed-VC на другой (hf_home, seedvc_dir)'; exit 1; }`
	// uv по ssh: установщик uv кладёт его в ~/.local/bin (старые — ~/.cargo/bin), а ssh без входа
	// в оболочку этих каталогов в PATH не добавляет — «uv: команда не найдена»
	uvPathCmd = `export PATH="$HOME/.local/bin:$HOME/.cargo/bin:$PATH" && `
)

func installSteps(host, archive string, whisper, seedvc bool, need float64, hfHome, seedvcDir, sepDir string) []step {
	steps := []step{
		{name: "проверка: видеокарта NVIDIA и драйвер", cmd: checkGPUCmd, must: true},
		{name: "проверка: место на диске", cmd: fmt.Sprintf(checkDiskCmd, int(math.Ceil(need))), must: true},
	}
	// база воркера — install.sh из архива: версия в ~/partscaster/releases, ссылка current, venv и пакеты,
	// наборы сэмплов, юнит, worker.env (HF_HOME); запуск — позже, после компонентов
	src, cleanup := shellQuote(archive), `rm -rf "$d"`
	if host != "" {
		// в домашний каталог (он точно есть); после распаковки архив удаляется
		name := filepath.Base(archive)
		steps = append(steps, step{name: "копировать архив воркера на " + host, must: true, local: true,
			cmd: fmt.Sprintf("scp -q %s %s:%s", src, shellQuote(host), shellQuote(name))})
		src = shellQuote("~/" + name)
		cleanup += " " + src
	}
	sepFlag := ""
	if sepDir != "" {
		sepFlag = " --sep-dir " + shellQuote(sepDir)
	}
	steps = append(steps,
		step{name: "воркер из архива (install.sh: код, окружение, пакеты, разделение на дорожки, модель генерации, наборы сэмплов)",
			must: true, cmd: fmt.Sprintf(
				`d=$(mktemp -d) && trap '%s' EXIT && tar xzf %s -C "$d" && bash "$d"/worker-*/install.sh --no-start --hf-home %s%s`,
				strings.ReplaceAll(cleanup, "'", `'\''`), src, shellQuote(hfHome), sepFlag)},
	)
	if whisper {
		steps = append(steps, step{name: "whisper (тексты треков)", cmd: uvPathCmd + "uv venv ~/whisper-venv && " +
			"uv pip install --python ~/whisper-venv/bin/python faster-whisper"})
	}
	if seedvc {
		dir := shellQuote(seedvcDir)
		steps = append(steps, step{name: "Seed-VC («голос альбома», эксперимент)", cmd: fmt.Sprintf(
			`~/partscaster/current/seedvc_install.sh %[1]s && (grep -q '^PARTSCASTER_SEEDVC_DIR=' ~/partscaster/worker.env || `+
				`printf 'PARTSCASTER_SEEDVC_DIR=%%s\n' %[1]s >> ~/partscaster/worker.env)`, dir)})
	}
	return append(steps,
		step{name: "systemd-юнит", cmd: "cp ~/partscaster/units/partscaster-worker.service ~/.config/systemd/user/ 2>/dev/null && " +
			// restart, а не только enable --now: при переустановке работающий воркер должен взять новую версию
			"systemctl --user daemon-reload && systemctl --user enable partscaster-worker && systemctl --user restart partscaster-worker || " +
			"echo 'юнит пропущен (запуск вручную: cd ~/partscaster/current && ~/yue/.venv/bin/python -m uvicorn yue_worker:app --port 8091)'"},
		step{name: "health", cmd: "sleep 3 && " + workerGet("/health", 10)},
	)
}

type step struct {
	name string
	cmd  string
	// must — без этого шага дальше нельзя (проверка GPU/места, venv): провал
	// останавливает установку, остальные шаги не выполняются
	must bool
	// local — выполнить на этом ПК даже при host (копирование архива на машину с GPU)
	local bool
}

// runSteps выполняет команды локально (host="") или через ssh; run=false —
// только печатает шаги (сухой прогон, ничего не выполняется). Шаги независимы,
// кроме must: его провал останавливает выполнение и возвращается ошибкой.
func runSteps(host string, steps []step, run bool) (string, error) {
	var b strings.Builder
	for i, st := range steps {
		fmt.Fprintf(&b, "— %s\n  $ %s\n", st.name, st.cmd)
		if !run {
			continue
		}
		cmd := exec.Command("bash", "-c", st.cmd)
		if host != "" && !st.local {
			cmd = exec.Command("ssh", host, "bash", "-s")
			cmd.Stdin = strings.NewReader(st.cmd)
		}
		out, err := cmd.CombinedOutput()
		if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
			b.WriteString("  " + strings.ReplaceAll(trimmed, "\n", "\n  ") + "\n")
		}
		if err == nil {
			continue
		}
		fmt.Fprintf(&b, "  [ошибка: %v]\n", err)
		if st.must {
			fmt.Fprintf(&b, "\nОСТАНОВЛЕНО: шаг «%s» обязателен; не выполнено шагов: %d.\n", st.name, len(steps)-i-1)
			return b.String(), fmt.Errorf("установка остановлена: шаг «%s» не прошёл", st.name)
		}
	}
	return b.String(), nil
}
