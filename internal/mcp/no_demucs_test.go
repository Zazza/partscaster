package mcp

import (
	"os/exec"
	"strings"
	"testing"
)

// Карточка internal-no-demucs, усл. 7 — MCP без выбора модели дорожек.
//
// Тест-кейсы (ТК → условие карточки):
//   ТК34 (усл. 2, 7) config_set: в схеме нет stems_model; переданное старым агентом stems_model воркеру
//                  не уходит (POST /config без stems_model);
//   ТК35 (усл. 2, 7) make_stems: в схеме нет model; вызов (даже с model от старого агента) шлёт
//                  POST /jobs/{id}/stems без строки запроса;
//   ТК36 (усл. 5, 7) install_worker: в схеме нет roformer и stems_model, sep_dir есть;
//   ТК37 (усл. 7)  сухой прогон install_worker без roformer/stems_model принят; план про разделение
//                  RoFormer, без «demucs»; с sep_dir — каталог пишется в worker.env (PARTSCASTER_SEP_PY);
//   ТК38 (усл. 7)  настоящая установка без roformer/stems_model не требует их (отказа с упоминанием
//                  roformer/stems_model нет; команды перехвачены заглушками, ничего не ставится);
//   ТК39 (усл. 7)  doctor проверяет roformer_available из /config воркера;
//   ТК40 (усл. 7)  описания и схемы всех инструментов без «demucs» (htdemucs в том числе).
// Внешняя граница — HTTP воркера (engineFakeWorker из engine_model_test.go) и оболочка (trapShells из
// install_test.go); инструментный слой не мокается.

// --- ТК34 ---

func TestNoDemucsConfigSetSchema(t *testing.T) {
	tl, ok := cppOnlyTools(t)["config_set"]
	if !ok {
		t.Fatal("config_set нет в tools/list")
	}
	if _, has := cppOnlyProps(t, tl)["stems_model"]; has {
		t.Error("в схеме config_set осталось stems_model")
	}
}

func TestNoDemucsConfigSetDropsStemsModel(t *testing.T) {
	s, fw := newEngineServer(t)
	out, ok := call(t, s, "config_set", map[string]any{"cpp_model": "q6", "stems_model": "roformer"})
	if !ok {
		t.Fatalf("config_set: %s", out)
	}
	for _, p := range fw.posted("/config") {
		if _, has := p.Body["stems_model"]; has {
			t.Errorf("воркеру ушло stems_model: %+v", p.Body)
		}
	}
}

// --- ТК35 ---

func TestNoDemucsMakeStemsSchema(t *testing.T) {
	tl, ok := cppOnlyTools(t)["make_stems"]
	if !ok {
		t.Fatal("make_stems нет в tools/list")
	}
	props := cppOnlyProps(t, tl)
	if _, has := props["model"]; has {
		t.Error("в схеме make_stems осталось model")
	}
	if _, has := props["job_id"]; !has {
		t.Error("в схеме make_stems нет job_id")
	}
}

func TestNoDemucsMakeStemsWithoutQuery(t *testing.T) {
	for _, args := range []map[string]any{
		{"job_id": 5},
		{"job_id": 5, "model": "htdemucs"}, // старый агент
	} {
		s, fw := newEngineServer(t)
		_, _ = call(t, s, "make_stems", args) // ответ фейка пустой — важен запрос
		posts := fw.posted("/jobs/5/stems")
		if len(posts) != 1 {
			t.Fatalf("%v: POST /jobs/5/stems — %d запросов, want 1 (%+v)", args, len(posts), fw.reqs)
		}
		if posts[0].Query != "" {
			t.Errorf("%v: запрос с параметрами %q — модели больше нет", args, posts[0].Query)
		}
	}
}

// --- ТК36 ---

func TestNoDemucsInstallWorkerSchema(t *testing.T) {
	tl, ok := cppOnlyTools(t)["install_worker"]
	if !ok {
		t.Fatal("install_worker нет в tools/list")
	}
	props := cppOnlyProps(t, tl)
	for _, gone := range []string{"roformer", "stems_model"} {
		if _, has := props[gone]; has {
			t.Errorf("в схеме install_worker осталось %s", gone)
		}
	}
	if _, has := props["sep_dir"]; !has {
		t.Error("в схеме install_worker нет sep_dir")
	}
}

// --- ТК37 ---

func TestNoDemucsInstallDryRunWithoutRoformer(t *testing.T) {
	log := trapShells(t)
	s, _ := newTestServer(t)
	out, ok := call(t, s, "install_worker", map[string]any{"whisper": false, "seedvc": false,
		"archive": dummyArchive(t)})
	if !ok {
		t.Fatalf("сухой прогон без roformer/stems_model отклонён: %s", out)
	}
	if !strings.Contains(out, "СУХОЙ ПРОГОН") {
		t.Errorf("ждали сухой прогон:\n%s", out)
	}
	if strings.Contains(strings.ToLower(out), "demucs") {
		t.Errorf("в плане упоминается demucs:\n%s", out)
	}
	if !strings.Contains(out, "RoFormer") {
		t.Errorf("в плане нет разделения RoFormer (ставится всегда):\n%s", out)
	}
	assertNothingRan(t, log)
}

func TestNoDemucsInstallDryRunSepDir(t *testing.T) {
	log := trapShells(t)
	s, _ := newTestServer(t)
	out, ok := call(t, s, "install_worker", map[string]any{"whisper": false, "seedvc": false,
		"sep_dir": "/data/sep", "archive": dummyArchive(t)})
	if !ok {
		t.Fatalf("сухой прогон с sep_dir отклонён: %s", out)
	}
	if !strings.Contains(out, "PARTSCASTER_SEP_PY") || !strings.Contains(out, "/data/sep") {
		t.Errorf("каталог разделения не пишется в worker.env (PARTSCASTER_SEP_PY):\n%s", out)
	}
	assertNothingRan(t, log)
}

// --- ТК38 ---

func TestNoDemucsInstallRunWithoutRoformer(t *testing.T) {
	trapShells(t)
	s, _ := newTestServer(t)
	out, ok := call(t, s, "install_worker", installRunArgs(t, nil))
	if ok {
		return
	}
	// план (в выводе и при сбое) называет разделение RoFormer — ловится только прежний отказ «передай roformer»
	low := strings.ToLower(out)
	if strings.Contains(low, "roformer=") || strings.Contains(low, "stems_model") {
		t.Errorf("установка требует roformer/stems_model, а RoFormer ставится всегда: %s", out)
	}
}

// --- ТК39 ---

func TestNoDemucsDoctorRoformerAvailable(t *testing.T) {
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("нет curl — doctor проверяет воркер им")
	}
	s, fw := newEngineServer(t)
	fw.mu.Lock()
	fw.cfg = map[string]any{"roformer_available": true, "whisper_available": false, "seedvc_available": false}
	fw.mu.Unlock()
	out, ok := call(t, s, "doctor", map[string]any{})
	if !ok {
		t.Fatalf("doctor: %s", out)
	}
	if !strings.Contains(out, "roformer_available") {
		t.Errorf("doctor не показывает roformer_available:\n%s", out)
	}
	if strings.Contains(out, "stems_model") {
		t.Errorf("doctor проверяет stems_model, которого у /config больше нет:\n%s", out)
	}
}

// --- ТК40 ---

func TestNoDemucsToolTexts(t *testing.T) {
	for name, tl := range cppOnlyTools(t) {
		text := strings.ToLower(tl.Description + string(tl.InputSchema))
		if strings.Contains(text, "demucs") {
			t.Errorf("%s: в описании/схеме упоминается demucs", name)
		}
	}
}
