package mcp

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Тесты установщика:
// сухой прогон ничего не выполняет, must-шаг останавливает установку,
// реальная установка требует confirm и явного выбора whisper/seedvc.

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func TestInstallRunStepsDryRunExecutesNothing(t *testing.T) {
	t.Parallel()
	f := filepath.Join(t.TempDir(), "dry")
	steps := []step{{name: "создать метку", cmd: "touch " + shellQuoteTest(f)}}
	out, err := runSteps("", steps, false)
	if err != nil {
		t.Fatalf("dry run: unexpected error %v", err)
	}
	if fileExists(f) {
		t.Fatalf("dry run executed the step: %s exists", f)
	}
	if !strings.Contains(out, "создать метку") || !strings.Contains(out, "touch "+shellQuoteTest(f)) {
		t.Fatalf("dry run output must list step name and command, got:\n%s", out)
	}
}

func TestInstallRunStepsRunsLocally(t *testing.T) {
	t.Parallel()
	f := filepath.Join(t.TempDir(), "ran")
	_, _ = runSteps("", []step{{name: "метка", cmd: "touch " + shellQuoteTest(f)}}, true)
	if !fileExists(f) {
		t.Fatalf("run=true did not execute the step locally")
	}
}

func TestInstallRunStepsNonMustFailureContinues(t *testing.T) {
	t.Parallel()
	f := filepath.Join(t.TempDir(), "after")
	out, _ := runSteps("", []step{
		{name: "необязательный провал", cmd: "exit 1"},
		{name: "следующий", cmd: "touch " + shellQuoteTest(f)},
	}, true)
	if !fileExists(f) {
		t.Fatalf("failing non-must step stopped later steps; output:\n%s", out)
	}
	if strings.Contains(out, "ОСТАНОВЛЕНО") {
		t.Fatalf("non-must failure must not stop the run; output:\n%s", out)
	}
}

func TestInstallRunStepsMustFailureStops(t *testing.T) {
	t.Parallel()
	f := filepath.Join(t.TempDir(), "never")
	out, err := runSteps("", []step{
		{name: "проверка", cmd: "exit 1", must: true},
		{name: "следующий", cmd: "touch " + shellQuoteTest(f)},
	}, true)
	if err == nil {
		t.Fatalf("failed must-step must return an error")
	}
	if fileExists(f) {
		t.Fatalf("step after failed must-step was executed")
	}
	if !strings.Contains(out, "ОСТАНОВЛЕНО") {
		t.Fatalf("output must contain ОСТАНОВЛЕНО, got:\n%s", out)
	}
}

func TestInstallWorkerDefaultIsDryRun(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t)
	start := time.Now()
	out, ok := call(t, s, "install_worker", map[string]any{})
	if !ok {
		t.Fatalf("install_worker without args must succeed as dry run, got error: %s", out)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("dry run took %v — looks like something was executed", d)
	}
	for _, want := range []string{"СУХОЙ ПРОГОН", "воркер", "whisper", "Seed-VC", "ГБ"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry run output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "[ошибка") {
		t.Errorf("dry run must not execute steps, but output has error lines:\n%s", out)
	}
}

func TestInstallWorkerRequiresConfirm(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t)
	out, ok := call(t, s, "install_worker", map[string]any{"dry_run": false, "whisper": false, "seedvc": false})
	if ok {
		t.Fatalf("dry_run=false without confirm must be an error, got:\n%s", out)
	}
	if !strings.Contains(out, "confirm") {
		t.Errorf("error should mention confirm: %s", out)
	}
}

func TestInstallWorkerRequiresExplicitComponents(t *testing.T) {
	t.Parallel()
	cases := []map[string]any{
		{"dry_run": false, "confirm": true},                  // оба не заданы
		{"dry_run": false, "confirm": true, "whisper": true}, // нет seedvc
		{"dry_run": false, "confirm": true, "seedvc": false}, // нет whisper
	}
	for _, args := range cases {
		s, _ := newTestServer(t)
		out, ok := call(t, s, "install_worker", args)
		if ok {
			t.Fatalf("args %v: must be an error (components not chosen), got:\n%s", args, out)
		}
		if !strings.Contains(out, "whisper") || !strings.Contains(out, "seedvc") {
			t.Errorf("args %v: error must mention whisper and seedvc: %s", args, out)
		}
		if !strings.Contains(strings.ToLower(out), "спрос") {
			t.Errorf("args %v: error must say the user has to be asked: %s", args, out)
		}
	}
}

// findNamed ищет шаг по имени (шаг компонента узнаётся по названию; упоминание
// в чужой команде шагом компонента не является).
func findNamed(steps []step, sub string) (step, bool) {
	for _, st := range steps {
		if strings.Contains(strings.ToLower(st.name), strings.ToLower(sub)) {
			return st, true
		}
	}
	return step{}, false
}

func findStep(steps []step, sub string) (step, bool) {
	for _, st := range steps {
		if strings.Contains(st.name, sub) || strings.Contains(st.cmd, sub) {
			return st, true
		}
	}
	return step{}, false
}

var numRe = regexp.MustCompile(`\d+(?:\.\d+)?`)

func diskNumbers(t *testing.T, steps []step) []float64 {
	t.Helper()
	if len(steps) < 2 {
		t.Fatalf("plan has %d steps, want at least 2", len(steps))
	}
	var nums []float64
	for _, m := range numRe.FindAllString(steps[1].name+" "+steps[1].cmd, -1) {
		v, _ := strconv.ParseFloat(m, 64)
		nums = append(nums, v)
	}
	return nums
}

// install_worker ставит воркер архивом.
//
// План: проверки GPU/места → (при host) локальный шаг scp архива на машину → распаковка →
// install.sh --no-start --hf-home <раскрытый путь> → прежние шаги компонентов, настройка,
// systemd, health. Файлы воркера по одному не копируются. Seed-VC берёт скрипт из
// ~/partscaster/current/.

// testArchive — путь к архиву воркера для чистых проверок плана (файл не читается).
const testArchive = "/tmp/yue-test/worker-9.9.9-test.tar.gz"

// remoteHost — машина с GPU в тестах плана (шаги не выполняются по ssh).
const remoteHost = "user@gpu-host"

func planBase(host string, whisper, seedvc bool, hfHome, seedvcDir string) (string, []step) {
	return workerInstallPlanFull(host, testArchive, whisper, seedvc, false, hfHome, seedvcDir, "", "")
}

func planRF(host string, whisper, seedvc, roformer bool, hfHome, seedvcDir string) (string, []step) {
	return workerInstallPlanFull(host, testArchive, whisper, seedvc, roformer, hfHome, seedvcDir, "", "")
}

func planSep(host string, whisper, seedvc, roformer bool, hfHome, seedvcDir, sepDir string) (string, []step) {
	return workerInstallPlanFull(host, testArchive, whisper, seedvc, roformer, hfHome, seedvcDir, sepDir, "")
}

func TestInstallPlanFirstStepsCheckGPUAndDisk(t *testing.T) {
	t.Parallel()
	for _, host := range []string{"", remoteHost} {
		_, steps := planBase(host, false, false, "~/yue/hf-cache", "~/partscaster/seedvc")
		if len(steps) < 2 {
			t.Fatalf("host=%q: plan too short: %d", host, len(steps))
		}
		if !steps[0].must || !strings.Contains(steps[0].cmd, "nvidia-smi") {
			t.Errorf("host=%q: first step must be a must-step checking GPU via nvidia-smi: %+v", host, steps[0])
		}
		if !steps[1].must || !strings.Contains(steps[1].cmd, "df") {
			t.Errorf("host=%q: second step must be a must-step checking disk space: %+v", host, steps[1])
		}
		// проверки — про машину с GPU, а не про этот ПК
		if steps[0].local || steps[1].local {
			t.Errorf("host=%q: GPU/disk checks must run on the worker machine, not locally", host)
		}
	}
}

