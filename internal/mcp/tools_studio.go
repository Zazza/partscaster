package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"partscaster/internal/config"
	"partscaster/internal/dsp"
	"partscaster/internal/studio"
	"partscaster/internal/yue"
)

func RegisterStudioTools(s *Server) {
	registerRevoiceTools(s)
	registerFxTools(s)
	registerPresetTools(s)
	registerDraftTools(s)
	registerRecipeTools(s)
	s.Register(Tool{
		Name:        "config_get",
		Description: "Текущие настройки: адрес воркера, пути данных, модель генерации (выбранная и рекомендуемая для карты) и движки воркера.",
		InputSchema: props(nil),
		Handler: func(s *Server, _ map[string]any) (string, error) {
			out := map[string]any{"server_url": s.client.GetURL()}
			if cfg, err := s.client.WorkerConfig(context.Background()); err == nil {
				out["worker"] = cfg
				if hint := modelHint(cfg); hint != "" {
					out["model"] = hint
				}
			} else {
				out["worker"] = "недоступен: " + err.Error()
			}
			return toJSON(out), nil
		},
	})

	s.Register(Tool{
		Name: "config_set",
		Description: "Настройки: server_url — адрес воркера (сохраняется и в settings приложения); " +
			"stems_auto — когда делать дорожки трека (studio — при открытии студии | always — сразу у каждого готового трека | manual — вручную); " +
			"fx_engine — звуковой движок вкл/выкл; " +
			"cpp_model — модель генерации (любая скачанная; рекомендуемая для карты — в config_get), cpp_low_vram — экономный режим; " +
			"модель и режим — со следующей генерации.",
		InputSchema: props(map[string]any{
			"server_url": prop("адрес воркера, напр. http://gpu-host:8091", "string"),
			"stems_auto": prop("когда делать дорожки: studio (при открытии студии) | always (сразу у каждого готового трека) | manual (вручную)", "string"),
			"fx_engine":  prop("звуковой движок воркера (fx_apply): false — выключить (откат на эффекты ffmpeg)", "boolean"),
			"cpp_model": prop("модель генерации: bf16 | q8 | q6 | q5 (должна быть скачана — engine_model_install). "+
				"Тот же сид на другой модели даёт другое исполнение; ноты и продолжения переносятся", "string"),
			"cpp_low_vram": prop("экономный режим (~4,5 ГБ видеопамяти, песня до ~3,5 мин)", "boolean"),
		}),
		Handler: func(s *Server, args map[string]any) (string, error) {
			if u := argString(args, "server_url"); u != "" {
				s.client.SetURL(strings.TrimRight(u, "/"))
				config.SaveSettings(config.Settings{ServerURL: s.client.GetURL()})
			}
			cfg := map[string]any{}
			if v := argString(args, "stems_auto"); v != "" {
				cfg["stems_auto"] = v
			}
			if v, ok := args["fx_engine"].(bool); ok {
				cfg["fx_engine"] = v
			}
			if v := argString(args, "cpp_model"); v != "" {
				cfg["cpp_model"] = v
			}
			if v, ok := args["cpp_low_vram"].(bool); ok {
				cfg["cpp_low_vram"] = v
			}
			if len(cfg) > 0 {
				if err := s.client.SetWorkerConfig(context.Background(), cfg); err != nil {
					return "", fmt.Errorf("воркер не принял настройки: %w (старая версия или недоступен)", err)
				}
			}
			return "ок. Проверь config_get", nil
		},
	})

	s.Register(Tool{
		Name: "engine_model_install",
		Description: "Скачать модель генерации (yue2.cpp) на воркер с проверкой суммы: " +
			"bf16 (7,2 ГБ, без сжатия, ~10 ГБ видеопамяти), q8 (3,8 ГБ, почти без потерь, ~8 ГБ), q6 (2,9 ГБ), q5 (2,6 ГБ). " +
			"Ждёт конца загрузки (минуты); готовая модель не качается заново. Потом — config_set cpp_model=<имя>.",
		InputSchema: props(map[string]any{
			"name": prop("модель: bf16 | q8 | q6 | q5", "string"),
		}, "name"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			name := argString(args, "name")
			if name == "" {
				return "", errors.New("name: bf16, q8, q6 или q5")
			}
			out, err := s.client.InstallEngineModel(context.Background(), name)
			if err != nil {
				return "", fmt.Errorf("модель %s не скачана: %w", name, err)
			}
			return fmt.Sprintf("модель %s скачана. Выбрать: config_set cpp_model=%s\n%s", name, name, toJSON(out)), nil
		},
	})

	s.Register(Tool{
		Name: "dsp_chains",
		Description: "Пресеты DSP-цепочек эффектов (ffmpeg на ПК): список, параметры и диапазоны крутилок. " +
			"group — группа (space, pedals, modulation, filter, dynamics, time, lofi, voice — разделы effects.md). " +
			"Цепочки с voice=true — примочки на голос (мегафон, телефон, перегруз, слэпбэк): применять на дорожку vocals; " +
			"for_voice=true — общий эффект, уместный и на голосе (реверб, удвоение, де-эссер…). " +
			"Цепочки с key (ducking, key=drums) — только на дорожку (stem, обычно other или bass): ключ — стем key трека. " +
			"Реверб и дилей на дорожке в окне from–to звучат хвостом и после to.",
		InputSchema: props(nil),
		Handler: func(s *Server, _ map[string]any) (string, error) {
			return toJSON(dsp.All()), nil
		},
	})

	s.Register(Tool{
		Name: "dsp_presets",
		Description: "Готовые наборы педалей (как в блоке «Педали» студии): id, название, описание и steps — " +
			"цепочка эффектов по порядку. Подставлять в dsp_preview / dsp_apply (steps) или спеку rebuild_sections.",
		InputSchema: props(nil),
		Handler: func(s *Server, _ map[string]any) (string, error) {
			return toJSON(dsp.Presets()), nil
		},
	})

	s.Register(Tool{
		Name: "dsp_apply",
		Description: "Применить DSP-цепочку к треку джобы (ffmpeg локально): «стена громкости», «кассета» и т.д. " +
			"params — {id параметра: число} (диапазоны — dsp_chains). Цепочки с key (ducking) — только со stem: " +
			"ключ — стем key трека, без stem — ошибка. steps [{chain, params, off}] вместо chain — цепочка эффектов " +
			"по порядку (педали; готовые наборы — dsp_presets): без stem — вариант dsp-pedals.flac.",
		InputSchema: props(map[string]any{
			"job_id": prop("ID джобы", "integer"),
			"chain":  prop("id цепочки (см. dsp_chains)", "string"),
			"params": prop("значения крутилок {param_id: число}", "object"),
			"steps": map[string]any{"type": "array", "description": "вместо chain: цепочка по порядку [{chain, params, off}]",
				"items": map[string]any{"type": "object"}},
			"stem": prop("эффект только на дорожку: vocals / drums / bass / other / guitar / piano (пусто — весь трек); "+
				"через пересборку дорожек, остальное не меняется. У голосовых цепочек (voice=true, напр. мегафон) "+
				"громкость обработанной дорожки выравнивается по исходной (RMS), дБ — сверху. Вызовы НЕ копятся: "+
				"каждый считается с чистой основы в один файл overdub-inst-0.flac — несколько эффектов подавай "+
				"одним rebuild_sections", "string"),
			"from": prop("со stem: с какой секунды", "number"),
			"to":   prop("со stem: по какую секунду (0 — до конца)", "number"),
			"db":   prop("со stem: дБ к обработанной дорожке поверх выравнивания (по умолчанию 0)", "number"),
		}, "job_id"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			params := argNumMap(args, "params")
			if params == nil {
				params = map[string]float64{}
			}
			steps, err := parseSteps(args["steps"])
			if err != nil {
				return "", err
			}
			if len(steps) == 0 && argString(args, "chain") == "" {
				return "", errors.New("нужен chain (одна цепочка) или steps (цепочка по порядку)")
			}
			if stem := argString(args, "stem"); stem != "" {
				res, err := studio.RebuildSections(context.Background(), s.client, argInt(args, "job_id"),
					[]studio.SectionSpec{{Chain: argString(args, "chain"), Params: params, Steps: steps, Stems: []string{stem},
						From: argFloat(args, "from"), To: argFloat(args, "to"), Db: argFloat(args, "db")}})
				if err != nil {
					return "", err
				}
				return toJSON(res.Variant), nil
			}
			if len(steps) > 0 {
				v, err := s.applySteps(argInt(args, "job_id"), steps)
				if err != nil {
					return "", err
				}
				return toJSON(v), nil
			}
			v, err := s.applyDsp(argInt(args, "job_id"), argString(args, "chain"), params)
			if err != nil {
				return "", err
			}
			return toJSON(v), nil
		},
	})

	s.Register(Tool{
		Name: "volume_envelope",
		Description: "Громкость по линии (как «линия громкости» по волне в студии): точки время → дБ, между ними " +
			"линейно в дБ, до первой и после последней — их дБ. Без stem — весь трек (вариант dsp-envelope.flac), " +
			"со stem — только эта дорожка через пересборку (остальное не меняется). Лесенка громкости, " +
			"подъём голоса в куплете, тише проигрыш. Версией-треком — variant_track.",
		InputSchema: props(map[string]any{
			"job_id": prop("ID джобы", "integer"),
			"points": map[string]any{"type": "array", "description": fmt.Sprintf("точки [{t: секунда, db: дБ}], "+
				"дБ %g…%+g, до %d точек", dsp.EnvMinDb, dsp.EnvMaxDb, dsp.EnvMaxPoints),
				"items": map[string]any{"type": "object"}},
			"stem": prop("только дорожка: vocals / drums / bass / other / guitar / piano (пусто — весь трек)", "string"),
		}, "job_id", "points"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			v, err := studio.VolumeEnvelope(context.Background(), s.client, argInt(args, "job_id"),
				argString(args, "stem"), argEnvelope(args, "points"))
			if err != nil {
				return "", err
			}
			return toJSON(v), nil
		},
	})

	s.Register(Tool{
		Name: "dsp_preview",
		Description: "Превью эффектов на куске трека (быстро послушать до применения): одна цепочка (chain + params) " +
			"или цепочка эффектов по порядку (steps — как педали). Без stem — весь трек, со stem — только эта дорожка " +
			"в миксе (остальное не меняется). Окно from–to (секунды трека; по умолчанию 20–35), плюс хвост реверба/" +
			"дилея до 3 с. Результат — вариант dsp-preview-*.flac с метриками (перезаписывается следующим превью).",
		InputSchema: props(map[string]any{
			"job_id": prop("ID джобы", "integer"),
			"chain":  prop("id цепочки (или steps)", "string"),
			"params": prop("значения крутилок {param_id: число}", "object"),
			"steps": map[string]any{"type": "array", "description": "цепочка по порядку: [{chain, params, off}] " +
				"(off: true — эффект выключен); цепочки с key (ducking) нельзя", "items": map[string]any{"type": "object"}},
			"stem": prop("дорожка: vocals / drums / bass / other / guitar / piano (пусто — весь трек)", "string"),
			"from": prop("с какой секунды трека", "number"),
			"to":   prop("по какую секунду трека", "number"),
			"solo": prop("со stem: только обработанная дорожка без остального микса (в плотном миксе звук эффекта маскируется)", "boolean"),
		}, "job_id"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			steps, err := argSteps(args)
			if err != nil {
				return "", err
			}
			v, err := s.fxPreview(argInt(args, "job_id"), argString(args, "stem"), steps,
				argFloat(args, "from"), argFloat(args, "to"), argBool(args, "solo"))
			if err != nil {
				return "", err
			}
			return toJSON(v), nil
		},
	})

	s.Register(Tool{
		Name:        "dsp_variants",
		Description: "Список применённых DSP-вариантов джобы (с метриками); label — что сделано в миксе (эффект · дорожка, вклейки).",
		InputSchema: props(map[string]any{
			"job_id": prop("ID джобы", "integer"),
		}, "job_id"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			vs, err := s.client.JobDspVariants(context.Background(), argInt(args, "job_id"))
			if err != nil {
				return "", err
			}
			return toJSON(vs), nil
		},
	})

	s.Register(Tool{
		Name:        "analyze_job",
		Description: "Метрики трека джобы (librosa): темп, крест-фактор, динамика, спектр. Для сравнения вариантов.",
		InputSchema: props(map[string]any{
			"job_id": prop("ID джобы", "integer"),
		}, "job_id"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			out, err := s.client.AnalyzeJob(context.Background(), argInt(args, "job_id"))
			if err != nil {
				return "", err
			}
			return toJSON(out), nil
		},
	})

	s.Register(Tool{
		Name: "job_peaks",
		Description: "Огибающая громкости артефакта джобы (волна студии): [min, max] амплитуды " +
			"по окнам, ~60 окон/с — найти провалы, тишину, вступления числами, выбрать место " +
			"правки. file пусто = основной трек; bins пусто = каноническое разрешение (кэш воркера).",
		InputSchema: props(map[string]any{
			"job_id": prop("ID джобы", "integer"),
			"file":   prop("аудио-артефакт: audio.flac / dsp-*.flac / overdub-*.flac / stem-*.flac (пусто = основной трек)", "string"),
			"bins":   prop("число окон огибающей (пусто = каноническое: ~60 в секунду, от 500 до 20000)", "integer"),
		}, "job_id"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			out, err := s.client.JobPeaks(context.Background(), argInt(args, "job_id"),
				argString(args, "file"), int(argInt(args, "bins")))
			if err != nil {
				return "", err
			}
			return toJSON(out), nil
		},
	})

	s.Register(Tool{
		Name: "job_spectrum",
		Description: "Спектрограмма артефакта джобы PNG (ось X — 0..длительность, та же шкала " +
			"времени, что волна студии): скачать в каталог загрузок, вернуть путь. Нужен ffmpeg " +
			"на воркере; нет — 503 (волна громкости job_peaks работает и без него).",
		InputSchema: props(map[string]any{
			"job_id": prop("ID джобы", "integer"),
			"file":   prop("аудио-артефакт (пусто = основной трек)", "string"),
		}, "job_id"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			id := argInt(args, "job_id")
			label := argString(args, "file")
			if label == "" {
				label = "audio"
			}
			b, err := s.client.JobSpectrum(context.Background(), id, argString(args, "file"))
			if err != nil {
				return "", err
			}
			if err := os.MkdirAll(s.downloadDir, 0o755); err != nil {
				return "", err
			}
			path := filepath.Join(s.downloadDir, fmt.Sprintf("partscaster-%d-%s.spectrum.png", id, label))
			if err := os.WriteFile(path, b, 0o644); err != nil {
				return "", err
			}
			return "сохранено: " + path, nil
		},
	})

	s.Register(Tool{
		Name: "make_stems",
		Description: "Разделить трек джобы на стемы (drums/bass/other/vocals) и подробные дорожки guitar/piano (внутри other) " +
			"и части барабанов kick/snare/toms/hh/ride/crash (внутри drums) — подробные в сумму трека не входят. " +
			"Модель — BS-Roformer-SW (+ DrumSep для частей барабанов), ~1,5 мин на трек.",
		InputSchema: props(map[string]any{
			"job_id": prop("ID джобы", "integer"),
		}, "job_id"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			defer s.fxCache().Invalidate(argInt(args, "job_id")) // стемы переписаны — кэш превью устарел
			out, err := s.client.MakeStems(context.Background(), argInt(args, "job_id"))
			if err != nil {
				return "", err
			}
			return toJSON(out), nil
		},
	})

	s.Register(Tool{
		Name: "guitar_part",
		Description: "Что сыграет гитарист «по аккордам плана» (звуки группы «Гитарист вписывается по аккордам» — блок " +
			"движка gtrnotes): удары — моменты ударов дорожки guitar, аккорд — из плана в такте звука. Ответ {style, " +
			"bars [{start, end, chord, section}], strikes [{t, midi[]}], beats, tune_cents}. style — манера: 0 авто по " +
			"частям песни (куплет — перебор, припев — бой, прочее — бас + аккорд), 1 бой, 2 перебор, 3 бас + аккорд, " +
			"4 квинты. Нужен план трека (транскрипция) и дорожка guitar. Для разбора «что звучит на N секунде».",
		InputSchema: props(map[string]any{
			"job_id": prop("ID джобы", "integer"),
			"style":  prop("манера 0–4 (по умолчанию 0 — авто)", "integer"),
		}, "job_id"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			id := argInt(args, "job_id")
			if id <= 0 {
				return "", fmt.Errorf("нужен job_id — ID джобы")
			}
			style := argInt(args, "style")
			if style < 0 || style > 4 {
				return "", fmt.Errorf("style — 0…4: авто, бой, перебор, бас + аккорд, квинты")
			}
			out, err := s.client.GuitarPart(context.Background(), id, int(style))
			if err != nil {
				return "", err
			}
			return toJSON(out), nil
		},
	})

	s.Register(Tool{
		Name:        "dsp_variant_delete",
		Description: "Удалить вариант эффекта/вклейки (файл + метрики). Деструктивное: требует confirm=true.",
		InputSchema: props(map[string]any{
			"job_id":  prop("ID джобы", "integer"),
			"file":    prop("имя файла варианта (dsp-*.flac / overdub-*.flac; дорожки stem-* не удаляются)", "string"),
			"confirm": prop("явное подтверждение пользователя", "boolean"),
		}, "job_id", "file", "confirm"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			if !argBool(args, "confirm") {
				return "", errConfirm
			}
			ok, err := s.client.DspVariantDelete(context.Background(),
				argInt(args, "job_id"), argString(args, "file"))
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("deleted=%v", ok), nil
		},
	})

	s.Register(Tool{
		Name:        "variant_track",
		Description: "Вариант DSP-эффекта (dsp-*.flac) отдельным треком с подписью эффекта — дальше работают стемы/минус/эффекты.",
		InputSchema: props(map[string]any{
			"job_id":    prop("ID джобы с вариантом", "integer"),
			"file":      prop("имя файла варианта, напр. dsp-tape.flac", "string"),
			"title":     prop("название нового трека (обычно «исходное · эффект»)", "string"),
			"voice_src": prop("ID рендера, чей голос подставлен в эту версию (после «перепеть»); 0 — голос не менялся", "integer"),
		}, "job_id", "file"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			id, err := s.client.VariantToTrack(context.Background(),
				argInt(args, "job_id"), argString(args, "file"), argString(args, "title"), argInt(args, "voice_src"))
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("вариант стал треком #%d", id), nil
		},
	})

	s.Register(Tool{
		Name: "continue_job",
		Description: "«Продолжение с места»: новый трек = трек до from_sec + продолжение моделью " +
			"(то же исполнение до отметки, без склейки; план и звучание можно изменить). Трек-вложение под родителем.",
		InputSchema: props(map[string]any{
			"job_id":    prop("ID готовой джобы", "integer"),
			"from_sec":  prop("с какой секунды играть заново", "number"),
			"seed":      prop("сид продолжения; 0 — случайный", "integer"),
			"abc":       prop("изменённый план (необязательно)", "string"),
			"style_add": prop("что изменить в звучании с этого места, напр. electric guitar enters and builds", "string"),
			"to_sec": prop("кусок вместо продолжения до конца: модель играет только до to_sec (+2 с хвоста), звук до from_sec — "+
				"трека; так проверяется правка в контексте (abc — полный план с правками); role fragment", "number"),
		}, "job_id", "from_sec"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			from, _ := args["from_sec"].(float64)
			// изменённый план — сначала проверка: битый — ошибка, предупреждения — в ответ
			note, err := planNote(s, argInt(args, "job_id"), argString(args, "abc"), from)
			if err != nil {
				return "", err
			}
			// to_sec передан — кусок (неверное окно воркер отвергнет 422), а не продолжение до конца песни
			if _, ok := args["to_sec"]; ok {
				to := argFloat(args, "to_sec")
				id, err := s.client.FragmentJob(context.Background(), argInt(args, "job_id"), from, to,
					argInt(args, "seed"), argString(args, "abc"), argString(args, "style_add"))
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("кусок поставлен: #%d", id) + note, nil
			}
			id, err := s.client.ContinueJob(context.Background(), argInt(args, "job_id"), from,
				argInt(args, "seed"), argString(args, "abc"), argString(args, "style_add"))
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("продолжение поставлено: #%d", id) + note, nil
		},
	})

	s.Register(Tool{
		Name:        "set_head",
		Description: "Основная версия песни: job_id — корень, head_id — он сам (0) или его версия-потомок.",
		InputSchema: props(map[string]any{
			"job_id":  prop("ID корня песни", "integer"),
			"head_id": prop("ID версии; 0 — сам трек", "integer"),
		}, "job_id", "head_id"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			if err := s.client.SetHead(context.Background(), argInt(args, "job_id"), argInt(args, "head_id")); err != nil {
				return "", err
			}
			return "основная версия обновлена", nil
		},
	})

	s.Register(Tool{
		Name: "job_update",
		Description: "Подпись и папка трека: человеческое название вместо номера (номер при переносе меняется) " +
			"и папка песни («Альбом», «Основы», «Эксперименты» или своя; версии следуют за корнем). " +
			"parent_id — перенести трек под другой (производная версия: в списке сворачивается под ним). " +
			"Поле не передано — не меняется; folder \"\" — убрать из папки.",
		InputSchema: props(map[string]any{
			"job_id":    prop("ID трека", "integer"),
			"title":     prop("новое название (непустое)", "string"),
			"folder":    prop("папка; пустая строка — без папки", "string"),
			"parent_id": prop("ID трека-родителя (существующий, не сам трек)", "integer"),
		}, "job_id"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			title, folder := optString(args, "title"), optString(args, "folder")
			_, hasParent := args["parent_id"]
			if title == nil && folder == nil && !hasParent {
				return "", fmt.Errorf("нечего менять: передай title, folder и/или parent_id")
			}
			id := argInt(args, "job_id")
			// сначала название/папка (воркер проверяет название): отказ не оставит трек уже перенесённым
			j, err := s.client.UpdateJob(context.Background(), id, title, folder)
			if err != nil {
				return "", err
			}
			if hasParent {
				if err := s.client.SetJobParent(context.Background(), id, argInt(args, "parent_id")); err != nil {
					return "", err
				}
			}
			where := "без папки"
			if j.Folder != "" {
				where = "папка «" + j.Folder + "»"
			}
			return fmt.Sprintf("#%d «%s», %s", j.ID, j.Title, where), nil
		},
	})

	s.Register(Tool{
		Name: "voice_convert",
		Description: "ЭКСПЕРИМЕНТ: голос узнаётся, но дрожит (свойство Seed-VC: не лечится шагами, дообучением, разделителем). " +
			"«Голос альбома»: новая версия трека — голос спет тембром образца (Seed-VC на воркере, " +
			"отдельная установка worker/seedvc_install.sh), музыка и мелодия прежние. Образец — окно дорожки " +
			"голоса другого трека: ref_job_id, ref_from (с; не задано — воркер сам берёт самый плотный кусок пения), " +
			"ref_dur (3–30 с, по умолчанию 25). Джоба идёт в очередь; дорожки делаются сами. Без Seed-VC — ошибка 503.",
		InputSchema: props(map[string]any{
			"job_id":     prop("ID трека, чей голос заменить", "integer"),
			"ref_job_id": prop("ID трека-образца (чей голос)", "integer"),
			"ref_from":   prop("начало окна образца, с (не задано — самый плотный кусок пения)", "number"),
			"ref_dur":    prop("длина окна образца, с (3–30, по умолчанию 25)", "number"),
			"steps":      prop("шаги диффузии (10–100, по умолчанию 50: меньше — быстрее, больше дрожи)", "integer"),
			"title":      prop("название версии", "string"),
		}, "job_id", "ref_job_id"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			// обязательность в схеме клиент MCP может не проверить — без id запрос ушёл бы с нулём
			if argInt(args, "job_id") <= 0 || argInt(args, "ref_job_id") <= 0 {
				return "", fmt.Errorf("нужны job_id (чей голос заменить) и ref_job_id (трек-образец)")
			}
			id, err := s.client.VoiceConvert(context.Background(), argInt(args, "job_id"), yue.VoiceParams{
				RefJobID: argInt(args, "ref_job_id"), RefFrom: argFloat(args, "ref_from"),
				RefDur: argFloat(args, "ref_dur"), Steps: int(argInt(args, "steps")), Title: argString(args, "title"),
			})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("джоба «голос» #%d в очереди (статус: jobs)", id), nil
		},
	})

	s.Register(Tool{
		Name: "voice_preview",
		Description: "ЭКСПЕРИМЕНТ «голос альбома», прослушать кусок: голос куска трека [from, to) (3–30 с) тембром " +
			"образца ref_job_id (Seed-VC, как voice_convert) → файл preview-vc-*.flac в папке трека; версии не создаёт. " +
			"Ответ — когда кусок готов (очередь видеокарты, минуты). Без Seed-VC — ошибка 503.",
		InputSchema: props(map[string]any{
			"job_id":     prop("ID трека, чей голос перевести", "integer"),
			"ref_job_id": prop("ID трека-образца (чей голос)", "integer"),
			"from":       prop("начало куска в треке, с", "number"),
			"to":         prop("конец куска в треке, с (кусок 3–30 с)", "number"),
			"ref_from":   prop("начало окна образца, с (не задано — самый плотный кусок пения)", "number"),
			"ref_dur":    prop("длина окна образца, с (3–30, по умолчанию 25)", "number"),
			"steps":      prop("шаги диффузии (10–100, по умолчанию 50)", "integer"),
		}, "job_id", "ref_job_id", "from", "to"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			// from может быть 0 — обязательность проверяем по наличию ключа
			_, hasFrom := args["from"]
			from, to := argFloat(args, "from"), argFloat(args, "to")
			if argInt(args, "job_id") <= 0 || argInt(args, "ref_job_id") <= 0 || !hasFrom || to <= from {
				return "", fmt.Errorf("нужны job_id, ref_job_id и кусок from < to")
			}
			r, err := s.client.VoicePreview(context.Background(), argInt(args, "job_id"), yue.VoicePreviewParams{
				RefJobID: argInt(args, "ref_job_id"), From: from, To: to, RefFrom: argFloat(args, "ref_from"),
				RefDur: argFloat(args, "ref_dur"), Steps: int(argInt(args, "steps")),
			})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("кусок %.1f–%.1f с: файл %s в папке трека #%d", r.From, r.To, r.File, argInt(args, "job_id")), nil
		},
	})

	s.Register(Tool{
		Name: "overdub",
		Description: "Овердаб: партия поверх трека джобы по его партитуре с новым стилем + микс (gain 0.1–1). " +
			"lyrics — текст голосовой партии: без него модель импровизирует вокализ (часто несуразный).",
		InputSchema: props(map[string]any{
			"job_id": prop("ID джобы", "integer"),
			"style":  prop("строка стиля партии (англ. теги)", "string"),
			"lyrics": prop("текст голосовой партии (пусто = вокализ; [Instrumental] = без голоса)", "string"),
			"gain":   prop("гейн микса 0.1–1 (не задан — 0.5); 0 — кавер новым треком без смешивания с оригиналом", "number"),
			"abc":    prop("свой план партии (пусто = партитура джобы); инструмент-только-в-куске строит soloInstrumentPlan", "string"),
			"seed":   prop("сид родителя = «та же интерпретация», ближе к оригиналу (0 = новая)", "integer"),
		}, "job_id", "style"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			// не задан — 0,5; явный 0 — кавер новым треком без смешивания с оригиналом (как «переиграть» в приложении)
			gain := 0.5
			if _, ok := args["gain"]; ok {
				gain = argFloat(args, "gain")
			}
			id, err := s.client.SubmitOverdub(context.Background(), argInt(args, "job_id"),
				argString(args, "style"), argString(args, "lyrics"), gain, argString(args, "abc"),
				argInt(args, "seed"))
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("джоба-овердаб #%d в очереди", id), nil
		},
	})

	s.Register(Tool{
		Name: "job_lyrics",
		Description: "Распознать текст из готового аудио джобы (faster-whisper): без повторной загрузки файла — для овердаба/кавера этой же джобы. " +
			"Есть дорожка голоса (make_stems) — распознаётся она, точнее, чем микс с гитарами.",
		InputSchema: props(map[string]any{
			"job_id":   prop("ID джобы (статус done)", "integer"),
			"language": prop("язык пения: en, ru, …; auto — определить самому; пусто — по стилю трека («English, …» → en)", "string"),
		}, "job_id"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			out, err := s.client.JobLyrics(context.Background(), argInt(args, "job_id"), argString(args, "language"))
			if err != nil {
				return "", err
			}
			return toJSON(out), nil
		},
	})

	s.Register(Tool{
		Name: "import_track",
		Description: "Импорт своего трека как джобы (дальше работают стемы/минус/эффекты/овердаб). " +
			"transcribe=true — сразу транскрипция для ролла (медленно).",
		InputSchema: props(map[string]any{
			"path":       prop("путь к аудиофайлу (flac/mp3/wav/ogg/m4a)", "string"),
			"transcribe": prop("сделать транскрипцию (для пиано-ролла/овердаба)", "boolean"),
			"title":      prop("название трека (пусто — имя файла)", "string"),
			"folder":     prop("папка («Альбом», «Основы», …; пусто — без папки)", "string"),
		}, "path"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			data, err := os.ReadFile(argString(args, "path"))
			if err != nil {
				return "", err
			}
			if len(data) > 200<<20 {
				return "", fmt.Errorf("файл слишком большой (>200 МБ)")
			}
			out, err := s.client.ImportTrack(context.Background(), filepath.Base(argString(args, "path")), data, argBool(args, "transcribe"))
			if err != nil {
				return "", err
			}
			// подпись и папка — отдельной правкой: у импорта на воркере их нет
			title, folder := nonEmpty(argString(args, "title")), nonEmpty(argString(args, "folder"))
			if id, ok := out["id"].(float64); ok && (title != nil || folder != nil) {
				j, err := s.client.UpdateJob(context.Background(), int64(id), title, folder)
				if err != nil {
					return "", fmt.Errorf("трек #%d импортирован, но подпись/папка не сохранились: %w", int64(id), err)
				}
				out["title"], out["folder"] = j.Title, j.Folder
			}
			return toJSON(out), nil
		},
	})

	s.Register(Tool{
		Name: "transcribe_job",
		Description: "Партитура для готового трека без неё (импорт без транскрипции или с упавшей, " +
			"DSP-вариант): SheetSage2 по треку → score.abc, дальше работают ролл/job_score/овердаб. " +
			"Готовый план не перезаписывается (409). Медленно, ждёт очередь к GPU.",
		InputSchema: props(map[string]any{
			"job_id": prop("ID джобы", "integer"),
		}, "job_id"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			out, err := s.client.TranscribeJob(context.Background(), argInt(args, "job_id"))
			if err != nil {
				return "", err
			}
			return toJSON(out), nil
		},
	})

	s.Register(Tool{
		Name: "voices_list",
		Description: "Карточки голосов примерочной: ручки характера (params), seed прослушивания, " +
			"жива ли исходная джоба (job_alive).",
		InputSchema: props(nil),
		Handler: func(s *Server, _ map[string]any) (string, error) {
			out, err := s.client.Voices(context.Background())
			if err != nil {
				return "", err
			}
			return toJSON(out), nil
		},
	})

	s.Register(Tool{
		Name: "voice_create",
		Description: "Сохранить карточку голоса из джобы-прослушивания (draft-джоба примерочной). " +
			"params — JSON ручек: {register, rough, creak, delivery, breath, extra}.",
		InputSchema: props(map[string]any{
			"name":   prop("имя голоса", "string"),
			"job_id": prop("ID готовой джобы-прослушивания", "integer"),
			"params": prop("JSON ручек примерочной", "string"),
			"seed":   prop("seed прослушивания (для воспроизводимости голоса)", "integer"),
		}, "name", "job_id"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			id, err := s.client.VoiceCreate(context.Background(),
				argString(args, "name"), argInt(args, "job_id"),
				argString(args, "params"), argInt(args, "seed"))
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("голос #%d сохранён (джоба #%d)", id, argInt(args, "job_id")), nil
		},
	})

	s.Register(Tool{
		Name: "voice_delete",
		Description: "Удалить карточку голоса и копию аудио на сервере. Необратимо. " +
			"Деструктивное: требует confirm=true (спроси пользователя).",
		InputSchema: props(map[string]any{
			"voice_id": prop("ID карточки голоса", "integer"),
			"confirm":  prop("явное подтверждение пользователя", "boolean"),
		}, "voice_id", "confirm"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			if !argBool(args, "confirm") {
				return "", errConfirm
			}
			ok, err := s.client.VoiceDelete(context.Background(), argInt(args, "voice_id"))
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("deleted=%v", ok), nil
		},
	})
}

