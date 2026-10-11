package yue

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Спецификация версий воркера (internal-own-track, условия 137–139, 141; ТК160, ТК161):
//   - GET /version → VersionInfo (version, api, source_hash, model, components, stems_model);
//   - совместимость — чистая функция CompatWarning: нет /version (404) или api < WorkerAPI →
//     «обновите воркер»; api > WorkerAPI → «обновите приложение»; api == WorkerAPI → тихо;
//     воркер недоступен (сеть) — не предупреждение о версии;
//   - Check проверяет введённый адрес (нормализует, опрашивает /health и /version) новым
//     клиентом, настройки не сохраняет; недоступен → ошибка.

const versionJSON = `{"version":"v0.9.0-3-gabc","api":1,"source_hash":"0123456789ab",` +
	`"model":"YuE2-3B yue2 0.4.1","components":{"whisper":true,"seedvc":false,"roformer":true},` +
	`"stems_model":"bs-roformer-sw"}`

// workerStub — воркер: /health и /version (version == "" — /version нет, 404 как у старого воркера).
func workerStub(t *testing.T, version string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok","model_loaded":true,"load_error":""}`))
		case "/version":
			if version == "" {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(version))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestWorkerAPIConst(t *testing.T) {
	t.Parallel()
	if WorkerAPI < 1 {
		t.Fatalf("WorkerAPI = %d, want >= 1", WorkerAPI)
	}
}

// Version входит в интерфейс Service (паритет: приложение и MCP идут через него).
func TestServiceHasVersion(t *testing.T) {
	t.Parallel()
	var s Service = New("http://127.0.0.1:1")
	_ = s.Version
}

func TestClientVersionDecodes(t *testing.T) {
	t.Parallel()
	var method, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		_, _ = w.Write([]byte(versionJSON))
	}))
	defer srv.Close()
	v, err := New(srv.URL).Version(context.Background())
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if method != http.MethodGet || path != "/version" {
		t.Errorf("request %s %s, want GET /version", method, path)
	}
	if v.Version != "v0.9.0-3-gabc" || v.API != 1 || v.SourceHash != "0123456789ab" ||
		v.Model != "YuE2-3B yue2 0.4.1" || v.StemsModel != "bs-roformer-sw" {
		t.Errorf("decoded %+v", v)
	}
	want := map[string]bool{"whisper": true, "seedvc": false, "roformer": true}
	if len(v.Components) != len(want) {
		t.Errorf("components %v, want %v", v.Components, want)
	}
	for k, b := range want {
		if got, ok := v.Components[k]; !ok || got != b {
			t.Errorf("components[%s] = %v (present %v), want %v", k, got, ok, b)
		}
	}
}

// Старый воркер без /version: ошибка — *StatusError с кодом 404 (по ней CompatWarning
// узнаёт «воркер устарел»).
func TestClientVersion404IsStatusError(t *testing.T) {
	t.Parallel()
	srv := workerStub(t, "")
	_, err := New(srv.URL).Version(context.Background())
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusNotFound {
		t.Fatalf("Version on old worker: err = %v (%T), want *StatusError{404}", err, err)
	}
}

func TestCompatWarning(t *testing.T) {
	t.Parallel()
	notFound := &StatusError{Code: 404, Msg: "yue /version: 404 Not Found"}
	cases := []struct {
		name string
		info *VersionInfo
		err  error
		want string // "" — тихо; иначе подстрока
	}{
		{"нет /version (404)", nil, notFound, "обновите воркер"},
		{"404 обёрнутая", nil, fmt.Errorf("version: %w", notFound), "обновите воркер"},
		{"api меньше требуемого", &VersionInfo{Version: "old", API: WorkerAPI - 1}, nil, "обновите воркер"},
		{"api равен", &VersionInfo{Version: "v1", API: WorkerAPI}, nil, ""},
		{"api больше известного", &VersionInfo{Version: "new", API: WorkerAPI + 1}, nil, "обновите приложение"},
		{"api намного больше", &VersionInfo{Version: "new", API: WorkerAPI + 5}, nil, "обновите приложение"},
		{"сетевой сбой", nil, errors.New("dial tcp 10.0.0.1:8091: connect: connection refused"), ""},
		{"таймаут", nil, context.DeadlineExceeded, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := CompatWarning(c.info, c.err)
			if c.want == "" {
				if got != "" {
					t.Fatalf("want no warning, got %q", got)
				}
				return
			}
			if !strings.Contains(got, c.want) {
				t.Fatalf("warning %q, want it to contain %q", got, c.want)
			}
		})
	}
}