func TestInstallPlanDiskGrowsWithComponents(t *testing.T) {
	t.Parallel()
	_, base := planBase("", false, false, "~/h", "~/s")
	_, withW := planBase("", true, false, "~/h", "~/s")
	_, withS := planBase("", false, true, "~/h", "~/s")
	nb, nw, ns := diskNumbers(t, base), diskNumbers(t, withW), diskNumbers(t, withS)
	// требование к месту — первое число шага, которое меняется от выбора компонентов
	growth := func(a, b []float64) float64 {
		if len(a) != len(b) {
			t.Fatalf("disk step shape differs: %v vs %v", a, b)
		}
		for i := range a {
			if b[i] != a[i] {
				return b[i] - a[i]
			}
		}
		return 0
	}
	if d := growth(nb, nw); d < 2 || d > 4 {
		t.Errorf("whisper should add ~3 GB to disk requirement, got %+.1f (%v -> %v)", d, nb, nw)
	}
	if d := growth(nb, ns); d < 12 || d > 16 {
		t.Errorf("seedvc should add ~14 GB to disk requirement, got %+.1f (%v -> %v)", d, nb, ns)
	}
}

func TestInstallPlanOptionalSteps(t *testing.T) {
	t.Parallel()
	for _, host := range []string{"", remoteHost} {
		_, none := planBase(host, false, false, "~/h", "/data/seedvc-x")
		if _, ok := findNamed(none, "whisper"); ok {
			t.Errorf("host=%q: whisper step present with whisper=false", host)
		}
		if _, ok := findNamed(none, "Seed-VC"); ok {
			t.Errorf("host=%q: Seed-VC step present with seedvc=false", host)
		}
		if _, ok := findStep(none, "/data/seedvc-x"); ok {
			t.Errorf("host=%q: seedvc dir used with seedvc=false", host)
		}

		_, all := planBase(host, true, true, "~/h", "/data/seedvc-x")
		if _, ok := findNamed(all, "whisper"); !ok {
			t.Errorf("host=%q: whisper step missing with whisper=true", host)
		}
		st, ok := findNamed(all, "Seed-VC")
		if !ok {
			t.Errorf("host=%q: Seed-VC step missing with seedvc=true", host)
		} else if _, ok := findStep(all, "/data/seedvc-x"); !ok {
			t.Errorf("host=%q: Seed-VC step does not use given dir: %+v", host, st)
		}
	}
}

// Файлы воркера по одному не копируются ни в одном режиме — только архив.
func TestInstallPlanNoPerFileCopy(t *testing.T) {
	t.Parallel()
	for _, host := range []string{"", remoteHost} {
		for mask := 0; mask < 8; mask++ {
			_, steps := workerInstallPlanFull(host, testArchive, mask&1 != 0, mask&2 != 0, mask&4 != 0,
				"~/h", "~/s", "", "")
			for _, st := range steps {
				for _, bad := range []string{"worker/*.py", "worker/*.json", "cp worker/", "worker/phrases"} {
					if strings.Contains(st.cmd, bad) {
						t.Errorf("host=%q: step %q copies worker files one by one (%s): %s", host, st.name, bad, st.cmd)
					}
				}
			}
		}
	}
}

// installShIndex — номер шага, запускающего install.sh из архива.
func installShIndex(t *testing.T, steps []step) int {
	t.Helper()
	for i, st := range steps {
		if strings.Contains(st.cmd, "install.sh") && strings.Contains(st.cmd, "--no-start") {
			return i
		}
	}
	t.Fatalf("no step runs install.sh --no-start")
	return -1
}

// При host первым после проверок — локальный шаг scp архива; остальные шаги — на машине с GPU.
func TestInstallPlanRemoteCopiesArchiveFirst(t *testing.T) {
	t.Parallel()
	archive := "/home/me/Yue builds/worker-1.2.3.tar.gz"
	_, steps := workerInstallPlanFull(remoteHost, archive, false, false, false, "~/h", "~/s", "", "")
	if len(steps) < 4 {
		t.Fatalf("plan too short: %d", len(steps))
	}
	scp := steps[2]
	if !scp.local {
		t.Errorf("step after checks must be a local step (scp from this PC): %+v", scp)
	}
	if !strings.Contains(scp.cmd, "scp") || !strings.Contains(scp.cmd, remoteHost) ||
		!strings.Contains(scp.cmd, "worker-1.2.3.tar.gz") {
		t.Errorf("step after checks must scp the archive to %s: %s", remoteHost, scp.cmd)
	}
	for i, st := range steps {
		if i != 2 && st.local {
			t.Errorf("step #%d %q is local, but only scp runs on this PC in remote mode", i, st.name)
		}
	}
	if k := installShIndex(t, steps); k <= 2 {
		t.Errorf("install.sh step #%d must come after scp (#2)", k)
	}
}

// scp получает путь архива одним аргументом (пробелы в пути), заглушка scp пишет аргументы.
func TestInstallPlanRemoteScpArchiveOneArg(t *testing.T) {
	t.Parallel()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}
	dir := filepath.Join(t.TempDir(), "my builds")
	archive := filepath.Join(dir, "worker-1.2.3.tar.gz")
	_, steps := workerInstallPlanFull(remoteHost, archive, false, false, false, "~/h", "~/s", "", "")
	if len(steps) < 3 {
		t.Fatalf("plan too short: %d", len(steps))
	}
	stubs := t.TempDir()
	log := filepath.Join(stubs, "scp.log")
	stub := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done >> " + shellQuoteTest(log) + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(stubs, "scp"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bash, "-c", steps[2].cmd)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "PATH="+stubs+":"+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("scp step failed: %v\n%s\ncmd: %s", err, out, steps[2].cmd)
	}
	data, _ := os.ReadFile(log)
	if !hasArg(string(data), archive) {
		t.Errorf("scp did not get archive path %q as one argument:\n%s", archive, data)
	}
	if !strings.Contains(string(data), remoteHost+":") {
		t.Errorf("scp destination is not on %s:\n%s", remoteHost, data)
	}
}

// Всё на одном ПК — архив никуда не копируется по scp.
func TestInstallPlanLocalNoScp(t *testing.T) {
	t.Parallel()
	_, steps := planBase("", true, true, "~/h", "~/s")
	for _, st := range steps {
		if strings.Contains(st.cmd, "scp ") {
			t.Errorf("local install must not scp: step %q: %s", st.name, st.cmd)
		}
	}
}

// install.sh — после проверок (и scp), до шагов компонентов, настройки, systemd и health.
func TestInstallPlanInstallShBeforeComponents(t *testing.T) {
	t.Parallel()
	for _, host := range []string{"", remoteHost} {
		_, steps := workerInstallPlanFull(host, testArchive, true, true, true, "~/h", "~/s", "/data/sep", "roformer")
		k := installShIndex(t, steps)
		if k < 2 {
			t.Errorf("host=%q: install.sh step #%d must come after GPU/disk checks", host, k)
		}
		for i, st := range steps[:k] {
			n := strings.ToLower(st.name)
			for _, later := range []string{"whisper", "seed-vc", "roformer", "systemd", "health", "настройка"} {
				if strings.Contains(n, later) {
					t.Errorf("host=%q: step #%d %q must come after install.sh (#%d)", host, i, st.name, k)
				}
			}
		}
		if !strings.Contains(steps[k].cmd, "--hf-home") {
			t.Errorf("host=%q: install.sh step lacks --hf-home: %s", host, steps[k].cmd)
		}
	}
}

func TestInstallPlanSeedVCScriptFromCurrent(t *testing.T) {
	t.Parallel()
	for _, host := range []string{"", remoteHost} {
		_, steps := planBase(host, false, true, "~/h", "~/s")
		st, ok := findNamed(steps, "Seed-VC")
		if !ok {
			t.Fatalf("host=%q: Seed-VC step missing", host)
		}
		if !strings.Contains(st.cmd, "partscaster/current/seedvc_install.sh") {
			t.Errorf("host=%q: Seed-VC must run ~/partscaster/current/seedvc_install.sh: %s", host, st.cmd)
		}
	}
}

