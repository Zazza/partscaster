package yue

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

// QueueOp — запись общей очереди воркера: генерация или долгая операция (стемы, эффекты,
// пресет звука…). State: waiting | running | done | error | canceled | interrupted.
type QueueOp struct {
	ID              int64  `json:"id"`
	Kind            string `json:"kind"` // generate | preset | rebuild | stems | fx | render | whisper…
	Title           string `json:"title"`
	JobID           int64  `json:"job_id"`
	Source          string `json:"source"` // app | mcp | other
	State           string `json:"state"`
	Pos             int64  `json:"pos"`
	ResultJobID     int64  `json:"result_job_id"`
	Error           string `json:"error"`
	CancelRequested bool   `json:"cancel_requested"`
	Client          bool   `json:"client"` // операция клиента (цепочка приложения/MCP), живёт на отметках
	CreatedAt       string `json:"created_at"`
	StartedAt       string `json:"started_at"`
	FinishedAt      string `json:"finished_at"`
	// ход операции клиента (бит с телом): шаг prepare | parts | levels | render | verify | fix, номер сборки,
	// оценка оставшегося (nil — оценки ещё нет), сек с начала шага (nil — хода нет)
	Stage      string   `json:"stage"`
	Pass       int      `json:"pass"`
	EtaS       *float64 `json:"eta_s"`
	StageSince *float64 `json:"stage_since"`
}

// QueueProgress — ход операции клиента для бита: шаг, номер сборки (≥ 1), оценка оставшегося в секундах
// (nil — оценки ещё нет; 0 — значение «вот-вот готово»).
type QueueProgress struct {
	Stage string   `json:"stage"`
	Pass  int      `json:"pass"`
	EtaS  *float64 `json:"eta_s"`
}

// QueueSnapshot — очередь целиком: идущая, ждущие по месту, последние завершённые.
type QueueSnapshot struct {
	Paused bool      `json:"paused"`
	Items  []QueueOp `json:"items"`
}

// QueueOpIn — новая операция клиента.
type QueueOpIn struct {
	Kind  string `json:"kind"`
	Title string `json:"title"`
	JobID int64  `json:"job_id"`
}

// QueueFinishIn — итог операции клиента: done (+ id версии) | error (+ текст) | canceled.
type QueueFinishIn struct {
	State       string `json:"state"`
	ResultJobID int64  `json:"result_job_id"`
	Error       string `json:"error"`
}

// Queuer — общая очередь долгих операций воркера (генерации, стемы, эффекты, цепочки
// приложения и ИИ). Отдельно от Service: фейки сервиса в тестах и старые обёртки очереди
// не знают — цепочка без очереди идёт сразу, как раньше (см. studio.RunOp).
type Queuer interface {
	QueueList(ctx context.Context) (*QueueSnapshot, error)
	QueueAdd(ctx context.Context, in QueueOpIn) (*QueueOp, error)
	// QueueBeat — отметка «жива»; с ходом (первый из progress) — ещё и шаг операции.
	QueueBeat(ctx context.Context, id int64, progress ...QueueProgress) (*QueueOp, error)
	QueueFinish(ctx context.Context, id int64, in QueueFinishIn) (*QueueOp, error)
	QueueMove(ctx context.Context, id int64, dir int) error
	QueueCancel(ctx context.Context, id int64) error
	QueuePause(ctx context.Context, paused bool) error
}

var _ Queuer = (*Client)(nil)

// ErrNoQueue — у сервиса нет очереди (фейк, обёртка над сервисом без очереди).
var ErrNoQueue = errors.New("очередь воркера недоступна")

type opKey struct{}

// WithOp — запросы с этим контекстом идут внутри операции очереди id (шаги цепочки
// не встают в очередь повторно).
func WithOp(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, opKey{}, id)
}

// OpFrom — операция очереди контекста; 0 — нет.
func OpFrom(ctx context.Context) int64 {
	id, _ := ctx.Value(opKey{}).(int64)
	return id
}

// SetSource — кто ходит к воркеру (app | mcp): заголовок каждого запроса, в очереди — пометка «ИИ».
func (c *Client) SetSource(s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.source = s
}

