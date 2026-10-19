package workerexe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Карточка internal-win-tray — логика окна установки и трея в internal/workerexe, тесты без Windows (усл. 11).
//
// Тест-кейсы (ТК → условие карточки):
//   ТК1 (усл. 3)  RecommendFromSource по настоящему worker/engine_cpp.py: 12282 → bf16, 10240 → q8,
//                 8188 → q8 эконом, 0 (неизвестно) → q8 эконом;
//   ТК2 (усл. 3)  пороги берутся из файла, своих в Go нет: синтетический engine_cpp.py 20000/15000 — границы
//                 по нему, 12282 → q8 эконом; комментарий после числа допустим;
//   ТК3 (усл. 3)  файла нет / строк порогов нет → q8 эконом;
//   ТК4 (усл. 2e, 11) RunInstall: этапы Packages → Sep → Model по порядку, перед каждым — Stage; порядок
//                 констант StagePackages < StageSep < StageModel < StageStart; StageStart RunInstall не даёт;
//   ТК5 (усл. 2e) вывод шага построчно уходит в Line (без \n), неполная последняя строка тоже; строки шага
//                 приходят до Stage следующего этапа;
//   ТК6 (усл. 2e, 11) сбой шага: ошибка с текстом шага, следующие шаги не вызываются; «Повторить» —
//                 повторный RunInstall снова идёт с первого шага;
//   ТК7 (усл. 2, «прервать») ctx отменён до шага / во время шага → следующий шаг не вызывается,
//                 errors.Is(err, context.Canceled);
//   ТК8 (усл. 9)  LoadLauncher/SaveLauncher: нет файла / битый → нули; LAN сохраняется и читается обратно
//                 (вкл и выкл); файл <home>/launcher.json; запись атомарна — временных файлов не остаётся;
//   ТК9 (усл. 7)  OpenWorkerLog: <home>/logs/worker.log (каталог создаётся), пустой; прошлый — worker.1.log,
//                 одна копия (прежний worker.1.log заменяется);
//   ТК10 (усл. 6) WorkerStateOf: процесс не работает → «остановился с ошибкой»; работает и /health ок →
//                 «работает»; /health не ок → «запускается»; Text у трёх разный, по-русски;
//   ТК11 (усл. 6) GenerationRunning по ответу /stats: running с job_id → true; null / нет поля / без job_id /
//                 битый JSON → false; queue.running не путается с running;
//   ТК12 (усл. 2f, 6) ConnectAddress: без сети → http://127.0.0.1:8091; сеть и адреса → первый адрес;
//                 сеть без адресов → 127.0.0.1; порт — Port;
//   ТК13 (усл. 2, 4) NeedSetup: свежая папка → true; пакеты + разделение + выбранная модель → false; нет
//                 модели / нет разделения / набор пакетов изменился → true;
//   ТК14 (усл. 1, 10) tools/release.sh собирает partscaster-worker.exe с -tags desktop,production и -H windowsgui.
// Внешние границы: uv — заглушка fakeUv (sep_install_test.go); шаги установки — фейк InstallSteps (настоящие
// шаги качают из сети). Отдельный файл: в красной фазе функций ещё нет.

// --- ТК1–ТК3: рекомендация модели ---

func TestRecommendFromSourceRealEngine(t *testing.T) {
	cases := []struct {
		mib   int
		model string
		low   bool
	}{
		{12282, "bf16", false},
		{10240, "q8", false},
		{8188, "q8", true},
		{0, "q8", true},
	}
	for _, c := range cases {
		model, low := RecommendFromSource("../../worker", c.mib)
		if model != c.model || low != c.low {
			t.Errorf("RecommendFromSource(worker, %d) = (%q, %v), ждали (%q, %v)", c.mib, model, low, c.model, c.low)
		}
	}
}

func TestRecommendFromSourceThresholdsFromFile(t *testing.T) {
	dir := t.TempDir()
	src := "import os\n\nRECOMMEND_BF16_MIB = 20000  # большие карты\nRECOMMEND_Q8_MIB = 15000     # средние\n"
	testWriteFile(t, filepath.Join(dir, "engine_cpp.py"), []byte(src))
	cases := []struct {
		mib   int
		model string
		low   bool
	}{
		{20000, "bf16", false},
		{19999, "q8", false},
		{15000, "q8", false},
		{14999, "q8", true},
		{12282, "q8", true}, // по настоящим порогам это bf16 — значит, пороги взяты из файла
	}
	for _, c := range cases {
		model, low := RecommendFromSource(dir, c.mib)
		if model != c.model || low != c.low {
			t.Errorf("пороги 20000/15000, %d МиБ: (%q, %v), ждали (%q, %v)", c.mib, model, low, c.model, c.low)
		}
	}
}

