package studio

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"partscaster/internal/yue"
)

// Операция клиента в общей очереди воркера: цепочка (пресет звука, пересборка) занимает
// одно место в очереди целиком — ждёт своей очереди, отмечается «жива», все её вызовы
// воркера идут с меткой операции (не встают в очередь повторно), закрывается итогом.

var (
	// ErrCanceled — операцию отменили (в приложении или инструментом очереди).
	ErrCanceled = errors.New("отменено человеком")
	// ErrInterrupted — воркер операцию больше не знает: перезапущен или клиент долго не отмечался.
	ErrInterrupted = errors.New("операция прервана: воркер её больше не знает (перезапуск или потеря связи)")
)

// opQueue — то, что нужно операции от очереди воркера (yue.Client, обёртка журнала).
type opQueue interface {
	QueueAdd(ctx context.Context, in yue.QueueOpIn) (*yue.QueueOp, error)
	QueueBeat(ctx context.Context, id int64) (*yue.QueueOp, error)
	QueueFinish(ctx context.Context, id int64, in yue.QueueFinishIn) (*yue.QueueOp, error)
}

// undoKey — откаты операции: что убрать, если операция кончится отменой или прерыванием.
type undoKey struct{}

type undoList struct {
	mu   sync.Mutex
	fns  []func(context.Context)
	done []func(context.Context) // AfterDone: только после итога «готово»
}

// OnCanceled — откат на случай, если операция очереди, внутри которой идёт ctx, кончится отменой или
// прерыванием (в том числе после того, как шаг уже вернул успех: отмена бывает принята в последний миг).
// Вне операции — ничего: отменять некому.
func OnCanceled(ctx context.Context, fn func(context.Context)) {
	if u, ok := ctx.Value(undoKey{}).(*undoList); ok {
		u.mu.Lock()
		u.fns = append(u.fns, fn)
		u.mu.Unlock()
	}
}

// AfterDone — fn после того, как операция очереди, внутри которой идёт ctx, закрыта итогом «готово»
// (отменённая или прерванная — fn не вызывается). Вне операции — сразу.
func AfterDone(ctx context.Context, fn func(context.Context)) {
	u, ok := ctx.Value(undoKey{}).(*undoList)
	if !ok {
		fn(ctx)
		return
	}
	u.mu.Lock()
	u.done = append(u.done, fn)
	u.mu.Unlock()
}

func (u *undoList) runDone(ctx context.Context) {
	u.mu.Lock()
	fns := u.done
	u.done = nil
	u.mu.Unlock()
	dctx := context.WithoutCancel(ctx)
	for _, fn := range fns {
		fn(dctx)
	}
}

func (u *undoList) run(ctx context.Context) {
	u.mu.Lock()
	fns := u.fns
	u.fns = nil
	u.mu.Unlock()
	if len(fns) == 0 {
		return
	}
	uctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	for i := len(fns) - 1; i >= 0; i-- {
		fns[i](uctx)
	}
}

// opPoll — как часто клиент спрашивает воркер об операции (очередь, метка отмены, «жива»);
// порог «прервано» на воркере — минуты, отметка раз в секунды его не задевает.
var opPoll = time.Second

// opBeatFails — сколько опросов подряд без ответа воркера терпим, пока операция ждёт (связь моргнула).
const opBeatFails = 60

// RunOp — fn как одна операция очереди воркера (kind — вид: preset, rebuild…; title — что показать;
// jobID — трек). ctx уже внутри операции — fn сразу (вложенная цепочка). У сервиса нет очереди
// (фейк, старый воркер без /queue) — fn сразу, как раньше. Итог: id версии от fn; отмена —
// ErrCanceled, пропажа операции — ErrInterrupted.
func RunOp(ctx context.Context, svc yue.Service, kind, title string, jobID int64,
	fn func(context.Context) (int64, error)) (int64, error) {
	if yue.OpFrom(ctx) != 0 {
		return fn(ctx)
	}
	q, ok := svc.(opQueue)
	if !ok {
		return fn(ctx)
	}
	op, err := q.QueueAdd(ctx, yue.QueueOpIn{Kind: kind, Title: title, JobID: jobID})
	if noQueue(err) {
		return fn(ctx)
	}
	if err != nil {
		return 0, err
	}
	return runOp(ctx, q, op, fn)
}

// StartOp — то же в фоне: операция регистрируется сразу (id — для слежения в интерфейсе),
// ожидание очереди и работа — в горутине. Без очереди у сервиса — ошибка (некуда поставить).
func StartOp(ctx context.Context, svc yue.Service, kind, title string, jobID int64,
	fn func(context.Context) (int64, error)) (*yue.QueueOp, error) {
	q, ok := svc.(opQueue)
	if !ok {
		return nil, yue.ErrNoQueue
	}
	op, err := q.QueueAdd(ctx, yue.QueueOpIn{Kind: kind, Title: title, JobID: jobID})
	if err != nil {
		return nil, err
	}
	go func() {
		if _, err := runOp(ctx, q, op, fn); err != nil {
			log.Printf("очередь: операция %d (%s «%s»): %v", op.ID, kind, title, err)
		}
	}()
	return op, nil
}

