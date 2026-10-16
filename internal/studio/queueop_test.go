package studio

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"partscaster/internal/yue"
)

// RunOp / StartOp (internal-shared-queue, условия 3, 11, 15, 16; ТК13, ТК18; контракт «Go»):
//
//	RunOp(ctx, svc, kind, title, jobID, fn func(ctx) (int64, error)) (int64, error)
//	StartOp(ctx, svc, kind, title, jobID, fn) (*yue.QueueOp, error)
//
// Внешняя граница — очередь воркера (QueueAdd/QueueBeat/QueueFinish), подменяется фейком.
// Интервал опроса в контракте не задан — ожидания ограничены waitLimit.

const waitLimit = 10 * time.Second

const fakeOpID int64 = 77

// opFake — фейковая очередь воркера. Остальные методы yue.Service не реализованы (паника).
type opFake struct {
	yue.Service
	mu       sync.Mutex
	addState string // состояние в ответе QueueAdd
	added    []yue.QueueOpIn
	beats    int
	// beat — ответ на n-й (с 1) вызов QueueBeat; nil — «running».
	beat     func(n int) (*yue.QueueOp, error)
	finished []yue.QueueFinishIn
	finishCh chan struct{}
}

func newOpFake() *opFake {
	return &opFake{addState: "waiting", finishCh: make(chan struct{}, 8)}
}

func (f *opFake) QueueAdd(_ context.Context, in yue.QueueOpIn) (*yue.QueueOp, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.added = append(f.added, in)
	return &yue.QueueOp{ID: fakeOpID, Kind: in.Kind, Title: in.Title, JobID: in.JobID, State: f.addState}, nil
}

func (f *opFake) QueueBeat(_ context.Context, id int64, _ ...yue.QueueProgress) (*yue.QueueOp, error) {
	f.mu.Lock()
	f.beats++
	n, fn := f.beats, f.beat
	f.mu.Unlock()
	if id != fakeOpID {
		return nil, &yue.StatusError{Code: http.StatusNotFound, Msg: "нет операции"}
	}
	if fn == nil {
		return &yue.QueueOp{ID: id, State: "running"}, nil
	}
	return fn(n)
}

func (f *opFake) QueueFinish(_ context.Context, id int64, in yue.QueueFinishIn) (*yue.QueueOp, error) {
	f.mu.Lock()
	f.finished = append(f.finished, in)
	f.mu.Unlock()
	f.finishCh <- struct{}{}
	return &yue.QueueOp{ID: id, State: in.State, ResultJobID: in.ResultJobID, Error: in.Error}, nil
}

func (f *opFake) snapshot() (added []yue.QueueOpIn, beats int, finished []yue.QueueFinishIn) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]yue.QueueOpIn(nil), f.added...), f.beats, append([]yue.QueueFinishIn(nil), f.finished...)
}

type runResult struct {
	id  int64
	err error
}

// runAsync — RunOp в горутине с ограничением по времени (тест не виснет при дедлоке).
func runAsync(ctx context.Context, svc yue.Service, fn func(context.Context) (int64, error)) <-chan runResult {
	ch := make(chan runResult, 1)
	go func() {
		id, err := RunOp(ctx, svc, "preset", "Warm Wire — пресет", 463, fn)
		ch <- runResult{id, err}
	}()
	return ch
}

func waitRun(t *testing.T, ch <-chan runResult) runResult {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(waitLimit):
		t.Fatalf("RunOp не вернулся за %s", waitLimit)
	}
	return runResult{}
}

func onlyFinish(t *testing.T, f *opFake) yue.QueueFinishIn {
	t.Helper()
	_, _, fin := f.snapshot()
	if len(fin) != 1 {
		t.Fatalf("QueueFinish вызван %d раз, нужно 1: %+v", len(fin), fin)
	}
	return fin[0]
}