func TestRecommendFromSourceMissing(t *testing.T) {
	// файла нет
	if model, low := RecommendFromSource(t.TempDir(), 30000); model != "q8" || !low {
		t.Errorf("нет engine_cpp.py: (%q, %v), ждали (q8, true)", model, low)
	}
	// файл есть, строк порогов нет
	dir := t.TempDir()
	testWriteFile(t, filepath.Join(dir, "engine_cpp.py"), []byte("def recommend(v):\n    return 'bf16'\n"))
	if model, low := RecommendFromSource(dir, 30000); model != "q8" || !low {
		t.Errorf("нет строк порогов: (%q, %v), ждали (q8, true)", model, low)
	}
}

// --- ТК4–ТК7: этапы установки ---

// fakeReporter — журнал событий в порядке прихода: "stage:<N>" и "line:<текст>". Под мьютексом: строки
// могут приходить из другой горутины.
type fakeReporter struct {
	mu     sync.Mutex
	events []string
}

func (r *fakeReporter) Stage(s Stage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, fmt.Sprintf("stage:%d", int(s)))
}

func (r *fakeReporter) Line(text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, "line:"+text)
}

func (r *fakeReporter) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.events...)
}

func (r *fakeReporter) stages() []string {
	var out []string
	for _, e := range r.all() {
		if strings.HasPrefix(e, "stage:") {
			out = append(out, e)
		}
	}
	return out
}

func stageEv(s Stage) string { return fmt.Sprintf("stage:%d", int(s)) }

// fakeSteps — шаги установки: пишут заданный вывод кусками, возвращают заданную ошибку, считают вызовы.
type fakeSteps struct {
	calls  []string
	output map[string][]string // шаг → куски вывода (пишутся отдельными Write)
	fail   map[string]error
	onCall map[string]func()
}

func (f *fakeSteps) step(name string, out io.Writer) error {
	f.calls = append(f.calls, name)
	if fn := f.onCall[name]; fn != nil {
		fn()
	}
	for _, chunk := range f.output[name] {
		if _, err := io.WriteString(out, chunk); err != nil {
			return err
		}
	}
	return f.fail[name]
}

func (f *fakeSteps) Packages(_ context.Context, out io.Writer) error { return f.step("packages", out) }
func (f *fakeSteps) Sep(_ context.Context, out io.Writer) error      { return f.step("sep", out) }
func (f *fakeSteps) Model(_ context.Context, out io.Writer) error    { return f.step("model", out) }

func TestStageOrder(t *testing.T) {
	if StagePackages >= StageSep || StageSep >= StageModel || StageModel >= StageStart {
		t.Errorf("порядок этапов: Packages=%d Sep=%d Model=%d Start=%d — ждали по возрастанию",
			StagePackages, StageSep, StageModel, StageStart)
	}
}

func TestRunInstallStagesInOrder(t *testing.T) {
	steps := &fakeSteps{}
	rep := &fakeReporter{}
	if err := RunInstall(context.Background(), steps, rep); err != nil {
		t.Fatalf("RunInstall: %v", err)
	}
	if got := strings.Join(steps.calls, ","); got != "packages,sep,model" {
		t.Errorf("шаги вызваны: %s, ждали packages,sep,model", got)
	}
	want := []string{stageEv(StagePackages), stageEv(StageSep), stageEv(StageModel)}
	if got := rep.stages(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("этапы в Reporter: %q, ждали %q (StageStart даёт запуск воркера, не RunInstall)", got, want)
	}
}

func TestRunInstallStageBeforeStep(t *testing.T) {
	rep := &fakeReporter{}
	steps := &fakeSteps{onCall: map[string]func(){}}
	expect := map[string]Stage{"packages": StagePackages, "sep": StageSep, "model": StageModel}
	for name, st := range expect {
		name, st := name, st
		steps.onCall[name] = func() {
			ev := rep.all()
			if len(ev) == 0 || ev[len(ev)-1] != stageEv(st) {
				t.Errorf("шаг %s начался, а последний этап в Reporter — %q, ждали %s", name, ev, stageEv(st))
			}
		}
	}
	if err := RunInstall(context.Background(), steps, rep); err != nil {
		t.Fatalf("RunInstall: %v", err)
	}
}

