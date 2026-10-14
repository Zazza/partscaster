// partscaster-mcp — MCP-сервер (stdio) для управления Partscaster через AI-агентов:
// генерация музыки, настройки, библиотека стилей, установка воркера.
//
// Адрес воркера: переменная PARTSCASTER_URL или сохранённые настройки приложения.
// Артефакты скачиваются в PARTSCASTER_MCP_DIR (по умолчанию ~/partscaster-mcp-downloads).
package main

import (
	"log"
	"os"
	"path/filepath"

	"partscaster/internal/config"
	"partscaster/internal/mcp"
	"partscaster/internal/yue"
)

// Вшиваются при сборке (make mcp): отпечаток исходников и каталог репозитория —
// по ним MCP замечает, что собран раньше, чем менялся код.
var (
	buildHash string
	srcDir    string
	version   string // git describe (make mcp/release)
)

func main() {
	log.SetFlags(0)

	cfg := config.Load()
	if u := os.Getenv("PARTSCASTER_URL"); u != "" {
		cfg.YueURL = u
	}
	dir := os.Getenv("PARTSCASTER_MCP_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("home dir: %v", err)
		}
		dir = filepath.Join(home, "partscaster-mcp-downloads")
	}

	// журнал правок (рецепт трека): изменяющие вызовы API пишутся после успеха
	srv := mcp.NewServer(yue.Journaled(yue.New(cfg.YueURL)), dir)
	srv.SetBuildInfo(buildHash, srcDir)
	srv.SetVersion(version)
	mcp.RegisterWorkflowTools(srv)
	mcp.RegisterStudioTools(srv)
	mcp.RegisterLibraryTools(srv)
	mcp.RegisterInstallTools(srv)

	if err := srv.Serve(os.Stdin, os.Stdout); err != nil {
		log.Fatalf("mcp serve: %v", err)
	}
}
