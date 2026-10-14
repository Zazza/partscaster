package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"partscaster/internal/yue"
)

// Спецификация версий в MCP:
//   - initialize → serverInfo.version = версия, вшитая при сборке (SetVersion), без неё "dev";
//   - status и doctor показывают версию воркера (/version);
//   - воркер старее (нет /version или api < WorkerAPI) — к выводу инструментов приписка
//     «обновите воркер»; воркер новее — «обновите приложение»; совместим или недоступен — тихо;
//   - /version опрашивается не чаще раза в 30 минут.

// Version по умолчанию у общего фейка — совместимый воркер: прочие тесты MCP не видят
// приписок о версии.
func (f *fakeService) Version(ctx context.Context) (*yue.VersionInfo, error) {
	return &yue.VersionInfo{Version: "test", API: yue.WorkerAPI, SourceHash: "000000000000",
		Components: map[string]bool{"whisper": false, "seedvc": false, "roformer": false}, StemsModel: "htdemucs"}, nil
}

// versionFake — фейк с настраиваемым ответом /version и счётчиком опросов.
type versionFake struct {
	*fakeService
	info  *yue.VersionInfo
	err   error
	calls atomic.Int32
}

func (v *versionFake) Version(ctx context.Context) (*yue.VersionInfo, error) {
	v.calls.Add(1)
	return v.info, v.err
}

func newVersionServer(t *testing.T, info *yue.VersionInfo, err error) (*Server, *versionFake) {
	t.Helper()
	vf := &versionFake{
		fakeService: &fakeService{url: "http://w:8091", health: &yue.HealthInfo{Status: "ok", ModelLoaded: true}},
		info:        info, err: err,
	}
	s := NewServer(vf, t.TempDir())
	RegisterWorkflowTools(s)
	RegisterStudioTools(s)
	RegisterLibraryTools(s)
	RegisterInstallTools(s)
	registerProbeTools(s)
	return s, vf
}

