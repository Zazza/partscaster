package yue

import (
	"context"
	"fmt"
	neturl "net/url"
)

// Plan — стадия плана: ABC-партитура без рендера (грузит модель, минуты).
func (c *Client) Plan(ctx context.Context, p PlanParams) (*PlanResult, error) {
	var out PlanResult
	if err := c.postJSON(ctx, "/plan", p, planTimeout, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// JobLyrics — текст из готового аудио джобы (whisper на стороне воркера,
// без повторной загрузки файла; есть дорожка голоса — по ней). language —
// код языка пения (en, ru, …; "auto" — определить); пусто — по стилю трека.
func (c *Client) JobLyrics(ctx context.Context, id int64, language string) (*LyricsResult, error) {
	var out LyricsResult
	path := fmt.Sprintf("/jobs/%d/lyrics", id)
	if language != "" {
		path += "?language=" + neturl.QueryEscape(language)
	}
	if err := c.post(ctx, path, gpuTimeout, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// WorkerConfig — настройки воркера: пути данных, дорожки, движки.
func (c *Client) WorkerConfig(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	if err := c.get(ctx, "/config", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SetWorkerConfig — смена настроек воркера на лету (stems_auto, fx_engine, cpp_model…).
func (c *Client) SetWorkerConfig(ctx context.Context, cfg map[string]any) error {
	return c.postJSON(ctx, "/config", cfg, requestTimeout, nil)
}
