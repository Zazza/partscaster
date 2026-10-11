package yue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Рецепт трека (docs/track-recipe.md): экспорт песни правилами без аудио и сессии импорта на воркере.

// ErrClaimed — сессию импорта уже ведёт другой (приложение или MCP), лиза не истекла.
var ErrClaimed = errors.New("импорт ведёт другой")

// RecipeImport — сессия импорта рецепта (воркер хранит, ведёт studio.RecipeRunner).
type RecipeImport struct {
	ID         int64           `json:"id"`
	Title      string          `json:"title"`
	Status     string          `json:"status"` // running | done | error
	Progress   string          `json:"progress"`
	Error      string          `json:"error"`
	Mode       string          `json:"mode"` // seed | plan
	RootJobID  int64           `json:"root_job_id"`
	CreatedAt  string          `json:"created_at"`
	FinishedAt string          `json:"finished_at"`
	Recipe     json.RawMessage `json:"recipe,omitempty"`
	State      json.RawMessage `json:"state,omitempty"`
}

// RecipeImportUpdate — изменения сессии (nil — поле не трогать).
type RecipeImportUpdate struct {
	// Owner — ведущий (как в ClaimRecipeImport): импорт перехватил другой — ErrClaimed, состояние не пишется;
	// "" — действие человека (повторить, убрать)
	Owner     string          `json:"owner,omitempty"`
	State     json.RawMessage `json:"state,omitempty"`
	Progress  *string         `json:"progress,omitempty"`
	Status    *string         `json:"status,omitempty"`
	Error     *string         `json:"error,omitempty"`
	RootJobID *int64          `json:"root_job_id,omitempty"`
}

// Recipe — рецепт песни трека id (от её корня), JSON как отдаёт воркер; 422 — не переносится (причина в ошибке).
func (c *Client) Recipe(ctx context.Context, id int64) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.get(ctx, fmt.Sprintf("/jobs/%d/recipe", id), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ImportRecipe — новая сессия импорта; mode: seed (тем же сидом) | plan (по плану нот).
func (c *Client) ImportRecipe(ctx context.Context, recipe json.RawMessage, mode string) (int64, error) {
	var out struct {
		ID int64 `json:"id"`
	}
	body := map[string]any{"recipe": recipe, "mode": mode}
	if err := c.postJSON(ctx, "/recipes/import", body, requestTimeout, &out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

func (c *Client) RecipeImports(ctx context.Context) ([]RecipeImport, error) {
	var out []RecipeImport
	if err := c.get(ctx, "/recipes/imports", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) RecipeImport(ctx context.Context, id int64) (*RecipeImport, error) {
	var out RecipeImport
	if err := c.get(ctx, fmt.Sprintf("/recipes/imports/%d", id), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ClaimRecipeImport — взять ведение сессии на 10 мин (повтор тем же owner продлевает); занято — ErrClaimed.
func (c *Client) ClaimRecipeImport(ctx context.Context, id int64, owner string) error {
	err := c.postJSON(ctx, fmt.Sprintf("/recipes/imports/%d/claim", id), map[string]string{"owner": owner}, requestTimeout, nil)
	var se *StatusError
	if errors.As(err, &se) && se.Code == http.StatusConflict {
		return ErrClaimed
	}
	return err
}

func (c *Client) SaveRecipeImport(ctx context.Context, id int64, u RecipeImportUpdate) (*RecipeImport, error) {
	var out RecipeImport
	err := c.call(ctx, http.MethodPatch, fmt.Sprintf("/recipes/imports/%d", id), requestTimeout, jsonReq(u), &out)
	var se *StatusError
	if errors.As(err, &se) && se.Code == http.StatusConflict {
		return nil, ErrClaimed
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}