// runSteps: шаг local выполняется на этом ПК даже при host; прочие — через ssh.
func TestInstallRunStepsLocalStepWithHost(t *testing.T) {
	stubs := t.TempDir()
	log := filepath.Join(stubs, "ssh.log")
	stub := "#!/bin/sh\necho \"ssh $*\" >> " + shellQuoteTest(log) + "\ncat >/dev/null\nexit 0\n"
	if err := os.WriteFile(filepath.Join(stubs, "ssh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubs+":"+os.Getenv("PATH"))
	localMark := filepath.Join(t.TempDir(), "local")
	remoteMark := filepath.Join(t.TempDir(), "remote")
	_, err := runSteps(remoteHost, []step{
		{name: "локальный", cmd: "touch " + shellQuoteTest(localMark), local: true},
		{name: "удалённый", cmd: "touch " + shellQuoteTest(remoteMark)},
	}, true)
	if err != nil {
		t.Fatalf("runSteps: %v", err)
	}
	if !fileExists(localMark) {
		t.Errorf("local step was not executed on this PC")
	}
	if fileExists(remoteMark) {
		t.Errorf("non-local step with host was executed locally instead of ssh")
	}
	data, _ := os.ReadFile(log)
	if n := strings.Count(string(data), "ssh "); n != 1 {
		t.Errorf("ssh called %d times, want 1 (only the non-local step):\n%s", n, data)
	}
	if !strings.Contains(string(data), remoteHost) {
		t.Errorf("ssh not called with host %s:\n%s", remoteHost, data)
	}
}

// trapAll — как trapShells, но и scp, make, tar, git, rsync: сухой прогон не должен
// запускать ничего, в том числе сборку архива (make worker-tar).
func trapAll(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\necho \"$0 $*\" >> " + shellQuoteTest(log) + "\nexit 0\n"
	for _, name := range []string{"bash", "sh", "ssh", "scp", "make", "tar", "git", "rsync", "curl", "uv"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	return log
}

func TestInstallWorkerDryRunExecutesNothing(t *testing.T) {
	for _, host := range []string{"", remoteHost} {
		for _, withArchive := range []bool{false, true} {
			t.Run(host+map[bool]string{true: "/archive", false: "/no-archive"}[withArchive], func(t *testing.T) {
				log := trapAll(t)
				s, _ := newTestServer(t)
				args := map[string]any{"host": host, "whisper": false, "seedvc": false, "roformer": false}
				if withArchive {
					args["archive"] = dummyArchive(t)
				}
				out, ok := call(t, s, "install_worker", args)
				if !ok {
					t.Fatalf("dry run must succeed, got error: %s", out)
				}
				if !strings.Contains(out, "СУХОЙ ПРОГОН") || !strings.Contains(out, "install.sh") {
					t.Errorf("dry run must show the plan with install.sh:\n%s", out)
				}
				assertNothingRan(t, log)
			})
		}
	}
}

// Сухой прогон с host и архивом показывает копирование именно этого архива.
func TestInstallWorkerDryRunShowsArchiveScp(t *testing.T) {
	log := trapAll(t)
	s, _ := newTestServer(t)
	archive := dummyArchive(t)
	out, ok := call(t, s, "install_worker", map[string]any{"host": remoteHost, "archive": archive,
		"whisper": false, "seedvc": false, "roformer": false})
	if !ok {
		t.Fatalf("dry run must succeed, got error: %s", out)
	}
	if !strings.Contains(out, "scp") || !strings.Contains(out, filepath.Base(archive)) {
		t.Errorf("remote dry run must show scp of %s:\n%s", archive, out)
	}
	assertNothingRan(t, log)
}

// dummyArchive — файл-архив для аргумента archive (содержимое не читается: команды подменены).
func dummyArchive(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "worker-9.9.9-test.tar.gz")
	if err := os.WriteFile(p, []byte("not a real archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// installSandbox — HOME во временном каталоге, заглушки внешних программ в PATH и
// настоящий архив воркера, в котором install.sh — заглушка: пишет свои аргументы в
// журнал и создаёт то, что создал бы настоящий (каталоги и пустой worker.env).
type installSandbox struct {
	home, stubs, log, archive string
}

const stubVersion = "9.9.9-test"

func newInstallSandbox(t *testing.T) *installSandbox {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found")
	}
	sb := &installSandbox{home: t.TempDir(), stubs: t.TempDir()}
	sb.log = filepath.Join(sb.stubs, "stubs.log")
	stub := "#!/bin/sh\n{ echo \"== $(basename \"$0\")\"; for a in \"$@\"; do printf '%s\\n' \"$a\"; done; } >> " +
		shellQuoteTest(sb.log) + "\nexit 0\n"
	for _, name := range []string{"uv", "systemctl", "curl", "nvidia-smi", "sleep", "scp", "ssh"} {
		if err := os.WriteFile(filepath.Join(sb.stubs, name), []byte(stub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// установленная версия: Seed-VC берёт скрипт из ~/partscaster/current/
	cur := filepath.Join(sb.home, "partscaster", "current")
	if err := os.MkdirAll(cur, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cur, "seedvc_install.sh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	installStub := "#!/bin/sh\n{ echo '== install.sh'; for a in \"$@\"; do printf '%s\\n' \"$a\"; done; } >> " +
		shellQuoteTest(sb.log) + "\n" +
		"mkdir -p \"$HOME/partscaster/data\" \"$HOME/partscaster/units\"\n" +
		"[ -f \"$HOME/partscaster/worker.env\" ] || : > \"$HOME/partscaster/worker.env\"\nexit 0\n"
	sb.archive = filepath.Join(sb.stubs, "worker-"+stubVersion+".tar.gz")
	writeTarGz(t, sb.archive, "worker-"+stubVersion, map[string]tarFile{
		"install.sh":        {installStub, 0o755},
		"seedvc_install.sh": {stub, 0o755},
		"VERSION":           {stubVersion + "\n", 0o644},
		"yue_worker.py":     {"# заглушка\n", 0o644},
		"requirements.txt":  {"fastapi\n", 0o644},
	})
	return sb
}

type tarFile struct {
	body string
	mode int64
}

// writeTarGz — архив с одним каталогом верхнего уровня top/ (раскладка worker-tar.sh).
func writeTarGz(t *testing.T, path, top string, files map[string]tarFile) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: top + "/", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	for name, tf := range files {
		hdr := &tar.Header{Name: top + "/" + name, Mode: tf.mode, Size: int64(len(tf.body)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(tf.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}

// plan — план локальной установки (host="") с архивом песочницы.
func (sb *installSandbox) plan(whisper, seedvc, roformer bool, hfHome, seedvcDir, sepDir, stemsModel string) []step {
	_, steps := workerInstallPlanFull("", sb.archive, whisper, seedvc, roformer, hfHome, seedvcDir, sepDir, stemsModel)
	return steps
}

// run выполняет шаги плана (кроме проверок видеокарты и места) в bash с HOME песочницы.
// Возвращает worker.env ("" — нет файла) и журнал заглушек.
func (sb *installSandbox) run(t *testing.T, steps []step) (env, calls string) {
	t.Helper()
	for i, st := range steps {
		if i < 2 { // проверки видеокарты и места — не про пути
			continue
		}
		cmd := exec.Command("bash", "-c", st.cmd)
		cmd.Dir = sb.home
		cmd.Env = append(os.Environ(), "HOME="+sb.home, "PATH="+sb.stubs+":"+os.Getenv("PATH"))
		// сбой шага не роняет тест: проверяются результаты (worker.env, аргументы заглушек)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Logf("step %q failed: %v\n%s", st.name, err, out)
		}
	}
	data, _ := os.ReadFile(filepath.Join(sb.home, "partscaster", "worker.env"))
	c, _ := os.ReadFile(sb.log)
	return string(data), string(c)
}

// invocations — вызовы заглушки name из журнала: аргументы каждого вызова.
func invocations(calls, name string) [][]string {
	var out [][]string
	cur := -1
	for _, line := range strings.Split(calls, "\n") {
		if rest, ok := strings.CutPrefix(line, "== "); ok {
			if rest == name {
				out = append(out, nil)
				cur = len(out) - 1
			} else {
				cur = -1
			}
			continue
		}
		if cur >= 0 {
			out[cur] = append(out[cur], line)
		}
	}
	// последний вызов заканчивается переводом строки — пустой хвост не аргумент
	for i := range out {
		if n := len(out[i]); n > 0 && out[i][n-1] == "" {
			out[i] = out[i][:n-1]
		}
	}
	return out
}

// installShHFHome — значение --hf-home единственного вызова install.sh (и что был --no-start).
func installShHFHome(t *testing.T, calls string) string {
	t.Helper()
	inv := invocations(calls, "install.sh")
	if len(inv) != 1 {
		t.Fatalf("install.sh called %d times, want 1; stub calls:\n%s", len(inv), calls)
	}
	args := inv[0]
	noStart, val, found := false, "", false
	for i, a := range args {
		if a == "--no-start" {
			noStart = true
		}
		if a == "--hf-home" && i+1 < len(args) {
			val, found = args[i+1], true
		}
	}
	if !noStart {
		t.Errorf("install.sh must get --no-start (systemd is started by later steps): %q", args)
	}
	if !found {
		t.Fatalf("install.sh did not get --hf-home <dir>: %q", args)
	}
	return val
}

// По умолчанию install.sh получает HF_HOME, раскрытый от $HOME (systemd тильду не раскрывает).
func TestInstallPlanInstallShGetsExpandedHFHome(t *testing.T) {
	t.Parallel()
	sb := newInstallSandbox(t)
	_, calls := sb.run(t, sb.plan(false, false, false, "~/yue/hf-cache", "~/s", "", ""))
	if got, want := installShHFHome(t, calls), filepath.Join(sb.home, "yue", "hf-cache"); got != want {
		t.Errorf("install.sh --hf-home %q, want %q", got, want)
	}
}

// «~» и «~/…» раскрываются в $HOME, остальное — как задано.
func TestInstallPathsTildeExpandsForInstallSh(t *testing.T) {
	t.Parallel()
	for in, rel := range map[string]string{
		"~/a/b": "a/b",
		"~":     "",
		"/abs":  "",
		"rel":   "",
	} {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			sb := newInstallSandbox(t)
			_, calls := sb.run(t, sb.plan(false, false, false, in, "~/s", "", ""))
			want := in
			switch {
			case in == "~":
				want = sb.home
			case rel != "":
				want = filepath.Join(sb.home, rel)
			}
			if got := installShHFHome(t, calls); got != want {
				t.Errorf("--hf-home for %q = %q, want %q", in, got, want)
			}
		})
	}
}

func TestInstallAppDefaultIsDryRun(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t)
	start := time.Now()
	out, ok := call(t, s, "install_app", map[string]any{})
	if !ok {
		t.Fatalf("install_app without args must succeed as dry run, got error: %s", out)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("dry run took %v — looks like something was executed", d)
	}
	if !strings.Contains(out, "СУХОЙ ПРОГОН") {
		t.Errorf("output lacks СУХОЙ ПРОГОН:\n%s", out)
	}
	if strings.Contains(out, "[ошибка") {
		t.Errorf("dry run must not execute steps:\n%s", out)
	}
}

// shellQuoteTest — путь в одинарных кавычках для sh (t.TempDir может содержать
// спецсимволы из имени теста).
func shellQuoteTest(p string) string {
	return "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
}

// RoFormer ставится только по выбору.

func TestInstallPlanRoformerOnlyWhenChosen(t *testing.T) {
	t.Parallel()
	_, off := planRF("", true, true, false, "~/h", "~/s")
	if st, ok := findNamed(off, "roformer"); ok {
		t.Errorf("RoFormer step present with roformer=false: %+v", st)
	}
	_, on := planRF("", false, false, true, "~/h", "~/s")
	if _, ok := findNamed(on, "roformer"); !ok {
		names := make([]string, 0, len(on))
		for _, st := range on {
			names = append(names, st.name)
		}
		t.Errorf("RoFormer step missing with roformer=true; steps: %q", names)
	}
}

func TestInstallPlanRoformerDisk(t *testing.T) {
	t.Parallel()
	growth := func(a, b []float64) float64 {
		if len(a) != len(b) {
			t.Fatalf("disk step shape differs: %v vs %v", a, b)
		}
		for i := range a {
			if b[i] != a[i] {
				return b[i] - a[i]
			}
		}
		return 0
	}
	for _, c := range []struct{ whisper, seedvc bool }{{false, false}, {true, true}} {
		_, old := planBase("", c.whisper, c.seedvc, "~/h", "~/s")
		_, off := planRF("", c.whisper, c.seedvc, false, "~/h", "~/s")
		_, on := planRF("", c.whisper, c.seedvc, true, "~/h", "~/s")
		no, nf, nn := diskNumbers(t, old), diskNumbers(t, off), diskNumbers(t, on)
		if d := growth(no, nf); d != 0 {
			t.Errorf("whisper=%v seedvc=%v: roformer=false must not change disk requirement, got %+.1f (%v -> %v)",
				c.whisper, c.seedvc, d, no, nf)
		}
		if d := growth(nf, nn); d < 6 || d > 8 {
			t.Errorf("whisper=%v seedvc=%v: roformer should add ~7 GB to disk requirement, got %+.1f (%v -> %v)",
				c.whisper, c.seedvc, d, nf, nn)
		}
	}
}

func TestInstallWorkerRequiresExplicitRoformer(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t)
	out, ok := call(t, s, "install_worker", map[string]any{
		"dry_run": false, "confirm": true, "whisper": false, "seedvc": false, "archive": dummyArchive(t),
	})
	if ok {
		t.Fatalf("install without explicit roformer must be an error, got:\n%s", out)
	}
	if !strings.Contains(out, "roformer") {
		t.Errorf("error must mention roformer: %s", out)
	}
	if !strings.Contains(strings.ToLower(out), "спрос") {
		t.Errorf("error must say the user has to be asked: %s", out)
	}
}

// Каталог RoFormer sep_dir.

// sepEnvSteps — шаги, которые пишут пути RoFormer в worker.env.
func sepEnvSteps(steps []step) []step {
	var out []step
	for _, st := range steps {
		if strings.Contains(st.cmd, "PARTSCASTER_SEP_PY") || strings.Contains(st.cmd, "PARTSCASTER_SEP_MODELS") {
			out = append(out, st)
		}
	}
	return out
}

// runEnvStep выполняет шаг с HOME во временном каталоге и возвращает worker.env.
func runEnvStep(t *testing.T, home string, st step) string {
	t.Helper()
	cmd := exec.Command("sh", "-c", st.cmd)
	cmd.Env = append(os.Environ(), "HOME="+home)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("worker.env step failed: %v\n%s\ncmd: %s", err, out, st.cmd)
	}
	data, err := os.ReadFile(filepath.Join(home, "partscaster", "worker.env"))
	if err != nil {
		t.Fatalf("worker.env not written under $HOME/partscaster: %v (cmd %s)", err, st.cmd)
	}
	return string(data)
}

func TestInstallSepDirVenvInDir(t *testing.T) {
	t.Parallel()
	_, steps := planSep("", false, false, true, "~/h", "~/s", "/data/sep")
	st, ok := findNamed(steps, "roformer")
	if !ok {
		t.Fatalf("RoFormer step missing with roformer=true")
	}
	if !strings.Contains(st.cmd, "/data/sep/venv") {
		t.Errorf("RoFormer step must install env into /data/sep/venv: %s", st.cmd)
	}
	if strings.Contains(st.cmd, "sep-venv") {
		t.Errorf("RoFormer step with sep_dir must not use ~/sep-venv: %s", st.cmd)
	}
}

func TestInstallSepDirWritesWorkerEnv(t *testing.T) {
	t.Parallel()
	_, steps := planSep("", false, false, true, "~/h", "~/s", "/data/sep")
	env := sepEnvSteps(steps)
	if len(env) == 0 {
		t.Fatalf("no step writes PARTSCASTER_SEP_PY/PARTSCASTER_SEP_MODELS to worker.env with sep_dir")
	}
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "partscaster"), 0o755); err != nil {
		t.Fatal(err)
	}
	var data string
	for _, st := range env {
		data = runEnvStep(t, home, st)
	}
	if got := envValues(data, "PARTSCASTER_SEP_PY"); len(got) != 1 || got[0] != "/data/sep/venv/bin/python" {
		t.Errorf("PARTSCASTER_SEP_PY in worker.env = %q, want [/data/sep/venv/bin/python]\n%s", got, data)
	}
	if got := envValues(data, "PARTSCASTER_SEP_MODELS"); len(got) != 1 || got[0] != "/data/sep/models" {
		t.Errorf("PARTSCASTER_SEP_MODELS in worker.env = %q, want [/data/sep/models]\n%s", got, data)
	}
	// повторная установка не дублирует строки
	for _, st := range env {
		data = runEnvStep(t, home, st)
	}
	if n := len(envValues(data, "PARTSCASTER_SEP_PY")); n != 1 {
		t.Errorf("after second run PARTSCASTER_SEP_PY appears %d times:\n%s", n, data)
	}
}

func TestInstallSepDirKeepsExistingWorkerEnv(t *testing.T) {
	t.Parallel()
	_, steps := planSep("", false, false, true, "~/h", "~/s", "/data/sep")
	env := sepEnvSteps(steps)
	if len(env) == 0 {
		t.Fatalf("no step writes PARTSCASTER_SEP_PY/PARTSCASTER_SEP_MODELS to worker.env with sep_dir")
	}
	home := t.TempDir()
	dir := filepath.Join(home, "partscaster")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	pre := "HF_HOME=/x/hf\nPARTSCASTER_SEP_PY=/opt/my/python\nPARTSCASTER_SEP_MODELS=/opt/my/models\n"
	if err := os.WriteFile(filepath.Join(dir, "worker.env"), []byte(pre), 0o644); err != nil {
		t.Fatal(err)
	}
	var data string
	for _, st := range env {
		data = runEnvStep(t, home, st)
	}
	if got := envValues(data, "PARTSCASTER_SEP_PY"); len(got) != 1 || got[0] != "/opt/my/python" {
		t.Errorf("already set PARTSCASTER_SEP_PY overwritten/duplicated: %q\n%s", got, data)
	}
	if got := envValues(data, "PARTSCASTER_SEP_MODELS"); len(got) != 1 || got[0] != "/opt/my/models" {
		t.Errorf("already set PARTSCASTER_SEP_MODELS overwritten/duplicated: %q\n%s", got, data)
	}
	if got := envValues(data, "HF_HOME"); len(got) != 1 || got[0] != "/x/hf" {
		t.Errorf("other worker.env lines lost: %q\n%s", got, data)
	}
}

func TestInstallSepDirHomeDiskAsWithoutRoformer(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ whisper, seedvc bool }{{false, false}, {true, true}} {
		_, off := planSep("", c.whisper, c.seedvc, false, "~/h", "~/s", "")
		_, on := planSep("", c.whisper, c.seedvc, true, "~/h", "~/s", "/data/sep")
		nf, nn := diskNumbers(t, off), diskNumbers(t, on)
		if len(nf) != len(nn) {
			t.Fatalf("disk step shape differs: %v vs %v", nf, nn)
		}
		for i := range nf {
			if nf[i] != nn[i] {
				t.Errorf("whisper=%v seedvc=%v: sep_dir set, home disk requirement must equal no-RoFormer: %v vs %v",
					c.whisper, c.seedvc, nf, nn)
				break
			}
		}
	}
}

func TestInstallNoSepDirDefaults(t *testing.T) {
	t.Parallel()
	_, steps := planSep("", false, false, true, "~/h", "~/s", "")
	st, ok := findNamed(steps, "roformer")
	if !ok {
		t.Fatalf("RoFormer step missing with roformer=true")
	}
	if !strings.Contains(st.cmd, "sep-venv") {
		t.Errorf("without sep_dir RoFormer env must be ~/sep-venv: %s", st.cmd)
	}
	if env := sepEnvSteps(steps); len(env) != 0 {
		t.Errorf("without sep_dir there must be no worker.env step for RoFormer, got %d: %+v", len(env), env)
	}
}

// Пути (sep_dir, hf_home, seedvc_dir) уходят в shell-команды одним словом:
// спецсимволы передаются буквально и не выполняются, «~/» раскрывается в
// $HOME; отклоняются только перевод строки и нулевой байт.

// trapShells подменяет PATH каталогом, где bash/sh/ssh только записывают факт
// вызова в журнал и выходят с 0: так видно, запускалась ли хоть одна команда,
// и настоящая установка на машине теста не происходит ни при каком исходе.
func trapShells(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\necho \"$0 $*\" >> " + shellQuoteTest(log) + "\nexit 0\n"
	for _, name := range []string{"bash", "sh", "ssh"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	return log
}

func envValues(data, key string) []string {
	var vals []string
	for _, line := range strings.Split(data, "\n") {
		if v, ok := strings.CutPrefix(line, key+"="); ok {
			vals = append(vals, v)
		}
	}
	return vals
}

// hasArg — заглушка получила аргумент ровно таким (одной строкой журнала).
func hasArg(calls, arg string) bool {
	for _, line := range strings.Split(calls, "\n") {
		if line == arg {
			return true
		}
	}
	return false
}

// trickyPaths — пути со спецсимволами; mark — файл, который появится, если
// подстановка выполнится. Каталог mark не должен содержать спецсимволов.
func trickyPaths(mark string) []string {
	return []string{
		"/data/sep$(touch " + mark + ")",
		"/data/sep`touch " + mark + "`",
		"/data/a;touch " + mark + ";b",
		"/data/it's",
		`/data/say "hi"`,
		"/data/my sep",
		"/data/Дорожки",
		"~/my sep$(touch " + mark + ")",
	}
}

func expandHome(p, home string) string {
	if p == "~" {
		return home
	}
	if r, ok := strings.CutPrefix(p, "~/"); ok {
		return home + "/" + r
	}
	return p
}

func TestInstallSepDirSpecialCharsLiteral(t *testing.T) {
	t.Parallel()
	for i, dir := range trickyPaths("MARK") {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			sb := newInstallSandbox(t)
			mark := filepath.Join(t.TempDir(), "executed")
			dir := strings.ReplaceAll(dir, "MARK", mark)
			env, calls := sb.run(t, sb.plan(false, false, true, "~/h", "~/s", dir, ""))
			if fileExists(mark) {
				t.Fatalf("substitution in sep_dir %q was executed", dir)
			}
			base := expandHome(dir, sb.home)
			if got := envValues(env, "PARTSCASTER_SEP_PY"); len(got) != 1 || got[0] != base+"/venv/bin/python" {
				t.Errorf("PARTSCASTER_SEP_PY = %q, want [%s]\n%s", got, base+"/venv/bin/python", env)
			}
			if got := envValues(env, "PARTSCASTER_SEP_MODELS"); len(got) != 1 || got[0] != base+"/models" {
				t.Errorf("PARTSCASTER_SEP_MODELS = %q, want [%s]\n%s", got, base+"/models", env)
			}
			if !hasArg(calls, base+"/venv") {
				t.Errorf("uv did not get venv path %q literally; stub calls:\n%s", base+"/venv", calls)
			}
		})
	}
}

// hf_home уходит в install.sh одним аргументом --hf-home: спецсимволы буквально, без выполнения.
func TestInstallHFHomeSpecialCharsLiteral(t *testing.T) {
	t.Parallel()
	for i, dir := range trickyPaths("MARK") {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			sb := newInstallSandbox(t)
			mark := filepath.Join(t.TempDir(), "executed")
			dir := strings.ReplaceAll(dir, "MARK", mark)
			_, calls := sb.run(t, sb.plan(false, false, false, dir, "~/s", "", ""))
			if fileExists(mark) {
				t.Fatalf("substitution in hf_home %q was executed", dir)
			}
			if got, want := installShHFHome(t, calls), expandHome(dir, sb.home); got != want {
				t.Errorf("--hf-home = %q, want %q", got, want)
			}
		})
	}
}

