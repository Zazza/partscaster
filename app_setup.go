package main

import (
	"os"
	"path/filepath"
	"runtime"

	"partscaster/internal/agentsetup"
	"partscaster/internal/yue"
)

// version — версия сборки (git describe), вшивается при make build/release.
var version = "dev"

func (a *App) YueAppVersion() string { return version }

// YueVersion — сборка воркера и предупреждение о совместимости ("" — всё в порядке);
// воркер недоступен — ошибка (о недоступности говорит статусбар, не плашка версий).
func (a *App) YueVersion() (map[string]any, error) {
	v, err := a.yue.Version(a.ctx)
	warn := yue.CompatWarning(v, err)
	if v == nil && warn == "" {
		return nil, err
	}
	return map[string]any{"version": v, "warning": warn}, nil
}

// YueCheckWorker — кнопка «Проверить»: воркер по введённому адресу, без сохранения.
func (a *App) YueCheckWorker(url string) (*yue.CheckResult, error) {
	return yue.Check(a.ctx, url)
}

type AgentSetup struct {
	MCPPath  string              `json:"mcp_path"`
	MCPFound bool                `json:"mcp_found"`
	URL      string              `json:"url"`
	Snippets agentsetup.Snippets `json:"snippets"`
}

func (a *App) mcpPath() string {
	exe, err := os.Executable()
	if err != nil {
		return agentsetup.MCPPath(".", runtime.GOOS)
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return agentsetup.MCPPath(filepath.Dir(exe), runtime.GOOS)
}

// YueAgentSetup — путь partscaster-mcp рядом с приложением и готовые фрагменты конфига с текущим адресом.
func (a *App) YueAgentSetup() AgentSetup {
	p, url := a.mcpPath(), a.yue.GetURL()
	_, err := os.Stat(p)
	return AgentSetup{MCPPath: p, MCPFound: err == nil, URL: url, Snippets: agentsetup.MakeSnippets(p, url)}
}

// YueConnectClaudeCode — кнопка «Подключить к Claude Code» (подтверждение — во фронтенде).
func (a *App) YueConnectClaudeCode() (string, error) {
	return agentsetup.ConnectClaudeCode(a.ctx, a.mcpPath(), a.yue.GetURL())
}
