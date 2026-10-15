package studio

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"partscaster/internal/yue"
)

// Регрессия кросс-ревью (internal-shared-queue, усл. 11 и 13): пресет звука отменили уже после того,
// как его версия трека создана, — отмена принята, значит промежуточной версии остаться не должно:
// созданная версия удаляется (DeleteJob с её id), итог операции — canceled, ApplySoundPreset → ErrCanceled.
// Старое поведение: отмену, пришедшую после последней отметки «жива», не замечали — операция
// закрывалась done, версия оставалась у трека.

// psQueueFake — фейковый воркер пресета (psFake) + фейковая очередь: метка отмены появляется
// в ответе beat только после того, как версия трека создана (VariantToTrack).
type psQueueFake struct {
	*psFake
	promoted atomic.Bool
	q        *opFake
}

func (f *psQueueFake) VariantToTrack(ctx context.Context, jobID int64, file, title string, src int64) (int64, error) {
	id, err := f.psFake.VariantToTrack(ctx, jobID, file, title, src)
	f.promoted.Store(true)
	return id, err
}

func (f *psQueueFake) QueueAdd(ctx context.Context, in yue.QueueOpIn) (*yue.QueueOp, error) {
	op, err := f.q.QueueAdd(ctx, in)
	if op != nil {
		op.State = "running" // свободный слот — сразу
	}
	return op, err
}

func (f *psQueueFake) QueueBeat(ctx context.Context, id int64) (*yue.QueueOp, error) {
	return f.q.QueueBeat(ctx, id)
}

func (f *psQueueFake) QueueFinish(ctx context.Context, id int64, in yue.QueueFinishIn) (*yue.QueueOp, error) {
	return f.q.QueueFinish(ctx, id, in)
}

func TestApplySoundPresetCanceledAfterVersionDeletesIt(t *testing.T) {
	t.Parallel()
	f := &psQueueFake{psFake: psSetup(t, nil), q: newOpFake()}
	f.q.beat = func(int) (*yue.QueueOp, error) {
		return &yue.QueueOp{ID: fakeOpID, State: "running", CancelRequested: f.promoted.Load()}, nil
	}
	p := psPreset(5, []yue.PresetSpec{psVocalCut()}, psFinal())

	_, err := ApplySoundPreset(withoutLiveLayer(context.Background()), f, psJobID, p)

	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("ApplySoundPreset err = %v, нужно ErrCanceled («отменено человеком»)", err)
	}
	if n := len(f.psFake.promoted); n != 1 {
		t.Fatalf("версия трека создана %d раз, тест рассчитан на одну: %+v", n, f.psFake.promoted)
	}
	want := fmt.Sprintf("job %d", psChild)
	found := false
	for _, d := range f.deleted {
		if d == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("созданная версия #%d не удалена после отмены; удаления: %v", psChild, f.deleted)
	}
	if fin := onlyFinish(t, f.q); fin.State != "canceled" || fin.ResultJobID != 0 {
		t.Fatalf("итог операции: %+v, нужно canceled без версии", fin)
	}
}

// psLateCancelFake — пресет из окна приложения: отмена принята воркером уже после того, как
// ApplySoundPreset вернул успех (beat отмену не показывал, а на «готово» воркер ответил canceled).
// Удаления версий фиксируются под замком с сигналом — откат идёт в фоне StartOp.
type psLateCancelFake struct {
	*psQueueFake
	mu      sync.Mutex
	deleted []int64
	delCh   chan struct{}
}

func (f *psLateCancelFake) QueueFinish(ctx context.Context, id int64, in yue.QueueFinishIn) (*yue.QueueOp, error) {
	op, err := f.psQueueFake.QueueFinish(ctx, id, in)
	if op != nil && in.State == "done" {
		op.State, op.ResultJobID = "canceled", 0
	}
	return op, err
}

func (f *psLateCancelFake) DeleteJob(ctx context.Context, id int64) (bool, error) {
	f.mu.Lock()
	f.deleted = append(f.deleted, id)
	f.mu.Unlock()
	f.delCh <- struct{}{}
	return f.psQueueFake.DeleteJob(ctx, id)
}

// Регрессия кросс-ревью круга 2 (усл. 11, 13, 16): пресет из окна (StartOp, внутри — ApplySoundPreset
// как вложенный вызов) отменён после успешного создания версии — «готово» опоздало, воркер ответил
// canceled. Созданная версия удаляется. Старое поведение: откат версии был только внутри
// ApplySoundPreset (по отметке «жива»); вложенный вызов возвращал успех, внешняя операция видела
// отмену только в ответе finish — версия оставалась у трека.
func TestStartOpPresetCanceledAfterSuccessDeletesVersion(t *testing.T) {
	t.Parallel()
	f := &psLateCancelFake{
		psQueueFake: &psQueueFake{psFake: psSetup(t, nil), q: newOpFake()},
		delCh:       make(chan struct{}, 8),
	}
	p := psPreset(5, []yue.PresetSpec{psVocalCut()}, psFinal())
	applied := make(chan error, 1)
	_, err := StartOp(context.Background(), f, "preset", "Warm Wire — пресет", psJobID,
		func(ctx context.Context) (int64, error) {
			res, err := ApplySoundPreset(withoutLiveLayer(ctx), f, psJobID, p)
			applied <- err
			if err != nil {
				return 0, err
			}
			return res.ChildID, nil
		})
	if err != nil {
		t.Fatalf("StartOp: %v", err)
	}
	select {
	case <-f.q.finishCh:
	case <-time.After(waitLimit):
		t.Fatal("операция StartOp не закрыта")
	}
	if aerr := <-applied; aerr != nil {
		t.Fatalf("сценарий: ApplySoundPreset должен был вернуть успех, err = %v", aerr)
	}
	select {
	case <-f.delCh:
	case <-time.After(waitLimit):
		t.Fatalf("созданная версия #%d не удалена после отмены", psChild)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	found := false
	for _, id := range f.deleted {
		if id == psChild {
			found = true
		}
	}
	if !found {
		t.Fatalf("удалена не та версия: %v, нужна #%d", f.deleted, psChild)
	}
}