func TestInstallSeedVCDirSpecialCharsLiteral(t *testing.T) {
	t.Parallel()
	for i, dir := range trickyPaths("MARK") {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			sb := newInstallSandbox(t)
			mark := filepath.Join(t.TempDir(), "executed")
			dir := strings.ReplaceAll(dir, "MARK", mark)
			env, calls := sb.run(t, sb.plan(false, true, false, "~/h", dir, "", ""))
			if fileExists(mark) {
				t.Fatalf("substitution in seedvc_dir %q was executed", dir)
			}
			want := expandHome(dir, sb.home)
			if !strings.Contains(calls, "== seedvc_install.sh") || !hasArg(calls, want) {
				t.Errorf("seedvc_install.sh did not get %q literally; stub calls:\n%s", want, calls)
			}
			if got := envValues(env, "PARTSCASTER_SEEDVC_DIR"); len(got) != 1 || got[0] != want {
				t.Errorf("PARTSCASTER_SEEDVC_DIR = %q, want [%s]\n%s", got, want, env)
			}
		})
	}
}

func TestInstallDefaultPathsExpandHome(t *testing.T) {
	t.Parallel()
	sb := newInstallSandbox(t)
	env, calls := sb.run(t, sb.plan(false, true, false, "~/yue/hf-cache", "~/partscaster/seedvc", "", ""))
	if got := installShHFHome(t, calls); got != sb.home+"/yue/hf-cache" {
		t.Errorf("--hf-home = %q, want %q", got, sb.home+"/yue/hf-cache")
	}
	if got := envValues(env, "PARTSCASTER_SEEDVC_DIR"); len(got) != 1 || got[0] != sb.home+"/partscaster/seedvc" {
		t.Errorf("PARTSCASTER_SEEDVC_DIR = %q, want [%s]", got, sb.home+"/partscaster/seedvc")
	}
	if !hasArg(calls, sb.home+"/partscaster/seedvc") {
		t.Errorf("seedvc_install.sh did not get expanded default dir; calls:\n%s", calls)
	}
}