func TestRunInstallLines(t *testing.T) {
	steps := &fakeSteps{output: map[string][]string{
		"packages": {"Resolved 12 pack", "ages\nInstalled torch\n"},
		"sep":      {"model 1/2\nmodel 2/2"}, // последняя строка без \n
		"model":    {"q8: 45%\n"},
	}}
	rep := &fakeReporter{}
	if err := RunInstall(context.Background(), steps, rep); err != nil {
		t.Fatalf("RunInstall: %v", err)
	}
	want := []string{
		stageEv(StagePackages), "line:Resolved 12 packages", "line:Installed torch",
		stageEv(StageSep), "line:model 1/2", "line:model 2/2",
		stageEv(StageModel), "line:q8: 45%",
	}
	if got := rep.all(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("события Reporter:\n  %q\nждали\n  %q", got, want)
	}
}

func TestRunInstallStepFailureAndRetry(t *testing.T) {
	steps := &fakeSteps{fail: map[string]error{"sep": errors.New("нет места на диске")}}
	rep := &fakeReporter{}
	err := RunInstall(context.Background(), steps, rep)
	if err == nil {
		t.Fatal("шаг Sep упал, а RunInstall без ошибки")
	}
	if !strings.Contains(err.Error(), "нет места на диске") {
		t.Errorf("ошибка %q без текста шага", err)
	}
	if got := strings.Join(steps.calls, ","); got != "packages,sep" {
		t.Errorf("после сбоя Sep вызваны шаги: %s, ждали packages,sep", got)
	}
	for _, e := range rep.stages() {
		if e == stageEv(StageModel) {
			t.Error("после сбоя Sep этап Model всё равно показан")
		}
	}
	// «Повторить»: снова с первого шага (шаги сами не перекачивают скачанное)
	steps.calls = nil
	steps.fail = nil
	rep2 := &fakeReporter{}
	if err := RunInstall(context.Background(), steps, rep2); err != nil {
		t.Fatalf("повторный RunInstall: %v", err)
	}
	if got := strings.Join(steps.calls, ","); got != "packages,sep,model" {
		t.Errorf("повтор вызвал шаги: %s, ждали packages,sep,model", got)
	}
}

func TestRunInstallFirstStepFailure(t *testing.T) {
	steps := &fakeSteps{fail: map[string]error{"packages": errors.New("uv pip: exit 1")}}
	err := RunInstall(context.Background(), steps, &fakeReporter{})
	if err == nil || !strings.Contains(err.Error(), "uv pip: exit 1") {
		t.Errorf("ошибка = %v, ждали с текстом шага", err)
	}
	if got := strings.Join(steps.calls, ","); got != "packages" {
		t.Errorf("после сбоя Packages вызваны: %s", got)
	}
}