// Успех: операция регистрируется с переданными kind/title/jobID, fn получает ctx с меткой операции,
// итог — done + id версии, RunOp возвращает этот id.
func TestRunOpSuccessFinishesDone(t *testing.T) {
	t.Parallel()
	f := newOpFake()
	var gotOp int64
	r := waitRun(t, runAsync(context.Background(), f, func(ctx context.Context) (int64, error) {
		gotOp = yue.OpFrom(ctx)
		return 812, nil
	}))
	if r.err != nil || r.id != 812 {
		t.Fatalf("RunOp = %d, %v; нужно 812, nil", r.id, r.err)
	}
	added, _, _ := f.snapshot()
	if len(added) != 1 || added[0].Kind != "preset" || added[0].Title != "Warm Wire — пресет" || added[0].JobID != 463 {
		t.Fatalf("QueueAdd: %+v", added)
	}
	if gotOp != fakeOpID {
		t.Fatalf("fn получил op=%d, нужно %d (вызовы воркера должны идти с меткой операции)", gotOp, fakeOpID)
	}
	fin := onlyFinish(t, f)
	if fin.State != "done" || fin.ResultJobID != 812 {
		t.Fatalf("итог: %+v, нужно done/812", fin)
	}
}

// Ожидание очереди: пока воркер отвечает waiting, fn не вызывается.
func TestRunOpWaitsForRunning(t *testing.T) {
	t.Parallel()
	f := newOpFake()
	const waitingBeats = 2
	f.beat = func(n int) (*yue.QueueOp, error) {
		if n <= waitingBeats {
			return &yue.QueueOp{ID: fakeOpID, State: "waiting"}, nil
		}
		return &yue.QueueOp{ID: fakeOpID, State: "running"}, nil
	}
	var beatsAtStart int
	r := waitRun(t, runAsync(context.Background(), f, func(ctx context.Context) (int64, error) {
		_, beatsAtStart, _ = f.snapshot()
		return 1, nil
	}))
	if r.err != nil {
		t.Fatalf("RunOp: %v", r.err)
	}
	if beatsAtStart <= waitingBeats {
		t.Fatalf("fn вызван после %d опросов, а running воркер ответил только на %d-й", beatsAtStart, waitingBeats+1)
	}
}

// Реентерабельность (усл. 3, ТК4): ctx уже внутри операции — fn сразу, без новой записи в очереди.
func TestRunOpReentrantNoQueueAdd(t *testing.T) {
	t.Parallel()
	f := newOpFake()
	ctx := yue.WithOp(context.Background(), 55)
	var gotOp int64
	r := waitRun(t, runAsync(ctx, f, func(ctx context.Context) (int64, error) {
		gotOp = yue.OpFrom(ctx)
		return 9, nil
	}))
	if r.err != nil || r.id != 9 {
		t.Fatalf("RunOp = %d, %v", r.id, r.err)
	}
	added, _, fin := f.snapshot()
	if len(added) != 0 || len(fin) != 0 {
		t.Fatalf("вложенный вызов не должен регистрировать/закрывать операцию: add=%v finish=%v", added, fin)
	}
	if gotOp != 55 {
		t.Fatalf("fn получил op=%d, нужно внешнюю 55", gotOp)
	}
}

// Ошибка шага: итог error с текстом, RunOp возвращает ошибку.
func TestRunOpFnErrorFinishesError(t *testing.T) {
	t.Parallel()
	f := newOpFake()
	r := waitRun(t, runAsync(context.Background(), f, func(ctx context.Context) (int64, error) {
		return 0, errors.New("нет стемов")
	}))
	if r.err == nil || !strings.Contains(r.err.Error(), "нет стемов") {
		t.Fatalf("RunOp err = %v, нужен текст ошибки fn", r.err)
	}
	fin := onlyFinish(t, f)
	if fin.State != "error" || !strings.Contains(fin.Error, "нет стемов") {
		t.Fatalf("итог: %+v, нужно error с текстом", fin)
	}
}