// Обработчик: спецсимволы не отклоняются (ни сухой прогон, ни установка),
// перевод строки и нулевой байт — ошибка с именем параметра до любых команд.
func TestInstallWorkerAcceptsSpecialCharPaths(t *testing.T) {
	for _, param := range []string{"sep_dir", "hf_home", "seedvc_dir"} {
		for i, dir := range trickyPaths("/nonexistent-dir/mark") {
			for _, dry := range []bool{true, false} {
				t.Run(param+"/"+strconv.Itoa(i)+map[bool]string{true: "/dry", false: "/run"}[dry], func(t *testing.T) {
					trapShells(t) // установка «выполняется» заглушками, на машине ничего не ставится
					s, _ := newTestServer(t)
					args := map[string]any{"whisper": false, "seedvc": true, "roformer": true, param: dir,
						"archive": dummyArchive(t)}
					if !dry {
						args["dry_run"] = false
						args["confirm"] = true
						args["stems_model"] = "htdemucs" // при roformer=true установка требует выбора
					}
					out, ok := call(t, s, "install_worker", args)
					if !ok {
						t.Errorf("%s %q (dry=%v) must be accepted, got error: %s", param, dir, dry, out)
					}
				})
			}
		}
	}
}

func TestInstallWorkerRejectsNewlineAndNUL(t *testing.T) {
	for _, param := range []string{"sep_dir", "hf_home", "seedvc_dir"} {
		for _, dir := range []string{"/data/a\nb", "/data/a\x00b", "/data/a\n"} {
			for _, dry := range []bool{true, false} {
				t.Run(param+map[bool]string{true: "/dry", false: "/run"}[dry], func(t *testing.T) {
					log := trapShells(t)
					s, _ := newTestServer(t)
					args := map[string]any{"whisper": false, "seedvc": true, "roformer": true, param: dir,
						"archive": dummyArchive(t)}
					if !dry {
						args["dry_run"] = false
						args["confirm"] = true
						// stems_model задан: ошибка должна быть именно из-за пути, а не из-за выбора модели
						args["stems_model"] = "roformer"
					}
					out, ok := call(t, s, "install_worker", args)
					if ok {
						t.Errorf("%s %q (dry=%v) must be an error, got:\n%s", param, dir, dry, out)
					}
					if !strings.Contains(out, param) {
						t.Errorf("error must mention %s: %s", param, out)
					}
					if data, err := os.ReadFile(log); err == nil && len(data) > 0 {
						t.Errorf("commands were executed with %s %q:\n%s", param, dir, data)
					}
				})
			}
		}
	}
}

