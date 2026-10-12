package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"yue-studio/internal/studio"
)

// Рецепт трека: экспорт песни правилами без аудио и импорт на этой установке.
// Импорт ведёт ведущий (studio.RecipeRunner) — здесь свой цикл, пока жив MCP; приложение ведёт тем же
// способом, захват на воркере не даёт сделать шаг дважды.

// recipeTick — пауза между шагами импорта, когда ждём генерацию
const recipeTick = 5 * time.Second

func registerRecipeTools(s *Server) {
	s.Register(Tool{
		Name: "recipe_export",
		Description: "Рецепт трека: песня правилами, без аудио (стиль, текст, сид, план; продолжения, версии, " +
			"эффекты, пресеты, склейки) — файл *.yue-track.json, чтобы на другой установке сделать новый, очень " +
			"похожий трек (recipe_import). Берётся вся песня трека (от корня). Правки, сделанные до журнала, " +
			"не входят — warnings в ответе. Импортированное аудио и «голос альбома» рецептом не переносятся.",
		InputSchema: props(map[string]any{
			"job_id": prop("любой трек песни", "integer"),
			"path":   prop("куда сохранить (пусто — каталог загрузок MCP)", "string"),
		}),
		Handler: func(s *Server, args map[string]any) (string, error) {
			id := argInt(args, "job_id")
			data, err := s.client.Recipe(context.Background(), id)
			if err != nil {
				return "", err
			}
			var rec studio.Recipe
			if err := json.Unmarshal(data, &rec); err != nil {
				return "", err
			}
			path := argString(args, "path")
			if path != "" && !strings.HasSuffix(path, ".yue-track.json") {
				return "", errors.New("path: файл рецепта — *.yue-track.json (чтобы не перезаписать чужой файл)")
			}
			if path == "" {
				if err := os.MkdirAll(s.downloadDir, 0o755); err != nil {
					return "", err
				}
				path = filepath.Join(s.downloadDir, fmt.Sprintf("track-%d.yue-track.json", id))
			}
			if err := os.WriteFile(path, data, 0o644); err != nil {
				return "", err
			}
			out := fmt.Sprintf("рецепт: %s\nтрек «%s», шагов: %d, модель: %s", path, rec.Track.Title, len(rec.Ops)+1, rec.WorkerModel)
			if len(rec.Warnings) > 0 {
				out += "\nпредупреждения:\n- " + strings.Join(rec.Warnings, "\n- ")
			}
			return out, nil
		},
	})

	s.Register(Tool{
		Name: "recipe_import",
		Description: "Импорт рецепта трека (*.yue-track.json): по его правилам создаётся НОВЫЙ трек — генерации заново " +
			"(очередь видеокарты) и те же правки по порядку. Исполнение очень близкое, но не идентичное; правки звука — " +
			"точно. mode: seed (по умолчанию, тот же сид) | plan (по плану нот — если модель воркера другой версии: " +
			"ноты те же, исполнение новое). Сначала без confirm — превью (название, шаги, модель, предупреждения), " +
			"покажи пользователю и спроси; импорт — confirm=true. Ход — recipe_imports.",
		InputSchema: props(map[string]any{
			"path":    prop("файл рецепта", "string"),
			"mode":    prop("seed | plan", "string"),
			"confirm": prop("начать импорт (спроси пользователя)", "boolean"),
		}),
		Handler: func(s *Server, args map[string]any) (string, error) {
			data, err := os.ReadFile(argString(args, "path"))
			if err != nil {
				return "", err
			}
			var rec studio.Recipe
			if err := json.Unmarshal(data, &rec); err != nil || rec.Format != "yue-track-recipe" {
				return "", errors.New("это не рецепт трека Yue Studio")
			}
			mode := argString(args, "mode")
			if mode == "" {
				mode = "seed"
			}
			if mode != "seed" && mode != "plan" {
				return "", fmt.Errorf("mode: seed или plan (получено %q)", mode)
			}
			preview := fmt.Sprintf("трек «%s», шагов: %d, модель рецепта: %s", rec.Track.Title, len(rec.Ops)+1, rec.WorkerModel)
			if v, err := s.client.Version(context.Background()); err == nil && v.Model != rec.WorkerModel {
				preview += fmt.Sprintf("\nмодель воркера другая (%s): исполнение может отличаться заметнее — можно mode=plan", v.Model)
			}
			if len(rec.Warnings) > 0 {
				preview += "\nпредупреждения рецепта:\n- " + strings.Join(rec.Warnings, "\n- ")
			}
			if !argBool(args, "confirm") {
				return "ПРЕВЬЮ — импорт не начат (confirm=true — начать).\n" + preview, nil
			}
			id, err := s.client.ImportRecipe(context.Background(), data, mode)
			if err != nil {
				return "", err
			}
			s.startRecipeLoop()
			return fmt.Sprintf("импорт #%d начат (%s). Ход — recipe_imports.\n%s", id, mode, preview), nil
		},
	})

	s.Register(Tool{
		Name: "recipe_imports",
		Description: "Импорты рецептов: статус (running/done/error), ход, ошибка, трек-корень. У упавшего: " +
			"retry=<id> — продолжить с того же шага (готовые не переделываются), dismiss=<id> — убрать из списка.",
		InputSchema: props(map[string]any{
			"retry":   prop("id упавшего импорта — продолжить", "integer"),
			"dismiss": prop("id импорта — убрать из списка", "integer"),
		}),
		Handler: func(s *Server, args map[string]any) (string, error) {
			for action, key := range map[string]string{"retry": "retry", "dismiss": "dismiss"} {
				if id := argInt(args, key); id != 0 {
					if err := studio.RecipeImportAction(context.Background(), s.client, id, action); err != nil {
						return "", err
					}
				}
			}
			list, err := s.client.RecipeImports(context.Background())
			if err != nil {
				return "", err
			}
			for _, imp := range list {
				if imp.Status == "running" {
					s.startRecipeLoop() // незаконченный импорт — ведём, пока жив MCP
					break
				}
			}
			return toJSON(list), nil
		},
	})
}

// startRecipeLoop — ведущий импорта в фоне MCP (один на процесс).
func (s *Server) startRecipeLoop() {
	s.recipeOnce.Do(func() {
		r := &studio.RecipeRunner{Svc: s.client, Owner: studio.RecipeOwner("mcp")}
		go func() {
			for {
				for r.Tick(context.Background()) {
				}
				time.Sleep(recipeTick)
			}
		}()
	})
}
