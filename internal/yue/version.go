package yue

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"partscaster/internal/config"
)

// WorkerAPI — версия API воркера, с которой работает этот клиент (приложение и MCP).
// Растёт вместе с WORKER_API воркера при несовместимом изменении.
const WorkerAPI = 1

// VersionInfo — GET /version воркера: какая сборка запущена и что в ней установлено.
type VersionInfo struct {
	Version    string          `json:"version"`
	API        int             `json:"api"`
	SourceHash string          `json:"source_hash"`
	Model      string          `json:"model"`
	Components map[string]bool `json:"components"`
	StemsModel string          `json:"stems_model"`
}

func (c *Client) Version(ctx context.Context) (*VersionInfo, error) {
	var v VersionInfo
	if err := c.get(ctx, "/version", &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// CompatWarning — что сказать человеку о совместимости по ответу /version; "" — всё в порядке
// или воркер просто недоступен (это не вопрос версий, о недоступности говорят в другом месте).
func CompatWarning(info *VersionInfo, err error) string {
	var se *StatusError
	switch {
	case errors.As(err, &se) && se.Code == http.StatusNotFound:
		return "Воркер старой версии (не сообщает версию): часть функций может не работать — обновите воркер."
	case err != nil || info == nil:
		return ""
	case info.API < WorkerAPI:
		return fmt.Sprintf("Воркер %s устарел (API %d, приложению нужен %d): обновите воркер.", info.Version, info.API, WorkerAPI)
	case info.API > WorkerAPI:
		return fmt.Sprintf("Воркер %s новее приложения (API %d, приложение знает %d): обновите приложение.", info.Version, info.API, WorkerAPI)
	}
	return ""
}

// CheckResult — проверка адреса воркера из настроек (до сохранения).
type CheckResult struct {
	URL     string
	Health  *HealthInfo
	Version *VersionInfo
	Warning string
}

// Check опрашивает воркер по введённому адресу отдельным клиентом: настройки и рабочий
// клиент приложения не меняются. Воркер не отвечает на /health — ошибка.
func Check(ctx context.Context, rawURL string) (*CheckResult, error) {
	u, err := config.NormalizeURL(rawURL)
	if err != nil {
		return nil, err
	}
	c := New(u)
	h, err := c.Health(ctx)
	if err != nil {
		return nil, fmt.Errorf("воркер по адресу %s не отвечает: %w", u, err)
	}
	v, verr := c.Version(ctx)
	return &CheckResult{URL: u, Health: h, Version: v, Warning: CompatWarning(v, verr)}, nil
}