func TestRunInstallCanceledBefore(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	steps := &fakeSteps{}
	err := RunInstall(ctx, steps, &fakeReporter{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("ctx отменён: ошибка %v, ждали context.Canceled", err)
	}
	if len(steps.calls) != 0 {
		t.Errorf("ctx отменён до начала, а шаги вызваны: %v", steps.calls)
	}
}

func TestRunInstallCanceledBetweenSteps(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// «Прервать» нажали, пока шёл Packages: шаг завершился, дальше идти нельзя
	steps := &fakeSteps{onCall: map[string]func(){"packages": cancel}}
	err := RunInstall(ctx, steps, &fakeReporter{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("ошибка %v, ждали context.Canceled", err)
	}
	if got := strings.Join(steps.calls, ","); got != "packages" {
		t.Errorf("после отмены вызваны шаги: %s, ждали только packages", got)
	}
}

// --- ТК8: настройки запуска ---

func TestLauncherSettingsMissingAndBroken(t *testing.T) {
	home := t.TempDir()
	if s := LoadLauncher(home); s != (LauncherSettings{}) {
		t.Errorf("нет файла: %+v, ждали нули", s)
	}
	testWriteFile(t, filepath.Join(home, "launcher.json"), []byte("{не json"))
	if s := LoadLauncher(home); s != (LauncherSettings{}) {
		t.Errorf("битый файл: %+v, ждали нули", s)
	}
}

func TestLauncherSettingsRoundTrip(t *testing.T) {
	home := t.TempDir()
	if err := SaveLauncher(home, LauncherSettings{LAN: true}); err != nil {
		t.Fatalf("SaveLauncher: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "launcher.json")); err != nil {
		t.Errorf("нет <home>/launcher.json: %v", err)
	}
	if s := LoadLauncher(home); !s.LAN {
		t.Error("сохранили LAN=true, прочитали false")
	}
	if err := SaveLauncher(home, LauncherSettings{LAN: false}); err != nil {
		t.Fatalf("SaveLauncher: %v", err)
	}
	if s := LoadLauncher(home); s.LAN {
		t.Error("выключили LAN, прочитали true")
	}
	// атомарная запись через временный файл — после неё в папке ничего лишнего
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "launcher.json" {
			t.Errorf("после SaveLauncher в папке остался %q", e.Name())
		}
	}
}

// --- ТК9: журнал ---

func writeAndClose(t *testing.T, f *os.File, text string) {
	t.Helper()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenWorkerLogRotation(t *testing.T) {
	home := t.TempDir()
	logPath := filepath.Join(home, "logs", "worker.log")
	prevPath := filepath.Join(home, "logs", "worker.1.log")

	f, err := OpenWorkerLog(home)
	if err != nil {
		t.Fatalf("OpenWorkerLog (каталога logs нет): %v", err)
	}
	if f == nil {
		t.Fatal("OpenWorkerLog вернула nil без ошибки")
	}
	if f.Name() != logPath {
		t.Errorf("журнал %q, ждали %q", f.Name(), logPath)
	}
	writeAndClose(t, f, "запуск 1\n")
	if _, err := os.Stat(prevPath); err == nil {
		t.Error("первый запуск, а worker.1.log уже есть")
	}

	f, err = OpenWorkerLog(home)
	if err != nil {
		t.Fatalf("OpenWorkerLog 2: %v", err)
	}
	if got := testReadFile(t, logPath); got != "" {
		t.Errorf("новый worker.log не пустой: %q", got)
	}
	if got := testReadFile(t, prevPath); got != "запуск 1\n" {
		t.Errorf("worker.1.log = %q, ждали журнал прошлого запуска", got)
	}
	writeAndClose(t, f, "запуск 2\n")

	f, err = OpenWorkerLog(home)
	if err != nil {
		t.Fatalf("OpenWorkerLog 3: %v", err)
	}
	writeAndClose(t, f, "запуск 3\n")
	if got := testReadFile(t, prevPath); got != "запуск 2\n" {
		t.Errorf("worker.1.log = %q, ждали только прошлый запуск (одна копия)", got)
	}
	if got := testReadFile(t, logPath); got != "запуск 3\n" {
		t.Errorf("worker.log = %q", got)
	}
	entries, err := os.ReadDir(filepath.Join(home, "logs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("в logs %q, ждали только worker.log и worker.1.log", names)
	}
}

// --- ТК10: состояние в трее ---

func TestWorkerStateOf(t *testing.T) {
	cases := []struct {
		proc, health bool
		want         WorkerState
	}{
		{false, false, StateFailed},
		{false, true, StateFailed}, // процесса нет — чужой ответ /health не считается
		{true, true, StateRunning},
		{true, false, StateStarting},
	}
	for _, c := range cases {
		if got := WorkerStateOf(c.proc, c.health); got != c.want {
			t.Errorf("WorkerStateOf(proc=%v, health=%v) = %d, ждали %d", c.proc, c.health, got, c.want)
		}
	}
}

func TestWorkerStateText(t *testing.T) {
	// формулировки — из условия 6 карточки
	want := map[WorkerState]string{
		StateStarting: "запускается",
		StateRunning:  "работает",
		StateFailed:   "ошибк",
	}
	seen := map[string]WorkerState{}
	for st, sub := range want {
		txt := st.Text()
		if txt == "" {
			t.Errorf("состояние %d: пустой текст", st)
			continue
		}
		if !strings.Contains(strings.ToLower(txt), sub) {
			t.Errorf("состояние %d: %q, ждали со словом %q", st, txt, sub)
		}
		if prev, dup := seen[txt]; dup {
			t.Errorf("состояния %d и %d с одинаковым текстом %q", prev, st, txt)
		}
		seen[txt] = st
	}
}

// --- ТК11: идёт генерация ---

func TestGenerationRunning(t *testing.T) {
	cases := []struct {
		name  string
		stats string
		want  bool
	}{
		{"идёт", `{"queue":{"queued":2,"running":1},"running":{"job_id":42,"title":"Gone","progress":0.3}}`, true},
		{"null", `{"queue":{"queued":0,"running":1},"running":null}`, false},
		{"нет поля", `{"queue":{"queued":0,"running":0}}`, false},
		{"без job_id", `{"running":{"title":"x"}}`, false},
		{"битый", `{"running":`, false},
		{"пусто", ``, false},
	}
	for _, c := range cases {
		if got := GenerationRunning([]byte(c.stats)); got != c.want {
			t.Errorf("%s: GenerationRunning(%s) = %v, ждали %v", c.name, c.stats, got, c.want)
		}
	}
}

// --- ТК12: адрес для подключения ---

func TestConnectAddress(t *testing.T) {
	local := fmt.Sprintf("http://127.0.0.1:%d", Port)
	cases := []struct {
		name string
		lan  bool
		ips  []string
		want string
	}{
		{"только этот компьютер", false, []string{"192.168.1.5"}, local},
		{"сеть", true, []string{"192.168.1.5", "10.0.0.7"}, fmt.Sprintf("http://192.168.1.5:%d", Port)},
		{"сеть без адресов", true, nil, local},
		{"сеть, пустой список", true, []string{}, local},
	}
	for _, c := range cases {
		if got := ConnectAddress(c.lan, c.ips); got != c.want {
			t.Errorf("%s: ConnectAddress = %q, ждали %q", c.name, got, c.want)
		}
	}
	if local != "http://127.0.0.1:8091" {
		t.Errorf("порт воркера изменился: %s", local)
	}
}

// --- ТК13: нужна ли установка ---

var setupReq = []byte("fastapi==0.115\n")

// readyHome — папка, где всё установлено: пакеты (venv + отпечаток), разделение (через заглушку uv),
// по желанию — выбранная модель.
func readyHome(t *testing.T, withModel bool) string {
	t.Helper()
	home := t.TempDir()
	testMakeVenv(t, home)
	if err := MarkInstalled(home, setupReq); err != nil {
		t.Fatal(err)
	}
	uv, _ := fakeUv(t)
	if err := InstallSep(home, uv, &bytes.Buffer{}); err != nil {
		t.Fatalf("InstallSep: %v", err)
	}
	if withModel {
		if err := WriteSettings(home, map[string]any{"cpp_model": "q8"}); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func TestNeedSetup(t *testing.T) {
	if !NeedSetup(t.TempDir(), setupReq) {
		t.Error("свежая папка: NeedSetup=false")
	}
	home := readyHome(t, true)
	if NeedSetup(home, setupReq) {
		t.Error("всё установлено и модель выбрана: NeedSetup=true — окно установки при каждом запуске")
	}
	if !NeedSetup(home, []byte("fastapi==0.116\n")) {
		t.Error("набор пакетов изменился (обновление): NeedSetup=false")
	}
	if !NeedSetup(readyHome(t, false), setupReq) {
		t.Error("модель не выбрана: NeedSetup=false")
	}
}

func TestNeedSetupWithoutSep(t *testing.T) {
	home := t.TempDir()
	testMakeVenv(t, home)
	if err := MarkInstalled(home, setupReq); err != nil {
		t.Fatal(err)
	}
	if err := WriteSettings(home, map[string]any{"cpp_model": "q8"}); err != nil {
		t.Fatal(err)
	}
	if !NeedSetup(home, setupReq) {
		t.Error("разделение не установлено: NeedSetup=false")
	}
}

// --- ТК14: сборка релиза ---

func TestReleaseBuildsWorkerExeWindowsGUI(t *testing.T) {
	b, err := os.ReadFile("../../tools/release.sh")
	if err != nil {
		t.Fatal(err)
	}
	src := strings.ReplaceAll(string(b), "\\\n", " ") // продолжения строк — одна команда
	found := false
	for _, ln := range strings.Split(src, "\n") {
		if !strings.Contains(ln, "go build") || !strings.Contains(ln, "partscaster-worker.exe") {
			continue
		}
		found = true
		for _, want := range []string{"desktop,production", "windowsgui"} {
			if !strings.Contains(ln, want) {
				t.Errorf("сборка partscaster-worker.exe без %q: %s", want, strings.TrimSpace(ln))
			}
		}
	}
	if !found {
		t.Fatal("в tools/release.sh нет строки go build ... partscaster-worker.exe")
	}
}
