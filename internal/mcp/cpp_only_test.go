package mcp

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Карточка internal-cpp-only — MCP без выбора движка (усл. 9).
//
// Тест-кейсы (ТК → условие карточки):
//   ТК18 (усл. 9) config_set: в схеме нет свойства engine, есть cpp_model и cpp_low_vram;
//                 config_set cpp_model=bf16 шлёт воркеру {"cpp_model":"bf16"} без engine;
//   ТК19 (усл. 9, 4) config_get показывает рекомендуемую модель из cpp_recommended воркера
//                 (имя модели и слово «рекоменд…»);
//   ТК20 (усл. 9, 5) install_worker: в схеме нет engine; cpp_model=q6 → в команде установки
//                 `--cpp-model q6`, без `--engine`; план без «PyTorch»;
//   ТК21 (усл. 9) описания engine_model_install и config_set (вместе со схемой) без «PyTorch» и
//                 «эксперимент»; описание install_worker без «PyTorch».
// Внешняя граница — HTTP воркера (фейк engineFakeWorker из engine_model_test.go) и сам протокол
// tools/list; инструментный слой не мокается.

type cppOnlyTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

func cppOnlyTools(t *testing.T) map[string]cppOnlyTool {
	t.Helper()
	s, _ := newTestServer(t)
	lines := rpcRoundTrip(t, s, []string{`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`})
	if len(lines) != 1 {
		t.Fatalf("responses: %d", len(lines))
	}
	var list struct {
		Result struct {
			Tools []cppOnlyTool `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &list); err != nil {
		t.Fatal(err)
	}
	out := map[string]cppOnlyTool{}
	for _, tl := range list.Result.Tools {
		out[tl.Name] = tl
	}
	return out
}

func cppOnlyProps(t *testing.T, tl cppOnlyTool) map[string]json.RawMessage {
	t.Helper()
	var sch struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(tl.InputSchema, &sch); err != nil {
		t.Fatalf("%s: схема: %v", tl.Name, err)
	}
	return sch.Properties
}

// --- ТК18 ---

func TestCppOnlyConfigSetSchemaWithoutEngine(t *testing.T) {
	tl, ok := cppOnlyTools(t)["config_set"]
	if !ok {
		t.Fatal("config_set нет в tools/list")
	}
	props := cppOnlyProps(t, tl)
	if _, has := props["engine"]; has {
		t.Error("в схеме config_set осталось свойство engine")
	}
	for _, want := range []string{"cpp_model", "cpp_low_vram"} {
		if _, has := props[want]; !has {
			t.Errorf("в схеме config_set нет %s", want)
		}
	}
}

func TestCppOnlyConfigSetSendsModelWithoutEngine(t *testing.T) {
	s, fw := newEngineServer(t)
	out, ok := call(t, s, "config_set", map[string]any{"cpp_model": "bf16"})
	if !ok {
		t.Fatalf("config_set: %s", out)
	}
	posts := fw.posted("/config")
	if len(posts) != 1 {
		t.Fatalf("POST /config: %d запросов, want 1 (%+v)", len(posts), fw.reqs)
	}
	b := posts[0].Body
	if b["cpp_model"] != "bf16" {
		t.Fatalf("воркеру ушло %v, want cpp_model=bf16", b)
	}
	if _, has := b["engine"]; has {
		t.Fatalf("воркеру ушёл engine: %v", b)
	}
}

// --- ТК19 ---

func TestCppOnlyConfigGetShowsRecommendation(t *testing.T) {
	s, fw := newEngineServer(t)
	fw.mu.Lock()
	fw.cfg = map[string]any{
		"cpp_model": "q6", "cpp_low_vram": false, "cpp_available": true,
		"cpp_models": map[string]any{
			"bf16": map[string]any{"file": "YuE2-3B-BF16.gguf", "size": 7130000000, "installed": false},
			"q6":   map[string]any{"file": "YuE2-3B-Q6_K.gguf", "size": 2900000000, "installed": true},
		},
		"cpp_recommended": map[string]any{"model": "bf16", "low_vram": false, "vram_mib": 12282},
	}
	fw.mu.Unlock()
	out, ok := call(t, s, "config_get", map[string]any{})
	if !ok {
		t.Fatalf("config_get: %s", out)
	}
	low := strings.ToLower(out)
	if !strings.Contains(low, "рекоменд") {
		t.Errorf("config_get не говорит о рекомендации:\n%s", out)
	}
	if !strings.Contains(low, "bf16") {
		t.Errorf("config_get не называет рекомендуемую модель bf16:\n%s", out)
	}
	if !strings.Contains(low, "q6") {
		t.Errorf("config_get не показывает текущую модель q6:\n%s", out)
	}
	// карта «12 ГБ» сообщает 12282 МиБ — показывается как 12 ГБ, а не 11
	if !strings.Contains(out, "12 ГБ") {
		t.Errorf("объём карты 12282 МиБ не показан как 12 ГБ:\n%s", out)
	}
}

// --- ТК20 ---

func TestCppOnlyInstallWorkerSchemaWithoutEngine(t *testing.T) {
	tl, ok := cppOnlyTools(t)["install_worker"]
	if !ok {
		t.Fatal("install_worker нет в tools/list")
	}
	props := cppOnlyProps(t, tl)
	if _, has := props["engine"]; has {
		t.Error("в схеме install_worker осталось свойство engine")
	}
	if _, has := props["cpp_model"]; !has {
		t.Error("в схеме install_worker нет cpp_model")
	}
}

func TestCppOnlyInstallWorkerModelFlag(t *testing.T) {
	s, _ := newTestServer(t)
	out, ok := call(t, s, "install_worker", map[string]any{"cpp_model": "q6"})
	if !ok {
		t.Fatalf("сухой прогон с cpp_model=q6: %s", out)
	}
	if !strings.Contains(out, "--cpp-model q6") {
		t.Errorf("в команде установки нет --cpp-model q6:\n%s", out)
	}
	if strings.Contains(out, "--engine") {
		t.Errorf("в команде установки остался --engine:\n%s", out)
	}
	if strings.Contains(out, "PyTorch") {
		t.Errorf("план упоминает движок PyTorch:\n%s", out)
	}
}

func TestCppOnlyInstallWorkerDefaultPlan(t *testing.T) {
	s, _ := newTestServer(t)
	out, ok := call(t, s, "install_worker", map[string]any{})
	if !ok {
		t.Fatalf("сухой прогон: %s", out)
	}
	for _, bad := range []string{"--engine", "PyTorch"} {
		if strings.Contains(out, bad) {
			t.Errorf("план без параметров содержит %q:\n%s", bad, out)
		}
	}
}

// --- ТК21 ---

func TestCppOnlyDescriptionsWithoutTorchEngine(t *testing.T) {
	tools := cppOnlyTools(t)
	for _, name := range []string{"engine_model_install", "config_set"} {
		tl, ok := tools[name]
		if !ok {
			t.Fatalf("%s нет в tools/list", name)
		}
		text := strings.ToLower(tl.Description + " " + string(tl.InputSchema))
		for _, bad := range []string{"pytorch", "эксперимент"} {
			if strings.Contains(text, bad) {
				t.Errorf("%s: в описании/схеме %q", name, bad)
			}
		}
	}
	tl := tools["install_worker"]
	if strings.Contains(strings.ToLower(tl.Description+" "+string(tl.InputSchema)), "pytorch") {
		t.Error("install_worker: в описании/схеме упоминается PyTorch")
	}
}

// config_set старым клиентом-агентом со значением, которого нет в схеме, не уходит воркеру как engine
// (воркер всё равно игнорирует поле; MCP не обещает выбор движка).
func TestCppOnlyConfigSetIgnoresOrRejectsEngine(t *testing.T) {
	s, fw := newEngineServer(t)
	fw.mu.Lock()
	fw.cfgPostStatus = http.StatusOK
	fw.cfgPostBody = `{}`
	fw.mu.Unlock()
	out, ok := call(t, s, "config_set", map[string]any{"engine": "torch", "cpp_low_vram": true})
	if !ok {
		return // отказ с ошибкой — допустимо
	}
	for _, p := range fw.posted("/config") {
		if _, has := p.Body["engine"]; has {
			t.Fatalf("config_set переслал engine воркеру: %v (%s)", p.Body, out)
		}
	}
}