// queueHeaders — заголовки очереди в запросе: источник клиента и операция из контекста.
func (c *Client) queueHeaders(ctx context.Context, r *http.Request) {
	c.mu.RLock()
	src := c.source
	c.mu.RUnlock()
	if src != "" {
		r.Header.Set("X-Partscaster-Source", src)
	}
	if id := OpFrom(ctx); id != 0 {
		r.Header.Set("X-Partscaster-Op", strconv.FormatInt(id, 10))
	}
}

func (c *Client) QueueList(ctx context.Context) (*QueueSnapshot, error) {
	var out QueueSnapshot
	if err := c.get(ctx, "/queue", &out); err != nil {
		return nil, err
	}
	if out.Items == nil {
		out.Items = []QueueOp{}
	}
	return &out, nil
}

func (c *Client) QueueAdd(ctx context.Context, in QueueOpIn) (*QueueOp, error) {
	var out QueueOp
	if err := c.postJSON(ctx, "/queue/ops", in, requestTimeout, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) QueueBeat(ctx context.Context, id int64, progress ...QueueProgress) (*QueueOp, error) {
	var out QueueOp
	path := fmt.Sprintf("/queue/ops/%d/beat", id)
	var err error
	if len(progress) > 0 {
		err = c.postJSON(ctx, path, progress[0], requestTimeout, &out)
	} else {
		err = c.post(ctx, path, requestTimeout, &out) // прежний бит: без тела, ход воркер оставляет как был
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) QueueFinish(ctx context.Context, id int64, in QueueFinishIn) (*QueueOp, error) {
	var out QueueOp
	if err := c.postJSON(ctx, fmt.Sprintf("/queue/ops/%d/finish", id), in, requestTimeout, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// QueueMove — ждущую запись на место раньше (dir −1) или позже (+1); край — без ошибки.
func (c *Client) QueueMove(ctx context.Context, id int64, dir int) error {
	return c.postJSON(ctx, fmt.Sprintf("/queue/ops/%d/move", id), map[string]int{"dir": dir}, requestTimeout, nil)
}

// QueueCancel — ждущая снимается сразу, идущая останавливается (шаг без точки прерывания доделывается).
func (c *Client) QueueCancel(ctx context.Context, id int64) error {
	return c.post(ctx, fmt.Sprintf("/queue/ops/%d/cancel", id), requestTimeout, nil)
}

// QueuePause — пауза очереди: новое не начинается, идущая доделывается.
func (c *Client) QueuePause(ctx context.Context, paused bool) error {
	return c.postJSON(ctx, "/queue/pause", map[string]bool{"paused": paused}, requestTimeout, nil)
}

// очередь через обёртку журнала: вызовы очереди в журнал правок не пишутся

func (j *journaled) queuer() (Queuer, error) {
	q, ok := j.Service.(Queuer)
	if !ok {
		return nil, ErrNoQueue
	}
	return q, nil
}

func (j *journaled) QueueList(ctx context.Context) (*QueueSnapshot, error) {
	q, err := j.queuer()
	if err != nil {
		return nil, err
	}
	return q.QueueList(ctx)
}

func (j *journaled) QueueAdd(ctx context.Context, in QueueOpIn) (*QueueOp, error) {
	q, err := j.queuer()
	if err != nil {
		return nil, err
	}
	return q.QueueAdd(ctx, in)
}

func (j *journaled) QueueBeat(ctx context.Context, id int64, progress ...QueueProgress) (*QueueOp, error) {
	q, err := j.queuer()
	if err != nil {
		return nil, err
	}
	return q.QueueBeat(ctx, id, progress...)
}

func (j *journaled) QueueFinish(ctx context.Context, id int64, in QueueFinishIn) (*QueueOp, error) {
	q, err := j.queuer()
	if err != nil {
		return nil, err
	}
	return q.QueueFinish(ctx, id, in)
}

func (j *journaled) QueueMove(ctx context.Context, id int64, dir int) error {
	q, err := j.queuer()
	if err != nil {
		return err
	}
	return q.QueueMove(ctx, id, dir)
}

func (j *journaled) QueueCancel(ctx context.Context, id int64) error {
	q, err := j.queuer()
	if err != nil {
		return err
	}
	return q.QueueCancel(ctx, id)
}

func (j *journaled) QueuePause(ctx context.Context, paused bool) error {
	q, err := j.queuer()
	if err != nil {
		return err
	}
	return q.QueuePause(ctx, paused)
}