// registerDraftTools — черновик студии (незаконченная работа над треком): правки «Правок трека» и ожидающие
// рендеры, как их ведёт приложение. Применить черновик агенту — rebuild_sections + variant_track.
func registerDraftTools(s *Server) {
	s.Register(Tool{
		Name: "draft_list",
		Description: "Черновики студии (незаконченная работа) всех треков: [{job_id, state, updated_at}]. " +
			"state — {v, edits (записи «Правок трека»), queue (ожидающие рендеры кусков), ts, gen?, applying?}: " +
			"gen — изменения с рендером {specs (приёмы плана), voice {params, name}, instrumental}, " +
			"applying — ход «применить всё» ({job_id, title} — ждём рендер, {finalize, title} — итог новой версии, {error, title}).",
		InputSchema: props(nil),
		Handler: func(s *Server, _ map[string]any) (string, error) {
			list, err := s.client.Drafts(context.Background())
			if err != nil {
				return "", err
			}
			if list == nil {
				list = []yue.Draft{}
			}
			return toJSON(list), nil
		},
	})
	s.Register(Tool{
		Name:        "draft_get",
		Description: "Черновик студии трека: {job_id, state, updated_at}; черновика нет — «no draft».",
		InputSchema: props(map[string]any{"job_id": prop("ID трека", "integer")}, "job_id"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			d, err := s.client.Draft(context.Background(), argInt(args, "job_id"))
			if err != nil {
				return "", err
			}
			if d == nil {
				return "no draft", nil
			}
			return toJSON(d), nil
		},
	})
	s.Register(Tool{
		Name: "draft_set",
		Description: "Записать черновик студии трека целиком (state — JSON-объект {v, edits, queue, ts, gen?, applying?}, " +
			"см. draft_list). Приложение подхватит его при следующем запуске; прежний черновик заменяется — " +
			"не затирайте applying, пока идёт «применить всё» (ход потеряется).",
		InputSchema: props(map[string]any{
			"job_id": prop("ID трека", "integer"),
			"state":  prop("состояние черновика — JSON-объект", "object"),
		}, "job_id", "state"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			st, ok := args["state"].(map[string]any)
			if !ok {
				return "", errors.New("state — JSON-объект {v, edits, queue, ts}")
			}
			b, err := json.Marshal(st)
			if err != nil {
				return "", err
			}
			if err := s.client.DraftPut(context.Background(), argInt(args, "job_id"), b); err != nil {
				return "", err
			}
			return "saved", nil
		},
	})
	s.Register(Tool{
		Name: "draft_clear",
		Description: "Удалить черновик студии трека — его неприменённые правки пропадут (сам трек не меняется). " +
			"Деструктивное: требует confirm=true (спроси пользователя).",
		InputSchema: props(map[string]any{
			"job_id":  prop("ID трека", "integer"),
			"confirm": prop("явное подтверждение пользователя", "boolean"),
		}, "job_id", "confirm"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			if !argBool(args, "confirm") {
				return "", errConfirm
			}
			ok, err := s.client.DraftDelete(context.Background(), argInt(args, "job_id"))
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("deleted=%v", ok), nil
		},
	})
}