// ТК13 (усл. 11): метка отмены в ответе beat во время работы — ctx fn отменён,
// следующий шаг не начат, итог canceled, RunOp → ErrCanceled («отменено человеком»).
func TestRunOpCancelRequestedStopsChain(t *testing.T) {
	t.Parallel()
	f := newOpFake()
	started := make(chan struct{})
	var once sync.Once
	f.beat = func(n int) (*yue.QueueOp, error) {
		select {
		case <-started:
			return &yue.QueueOp{ID: fakeOpID, State: "running", CancelRequested: true}, nil
		default:
			return &yue.QueueOp{ID: fakeOpID, State: "running"}, nil
		}
	}
	var mu sync.Mutex
	var steps []int
	ctxCanceled := false
	r := waitRun(t, runAsync(context.Background(), f, func(ctx context.Context) (int64, error) {
		for step := 1; step <= 3; step++ {
			if ctx.Err() != nil {
				return 0, ctx.Err()
			}
			mu.Lock()
			steps = append(steps, step)
			mu.Unlock()
			once.Do(func() { close(started) })
			select {
			case <-ctx.Done():
				mu.Lock()
				ctxCanceled = true
				mu.Unlock()
			case <-time.After(waitLimit):
				return 0, errors.New("ctx шага не отменён")
			}
		}
		return 900, nil // «версия» — не должна появиться
	}))
	if !errors.Is(r.err, ErrCanceled) {
		t.Fatalf("RunOp err = %v, нужно ErrCanceled", r.err)
	}
	if !strings.Contains(r.err.Error(), "отменено") {
		t.Fatalf("текст ошибки для агента/человека: %q", r.err.Error())
	}
	if r.id != 0 {
		t.Fatalf("после отмены версия не возвращается: %d", r.id)
	}
	mu.Lock()
	defer mu.Unlock()
	if !ctxCanceled {
		t.Fatal("ctx fn не отменён")
	}
	if len(steps) != 1 {
		t.Fatalf("после отмены начат следующий шаг: шаги %v", steps)
	}
	if fin := onlyFinish(t, f); fin.State != "canceled" || fin.ResultJobID != 0 {
		t.Fatalf("итог: %+v, нужно canceled без версии", fin)
	}
}

// Отмена ответом 409 на beat — то же, что метка отмены.
func TestRunOpBeatConflictIsCancel(t *testing.T) {
	t.Parallel()
	f := newOpFake()
	started := make(chan struct{})
	f.beat = func(n int) (*yue.QueueOp, error) {
		select {
		case <-started:
			return nil, &yue.StatusError{Code: http.StatusConflict, Msg: "отменено"}
		default:
			return &yue.QueueOp{ID: fakeOpID, State: "running"}, nil
		}
	}
	r := waitRun(t, runAsync(context.Background(), f, func(ctx context.Context) (int64, error) {
		close(started)
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(waitLimit):
			return 0, errors.New("ctx шага не отменён")
		}
	}))
	if !errors.Is(r.err, ErrCanceled) {
		t.Fatalf("RunOp err = %v, нужно ErrCanceled", r.err)
	}
}

// Усл. 10: отменённая в ожидании операция — работа не начинается, агент получает «отменено».
func TestRunOpCanceledWhileWaitingSkipsFn(t *testing.T) {
	t.Parallel()
	f := newOpFake()
	f.beat = func(n int) (*yue.QueueOp, error) {
		return &yue.QueueOp{ID: fakeOpID, State: "canceled"}, nil
	}
	called := false
	r := waitRun(t, runAsync(context.Background(), f, func(ctx context.Context) (int64, error) {
		called = true
		return 1, nil
	}))
	if called {
		t.Fatal("fn вызван у отменённой в ожидании операции")
	}
	if !errors.Is(r.err, ErrCanceled) {
		t.Fatalf("RunOp err = %v, нужно ErrCanceled", r.err)
	}
}

