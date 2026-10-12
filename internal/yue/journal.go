package yue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
)

// Журнал правок трека (рецепт): что сделано с треком — чтобы на другой установке
// повторить. Пишет только эта сторона (приложение и MCP): правки студии считаются здесь, воркер видит
// лишь готовые файлы. Прямые вызовы API пишет обёртка Journaled, составные операции studio — Record
// одной записью, глуша журнал своих внутренних вызовов (WithoutJournal).

// JobOp — запись журнала: операция, её параметры как в API, итог (файл варианта или "job:<id>") и
// треки, на которые ссылаются параметры.
type JobOp struct {
	Op     string          `json:"op"`
	Params json.RawMessage `json:"params"`
	Result string          `json:"result"`
	Refs   []int64         `json:"refs"`
}

// OpRecorder — сервис, который умеет записать операцию в журнал воркера (Client и обёртка Journaled).
type OpRecorder interface {
	RecordOp(ctx context.Context, jobID int64, op JobOp) error
}

// RecordOp — POST /jobs/{id}/ops.
func (c *Client) RecordOp(ctx context.Context, jobID int64, op JobOp) error {
	if op.Params == nil {
		op.Params = json.RawMessage("{}")
	}
	if op.Refs == nil {
		op.Refs = []int64{}
	}
	return c.postJSON(ctx, fmt.Sprintf("/jobs/%d/ops", jobID), op, requestTimeout, nil)
}

type noJournalKey struct{}

// WithoutJournal — вызовы с этим контекстом не журналируются (внутренние шаги составной операции).
func WithoutJournal(ctx context.Context) context.Context {
	if ctx == nil { // приложение до startup (тесты биндингов) — контекста ещё нет
		ctx = context.Background()
	}
	return context.WithValue(ctx, noJournalKey{}, true)
}

func journalOff(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	off, _ := ctx.Value(noJournalKey{}).(bool)
	return off
}

// Record — записать операцию, если сервис умеет и контекст не «без журнала». Сбой записи не роняет
// правку: она уже сделана, теряется только её повторяемость (в лог).
func Record(ctx context.Context, svc Service, jobID int64, op JobOp) {
	r, ok := svc.(OpRecorder)
	if !ok || journalOff(ctx) {
		return
	}
	// своя отсрочка: правка могла занять минуты и съесть срок исходного контекста
	if ctx == nil {
		ctx = context.Background()
	}
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), requestTimeout)
	defer cancel()
	if err := r.RecordOp(rctx, jobID, op); err != nil {
		log.Printf("журнал правок: #%d %s: %v", jobID, op.Op, err)
	}
}

// Params — параметры операции как JSON (ошибка маршалинга невозможна для типов этого пакета — пусто).
func Params(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return b
}

func JobResult(id int64) string { return fmt.Sprintf("job:%d", id) }

// journaled — Service, который пишет прямые изменяющие вызовы в журнал после их успеха.
type journaled struct {
	Service
}

// Journaled оборачивает сервис журналом (приложение и MCP); повторная обёртка — тот же сервис.
func Journaled(svc Service) Service {
	if _, ok := svc.(*journaled); ok {
		return svc
	}
	return &journaled{Service: svc}
}

func (j *journaled) RecordOp(ctx context.Context, jobID int64, op JobOp) error {
	r, ok := j.Service.(OpRecorder)
	if !ok {
		return nil
	}
	return r.RecordOp(ctx, jobID, op)
}

func (j *journaled) record(ctx context.Context, jobID int64, op string, params any, result string, refs ...int64) {
	Record(ctx, j, jobID, JobOp{Op: op, Params: Params(params), Result: result, Refs: refs})
}

// actualSeed — сид новой джобы, как его записал воркер (0 в заявке — воркер выбрал сам или взял у родителя):
// без него импорт сыграл бы другое исполнение.
func (j *journaled) actualSeed(ctx context.Context, id, asked int64) int64 {
	if asked != 0 || journalOff(ctx) {
		return asked
	}
	jobs, err := j.Jobs(ctx)
	if err != nil {
		return asked
	}
	for _, x := range jobs {
		if x.ID == id {
			return x.Seed
		}
	}
	return asked
}

