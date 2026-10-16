package mcp

import (
	"encoding/json"
	"testing"

	"partscaster/internal/yue"
)

// queue_list — поля хода как у воркера (internal-preset-progress, условие 6):
// stage, pass, eta_s, stage_since; у записи без хода eta_s/stage_since — null.
func TestQueueListShowsProgressFields(t *testing.T) {
	t.Parallel()
	s, fake := newQueueServer(t)
	eta, since := 80.0, 41.0
	fake.snap = &yue.QueueSnapshot{Items: []yue.QueueOp{
		{ID: 31, Kind: "preset", Title: "Warm Wire — пресет", State: "running",
			Stage: "render", Pass: 2, EtaS: &eta, StageSince: &since},
		{ID: 32, Kind: "generate", Title: "Gone", State: "waiting"},
	}}
	out, ok := call(t, s, "queue_list", map[string]any{})
	if !ok {
		t.Fatalf("queue_list: %s", out)
	}
	var snap struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &snap); err != nil {
		t.Fatalf("ответ не JSON: %v: %s", err, out)
	}
	if len(snap.Items) != 2 {
		t.Fatalf("записей %d: %s", len(snap.Items), out)
	}
	run := snap.Items[0]
	if run["stage"] != "render" || run["pass"] != float64(2) || run["eta_s"] != 80.0 || run["stage_since"] != 41.0 {
		t.Fatalf("ход идущей: %v", run)
	}
	wait := snap.Items[1]
	for _, k := range []string{"stage", "pass", "eta_s", "stage_since"} {
		if _, ok := wait[k]; !ok {
			t.Errorf("у записи без хода нет поля %s: %v", k, wait)
		}
	}
	if wait["eta_s"] != nil || wait["stage_since"] != nil {
		t.Errorf("запись без хода: eta_s=%v stage_since=%v, нужно null", wait["eta_s"], wait["stage_since"])
	}
}