// Операция пропала на воркере (404 на beat, interrupted по порогу) — ErrInterrupted, fn не вызывается.
func TestRunOpBeatNotFoundIsInterrupted(t *testing.T) {
	t.Parallel()
	f := newOpFake()
	f.beat = func(n int) (*yue.QueueOp, error) {
		return nil, &yue.StatusError{Code: http.StatusNotFound, Msg: "нет операции"}
	}
	called := false
	r := waitRun(t, runAsync(context.Background(), f, func(ctx context.Context) (int64, error) {
		called = true
		return 1, nil
	}))
	if called {
		t.Fatal("fn вызван у пропавшей операции")
	}
	if !errors.Is(r.err, ErrInterrupted) {
		t.Fatalf("RunOp err = %v, нужно ErrInterrupted", r.err)
	}
}

// Операция пропала посреди работы — ctx fn отменён, ErrInterrupted.
func TestRunOpLostDuringFnIsInterrupted(t *testing.T) {
	t.Parallel()
	f := newOpFake()
	started := make(chan struct{})
	f.beat = func(n int) (*yue.QueueOp, error) {
		select {
		case <-started:
			return nil, &yue.StatusError{Code: http.StatusNotFound, Msg: "нет операции"}
		default:
			return &yue.QueueOp{ID: fakeOpID, State: "running"}, nil
		}
	}
	r := waitRun(t, runAsync(context.Background(), f, func(ctx context.Context) (int64, error) {
		close(started)
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(waitLimit):
			return 0, errors.New("ctx шага не отменён")
		}
	}))
	if !errors.Is(r.err, ErrInterrupted) {
		t.Fatalf("RunOp err = %v, нужно ErrInterrupted", r.err)
	}
}

// ТК18 (усл. 16): StartOp регистрирует операцию синхронно и сразу возвращает её id;
// ожидание и работа — в фоне, по окончании операция закрывается итогом.
func TestStartOpReturnsIDImmediately(t *testing.T) {
	t.Parallel()
	f := newOpFake()
	release := make(chan struct{})
	op, err := StartOp(context.Background(), f, "preset", "Gone — пресет", 502, func(ctx context.Context) (int64, error) {
		<-release
		return 640, nil
	})
	if err != nil {
		t.Fatalf("StartOp: %v", err)
	}
	if op == nil || op.ID != fakeOpID {
		t.Fatalf("StartOp вернул %+v, нужен id операции воркера %d", op, fakeOpID)
	}
	added, _, fin := f.snapshot()
	if len(added) != 1 || added[0].Kind != "preset" || added[0].JobID != 502 {
		t.Fatalf("регистрация должна быть синхронной: %+v", added)
	}
	if len(fin) != 0 {
		t.Fatalf("работа ещё идёт, а операция закрыта: %+v", fin)
	}
	close(release)
	select {
	case <-f.finishCh:
	case <-time.After(waitLimit):
		t.Fatal("операция StartOp не закрыта")
	}
	if fin := onlyFinish(t, f); fin.State != "done" || fin.ResultJobID != 640 {
		t.Fatalf("итог: %+v", fin)
	}
}

// Регрессия кросс-ревью (усл. 11): fn завершилась успехом, а отмена пришла уже после последней
// отметки «жива» (пока fn работала, beat отвечал running; следующий beat — метка отмены).
// «Готово» поверх принятой отмены не пишется: RunOp → ErrCanceled, итог canceled.
// Старое поведение: после fn итог брался только из её результата — операция закрывалась done.
func TestRunOpCancelAfterLastBeatIsCanceled(t *testing.T) {
	t.Parallel()
	f := newOpFake()
	doneCh := make(chan struct{})
	f.beat = func(int) (*yue.QueueOp, error) {
		select {
		case <-doneCh:
			return &yue.QueueOp{ID: fakeOpID, State: "running", CancelRequested: true}, nil
		default:
			return &yue.QueueOp{ID: fakeOpID, State: "running"}, nil
		}
	}
	r := waitRun(t, runAsync(context.Background(), f, func(ctx context.Context) (int64, error) {
		close(doneCh) // отмена принята воркером к концу шага, после последней отметки
		return 812, nil
	}))
	if !errors.Is(r.err, ErrCanceled) {
		t.Fatalf("RunOp = %d, %v; нужно ErrCanceled", r.id, r.err)
	}
	if r.id != 0 {
		t.Fatalf("после отмены версия не возвращается: %d", r.id)
	}
	if fin := onlyFinish(t, f); fin.State != "canceled" || fin.ResultJobID != 0 {
		t.Fatalf("итог: %+v, нужно canceled без версии (не done)", fin)
	}
}