// noQueue — воркер без общей очереди (старая версия: 404 на /queue/ops) или сервис без неё.
func noQueue(err error) bool {
	if errors.Is(err, yue.ErrNoQueue) {
		return true
	}
	var se *yue.StatusError
	return errors.As(err, &se) && se.Code == http.StatusNotFound
}

func statusCode(err error) int {
	var se *yue.StatusError
	if errors.As(err, &se) {
		return se.Code
	}
	return 0
}

// beatVerdict — что ответ воркера значит для операции: nil — продолжаем; ErrCanceled / ErrInterrupted.
func beatVerdict(cur *yue.QueueOp, err error) error {
	switch {
	case err == nil && (cur.CancelRequested || cur.State == "canceled"):
		return ErrCanceled
	case err == nil && (cur.State == "running" || cur.State == "waiting"):
		return nil
	case err == nil:
		return ErrInterrupted // done/error/interrupted не нами — операции больше нет
	case statusCode(err) == http.StatusConflict:
		return ErrCanceled
	case statusCode(err) == http.StatusNotFound:
		return ErrInterrupted
	}
	return nil // сеть моргнула — решит следующий опрос
}

func runOp(ctx context.Context, q opQueue, op *yue.QueueOp, fn func(context.Context) (int64, error)) (int64, error) {
	if err := waitTurn(ctx, q, op); err != nil {
		if errors.Is(err, ErrCanceled) {
			finishOp(ctx, q, op.ID, yue.QueueFinishIn{State: "canceled", Error: ErrCanceled.Error()})
		}
		return 0, err
	}
	undo := &undoList{}
	wctx, cancel := context.WithCancelCause(context.WithValue(yue.WithOp(ctx, op.ID), undoKey{}, undo))
	defer cancel(nil)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { // отметки «жива» и метка отмены, пока идёт работа
		defer close(done)
		t := time.NewTicker(opPoll)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				cur, err := q.QueueBeat(ctx, op.ID)
				if v := beatVerdict(cur, err); v != nil {
					cancel(v)
					return
				}
			}
		}
	}()
	res, ferr := fn(wctx)
	close(stop)
	<-done
	cause := context.Cause(wctx)
	if !errors.Is(cause, ErrCanceled) && !errors.Is(cause, ErrInterrupted) {
		// последняя сверка: отмена могла прийти после прошлой отметки (шаг упал с 409, или цепочка
		// проглотила ошибку шага и дошла до конца) — «готово» поверх принятой отмены не пишем
		cur, err := q.QueueBeat(ctx, op.ID)
		if v := beatVerdict(cur, err); v != nil {
			cause = v
		}
	}
	switch {
	case errors.Is(cause, ErrInterrupted):
		undo.run(ctx)
		return 0, ErrInterrupted
	case errors.Is(cause, ErrCanceled):
		undo.run(ctx)
		finishOp(ctx, q, op.ID, yue.QueueFinishIn{State: "canceled", Error: ErrCanceled.Error()})
		return 0, ErrCanceled
	case ferr != nil:
		undo.run(ctx) // итог шага не нужен — откат того, что успело появиться
		finishOp(ctx, q, op.ID, yue.QueueFinishIn{State: "error", Error: ferr.Error()})
		return 0, ferr
	}
	// воркер сам решает итог: отмена, принятая между последней сверкой и «готово», превращает его в canceled
	if fin := finishOp(ctx, q, op.ID, yue.QueueFinishIn{State: "done", ResultJobID: res}); fin != nil && fin.State == "canceled" {
		undo.run(ctx)
		return 0, ErrCanceled
	}
	undo.runDone(ctx)
	return res, nil
}

// waitTurn — ждать выдачи слота: опрос воркера, пока операция ждёт.
func waitTurn(ctx context.Context, q opQueue, op *yue.QueueOp) error {
	state, fails := op.State, 0
	for state != "running" {
		if v := beatVerdict(op, nil); state != "waiting" && v != nil {
			return v
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(opPoll):
		}
		cur, err := q.QueueBeat(ctx, op.ID)
		if v := beatVerdict(cur, err); v != nil {
			return v
		}
		if err != nil {
			if fails++; fails >= opBeatFails {
				return err
			}
			continue
		}
		fails = 0
		op, state = cur, cur.State
	}
	return nil
}

// finishOp — итог операции; приложение закрывается (ctx отменён) — итог всё равно доставляется.
func finishOp(ctx context.Context, q opQueue, id int64, in yue.QueueFinishIn) *yue.QueueOp {
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	fin, err := q.QueueFinish(fctx, id, in)
	if err != nil {
		log.Printf("очередь: итог операции %d (%s): %v", id, in.State, err)
		return nil
	}
	return fin
}
