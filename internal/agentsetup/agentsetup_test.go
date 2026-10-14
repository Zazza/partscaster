package agentsetup

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// Спецификация раздела «Для AI-агентов»:
//   - команда «Подключить к Claude Code» — `claude mcp add partscaster --scope user
//     -e PARTSCASTER_URL=<адрес> -- <путь к partscaster-mcp>`; путь и адрес — отдельными аргументами
//     (с пробелами — одним аргументом);
//   - фрагменты конфига (Claude Code / Claude Desktop / Codex) — валидные (sh / JSON / TOML)
//     с тем же путём и адресом;
//   - путь partscaster-mcp — рядом с приложением, на Windows с .exe.

// samples — пути и адреса, в том числе с пробелами, кавычками и обратными слэшами.
var samples = []struct{ path, url string }{
	{"/opt/yue/partscaster-mcp", "http://localhost:8091"},
	{"/home/u/Partscaster/partscaster-mcp", "http://192.168.1.5:8091"},
	{"/Applications/Partscaster.app/Contents/MacOS/partscaster-mcp", "https://gpu.lan"},
	{"/home/u/it's mine/partscaster-mcp", "http://h:9000"},
	{`/data/say "hi"/partscaster-mcp`, "http://h:8091"},
	{`C:\Program Files\Partscaster\partscaster-mcp.exe`, "http://10.0.0.2:8091"},
	{"/home/u/$HOME `x`/partscaster-mcp", "http://h:8091"},
}

func TestClaudeAddArgs(t *testing.T) {
	t.Parallel()
	for _, s := range samples {
		got := ClaudeAddArgs(s.path, s.url)
		want := []string{"mcp", "add", "partscaster", "--scope", "user", "-e", "PARTSCASTER_URL=" + s.url, "--", s.path}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ClaudeAddArgs(%q, %q) =\n %q\nwant\n %q", s.path, s.url, got, want)
		}
	}
}

// Строка для терминала исполняется sh и даёт `claude` ровно те же аргументы, что
// ClaudeAddArgs (заглушка claude печатает свои аргументы по одному на строку).
// Не параллельный: файл-заглушка пишется и тут же исполняется, а fork в параллельном тесте
// (python3 для TOML) может унаследовать открытый на запись дескриптор — exec падает
// «text file busy» (ETXTBSY). Последовательные тесты идут до параллельных — гонки нет.
func TestSnippetClaudeCodeShellCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("проверка через sh")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not found")
	}
	stubs := t.TempDir()
	stub := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done\n"
	if err := os.WriteFile(filepath.Join(stubs, "claude"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, s := range samples {
		cmdline := MakeSnippets(s.path, s.url).ClaudeCode
		if !strings.HasPrefix(strings.TrimSpace(cmdline), "claude ") {
			t.Errorf("ClaudeCode snippet must start with `claude `: %q", cmdline)
			continue
		}
		cmd := exec.Command(sh, "-c", cmdline)
		cmd.Env = append(os.Environ(), "PATH="+stubs+":/usr/bin:/bin")
		out, err := cmd.Output()
		if err != nil {
			t.Errorf("ClaudeCode snippet %q failed in sh: %v", cmdline, err)
			continue
		}
		got := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
		if want := ClaudeAddArgs(s.path, s.url); !reflect.DeepEqual(got, want) {
			t.Errorf("snippet %q gives claude args\n %q\nwant\n %q", cmdline, got, want)
		}
	}
}

func TestSnippetClaudeDesktopJSON(t *testing.T) {
	t.Parallel()
	for _, s := range samples {
		raw := MakeSnippets(s.path, s.url).ClaudeDesktop
		var cfg struct {
			MCPServers map[string]struct {
				Command string            `json:"command"`
				Env     map[string]string `json:"env"`
			} `json:"mcpServers"`
		}
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			t.Errorf("ClaudeDesktop snippet is not valid JSON: %v\n%s", err, raw)
			continue
		}
		srv, ok := cfg.MCPServers["partscaster"]
		if !ok {
			t.Errorf("no mcpServers.partscaster in:\n%s", raw)
			continue
		}
		if srv.Command != s.path {
			t.Errorf("command = %q, want %q", srv.Command, s.path)
		}
		if srv.Env["PARTSCASTER_URL"] != s.url {
			t.Errorf("env.PARTSCASTER_URL = %q, want %q", srv.Env["PARTSCASTER_URL"], s.url)
		}
	}
}

// TOML разбирается python3 (tomllib, Python ≥ 3.11): в go.mod своего TOML-парсера нет,
// а самодельный разбор в тесте зеркалил бы реализацию.
func TestSnippetCodexTOML(t *testing.T) {
	t.Parallel()
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found")
	}
	if err := exec.Command(py, "-c", "import tomllib").Run(); err != nil {
		t.Skip("python3 without tomllib (< 3.11)")
	}
	for _, s := range samples {
		raw := MakeSnippets(s.path, s.url).Codex
		if !strings.Contains(raw, "[mcp_servers.partscaster]") {
			t.Errorf("Codex snippet lacks [mcp_servers.partscaster]:\n%s", raw)
		}
		cmd := exec.Command(py, "-I", "-c",
			"import json,sys,tomllib; print(json.dumps(tomllib.loads(sys.stdin.read())))")
		cmd.Stdin = strings.NewReader(raw)
		out, err := cmd.Output()
		if err != nil {
			t.Errorf("Codex snippet is not valid TOML: %v\n%s", err, raw)
			continue
		}
		var cfg struct {
			MCPServers map[string]struct {
				Command string            `json:"command"`
				Env     map[string]string `json:"env"`
			} `json:"mcp_servers"`
		}
		if err := json.Unmarshal(out, &cfg); err != nil {
			t.Fatalf("decode tomllib output: %v\n%s", err, out)
		}
		srv, ok := cfg.MCPServers["partscaster"]
		if !ok {
			t.Errorf("no mcp_servers.partscaster in parsed TOML: %s", out)
			continue
		}
		if srv.Command != s.path {
			t.Errorf("command = %q, want %q\n%s", srv.Command, s.path, raw)
		}
		if srv.Env["PARTSCASTER_URL"] != s.url {
			t.Errorf("env.PARTSCASTER_URL = %q, want %q\n%s", srv.Env["PARTSCASTER_URL"], s.url, raw)
		}
	}
}

func TestMCPPath(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(string(filepath.Separator)+"opt", "Partscaster")
	for _, goos := range []string{"linux", "darwin"} {
		if got, want := MCPPath(dir, goos), filepath.Join(dir, "partscaster-mcp"); got != want {
			t.Errorf("MCPPath(%q, %s) = %q, want %q", dir, goos, got, want)
		}
	}
	got := MCPPath(dir, "windows")
	if !strings.HasSuffix(got, "partscaster-mcp.exe") || !strings.HasPrefix(got, dir) {
		t.Errorf("MCPPath(%q, windows) = %q, want %s/…/partscaster-mcp.exe", dir, got, dir)
	}
	if filepath.Dir(got) != dir {
		t.Errorf("MCPPath windows must be next to the app: dir(%q) != %q", got, dir)
	}
}
