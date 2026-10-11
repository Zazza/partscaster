package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"yue-studio/internal/studio"
	"yue-studio/internal/yue"
)

// Рецепт трека (docs/track-recipe.md): экспорт песни правилами в файл и импорт на этой установке.

// recipeTick — как часто приложение делает шаг импорта рецепта (генерации ждут на воркере)
const recipeTick = 5 * time.Second

// recipeMaxBytes — рецепт без аудио: десятки КБ; больше — не рецепт
const recipeMaxBytes = 5 << 20

// recipeLoop — ведущий импорта, пока приложение открыто (MCP ведёт своим циклом; захват не даёт сделать шаг дважды).
func (a *App) recipeLoop(ctx context.Context) {
	r := &studio.RecipeRunner{Svc: a.yue, Owner: studio.RecipeOwner("app")}
	t := time.NewTicker(recipeTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for r.Tick(ctx) && ctx.Err() == nil { // правки без генерации — подряд, без ожидания тика
			}
		}
	}
}

// RecipeExport — итог экспорта: путь ("" — отмена) и что в рецепт не вошло.
type RecipeExport struct {
	Path     string   `json:"path"`
	Warnings []string `json:"warnings"`
}

// YueExportRecipe — рецепт песни трека в файл *.yue-track.json (диалог сохранения).
func (a *App) YueExportRecipe(jobID int64) (*RecipeExport, error) {
	data, err := a.yue.Recipe(a.ctx, jobID)
	if err != nil {
		return nil, err
	}
	var head struct {
		Track struct {
			Title string `json:"title"`
		} `json:"track"`
		Warnings []string `json:"warnings"`
	}
	_ = json.Unmarshal(data, &head)
	target, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: recipeFileName(head.Track.Title, jobID),
		Filters:         []runtime.FileFilter{{DisplayName: "Рецепт трека", Pattern: "*.yue-track.json"}},
	})
	if err != nil || target == "" {
		return &RecipeExport{}, err
	}
	if err := os.WriteFile(filepath.Clean(target), data, 0o644); err != nil {
		return nil, err
	}
	return &RecipeExport{Path: target, Warnings: head.Warnings}, nil
}

// recipeFileName — «Название.yue-track.json» без символов, которые не любят файловые системы.
func recipeFileName(title string, id int64) string {
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 32 {
			return '_'
		}
		return r
	}, strings.TrimSpace(title))
	if name == "" {
		name = fmt.Sprintf("track-%d", id)
	}
	return name + ".yue-track.json"
}

// YuePickRecipe — выбрать файл рецепта для импорта: его текст (превью строит фронтенд); "" — отмена.
func (a *App) YuePickRecipe() (string, error) {
	src, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Импорт рецепта трека",
		Filters: []runtime.FileFilter{{DisplayName: "Рецепт трека", Pattern: "*.yue-track.json;*.json"}},
	})
	if err != nil || src == "" {
		return "", err
	}
	info, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	if info.Size() > recipeMaxBytes {
		return "", errors.New("файл слишком большой для рецепта трека")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// YueImportRecipe — начать импорт (mode seed | plan); дальше его ведёт recipeLoop.
func (a *App) YueImportRecipe(recipe string, mode string) (int64, error) {
	return a.yue.ImportRecipe(a.ctx, json.RawMessage(recipe), mode)
}

// YueRecipeImportAction — у упавшего импорта: retry (продолжить с того же шага) | dismiss (убрать из списка).
func (a *App) YueRecipeImportAction(id int64, action string) error {
	return studio.RecipeImportAction(a.ctx, a.yue, id, action)
}

// YueRecipeImports — сессии импорта (прогресс в списке треков).
func (a *App) YueRecipeImports() ([]yue.RecipeImport, error) {
	return a.yue.RecipeImports(a.ctx)
}