// Шаги установки корректны для bash.

func TestInstallAllStepsBashSyntax(t *testing.T) {
	t.Parallel()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}
	for _, host := range []string{"", remoteHost} {
		for _, archive := range []string{testArchive, "/home/me/my builds/worker-1.0.tar.gz"} {
			for mask := 0; mask < 8; mask++ {
				for _, sepDir := range []string{"", "/data/sep", "~/my sep$(id)"} {
					w, sv, rf := mask&1 != 0, mask&2 != 0, mask&4 != 0
					_, steps := workerInstallPlanFull(host, archive, w, sv, rf, "~/yue/hf-cache", "~/partscaster/seedvc", sepDir, "")
					for _, st := range steps {
						cmd := exec.Command(bash, "-n", "-c", st.cmd)
						if out, err := cmd.CombinedOutput(); err != nil {
							t.Errorf("step %q (host=%q archive=%q whisper=%v seedvc=%v roformer=%v sep_dir=%q) is not valid bash: %v\n%s\ncmd: %s",
								st.name, host, archive, w, sv, rf, sepDir, err, out, st.cmd)
						}
					}
				}
			}
		}
	}
}

// Без systemd (systemctl падает) и без юнита шаг «systemd-юнит» не падает,
// а печатает подсказку о ручном запуске.
func TestInstallSystemdStepFallbackHint(t *testing.T) {
	t.Parallel()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}
	_, steps := planSep("", false, false, false, "~/h", "~/s", "")
	st, ok := findNamed(steps, "systemd")
	if !ok {
		t.Fatalf("systemd step missing")
	}
	for _, withUnit := range []bool{false, true} {
		t.Run(map[bool]string{false: "no-unit", true: "unit-no-systemd"}[withUnit], func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			if withUnit {
				units := filepath.Join(home, "partscaster", "units")
				if err := os.MkdirAll(units, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(units, "partscaster-worker.service"), []byte("[Unit]\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Join(home, ".config", "systemd", "user"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			// systemctl-заглушка: «systemd недоступен»; настоящий systemctl не вызывается
			stubs := t.TempDir()
			script := "#!/bin/sh\necho 'Failed to connect to bus' >&2\nexit 1\n"
			if err := os.WriteFile(filepath.Join(stubs, "systemctl"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(bash, "-c", st.cmd)
			cmd.Dir = home
			cmd.Env = append(os.Environ(), "HOME="+home, "PATH="+stubs+":"+os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("systemd step must not fail without systemd: %v\n%s", err, out)
			}
			if !strings.Contains(string(out), "вручную") || !strings.Contains(string(out), "yue_worker") {
				t.Errorf("fallback must print manual start hint, got:\n%s", out)
			}
		})
	}
}

// Модель дорожек по умолчанию при установке.
//
// stems_model (htdemucs | roformer): при roformer=true и установке обязателен —
// агент спрашивает пользователя; roformer без roformer=true — ошибка; без
// RoFormer и без выбора — настройка не меняется (переустановка не сбрасывает
// выбранный ранее RoFormer). Выбор пишется в ~/partscaster/data/settings.json
// слиянием до запуска юнита; план показывает выбор; при roformer после
// установки — проверка действующей модели (/config stems_model = bs-roformer-sw)
// с понятным предупреждением.

// installRunArgs — аргументы настоящей установки (не сухой прогон); архив задан явно,
// чтобы обработчик не собирал его (make worker-tar) на машине теста.
func installRunArgs(t *testing.T, extra map[string]any) map[string]any {
	t.Helper()
	args := map[string]any{"dry_run": false, "confirm": true, "whisper": false, "seedvc": false,
		"archive": dummyArchive(t)}
	for k, v := range extra {
		args[k] = v
	}
	return args
}

// assertNothingRan — ни одна команда установки не запускалась (журнал trapShells пуст).
func assertNothingRan(t *testing.T, log string) {
	t.Helper()
	if data, err := os.ReadFile(log); err == nil && len(data) > 0 {
		t.Errorf("commands were executed:\n%s", data)
	}
}

func TestInstallStemsModelRequiredWithRoformer(t *testing.T) {
	log := trapShells(t)
	s, _ := newTestServer(t)
	out, ok := call(t, s, "install_worker", installRunArgs(t, map[string]any{"roformer": true}))
	if ok {
		t.Fatalf("install with roformer=true and no stems_model must be an error, got:\n%s", out)
	}
	if !strings.Contains(out, "stems_model") {
		t.Errorf("error must mention stems_model: %s", out)
	}
	if !strings.Contains(strings.ToLower(out), "спрос") {
		t.Errorf("error must say the user has to be asked: %s", out)
	}
	assertNothingRan(t, log)
}

func TestInstallStemsModelDryRunWithoutChoiceOK(t *testing.T) {
	log := trapShells(t)
	s, _ := newTestServer(t)
	out, ok := call(t, s, "install_worker", map[string]any{"whisper": false, "seedvc": false, "roformer": true,
		"archive": dummyArchive(t)})
	if !ok {
		t.Fatalf("dry run with roformer=true and no stems_model must succeed, got error: %s", out)
	}
	if !strings.Contains(out, "СУХОЙ ПРОГОН") {
		t.Errorf("expected dry run output:\n%s", out)
	}
	assertNothingRan(t, log)
}

func TestInstallStemsModelRoformerRequiresRoformerInstall(t *testing.T) {
	for _, dry := range []bool{true, false} {
		t.Run(map[bool]string{true: "dry", false: "run"}[dry], func(t *testing.T) {
			log := trapShells(t)
			s, _ := newTestServer(t)
			args := map[string]any{"whisper": false, "seedvc": false, "roformer": false, "stems_model": "roformer",
				"archive": dummyArchive(t)}
			if !dry {
				args = installRunArgs(t, args)
			}
			out, ok := call(t, s, "install_worker", args)
			if ok {
				t.Fatalf("stems_model=roformer with roformer=false must be an error, got:\n%s", out)
			}
			if !strings.Contains(out, "roformer") {
				t.Errorf("error must mention roformer: %s", out)
			}
			assertNothingRan(t, log)
		})
	}
}

func TestInstallStemsModelUnknownValue(t *testing.T) {
	for _, val := range []string{"mdx", "Demucs", " htdemucs"} {
		for _, dry := range []bool{true, false} {
			t.Run(val+map[bool]string{true: "/dry", false: "/run"}[dry], func(t *testing.T) {
				log := trapShells(t)
				s, _ := newTestServer(t)
				args := map[string]any{"whisper": false, "seedvc": false, "roformer": true, "stems_model": val,
					"archive": dummyArchive(t)}
				if !dry {
					args = installRunArgs(t, args)
				}
				out, ok := call(t, s, "install_worker", args)
				if ok {
					t.Fatalf("unknown stems_model %q must be an error, got:\n%s", val, out)
				}
				if !strings.Contains(out, "stems_model") {
					t.Errorf("error must mention stems_model: %s", out)
				}
				assertNothingRan(t, log)
			})
		}
	}
}

// Без RoFormer и без stems_model шага записи stems_model в settings.json нет:
// переустановка (например, ради whisper) не сбрасывает выбранный ранее RoFormer.
func TestInstallStemsModelUnchangedWithoutChoice(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ whisper, seedvc bool }{{false, false}, {true, false}, {false, true}} {
		_, steps := workerInstallPlanFull("", testArchive, c.whisper, c.seedvc, false, "~/h", "~/s", "", "")
		for _, st := range steps {
			if strings.Contains(st.cmd, "settings.json") && strings.Contains(st.cmd, "stems_model") {
				t.Errorf("whisper=%v seedvc=%v, no RoFormer, no stems_model: unexpected settings step %q: %s",
					c.whisper, c.seedvc, st.name, st.cmd)
			}
		}
	}
}

// Полный прогон шагов без RoFormer и без выбора: ранее выбранный RoFormer
// в settings.json сохраняется, прочие ключи тоже.
func TestInstallStemsModelReinstallKeepsRoformerSetting(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found")
	}
	sb := newInstallSandbox(t)
	fakeWorkerVenv(t, sb.home)
	dir := filepath.Join(sb.home, "partscaster", "data")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	pre := `{"stems_model": "roformer", "llm_model": "qwen"}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(pre), 0o644); err != nil {
		t.Fatal(err)
	}
	sb.run(t, sb.plan(true, false, false, "~/h", "~/s", "", ""))
	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("settings.json disappeared after reinstall: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v\n%s", err, data)
	}
	if m["stems_model"] != "roformer" || m["llm_model"] != "qwen" {
		t.Errorf("reinstall without stems_model changed settings: %v", m)
	}
}

// Явный stems_model=htdemucs без RoFormer — шаг записи есть, план называет модель.
func TestInstallStemsModelExplicitHtdemucsWithoutRoformer(t *testing.T) {
	t.Parallel()
	plan, steps := workerInstallPlanFull("", testArchive, false, false, false, "~/h", "~/s", "", "htdemucs")
	_, st := settingsStep(t, steps)
	if !strings.Contains(st.cmd, "htdemucs") {
		t.Errorf("settings step does not contain htdemucs: %s", st.cmd)
	}
	if !strings.Contains(strings.ToLower(plan), "demucs") {
		t.Errorf("plan for explicit htdemucs does not name it:\n%s", plan)
	}
	s, _ := newTestServer(t)
	out, ok := call(t, s, "install_worker", map[string]any{"whisper": false, "seedvc": false, "roformer": false,
		"stems_model": "htdemucs", "archive": dummyArchive(t)})
	if !ok {
		t.Fatalf("dry run with explicit htdemucs without roformer must succeed, got error: %s", out)
	}
}

// Установка без RoFormer (без stems_model) не требует выбора модели.
func TestInstallStemsModelNotRequiredWithoutRoformer(t *testing.T) {
	trapShells(t)
	s, _ := newTestServer(t)
	out, ok := call(t, s, "install_worker", installRunArgs(t, map[string]any{"roformer": false}))
	if !ok && strings.Contains(out, "stems_model") {
		t.Fatalf("install without RoFormer must not require stems_model, got error: %s", out)
	}
}

func TestInstallStemsModelPlanNamesChoice(t *testing.T) {
	t.Parallel()
	planH, _ := workerInstallPlanFull("", testArchive, false, false, true, "~/h", "~/s", "", "htdemucs")
	planR, _ := workerInstallPlanFull("", testArchive, false, false, true, "~/h", "~/s", "", "roformer")
	// в плане модель может называться по-человечески («Demucs»)
	if !strings.Contains(strings.ToLower(planH), "demucs") {
		t.Errorf("plan for stems_model=htdemucs does not name it:\n%s", planH)
	}
	if !strings.Contains(strings.ToLower(planR), "roformer") {
		t.Errorf("plan for stems_model=roformer does not name it:\n%s", planR)
	}
	if planH == planR {
		t.Errorf("plan text does not depend on stems_model choice:\n%s", planH)
	}
}

// settingsStep — шаг, который пишет stems_model в settings.json воркера.
func settingsStep(t *testing.T, steps []step) (int, step) {
	t.Helper()
	for i, st := range steps {
		if strings.Contains(st.cmd, "settings.json") && strings.Contains(st.cmd, "stems_model") {
			return i, st
		}
	}
	t.Fatalf("no step writes stems_model to settings.json")
	return -1, step{}
}

// fakeWorkerVenv — окружение воркера (~/yue/.venv, его создаёт install.sh раньше в
// установке) подменяется системным python3: в тесте uv не запускается.
func fakeWorkerVenv(t *testing.T, home string) {
	t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found")
	}
	bin := filepath.Join(home, "yue", ".venv", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"python", "python3"} {
		if err := os.Symlink(py, filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
}

// runSettingsStep выполняет шаг с HOME во временном каталоге и возвращает
// разобранный ~/partscaster/data/settings.json.
func runSettingsStep(t *testing.T, home string, st step) map[string]any {
	t.Helper()
	fakeWorkerVenv(t, home)
	cmd := exec.Command("bash", "-c", st.cmd)
	cmd.Dir = home
	cmd.Env = append(os.Environ(), "HOME="+home)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("settings step failed: %v\n%s\ncmd: %s", err, out, st.cmd)
	}
	data, err := os.ReadFile(filepath.Join(home, "partscaster", "data", "settings.json"))
	if err != nil {
		t.Fatalf("settings.json not written under $HOME/partscaster/data: %v (cmd %s)", err, st.cmd)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v\n%s", err, data)
	}
	return m
}

func TestInstallStemsModelWritesSettingsMerged(t *testing.T) {
	t.Parallel()
	for _, model := range []string{"htdemucs", "roformer"} {
		t.Run(model, func(t *testing.T) {
			t.Parallel()
			_, steps := workerInstallPlanFull("", testArchive, false, false, true, "~/h", "~/s", "", model)
			_, st := settingsStep(t, steps)
			if !strings.Contains(st.cmd, model) {
				t.Errorf("settings step does not contain value %q: %s", model, st.cmd)
			}
			home := t.TempDir()
			dir := filepath.Join(home, "partscaster", "data")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			other := "htdemucs"
			if model == "htdemucs" {
				other = "roformer"
			}
			pre := `{"stems_model": "` + other + `", "llm_model": "qwen", "name": "Дорожки", "nested": {"a": 1}}`
			if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(pre), 0o644); err != nil {
				t.Fatal(err)
			}
			m := runSettingsStep(t, home, st)
			if m["stems_model"] != model {
				t.Errorf("stems_model = %v, want %q", m["stems_model"], model)
			}
			if m["llm_model"] != "qwen" || m["name"] != "Дорожки" {
				t.Errorf("other settings keys lost or changed: %v", m)
			}
			if n, ok := m["nested"].(map[string]any); !ok || n["a"] != float64(1) {
				t.Errorf("nested settings key lost: %v", m)
			}
		})
	}
}

// settings.json ещё нет — шаг создаёт его. Каталог data/ на чистой машине
// создают предыдущие шаги — это проверяет TestInstallStemsModelFullRunWritesSettings.
func TestInstallStemsModelWritesSettingsFresh(t *testing.T) {
	t.Parallel()
	_, steps := workerInstallPlanFull("", testArchive, false, false, true, "~/h", "~/s", "", "roformer")
	_, st := settingsStep(t, steps)
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "partscaster", "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := runSettingsStep(t, home, st)
	if m["stems_model"] != "roformer" {
		t.Errorf("stems_model = %v, want roformer", m["stems_model"])
	}
}

// Полный прогон шагов на чистом HOME (внешние программы — заглушки):
// settings.json появляется с выбранной моделью.
func TestInstallStemsModelFullRunWritesSettings(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found")
	}
	sb := newInstallSandbox(t)
	fakeWorkerVenv(t, sb.home)
	sb.run(t, sb.plan(false, false, false, "~/h", "~/s", "", "htdemucs"))
	data, err := os.ReadFile(filepath.Join(sb.home, "partscaster", "data", "settings.json"))
	if err != nil {
		t.Fatalf("settings.json not written by install steps: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v\n%s", err, data)
	}
	if m["stems_model"] != "htdemucs" {
		t.Errorf("stems_model = %v, want htdemucs", m["stems_model"])
	}
}

func TestInstallStemsModelSettingsBeforeSystemd(t *testing.T) {
	t.Parallel()
	for _, host := range []string{"", remoteHost} {
		for _, c := range []struct {
			roformer bool
			model    string
		}{{false, "htdemucs"}, {true, "htdemucs"}, {true, "roformer"}} {
			_, steps := workerInstallPlanFull(host, testArchive, false, false, c.roformer, "~/h", "~/s", "", c.model)
			si, _ := settingsStep(t, steps)
			ui := -1
			for i, st := range steps {
				if strings.Contains(strings.ToLower(st.name), "systemd") {
					ui = i
					break
				}
			}
			if ui < 0 {
				t.Fatalf("host=%q: systemd step missing (roformer=%v model=%s)", host, c.roformer, c.model)
			}
			if si > ui {
				t.Errorf("host=%q roformer=%v model=%s: settings step (#%d) must come before systemd unit (#%d)",
					host, c.roformer, c.model, si, ui)
			}
		}
	}
}

// runCheckStep выполняет шаг с curl-заглушкой, отдающей config, и
// sleep-заглушкой; возвращает вывод и ошибку.
func runCheckStep(t *testing.T, st step, config string) (string, error) {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}
	stubs := t.TempDir()
	cfg := filepath.Join(stubs, "config.json")
	if err := os.WriteFile(cfg, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	curl := "#!/bin/sh\ncat " + shellQuoteTest(cfg) + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(stubs, "curl"), []byte(curl), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stubs, "sleep"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	cmd := exec.Command(bash, "-c", st.cmd)
	cmd.Dir = home
	cmd.Env = append(os.Environ(), "HOME="+home, "PATH="+stubs+":"+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Проверка после установки — по действующей модели воркера (/config
// stems_model = bs-roformer-sw), а не по roformer_available: RoFormer может
// быть установлен, но воркер делать Demucs.
func TestInstallStemsModelRoformerFinalCheck(t *testing.T) {
	t.Parallel()
	_, steps := workerInstallPlanFull("", testArchive, false, false, true, "~/h", "~/s", "", "roformer")
	last := steps[len(steps)-1]
	if last.must {
		t.Errorf("roformer check step must not be a must-step (warning, not stop)")
	}
	t.Run("not-active", func(t *testing.T) {
		t.Parallel()
		for _, cfg := range []string{
			`{"roformer_available": true, "stems_model": "htdemucs"}`,
			`{"roformer_available":true,"stems_model":"htdemucs"}`,
			`{"roformer_available": false, "stems_model": "htdemucs"}`,
			`{"roformer_available": true}`,
			``,
		} {
			out, err := runCheckStep(t, last, cfg)
			if err == nil {
				t.Errorf("config %q: check must fail when active model is not bs-roformer-sw; output:\n%s", cfg, out)
			}
			if strings.TrimSpace(out) == "" || !strings.Contains(strings.ToLower(out), "roformer") {
				t.Errorf("config %q: check must print a warning mentioning RoFormer, got:\n%s", cfg, out)
			}
		}
	})
	t.Run("active", func(t *testing.T) {
		t.Parallel()
		for _, cfg := range []string{
			`{"roformer_available": true, "stems_model": "bs-roformer-sw"}`,
			`{"roformer_available":true,"stems_model":"bs-roformer-sw"}`,
		} {
			out, err := runCheckStep(t, last, cfg)
			if err != nil {
				t.Errorf("config %q: check must pass when active model is bs-roformer-sw: %v\n%s", cfg, err, out)
			}
		}
	})
}

func TestInstallStemsModelNoRoformerCheckForHtdemucs(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		roformer bool
		model    string
	}{{false, "htdemucs"}, {true, "htdemucs"}, {false, ""}} {
		_, steps := workerInstallPlanFull("", testArchive, false, false, c.roformer, "~/h", "~/s", "", c.model)
		for _, st := range steps {
			if strings.Contains(st.cmd, "bs-roformer-sw") || strings.Contains(st.cmd, "roformer_available") {
				t.Errorf("roformer=%v stems_model=%q: unexpected RoFormer check %q", c.roformer, c.model, st.name)
			}
		}
	}
}

// noCurlPath — каталог со ссылками на обычные утилиты, но без curl (на машине теста curl
// может быть в /usr/bin — весь /usr/bin в PATH не кладётся).
func noCurlPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"bash", "sh", "env", "cat", "printf", "echo", "test", "true", "false",
		"sed", "grep", "awk", "tr", "cut", "head", "tail", "ls", "mkdir", "rm", "cp", "mv", "ln",
		"readlink", "dirname", "basename", "id", "seq", "date", "touch", "chmod", "mktemp", "wc", "sort"} {
		if p, err := exec.LookPath(name); err == nil {
			if err := os.Symlink(p, filepath.Join(dir, name)); err != nil {
				t.Fatal(err)
			}
		}
	}
	return dir
}

// На машине без curl шаг health проверяет /health python-ом из venv
// воркера (~/yue/.venv/bin/python).
func TestInstallHealthStepWithoutCurlUsesVenvPython(t *testing.T) {
	sb := newInstallSandbox(t)
	steps := sb.plan(false, false, false, "", "", "", "")
	st, ok := findNamed(steps, "health")
	if !ok {
		t.Fatalf("no health step in plan: %+v", steps)
	}
	stubs := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubs, "sleep"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	mark := filepath.Join(t.TempDir(), "venv-python.called")
	pyDir := filepath.Join(sb.home, "yue", ".venv", "bin")
	if err := os.MkdirAll(pyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	py := "#!/bin/sh\necho called >> " + shellQuoteTest(mark) + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(pyDir, "python"), []byte(py), 0o755); err != nil {
		t.Fatal(err)
	}
	path := stubs + ":" + noCurlPath(t)
	bash := filepath.Join(strings.Split(path, ":")[1], "bash")
	cmd := exec.Command(bash, "-c", st.cmd)
	cmd.Dir = sb.home
	cmd.Stdin = strings.NewReader("")
	cmd.Env = []string{"HOME=" + sb.home, "PATH=" + path}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("health step without curl failed: %v\n%s\ncmd: %s", err, out, st.cmd)
	}
	if !fileExists(mark) {
		t.Fatalf("~/yue/.venv/bin/python not called by health step without curl\n%s\ncmd: %s", out, st.cmd)
	}
}

// Шаги компонентов, которые зовут uv, видят каталоги его установщика: по ssh без входа в оболочку
// ~/.local/bin и ~/.cargo/bin в PATH нет («uv: команда не найдена» при деплое 2026-10-16).
func TestInstallStepsFindUvOutsidePath(t *testing.T) {
	t.Parallel()
	n := 0
	for _, st := range installSteps("user@gpu-host", "", true, true, true, 20, "~/hf", "~/seedvc", "") {
		i := strings.Index(st.cmd, "uv venv")
		if i < 0 {
			continue
		}
		n++
		head := st.cmd[:i]
		for _, dir := range []string{"$HOME/.local/bin", "$HOME/.cargo/bin"} {
			if !strings.Contains(head, dir) {
				t.Errorf("шаг %q: %s не в PATH до вызова uv:\n%s", st.name, dir, st.cmd)
			}
		}
	}
	if n < 2 {
		t.Fatalf("ждали шаги RoFormer и whisper с uv, нашли %d", n)
	}
}