// finishCancelsFake — очередь, где отмена принята воркером между последней сверкой клиента и его
// «готово»: на QueueFinish(done) воркер отвечает итогом canceled (метка отмены уже стояла).
type finishCancelsFake struct {
	*opFake
}

func (f *finishCancelsFake) QueueFinish(ctx context.Context, id int64, in yue.QueueFinishIn) (*yue.QueueOp, error) {
	op, err := f.opFake.QueueFinish(ctx, id, in)
	if op != nil && in.State == "done" {
		op.State, op.ResultJobID = "canceled", 0
	}
	return op, err
}

// Регрессия кросс-ревью круга 2 (усл. 11): клиент прислал «готово», а воркер ответил, что операция
// уже отменена (отмена принята после последней сверки). Итог для вызывающего — «отменено», не id версии.
// Старое поведение: ответ QueueFinish не читался — RunOp возвращал id версии как успех.
func TestRunOpFinishAnsweredCanceledIsCanceled(t *testing.T) {
	t.Parallel()
	f := &finishCancelsFake{newOpFake()}
	r := waitRun(t, runAsync(context.Background(), f, func(ctx context.Context) (int64, error) {
		return 812, nil
	}))
	if !errors.Is(r.err, ErrCanceled) {
		t.Fatalf("RunOp = %d, %v; нужно ErrCanceled", r.id, r.err)
	}
	if r.id != 0 {
		t.Fatalf("после отмены версия не возвращается: %d", r.id)
	}
}

// OnCanceled вне операции очереди — ничего не делает: откат не вызывается ни сразу, ни потом.
func TestOnCanceledOutsideOpIsNoop(t *testing.T) {
	t.Parallel()
	called := make(chan struct{}, 1)
	OnCanceled(context.Background(), func(context.Context) { called <- struct{}{} })
	select {
	case <-called:
		t.Fatal("откат вызван вне операции")
	case <-time.After(50 * time.Millisecond):
	}
}

// onCanceledRun — RunOp, внутри fn регистрирует откат и возвращает result; счётчик вызовов отката.
func onCanceledRun(t *testing.T, f yue.Service, fn func(ctx context.Context) (int64, error)) (runResult, chan struct{}) {
	t.Helper()
	called := make(chan struct{}, 4)
	r := waitRun(t, runAsync(context.Background(), f, func(ctx context.Context) (int64, error) {
		OnCanceled(ctx, func(context.Context) { called <- struct{}{} })
		return fn(ctx)
	}))
	return r, called
}

func waitCalled(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(waitLimit):
		t.Fatalf("откат не вызван: %s", what)
	}
}

// OnCanceled внутри RunOp: успешная операция — откат не вызывается.
func TestOnCanceledNotCalledOnSuccess(t *testing.T) {
	t.Parallel()
	f := newOpFake()
	r, called := onCanceledRun(t, f, func(ctx context.Context) (int64, error) { return 812, nil })
	if r.err != nil || r.id != 812 {
		t.Fatalf("RunOp = %d, %v", r.id, r.err)
	}
	select {
	case <-called:
		t.Fatal("откат вызван у успешной операции")
	case <-time.After(50 * time.Millisecond):
	}
}

// OnCanceled внутри RunOp: метка отмены во время работы — откат вызывается.
func TestOnCanceledCalledOnCancel(t *testing.T) {
	t.Parallel()
	f := newOpFake()
	started := make(chan struct{})
	f.beat = func(int) (*yue.QueueOp, error) {
		select {
		case <-started:
			return &yue.QueueOp{ID: fakeOpID, State: "running", CancelRequested: true}, nil
		default:
			return &yue.QueueOp{ID: fakeOpID, State: "running"}, nil
		}
	}
	r, called := onCanceledRun(t, f, func(ctx context.Context) (int64, error) {
		close(started)
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(waitLimit):
			return 0, errors.New("ctx шага не отменён")
		}
	})
	if !errors.Is(r.err, ErrCanceled) {
		t.Fatalf("RunOp err = %v, нужно ErrCanceled", r.err)
	}
	waitCalled(t, called, "отмена")
}

