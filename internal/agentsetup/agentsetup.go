// Package agentsetup — раздел настроек «Для AI-агентов»: готовые фрагменты конфига
// MCP-клиентов и подключение partscaster-mcp к Claude Code одной кнопкой.
package agentsetup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const ServerName = "partscaster"

// LegacyServerName — имя сервера до переименования проекта; старую регистрацию
// снимаем при подключении, чтобы у агентов не осталось битой ссылки на yue-mcp.
const LegacyServerName = "yue-studio"

// MCPPath — partscaster-mcp рядом с исполняемым файлом приложения (так его кладёт релизный пакет,
// и так же лежат build/bin/partscaster и build/bin/partscaster-mcp при сборке из исходников).
func MCPPath(exeDir, goos string) string {
	name := "partscaster-mcp"
	if goos == "windows" {
		name += ".exe"
	}
	return filepath.Join(exeDir, name)
}

// ClaudeAddArgs — аргументы `claude` для регистрации сервера на уровне пользователя
// (виден во всех проектах); адрес воркера передаётся переменной PARTSCASTER_URL.
func ClaudeAddArgs(mcpPath, url string) []string {
	return []string{"mcp", "add", ServerName, "--scope", "user", "-e", "PARTSCASTER_URL=" + url, "--", mcpPath}
}

type Snippets struct {
	ClaudeCode    string `json:"claude_code"`    // команда для терминала
	ClaudeDesktop string `json:"claude_desktop"` // JSON для claude_desktop_config.json
	Codex         string `json:"codex"`          // TOML для ~/.codex/config.toml
}

func MakeSnippets(mcpPath, url string) Snippets {
	args := ClaudeAddArgs(mcpPath, url)
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = shellQuote(a)
	}
	desktop := map[string]any{"mcpServers": map[string]any{
		ServerName: map[string]any{"command": mcpPath, "env": map[string]string{"PARTSCASTER_URL": url}},
	}}
	return Snippets{
		ClaudeCode:    "claude " + strings.Join(quoted, " "),
		ClaudeDesktop: jsonText(desktop, "  "),
		Codex: fmt.Sprintf("[mcp_servers.%s]\ncommand = %s\nenv = { PARTSCASTER_URL = %s }\n",
			ServerName, jsonText(mcpPath, ""), jsonText(url, "")),
	}
}

// ConnectClaudeCode выполняет `claude mcp add …`; уже зарегистрированный сервер
// сначала снимается, чтобы новый путь и адрес заменили старые.
func ConnectClaudeCode(ctx context.Context, mcpPath, url string) (string, error) {
	if _, err := os.Stat(mcpPath); err != nil {
		return "", fmt.Errorf("partscaster-mcp не найден рядом с приложением (%s) — соберите его: make mcp", mcpPath)
	}
	claude, err := exec.LookPath("claude")
	if err != nil {
		return "", errors.New("не найден Claude Code (команда claude) — установите его или скопируйте команду вручную")
	}
	_ = exec.CommandContext(ctx, claude, "mcp", "remove", ServerName, "--scope", "user").Run()       // не было — не ошибка
	_ = exec.CommandContext(ctx, claude, "mcp", "remove", LegacyServerName, "--scope", "user").Run() // регистрация до переименования
	out, err := exec.CommandContext(ctx, claude, ClaudeAddArgs(mcpPath, url)...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("claude mcp add: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// shellQuote — одинарные кавычки для sh: внутри ничего не раскрывается, сама «'» — как '\”.
func shellQuote(s string) string {
	safe := func(r rune) bool {
		return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./:=@", r)
	}
	if s != "" && strings.IndexFunc(s, func(r rune) bool { return !safe(r) }) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// jsonText — JSON без HTML-экранирования; строка в JSON — заодно валидная строка TOML.
func jsonText(v any, indent string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", indent)
	_ = enc.Encode(v)
	return strings.TrimSuffix(b.String(), "\n")
}
