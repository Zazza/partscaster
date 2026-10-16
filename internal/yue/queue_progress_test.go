package yue

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// Ход операции очереди и родитель трека (internal-preset-progress, условия 2, 3, 5, 6):
//
//	QueueProgress{Stage string; Pass int; EtaS *float64}
//	QueueBeat(ctx, id, progress ...QueueProgress) — без хода тела нет (прежний бит), с ходом — {stage, pass, eta_s}
//	QueueOp: Stage, Pass, EtaS *float64, StageSince *float64 (json stage, pass, eta_s, stage_since)
//	SetJobParent(ctx, jobID, parentID) error — PATCH /jobs/{id} {"parent_id": parentID}
//
// Внешняя граница — HTTP воркера (httptest, queueServer из queue_test.go).

func f64(v float64) *float64 { return &v }

func TestQueueBeatWithProgressSendsBody(t *testing.T) {
	t.Parallel()
	c, reqs := queueServer(t, http.StatusOK, opJSON)
	if _, err := c.QueueBeat(context.Background(), 5, QueueProgress{Stage: "render", Pass: 2, EtaS: f64(80)}); err != nil {
		t.Fatalf("QueueBeat: %v", err)
	}
	r := lastReq(t, reqs)
	if r.method != http.MethodPost || r.path != "/queue/ops/5/beat" {
		t.Fatalf("запрос %s %s", r.method, r.path)
	}
	if r.body["stage"] != "render" || r.body["pass"] != float64(2) || r.body["eta_s"] != float64(80) {
		t.Fatalf("тело бита: %v, нужно stage=render pass=2 eta_s=80", r.body)
	}
}

// Оценки ещё нет (до первой сверки) — eta_s null, а не 0: 0 значит «вот-вот готово».
func TestQueueBeatProgressWithoutEtaSendsNull(t *testing.T) {
	t.Parallel()
	c, reqs := queueServer(t, http.StatusOK, opJSON)
	if _, err := c.QueueBeat(context.Background(), 5, QueueProgress{Stage: "prepare", Pass: 1}); err != nil {
		t.Fatalf("QueueBeat: %v", err)
	}
	r := lastReq(t, reqs)
	if r.body["stage"] != "prepare" || r.body["pass"] != float64(1) {
		t.Fatalf("тело бита: %v", r.body)
	}
	if v, ok := r.body["eta_s"]; ok && v != nil {
		t.Fatalf("eta_s = %v, нужно null/нет", v)
	}
}

// eta 0 — значение, уходит как 0.
func TestQueueBeatProgressEtaZero(t *testing.T) {
	t.Parallel()
	c, reqs := queueServer(t, http.StatusOK, opJSON)
	_, _ = c.QueueBeat(context.Background(), 5, QueueProgress{Stage: "fix", Pass: 2, EtaS: f64(0)})
	if v, ok := lastReq(t, reqs).body["eta_s"]; !ok || v != float64(0) {
		t.Fatalf("eta_s = %v (есть: %v), нужно 0", v, ok)
	}
}

// Без хода — прежний бит: тела нет.
func TestQueueBeatWithoutProgressHasNoBody(t *testing.T) {
	t.Parallel()
	c, reqs := queueServer(t, http.StatusOK, opJSON)
	if _, err := c.QueueBeat(context.Background(), 5); err != nil {
		t.Fatalf("QueueBeat: %v", err)
	}
	if r := lastReq(t, reqs); len(r.body) != 0 {
		t.Fatalf("бит без хода с телом: %v", r.body)
	}
}

// Неверный stage воркер отвергает 422 — клиенту *StatusError.
func TestQueueBeatProgress422IsStatusError(t *testing.T) {
	t.Parallel()
	c, _ := queueServer(t, http.StatusUnprocessableEntity, `{"detail":"stage"}`)
	_, err := c.QueueBeat(context.Background(), 5, QueueProgress{Stage: "mixing", Pass: 1})
	wantStatus(t, err, http.StatusUnprocessableEntity)
}

// Через обёртку журнала ход доходит до воркера тем же телом.
func TestJournaledQueueBeatForwardsProgress(t *testing.T) {
	t.Parallel()
	c, reqs := queueServer(t, http.StatusOK, opJSON)
	q, ok := Journaled(c).(Queuer)
	if !ok {
		t.Fatal("обёртка журнала без очереди")
	}
	if _, err := q.QueueBeat(context.Background(), 5, QueueProgress{Stage: "verify", Pass: 1, EtaS: f64(12.5)}); err != nil {
		t.Fatalf("QueueBeat: %v", err)
	}
	r := lastReq(t, reqs)
	if r.path != "/queue/ops/5/beat" || r.body["stage"] != "verify" || r.body["eta_s"] != 12.5 {
		t.Fatalf("запрос %s %v", r.path, r.body)
	}
}

func TestQueueOpDecodesProgress(t *testing.T) {
	t.Parallel()
	var op QueueOp
	if err := json.Unmarshal([]byte(`{"id":5,"state":"running","stage":"render","pass":2,"eta_s":80,"stage_since":41.5}`), &op); err != nil {
		t.Fatal(err)
	}
	if op.Stage != "render" || op.Pass != 2 || op.EtaS == nil || *op.EtaS != 80 || op.StageSince == nil || *op.StageSince != 41.5 {
		t.Fatalf("ход не прочитан: %+v", op)
	}
	var empty QueueOp
	if err := json.Unmarshal([]byte(`{"id":6,"state":"waiting","stage":"","pass":null,"eta_s":null,"stage_since":null}`), &empty); err != nil {
		t.Fatal(err)
	}
	if empty.Stage != "" || empty.Pass != 0 || empty.EtaS != nil || empty.StageSince != nil {
		t.Fatalf("запись без хода: %+v", empty)
	}
}

func TestSetJobParentPatchesParentOnly(t *testing.T) {
	t.Parallel()
	var got updateReq
	c := updateServer(t, 200, `{"id":12,"title":"x","status":"done","parent_id":7}`, &got)
	if err := c.SetJobParent(context.Background(), 12, 7); err != nil {
		t.Fatalf("SetJobParent: %v", err)
	}
	if got.method != http.MethodPatch || got.path != "/jobs/12" {
		t.Fatalf("запрос %s %s", got.method, got.path)
	}
	if len(got.body) != 1 || got.body["parent_id"] != float64(7) {
		t.Fatalf("тело: %v, нужно только parent_id=7", got.body)
	}
}

func TestSetJobParent422IsStatusError(t *testing.T) {
	t.Parallel()
	var got updateReq
	c := updateServer(t, http.StatusUnprocessableEntity, `{"detail":"parent"}`, &got)
	wantStatus(t, c.SetJobParent(context.Background(), 12, 12), http.StatusUnprocessableEntity)
}

func TestJournaledSetJobParentReachesWorker(t *testing.T) {
	t.Parallel()
	c, reqs := queueServer(t, http.StatusOK, `{"id":12,"status":"done","parent_id":7}`)
	if err := Journaled(c).SetJobParent(context.Background(), 12, 7); err != nil {
		t.Fatalf("SetJobParent: %v", err)
	}
	for _, r := range reqs() {
		if r.method == http.MethodPatch && r.path == "/jobs/12" && r.body["parent_id"] == float64(7) {
			return
		}
	}
	t.Fatalf("PATCH /jobs/12 {parent_id:7} не дошёл: %+v", reqs())
}