// OnCanceled внутри RunOp: «готово» опоздало — воркер ответил canceled; откат вызывается.
func TestOnCanceledCalledWhenFinishAnswersCanceled(t *testing.T) {
	t.Parallel()
	f := &finishCancelsFake{newOpFake()}
	r, called := onCanceledRun(t, f, func(ctx context.Context) (int64, error) { return 812, nil })
	if !errors.Is(r.err, ErrCanceled) {
		t.Fatalf("RunOp err = %v, нужно ErrCanceled", r.err)
	}
	waitCalled(t, called, "отмена в ответе на «готово»")
}

// OnCanceled внутри RunOp: операция пропала посреди работы (прерывание) — откат вызывается.
func TestOnCanceledCalledOnInterrupted(t *testing.T) {
	t.Parallel()
	f := newOpFake()
	started := make(chan struct{})
	f.beat = func(int) (*yue.QueueOp, error) {
		select {
		case <-started:
			return nil, &yue.StatusError{Code: http.StatusNotFound, Msg: "нет операции"}
		default:
			return &yue.QueueOp{ID: fakeOpID, State: "running"}, nil
		}
	}
	r, called := onCanceledRun(t, f, func(ctx context.Context) (int64, error) {
		close(started)
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(waitLimit):
			return 0, errors.New("ctx шага не отменён")
		}
	})
	if !errors.Is(r.err, ErrInterrupted) {
		t.Fatalf("RunOp err = %v, нужно ErrInterrupted", r.err)
	}
	waitCalled(t, called, "прерывание")
}

// AfterDone (доработка кросс-ревью: запись в журнал только после итога «готово»):
// вне операции очереди — fn сразу; внутри RunOp/StartOp — fn после QueueFinish(done);
// при отмене, прерывании и ошибке — не вызывается.

// AfterDone вне операции — fn вызвана сразу, до возврата AfterDone, ровно раз.
func TestAfterDoneOutsideOpCallsImmediately(t *testing.T) {
	t.Parallel()
	calls := 0
	AfterDone(context.Background(), func(context.Context) { calls++ })
	if calls != 1 {
		t.Fatalf("вне операции fn вызвана %d раз до возврата AfterDone, нужно 1 сразу", calls)
	}
}

// afterDoneCall — снимок очереди в момент вызова fn из AfterDone.
type afterDoneCall struct {
	finished []yue.QueueFinishIn
}

// afterDoneRun — RunOp, внутри fn регистрирует AfterDone; inFn — была ли fn AfterDone вызвана
// ещё до возврата шага (то есть до итога операции).
func afterDoneRun(t *testing.T, f yue.Service, q *opFake, fn func(ctx context.Context) (int64, error)) (r runResult, calls chan afterDoneCall, inFn bool) {
	t.Helper()
	calls = make(chan afterDoneCall, 4)
	r = waitRun(t, runAsync(context.Background(), f, func(ctx context.Context) (int64, error) {
		AfterDone(ctx, func(context.Context) {
			_, _, fin := q.snapshot()
			calls <- afterDoneCall{finished: fin}
		})
		id, err := fn(ctx)
		inFn = len(calls) > 0
		return id, err
	}))
	return r, calls, inFn
}

func noAfterDone(t *testing.T, calls <-chan afterDoneCall, what string) {
	t.Helper()
	select {
	case c := <-calls:
		t.Fatalf("AfterDone вызвана при %s (итоги очереди в момент вызова: %+v)", what, c.finished)
	case <-time.After(100 * time.Millisecond):
	}
}

