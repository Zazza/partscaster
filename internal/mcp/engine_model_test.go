package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"partscaster/internal/yue"
)

// Спецификация:
//   - config_set: cpp_model, cpp_low_vram уходят воркеру в POST /config как заданы
//     (cpp_low_vram — bool; false — тоже значение); выбора движка нет (internal-cpp-only);
//     отказ воркера — ошибка с его текстом;
//   - config_get: показывает cpp_model, cpp_low_vram из GET /config воркера;
//   - engine_model_install {name}: POST /engine/models/install?name=<имя>, ждёт окончания,
//     печатает итог; ошибка воркера → ошибка инструмента с текстом воркера;
//   - install_worker: cpp_model; сухой прогон показывает выбор (--cpp-model q8 в команде
//     install.sh); cpp_model вне auto|bf16|q8|q6|q5 → ошибка до шагов; без cpp_model — план без флага.
// Внешняя граница — HTTP воркера: настоящий yue.Client против фейкового воркера (httptest),
// инструментный слой не мокается.

type engineWorkerReq struct {
	Method, Path, Query string
	Body                map[string]any
}

type engineFakeWorker struct {
	mu      sync.Mutex
	reqs    []engineWorkerReq
	cfg     map[string]any // что отдаёт GET /config
	install struct {
		status int
		body   string
	}
	cfgPostStatus int // ответ на POST /config (0 — 200)
	cfgPostBody   string
}

