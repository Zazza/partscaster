package yue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Черновик студии — незаконченная работа над треком (правки «Правок трека» и
// ожидающие рендеры). Содержимое state ведёт приложение, воркер его хранит как есть.
// Это не правка трека — в журнал не пишется.

func (c *Client) Drafts(ctx context.Context) ([]Draft, error) {
	var out []Draft
	if err := c.get(ctx, "/drafts", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Draft — черновик трека; нет черновика (404) — nil без ошибки.
func (c *Client) Draft(ctx context.Context, jobID int64) (*Draft, error) {
	var out Draft
	err := c.get(ctx, fmt.Sprintf("/jobs/%d/draft", jobID), &out)
	var se *StatusError
	if errors.As(err, &se) && se.Code == http.StatusNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DraftPut(ctx context.Context, jobID int64, state json.RawMessage) error {
	return c.call(ctx, http.MethodPut, fmt.Sprintf("/jobs/%d/draft", jobID), requestTimeout,
		jsonReq(map[string]json.RawMessage{"state": state}), nil)
}

func (c *Client) DraftDelete(ctx context.Context, jobID int64) (bool, error) {
	var out struct {
		Deleted bool `json:"deleted"`
	}
	if err := c.del(ctx, fmt.Sprintf("/jobs/%d/draft", jobID), &out); err != nil {
		return false, err
	}
	return out.Deleted, nil
}
