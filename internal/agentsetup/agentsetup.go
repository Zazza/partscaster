// Package agentsetup — раздел настроек «Для AI-агентов»: готовые фрагменты конфига
// MCP-клиентов и подключение yue-mcp к Claude Code одной кнопкой.
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

// ServerName — имя MCP-сервера в конфигах клиентов.
const ServerName = "yue-studio"

// MCPPath — yue-mcp рядом с исполняемым файлом приложения (так его кладёт релизный пакет,
// и так же лежат build/bin/yue-studio и build/bin/yue-mcp при сборке из исходников).
func MCPPath(exeDir, goos string) string {
	name := "yue-mcp"
	if goos == "windows" {
		name += ".exe"
	}
	return filepath.Join(exeDir, name)
}

// ClaudeAddArgs — аргументы `claude` для регистрации сервера на уровне пользователя
// (виден во всех проектах); адрес воркера передаётся переменной YUE_URL.
func ClaudeAddArgs(mcpPath, url string) []string {
	return []string{"mcp", "add", ServerName, "--scope", "user", "-e", "YUE_URL=" + url, "--", mcpPath}
}

// Snippets — фрагменты конфигурации для копирования.
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
		ServerName: map[string]any{"command": mcpPath, "env": map[string]string{"YUE_URL": url}},
	}}
	return Snippets{
		ClaudeCode:    "claude " + strings.Join(quoted, " "),
		ClaudeDesktop: jsonText(desktop, "  "),
		Codex: fmt.Sprintf("[mcp_servers.%s]\ncommand = %s\nenv = { YUE_URL = %s }\n",
			ServerName, jsonText(mcpPath, ""), jsonText(url, "")),
	}
}

// ConnectClaudeCode выполняет `claude mcp add …`; уже зарегистрированный сервер
// сначала снимается, чтобы новый путь и адрес заменили старые.
func ConnectClaudeCode(ctx context.Context, mcpPath, url string) (string, error) {
	if _, err := os.Stat(mcpPath); err != nil {
		return "", fmt.Errorf("yue-mcp не найден рядом с приложением (%s) — соберите его: make mcp", mcpPath)
	}
	claude, err := exec.LookPath("claude")
	if err != nil {
		return "", errors.New("не найден Claude Code (команда claude) — установите его или скопируйте команду вручную")
	}
	_ = exec.CommandContext(ctx, claude, "mcp", "remove", ServerName, "--scope", "user").Run() // не было — не ошибка
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