// Успех: fn AfterDone вызвана ровно раз, не во время шага, а после того, как операция закрыта done.
func TestAfterDoneCalledOnceAfterFinishDone(t *testing.T) {
	t.Parallel()
	f := newOpFake()
	r, calls, inFn := afterDoneRun(t, f, f, func(ctx context.Context) (int64, error) { return 812, nil })
	if r.err != nil || r.id != 812 {
		t.Fatalf("RunOp = %d, %v", r.id, r.err)
	}
	if inFn {
		t.Fatal("AfterDone вызвана во время шага, до итога операции")
	}
	var c afterDoneCall
	select {
	case c = <-calls:
	case <-time.After(waitLimit):
		t.Fatal("AfterDone не вызвана у успешной операции")
	}
	if len(c.finished) != 1 || c.finished[0].State != "done" {
		t.Fatalf("в момент вызова AfterDone итоги очереди %+v, нужен уже записанный done", c.finished)
	}
	noAfterDone(t, calls, "повторе (второй вызов у одной операции)")
}

// Метка отмены в beat во время шага — AfterDone не вызывается.
func TestAfterDoneNotCalledOnCancel(t *testing.T) {
	t.Parallel()
	f := newOpFake()
	started := make(chan struct{})
	f.beat = func(int) (*yue.QueueOp, error) {
		select {
		case <-started:
			return &yue.QueueOp{ID: fakeOpID, State: "running", CancelRequested: true}, nil
		default:
			return &yue.QueueOp{ID: fakeOpID, State: "running"}, nil
		}
	}
	r, calls, _ := afterDoneRun(t, f, f, func(ctx context.Context) (int64, error) {
		close(started)
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(waitLimit):
			return 0, errors.New("ctx шага не отменён")
		}
	})
	if !errors.Is(r.err, ErrCanceled) {
		t.Fatalf("RunOp err = %v, нужно ErrCanceled", r.err)
	}
	noAfterDone(t, calls, "отмене меткой в beat")
}

// Шаг успешен, но воркер на «готово» ответил canceled — AfterDone не вызывается.
func TestAfterDoneNotCalledWhenFinishAnswersCanceled(t *testing.T) {
	t.Parallel()
	f := &finishCancelsFake{newOpFake()}
	r, calls, _ := afterDoneRun(t, f, f.opFake, func(ctx context.Context) (int64, error) { return 812, nil })
	if !errors.Is(r.err, ErrCanceled) {
		t.Fatalf("RunOp err = %v, нужно ErrCanceled", r.err)
	}
	noAfterDone(t, calls, "ответе canceled на «готово»")
}

// Операция пропала посреди шага (404) — AfterDone не вызывается.
func TestAfterDoneNotCalledOnInterrupted(t *testing.T) {
	t.Parallel()
	f := newOpFake()
	started := make(chan struct{})
	f.beat = func(int) (*yue.QueueOp, error) {
		select {
		case <-started:
			return nil, &yue.StatusError{Code: http.StatusNotFound, Msg: "нет операции"}
		default:
			return &yue.QueueOp{ID: fakeOpID, State: "running"}, nil
		}
	}
	r, calls, _ := afterDoneRun(t, f, f, func(ctx context.Context) (int64, error) {
		close(started)
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(waitLimit):
			return 0, errors.New("ctx шага не отменён")
		}
	})
	if !errors.Is(r.err, ErrInterrupted) {
		t.Fatalf("RunOp err = %v, нужно ErrInterrupted", r.err)
	}
	noAfterDone(t, calls, "пропаже операции (404)")
}

// Ошибка шага — AfterDone не вызывается.
func TestAfterDoneNotCalledOnError(t *testing.T) {
	t.Parallel()
	f := newOpFake()
	r, calls, _ := afterDoneRun(t, f, f, func(ctx context.Context) (int64, error) {
		return 0, errors.New("нет стемов")
	})
	if r.err == nil {
		t.Fatal("RunOp без ошибки, нужна ошибка шага")
	}
	noAfterDone(t, calls, "ошибке шага")
}