func (j *journaled) Submit(ctx context.Context, p SubmitParams) (int64, error) {
	id, err := j.Service.Submit(ctx, p)
	if err != nil || p.ParentID == 0 {
		return id, err // корень песни — в столбцах трека, журнал не нужен
	}
	p.Seed = j.actualSeed(ctx, id, p.Seed)
	j.record(ctx, p.ParentID, "submit", p, JobResult(id))
	return id, nil
}

func (j *journaled) ContinueJob(ctx context.Context, jobID int64, fromSec float64, seed int64, abc, styleAdd string) (int64, error) {
	id, err := j.Service.ContinueJob(ctx, jobID, fromSec, seed, abc, styleAdd)
	if err != nil {
		return id, err
	}
	j.record(ctx, jobID, "continue", map[string]any{"from_sec": fromSec, "seed": j.actualSeed(ctx, id, seed),
		"abc": abc, "style_add": styleAdd}, JobResult(id))
	return id, nil
}

func (j *journaled) FragmentJob(ctx context.Context, jobID int64, fromSec, toSec float64, seed int64, abc, styleAdd string) (int64, error) {
	id, err := j.Service.FragmentJob(ctx, jobID, fromSec, toSec, seed, abc, styleAdd)
	if err != nil {
		return id, err
	}
	j.record(ctx, jobID, "fragment", map[string]any{"from_sec": fromSec, "to_sec": toSec,
		"seed": j.actualSeed(ctx, id, seed), "abc": abc, "style_add": styleAdd}, JobResult(id))
	return id, nil
}

func (j *journaled) SubmitOverdub(ctx context.Context, id int64, style, lyrics string, gain float64, abc string, seed int64) (int64, error) {
	nid, err := j.Service.SubmitOverdub(ctx, id, style, lyrics, gain, abc, seed)
	if err != nil {
		return nid, err
	}
	j.record(ctx, id, "overdub", map[string]any{"style": style, "lyrics": lyrics, "gain": gain, "abc": abc,
		"seed": j.actualSeed(ctx, nid, seed)}, JobResult(nid))
	return nid, nil
}

func (j *journaled) VariantToTrack(ctx context.Context, jobID int64, file, title string, voiceSrc int64) (int64, error) {
	id, err := j.Service.VariantToTrack(ctx, jobID, file, title, voiceSrc)
	if err != nil {
		return id, err
	}
	var refs []int64
	if voiceSrc != 0 {
		refs = append(refs, voiceSrc)
	}
	j.record(ctx, jobID, "variant_track", map[string]any{"file": file, "title": title, "voice_src": voiceSrc},
		JobResult(id), refs...)
	return id, nil
}

func (j *journaled) ApplyFx(ctx context.Context, id int64, req FxRequest) (*DspVariant, error) {
	v, err := j.Service.ApplyFx(ctx, id, req)
	if err != nil || req.Preview || v == nil {
		return v, err
	}
	j.record(ctx, id, "fx", req, v.File)
	return v, nil
}

func (j *journaled) MakeMinus(ctx context.Context, id int64, exclude []string) (map[string]any, error) {
	out, err := j.Service.MakeMinus(ctx, id, exclude)
	if err != nil {
		return out, err
	}
	file, _ := out["file"].(string)
	j.record(ctx, id, "minus", map[string]any{"exclude": exclude}, file)
	return out, nil
}

func (j *journaled) UpdateJob(ctx context.Context, jobID int64, title, folder *string) (*Job, error) {
	job, err := j.Service.UpdateJob(ctx, jobID, title, folder)
	if err != nil {
		return job, err
	}
	p := map[string]any{}
	if title != nil {
		p["title"] = *title
	}
	if folder != nil {
		p["folder"] = *folder
	}
	j.record(ctx, jobID, "update", p, "")
	return job, nil
}

func (j *journaled) SetHead(ctx context.Context, jobID, headID int64) error {
	if err := j.Service.SetHead(ctx, jobID, headID); err != nil {
		return err
	}
	var refs []int64
	if headID != 0 {
		refs = append(refs, headID)
	}
	j.record(ctx, jobID, "set_head", map[string]any{"head_id": headID}, "", refs...)
	return nil
}