func serverInfoVersion(t *testing.T, s *Server) string {
	t.Helper()
	resp := s.dispatch(rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize",
		Params: json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"1"}}`)})
	if resp.Error != nil {
		t.Fatalf("initialize error: %+v", resp.Error)
	}
	b, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		ServerInfo struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatalf("decode initialize result: %v\n%s", err, b)
	}
	return r.ServerInfo.Version
}

func TestInitializeVersionDefaultDev(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t)
	if got := serverInfoVersion(t, s); got != "dev" {
		t.Fatalf("serverInfo.version without SetVersion = %q, want dev", got)
	}
}

func TestInitializeVersionFromSetVersion(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"v0.12.0", "v0.12.0-5-g1a2b3c4-dirty", "1a2b3c4"} {
		s, _ := newTestServer(t)
		s.SetVersion(v)
		if got := serverInfoVersion(t, s); got != v {
			t.Errorf("serverInfo.version = %q, want %q", got, v)
		}
	}
}

func compatibleInfo() *yue.VersionInfo {
	return &yue.VersionInfo{Version: "v0.12.0-7-gfeed42", API: yue.WorkerAPI, SourceHash: "fedcba987654",
		Model: "YuE2-3B", Components: map[string]bool{"whisper": true, "seedvc": false, "roformer": true},
		StemsModel: "bs-roformer-sw"}
}

func TestStatusShowsWorkerVersion(t *testing.T) {
	t.Parallel()
	s, _ := newVersionServer(t, compatibleInfo(), nil)
	out, ok := call(t, s, "status", map[string]any{})
	if !ok {
		t.Fatalf("status failed: %s", out)
	}
	for _, want := range []string{"v0.12.0-7-gfeed42", "fedcba987654"} {
		if !strings.Contains(out, want) {
			t.Errorf("status output lacks worker version data %q:\n%s", want, out)
		}
	}
}

// doctor: внешние команды подменены trapShells — на машине теста ничего не выполняется.
func TestDoctorShowsWorkerVersion(t *testing.T) {
	trapShells(t)
	s, _ := newVersionServer(t, compatibleInfo(), nil)
	out, ok := call(t, s, "doctor", map[string]any{})
	if !ok {
		t.Fatalf("doctor failed: %s", out)
	}
	if !strings.Contains(out, "v0.12.0-7-gfeed42") {
		t.Errorf("doctor output lacks worker version:\n%s", out)
	}
}

func TestToolCallWorkerOutdatedWarning(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		info *yue.VersionInfo
		err  error
	}{
		"api меньше":   {&yue.VersionInfo{Version: "old", API: yue.WorkerAPI - 1}, nil},
		"нет /version": {nil, &yue.StatusError{Code: 404, Msg: "yue /version: 404 Not Found"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, _ := newVersionServer(t, c.info, c.err)
			got := callTool(t, s, "probe_ok").Content[0].Text
			if !strings.Contains(got, "обновите воркер") || !strings.Contains(got, "ok-result") {
				t.Fatalf("tool output must keep result and add «обновите воркер»: %q", got)
			}
		})
	}
}

func TestToolCallWorkerNewerWarning(t *testing.T) {
	t.Parallel()
	s, _ := newVersionServer(t, &yue.VersionInfo{Version: "future", API: yue.WorkerAPI + 1}, nil)
	got := callTool(t, s, "probe_ok").Content[0].Text
	if !strings.Contains(got, "обновите приложение") || !strings.Contains(got, "ok-result") {
		t.Fatalf("tool output must keep result and add «обновите приложение»: %q", got)
	}
}

func TestToolCallNoVersionWarningWhenCompatibleOrUnreachable(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		info *yue.VersionInfo
		err  error
	}{
		"совместим":  {compatibleInfo(), nil},
		"недоступен": {nil, errors.New("dial tcp 10.0.0.1:8091: connect: connection refused")},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, _ := newVersionServer(t, c.info, c.err)
			if got := callTool(t, s, "probe_ok").Content[0].Text; got != "ok-result" {
				t.Fatalf("text = %q, want exactly ok-result", got)
			}
			if got := callTool(t, s, "probe_err").Content[0].Text; got != "boom" {
				t.Fatalf("error text = %q, want exactly boom", got)
			}
		})
	}
}

// Проверка не чаще раза в 30 минут: серия вызовов подряд опрашивает /version один раз,
// а приписка остаётся на каждом вызове.
func TestToolCallVersionCheckThrottled(t *testing.T) {
	t.Parallel()
	s, vf := newVersionServer(t, &yue.VersionInfo{Version: "old", API: yue.WorkerAPI - 1}, nil)
	for i := 0; i < 5; i++ {
		got := callTool(t, s, "probe_ok").Content[0].Text
		if !strings.Contains(got, "обновите воркер") {
			t.Fatalf("call %d: cached warning lost: %q", i, got)
		}
	}
	if n := vf.calls.Load(); n != 1 {
		t.Fatalf("/version polled %d times within 30 min, want 1", n)
	}
}

// Неудачная проверка версии (воркер недоступен) тоже запоминается —
// выключенная машина не стоит таймаута на каждом вызове; приписок нет.
func TestToolCallUnreachableVersionCheckCached(t *testing.T) {
	t.Parallel()
	s, vf := newVersionServer(t, nil, errors.New("dial tcp 10.0.0.1:8091: connect: connection refused"))
	for i := 0; i < 5; i++ {
		if got := callTool(t, s, "probe_ok").Content[0].Text; got != "ok-result" {
			t.Fatalf("call %d: text = %q, want exactly ok-result", i, got)
		}
	}
	if n := vf.calls.Load(); n != 1 {
		t.Fatalf("/version polled %d times for unreachable worker within 30 min, want 1", n)
	}
}

// urlVersionFake — ответ /version зависит от текущего адреса воркера (GetURL); опросы — по адресам.
type urlVersionFake struct {
	*fakeService
	mu    sync.Mutex
	byURL map[string]*yue.VersionInfo
	polls map[string]int
}

func (u *urlVersionFake) Version(ctx context.Context) (*yue.VersionInfo, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	url := u.GetURL()
	u.polls[url]++
	return u.byURL[url], nil
}

// Кэш совместимости привязан к адресу — после смены адреса воркера
// следующий вызов проверяет новый воркер сразу, не дожидаясь 30 минут.
func TestToolCallVersionRecheckedOnURLChange(t *testing.T) {
	t.Parallel()
	const a, b = "http://old-gpu:8091", "http://new-gpu:8091"
	uf := &urlVersionFake{
		fakeService: &fakeService{url: a, health: &yue.HealthInfo{Status: "ok", ModelLoaded: true}},
		byURL: map[string]*yue.VersionInfo{
			a: {Version: "old", API: yue.WorkerAPI - 1},
			b: compatibleInfo(),
		},
		polls: map[string]int{},
	}
	s := NewServer(uf, t.TempDir())
	registerProbeTools(s)
	if got := callTool(t, s, "probe_ok").Content[0].Text; !strings.Contains(got, "обновите воркер") {
		t.Fatalf("worker A is outdated, want «обновите воркер»: %q", got)
	}
	uf.SetURL(b)
	if got := callTool(t, s, "probe_ok").Content[0].Text; got != "ok-result" {
		t.Fatalf("after switching to compatible worker B text = %q, want exactly ok-result", got)
	}
	uf.mu.Lock()
	defer uf.mu.Unlock()
	if uf.polls[b] != 1 {
		t.Fatalf("/version of B polled %d times, want 1 (polls: %v)", uf.polls[b], uf.polls)
	}
}