// Устаревший воркер и устаревшее приложение — разные советы: текст «обновите воркер»
// не должен говорить «обновите приложение» и наоборот.
func TestCompatWarningAdviceNotMixed(t *testing.T) {
	t.Parallel()
	old := CompatWarning(&VersionInfo{API: WorkerAPI - 1}, nil)
	newer := CompatWarning(&VersionInfo{API: WorkerAPI + 1}, nil)
	if strings.Contains(old, "обновите приложение") {
		t.Errorf("old worker warning advises updating the app: %q", old)
	}
	if strings.Contains(newer, "обновите воркер") {
		t.Errorf("newer worker warning advises updating the worker: %q", newer)
	}
}

// isolateSettings — каталог настроек приложения во временном каталоге; возвращает
// каталог, в котором искать появившиеся файлы.
func isolateSettings(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("APPDATA", dir)
	t.Setenv("YUE_URL", "")
	return dir
}

func assertNoFiles(t *testing.T, dir string) {
	t.Helper()
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			t.Errorf("Check wrote a file (settings must not be saved): %s", p)
		}
		return nil
	})
}

func TestCheckOK(t *testing.T) {
	dir := isolateSettings(t)
	srv := workerStub(t, versionJSON)
	res, err := Check(context.Background(), srv.URL+"/")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.URL != srv.URL {
		t.Errorf("URL = %q, want normalized %q", res.URL, srv.URL)
	}
	if res.Health == nil || res.Health.Status != "ok" || !res.Health.ModelLoaded {
		t.Errorf("Health = %+v", res.Health)
	}
	if res.Version == nil || res.Version.Version != "v0.9.0-3-gabc" || res.Version.API != 1 {
		t.Errorf("Version = %+v", res.Version)
	}
	if res.Warning != "" {
		t.Errorf("compatible worker: Warning = %q, want empty", res.Warning)
	}
	assertNoFiles(t, dir)
}

// Адрес без схемы дополняется (http://), тот же воркер находится.
func TestCheckNormalizesAddress(t *testing.T) {
	isolateSettings(t)
	srv := workerStub(t, versionJSON)
	hostPort := strings.TrimPrefix(srv.URL, "http://")
	res, err := Check(context.Background(), "  "+hostPort+"  ")
	if err != nil {
		t.Fatalf("Check(%q): %v", hostPort, err)
	}
	if res.URL != srv.URL {
		t.Errorf("URL = %q, want %q", res.URL, srv.URL)
	}
	if res.Version == nil || res.Version.Version != "v0.9.0-3-gabc" {
		t.Errorf("Version = %+v", res.Version)
	}
}

// Старый воркер (нет /version) — не ошибка проверки: воркер отвечает, но с советом обновить.
func TestCheckOldWorkerWarns(t *testing.T) {
	isolateSettings(t)
	srv := workerStub(t, "")
	res, err := Check(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Check on old worker must not fail: %v", err)
	}
	if res.Health == nil || res.Health.Status != "ok" {
		t.Errorf("Health = %+v", res.Health)
	}
	if !strings.Contains(res.Warning, "обновите воркер") {
		t.Errorf("Warning = %q, want «обновите воркер»", res.Warning)
	}
}

func TestCheckNewerWorkerWarns(t *testing.T) {
	isolateSettings(t)
	srv := workerStub(t, fmt.Sprintf(`{"version":"v9","api":%d,"source_hash":"0123456789ab","model":"",`+
		`"components":{"whisper":false,"seedvc":false,"roformer":false},"stems_model":"htdemucs"}`, WorkerAPI+1))
	res, err := Check(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !strings.Contains(res.Warning, "обновите приложение") {
		t.Errorf("Warning = %q, want «обновите приложение»", res.Warning)
	}
}

func TestCheckUnreachableIsError(t *testing.T) {
	dir := isolateSettings(t)
	srv := workerStub(t, versionJSON)
	addr := srv.URL
	srv.Close() // по адресу никто не слушает
	if res, err := Check(context.Background(), addr); err == nil {
		t.Fatalf("Check on closed address: want error, got %+v", res)
	}
	assertNoFiles(t, dir)
}

func TestCheckEmptyAddressIsError(t *testing.T) {
	isolateSettings(t)
	for _, in := range []string{"", "   "} {
		if res, err := Check(context.Background(), in); err == nil {
			t.Errorf("Check(%q): want error, got %+v", in, res)
		}
	}
}
