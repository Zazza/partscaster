package mcp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"partscaster/internal/yue"
)

const compatInterval = 30 * time.Minute

// compatChecker — приписка к ответам инструментов, когда воркер и MCP разных поколений API
// (yue.CompatWarning). Ответ запоминается на compatInterval — и неудачный тоже: выключенная
// GPU-машина иначе стоила бы таймаута соединения на каждом вызове инструмента.
type compatChecker struct {
	client yue.Service
	now    func() time.Time

	mu      sync.Mutex
	checked time.Time
	url     string // адрес воркера, для которого запомнен ответ: смена адреса (config_set) — новая проверка
	warning string
}

func (c *compatChecker) text() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	url := c.client.GetURL()
	if now := c.now(); c.checked.IsZero() || url != c.url || now.Sub(c.checked) >= compatInterval {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		info, err := c.client.Version(ctx)
		cancel()
		c.checked, c.url, c.warning = now, url, yue.CompatWarning(info, err)
	}
	if c.warning == "" {
		return ""
	}
	return "⚠ " + c.warning + "\n\n"
}

// SetVersion — версия сборки (git describe, вшивается при make mcp/release): serverInfo.version.
func (s *Server) SetVersion(v string) {
	if v != "" {
		s.version = v
	}
}

// workerVersionLine — сборка воркера для doctor: версия, API, отпечаток кода и совет при расхождении.
func workerVersionLine(s *Server) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	v, err := s.client.Version(ctx)
	line := "== версия воркера\n"
	switch {
	case v != nil:
		line += fmt.Sprintf("%s (API %d, код %s, модель %s)\n", v.Version, v.API, v.SourceHash, v.Model)
	case err != nil:
		line += "не получена: " + err.Error() + "\n"
	}
	if w := yue.CompatWarning(v, err); w != "" {
		line += "⚠ " + w + "\n"
	}
	return line + "\n"
}