func (w *engineFakeWorker) posted(path string) []engineWorkerReq {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []engineWorkerReq
	for _, r := range w.reqs {
		if r.Method == http.MethodPost && r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

func newEngineServer(t *testing.T) (*Server, *engineFakeWorker) {
	t.Helper()
	// настройки приложения — во временном каталоге (как newStudioCovServer)
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("PARTSCASTER_URL", "")
	t.Setenv("TMPDIR", t.TempDir())

	fw := &engineFakeWorker{cfg: map[string]any{}}
	fw.install.status = http.StatusOK
	fw.install.body = `{"name":"q8","file":"YuE2-3B-Q8_0.gguf","installed":true,"size":3810232064}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := engineWorkerReq{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery}
		if r.Method == http.MethodPost {
			b, _ := io.ReadAll(r.Body)
			if len(b) > 0 {
				_ = json.Unmarshal(b, &req.Body)
			}
		}
		fw.mu.Lock()
		fw.reqs = append(fw.reqs, req)
		cfg, _ := json.Marshal(fw.cfg)
		st, ib := fw.install.status, fw.install.body
		cs, cb := fw.cfgPostStatus, fw.cfgPostBody
		fw.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/config" && r.Method == http.MethodPost && cs != 0:
			w.WriteHeader(cs)
			_, _ = w.Write([]byte(cb))
		case r.URL.Path == "/config" && r.Method == http.MethodGet:
			_, _ = w.Write(cfg)
		case r.URL.Path == "/engine/models/install":
			w.WriteHeader(st)
			_, _ = w.Write([]byte(ib))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)
	s := NewServer(yue.New(srv.URL), t.TempDir())
	RegisterWorkflowTools(s)
	RegisterStudioTools(s)
	RegisterLibraryTools(s)
	RegisterInstallTools(s)
	return s, fw
}

func TestEngineConfigSetSendsModelFields(t *testing.T) {
	s, fw := newEngineServer(t)
	out, ok := call(t, s, "config_set", map[string]any{
		"cpp_model": "q6", "cpp_low_vram": true,
	})
	if !ok {
		t.Fatalf("config_set: %s", out)
	}
	posts := fw.posted("/config")
	if len(posts) != 1 {
		t.Fatalf("POST /config: %d запросов, want 1 (%+v)", len(posts), fw.reqs)
	}
	b := posts[0].Body
	if b["cpp_model"] != "q6" {
		t.Fatalf("воркеру ушло %v, want cpp_model=q6", b)
	}
	if v, isBool := b["cpp_low_vram"].(bool); !isBool || !v {
		t.Fatalf("cpp_low_vram = %#v, want bool true", b["cpp_low_vram"])
	}
}

// false — значение, а не «не задано»: выключение экономного режима доходит до воркера.
func TestEngineConfigSetLowVramFalseIsSent(t *testing.T) {
	s, fw := newEngineServer(t)
	if out, ok := call(t, s, "config_set", map[string]any{"cpp_low_vram": false}); !ok {
		t.Fatalf("config_set: %s", out)
	}
	posts := fw.posted("/config")
	if len(posts) != 1 {
		t.Fatalf("POST /config: %d запросов, want 1", len(posts))
	}
	v, present := posts[0].Body["cpp_low_vram"]
	if !present {
		t.Fatalf("cpp_low_vram=false не ушло воркеру: %v", posts[0].Body)
	}
	if b, isBool := v.(bool); !isBool || b {
		t.Fatalf("cpp_low_vram = %#v, want bool false", v)
	}
}

// Задана только модель — экономный режим не трогается.
func TestEngineConfigSetOnlyGivenKeys(t *testing.T) {
	s, fw := newEngineServer(t)
	if out, ok := call(t, s, "config_set", map[string]any{"cpp_model": "bf16"}); !ok {
		t.Fatalf("config_set: %s", out)
	}
	posts := fw.posted("/config")
	if len(posts) != 1 {
		t.Fatalf("POST /config: %d запросов, want 1", len(posts))
	}
	b := posts[0].Body
	if b["cpp_model"] != "bf16" {
		t.Fatalf("cpp_model = %v, want bf16", b["cpp_model"])
	}
	for _, k := range []string{"engine", "cpp_low_vram"} {
		if _, has := b[k]; has {
			t.Fatalf("незаданное %s ушло воркеру: %v", k, b)
		}
	}
}

// Воркер отказал (422 «модель не скачана») — ошибка инструмента с его текстом.
func TestEngineConfigSetWorkerRefusalIsError(t *testing.T) {
	s, fw := newEngineServer(t)
	fw.mu.Lock()
	fw.cfgPostStatus = http.StatusUnprocessableEntity
	fw.cfgPostBody = `{"detail":"модель не скачана: q6"}`
	fw.mu.Unlock()
	out, ok := call(t, s, "config_set", map[string]any{"cpp_model": "q6"})
	if ok {
		t.Fatalf("отказ воркера скрыт: %s", out)
	}
	if !strings.Contains(out, "модель не скачана: q6") {
		t.Fatalf("причина отказа потеряна: %s", out)
	}
}

func TestEngineConfigGetShowsEngineFields(t *testing.T) {
	s, fw := newEngineServer(t)
	fw.mu.Lock()
	fw.cfg = map[string]any{
		"cpp_model": "q6", "cpp_low_vram": true, "cpp_available": true,
		"cpp_models": map[string]any{
			"q6": map[string]any{"file": "YuE2-3B-Q6_K.gguf", "size": 2900000000, "installed": true},
		},
	}
	fw.mu.Unlock()
	out, ok := call(t, s, "config_get", map[string]any{})
	if !ok {
		t.Fatalf("config_get: %s", out)
	}
	for _, want := range []string{"cpp_model", "q6", "cpp_low_vram"} {
		if !strings.Contains(out, want) {
			t.Errorf("config_get не показывает %q:\n%s", want, out)
		}
	}
}

func TestEngineModelInstallPostsName(t *testing.T) {
	s, fw := newEngineServer(t)
	out, ok := call(t, s, "engine_model_install", map[string]any{"name": "q8"})
	if !ok {
		t.Fatalf("engine_model_install: %s", out)
	}
	posts := fw.posted("/engine/models/install")
	if len(posts) != 1 {
		t.Fatalf("POST /engine/models/install: %d запросов, want 1 (%+v)", len(posts), fw.reqs)
	}
	if posts[0].Query != "name=q8" {
		t.Fatalf("запрос ?%s, want ?name=q8", posts[0].Query)
	}
	// итог — про скачанную модель (имя или файл из ответа воркера)
	low := strings.ToLower(out)
	if !strings.Contains(low, "q8") {
		t.Fatalf("итог не называет модель: %s", out)
	}
}

func TestEngineModelInstallWorkerErrorIsToolError(t *testing.T) {
	s, fw := newEngineServer(t)
	fw.mu.Lock()
	fw.install.status = http.StatusBadGateway
	fw.install.body = `{"detail":"sha256 mismatch"}`
	fw.mu.Unlock()
	out, ok := call(t, s, "engine_model_install", map[string]any{"name": "q8"})
	if ok {
		t.Fatalf("ошибка воркера скрыта: %s", out)
	}
	if !strings.Contains(out, "sha256 mismatch") {
		t.Fatalf("текст ошибки воркера потерян: %s", out)
	}
}

// Без имени — ошибка, воркер не вызывается (иначе «что-то» скачалось бы молча).
func TestEngineModelInstallWithoutNameIsError(t *testing.T) {
	for name, args := range map[string]map[string]any{
		"нет name":    {},
		"пустое name": {"name": ""},
	} {
		t.Run(name, func(t *testing.T) {
			s, fw := newEngineServer(t)
			out, ok := call(t, s, "engine_model_install", args)
			if ok {
				t.Fatalf("без имени модели — успех: %s", out)
			}
			if n := len(fw.posted("/engine/models/install")); n != 0 {
				t.Fatalf("воркер вызван без имени: %d запросов", n)
			}
		})
	}
}

func TestEngineInstallWorkerDryRunShowsModelChoice(t *testing.T) {
	s, _ := newTestServer(t)
	out, ok := call(t, s, "install_worker", map[string]any{"cpp_model": "q8"})
	if !ok {
		t.Fatalf("сухой прогон с cpp_model=q8: %s", out)
	}
	if !strings.Contains(out, "СУХОЙ ПРОГОН") {
		t.Fatalf("без dry_run — не сухой прогон:\n%s", out)
	}
	if !strings.Contains(out, "--cpp-model q8") {
		t.Errorf("план не показывает --cpp-model q8:\n%s", out)
	}
	if strings.Contains(out, "--engine") {
		t.Errorf("в плане флаг удалённого выбора движка:\n%s", out)
	}
}

// Без cpp_model — install.sh без флага (выбор пользователя или рекомендуемую решает install.sh).
func TestEngineInstallWorkerDryRunWithoutModelUnchanged(t *testing.T) {
	s, _ := newTestServer(t)
	out, ok := call(t, s, "install_worker", map[string]any{})
	if !ok {
		t.Fatalf("сухой прогон: %s", out)
	}
	for _, bad := range []string{"--engine", "--cpp-model"} {
		if strings.Contains(out, bad) {
			t.Errorf("без cpp_model в плане есть %q:\n%s", bad, out)
		}
	}
}

// cpp_model ∈ auto|bf16|q8|q6|q5, иное → ошибка до шагов.
func TestEngineInstallWorkerBadCppModelIsError(t *testing.T) {
	for name, args := range map[string]map[string]any{
		"cpp_model=q4":   {"cpp_model": "q4"},
		"cpp_model=Q8":   {"cpp_model": "Q8"},
		"cpp_model=int8": {"cpp_model": "int8"},
	} {
		t.Run(name, func(t *testing.T) {
			s, _ := newTestServer(t)
			out, ok := call(t, s, "install_worker", args)
			if ok {
				t.Fatalf("%v принято:\n%s", args, out)
			}
			if strings.Contains(out, "СУХОЙ ПРОГОН") || strings.Contains(out, "install.sh") {
				t.Errorf("при ошибке показан план:\n%s", out)
			}
		})
	}
}

// cpp_model=auto допустим (выбор по видеопамяти делает install.sh).
func TestEngineInstallWorkerCppModelAutoAccepted(t *testing.T) {
	s, _ := newTestServer(t)
	out, ok := call(t, s, "install_worker", map[string]any{"cpp_model": "auto"})
	if !ok {
		t.Fatalf("cpp_model=auto отклонено: %s", out)
	}
	if !strings.Contains(out, "--cpp-model auto") {
		t.Errorf("план не показывает --cpp-model auto:\n%s", out)
	}
}

// Настоящий запуск с неверной моделью — ни один шаг не выполнен. Команды шагов, которые
// могли бы запуститься первыми (проверка GPU, места, копирование), подменены заглушками,
// пишущими в журнал и падающими.
func TestEngineInstallWorkerBadCppModelRunsNoSteps(t *testing.T) {
	stubs := t.TempDir()
	log := filepath.Join(stubs, "calls.log")
	for _, name := range []string{"nvidia-smi", "df", "ssh", "scp", "tar"} {
		stub := "#!/bin/sh\necho \"" + name + " $*\" >> " + shellQuoteTest(log) + "\nexit 1\n"
		if err := os.WriteFile(filepath.Join(stubs, name), []byte(stub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", stubs+":"+os.Getenv("PATH"))
	base := map[string]any{"dry_run": false, "confirm": true, "whisper": false, "seedvc": false}
	for name, extra := range map[string]map[string]any{
		"cpp_model=q4":   {"cpp_model": "q4"},
		"cpp_model=int8": {"cpp_model": "int8"},
	} {
		args := map[string]any{}
		for k, v := range base {
			args[k] = v
		}
		for k, v := range extra {
			args[k] = v
		}
		s, _ := newTestServer(t)
		out, ok := call(t, s, "install_worker", args)
		if ok {
			t.Fatalf("%s: принято при настоящем запуске:\n%s", name, out)
		}
		if data, _ := os.ReadFile(log); len(data) != 0 {
			t.Fatalf("%s: выполнены шаги:\n%s", name, data)
		}
	}
}
