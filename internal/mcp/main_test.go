package mcp

import (
	"fmt"
	"os"
	"testing"
)

// TestMain — свой TMPDIR на прогон пакета: каждый тестовый сервер заводит каталог кэша превью
// (fxCache, os.MkdirTemp), после прогона каталог удаляется целиком — в системном /tmp ничего не копится.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "yue-mcp-test-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "TMPDIR для тестов:", err)
		os.Exit(1)
	}
	_ = os.Setenv("TMPDIR", dir)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
