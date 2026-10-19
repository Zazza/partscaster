package yue

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"time"
)

// modelTimeout — загрузка модели yue2.cpp: до 7,2 ГБ (BF16) при домашних 2–5 МБ/с.
const modelTimeout = 3 * time.Hour

// modelHTTP — без ResponseHeaderTimeout общего транспорта (15 мин): воркер отвечает, когда модель скачана;
// предел задаёт контекст (modelTimeout).
var modelHTTP = &http.Client{Transport: &http.Transport{
	DialContext: (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	Proxy:       nil,
}}

// InstallEngineModel — воркер скачивает модель генерации yue2.cpp и VAE с проверкой суммы;
// ждёт конца загрузки, готовые файлы не качаются заново.
func (c *Client) InstallEngineModel(ctx context.Context, name string) (map[string]any, error) {
	var out map[string]any
	path := "/engine/models/install?name=" + url.QueryEscape(name)
	if err := c.callWith(modelHTTP, ctx, http.MethodPost, path, modelTimeout, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// EngineModelProgress — идущая загрузка модели {name, file, done, total}; нет — пустой объект.
func (c *Client) EngineModelProgress(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	if err := c.get(ctx, "/engine/models/progress", &out); err != nil {
		return nil, err
	}
	return out, nil
}
