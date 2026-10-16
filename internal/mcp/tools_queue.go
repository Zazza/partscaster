package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"partscaster/internal/yue"
)

// WorkerClient — клиент воркера для MCP: источник «mcp» (в общей очереди — пометка «ИИ»),
// журнал правок (изменяющие вызовы API пишутся после успеха).
func WorkerClient(url string) yue.Service {
	c := yue.New(url)
	c.SetSource("mcp")
	return yue.Journaled(c)
}

// queueView — очередь воркера глазами инструментов (yue.Client и обёртка журнала).
type queueView interface {
	QueueList(ctx context.Context) (*yue.QueueSnapshot, error)
	QueueMove(ctx context.Context, id int64, dir int) error
	QueueCancel(ctx context.Context, id int64) error
	QueuePause(ctx context.Context, paused bool) error
}

func (s *Server) queue() (queueView, error) {
	q, ok := s.client.(queueView)
	if !ok {
		return nil, yue.ErrNoQueue
	}
	return q, nil
}

// registerQueueTools — общая очередь долгих операций воркера: то же, что видит человек
// в левом блоке приложения (генерации, пресеты, стемы, эффекты — от приложения и от ИИ).
func registerQueueTools(s *Server) {
	s.Register(Tool{
		Name: "queue_list",
		Description: "Общая очередь воркера: идущая операция, ждущие по порядку, последние завершённые; пауза. " +
			"Запись: id (для queue_move/queue_cancel), kind (generate — генерация, preset — пресет звука, rebuild — " +
			"пересборка, stems, fx, transcribe, whisper, voice, plan, preview), title, job_id (трек), source " +
			"(app — человек в приложении, mcp — ИИ), state (waiting | running | done | error | canceled | " +
			"interrupted), result_job_id (итог — версия трека), error; ход операции клиента (пресет): stage (prepare | parts | " +
			"levels | render | verify | fix), pass (номер сборки), eta_s (оценка оставшегося, с; null — ещё нет), " +
			"stage_since (с с начала шага; null — хода нет). Долгие инструменты (sound_preset_apply, " +
			"rebuild_sections, make_stems, fx_apply…) встают в эту очередь и ждут своей очереди; человек видит их " +
			"с пометкой «ИИ» и может отменить — тогда инструмент вернёт ошибку «отменено человеком».",
		InputSchema: props(map[string]any{}),
		Handler: func(s *Server, args map[string]any) (string, error) {
			q, err := s.queue()
			if err != nil {
				return "", err
			}
			snap, err := q.QueueList(context.Background())
			if err != nil {
				return "", err
			}
			b, err := json.Marshal(snap)
			return string(b), err
		},
	})

	s.Register(Tool{
		Name:        "queue_move",
		Description: "Подвинуть ждущую запись очереди: dir −1 — на место раньше, 1 — позже. Идущую и завершённые не двигают.",
		InputSchema: props(map[string]any{
			"id":  prop("id записи из queue_list", "integer"),
			"dir": prop("−1 — раньше, 1 — позже", "integer"),
		}, "id", "dir"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			id, dir := argInt(args, "id"), argInt(args, "dir")
			if id <= 0 || (dir != -1 && dir != 1) {
				return "", fmt.Errorf("нужны id записи и dir −1 или 1")
			}
			q, err := s.queue()
			if err != nil {
				return "", err
			}
			if err := q.QueueMove(context.Background(), id, int(dir)); err != nil {
				return "", err
			}
			return fmt.Sprintf("moved id=%d dir=%d", id, dir), nil
		},
	})

	s.Register(Tool{
		Name: "queue_cancel",
		Description: "Отменить запись очереди: ждущую — снять, идущую — остановить (внешние процессы — сразу, " +
			"шаг внутри воркера — по окончании шага). Деструктивное: требует confirm=true (спроси пользователя).",
		InputSchema: props(map[string]any{
			"id":      prop("id записи из queue_list", "integer"),
			"confirm": prop("явное подтверждение пользователя", "boolean"),
		}, "id", "confirm"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			if !argBool(args, "confirm") {
				return "", errConfirm
			}
			id := argInt(args, "id")
			if id <= 0 {
				return "", fmt.Errorf("нужен id записи из queue_list")
			}
			q, err := s.queue()
			if err != nil {
				return "", err
			}
			if err := q.QueueCancel(context.Background(), id); err != nil {
				return "", err
			}
			return fmt.Sprintf("canceled id=%d", id), nil
		},
	})

	s.Register(Tool{
		Name:        "queue_pause",
		Description: "Пауза общей очереди: paused=true — новое не начинается (идущая доделывается), false — снять паузу.",
		InputSchema: props(map[string]any{
			"paused": prop("true — пауза, false — продолжить", "boolean"),
		}, "paused"),
		Handler: func(s *Server, args map[string]any) (string, error) {
			q, err := s.queue()
			if err != nil {
				return "", err
			}
			p := argBool(args, "paused")
			if err := q.QueuePause(context.Background(), p); err != nil {
				return "", err
			}
			return fmt.Sprintf("paused=%v", p), nil
		},
	})
}