// applyDsp — конвейер как в приложении (app.go): скачать flac → ffmpeg → залить вариант.
func (s *Server) applyDsp(jobID int64, chainID string, params map[string]float64) (*yue.DspVariant, error) {
	return studio.ApplyChain(context.Background(), s.client, jobID, chainID, params)
}

// applySteps — цепочка эффектов по порядку на весь трек: вариант dsp-pedals.flac.
func (s *Server) applySteps(jobID int64, steps []dsp.Step) (*yue.DspVariant, error) {
	return studio.ApplySteps(context.Background(), s.client, jobID, steps, "")
}

// modelHint — модель генерации человеческой строкой: выбранная и рекомендуемая для карты воркера
// (cpp_recommended; у старого воркера поля нет — "").
func modelHint(cfg map[string]any) string {
	rec, ok := cfg["cpp_recommended"].(map[string]any)
	if !ok {
		return ""
	}
	name := func(m any, low any) string {
		s, _ := m.(string)
		if b, _ := low.(bool); b {
			return strings.ToUpper(s) + " (экономный режим)"
		}
		return strings.ToUpper(s)
	}
	card := "видеопамять неизвестна"
	if mib, _ := rec["vram_mib"].(float64); mib > 0 { // числа JSON — float64; карта «12 ГБ» сообщает 12282 МиБ
		card = fmt.Sprintf("карта %.0f ГБ", mib/1024)
	}
	return fmt.Sprintf("выбрана %s; рекомендуемая для этой машины (%s) — %s",
		name(cfg["cpp_model"], cfg["cpp_low_vram"]), card, name(rec["model"], rec["low_vram"]))
}
