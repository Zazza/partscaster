package yue

import (
	"encoding/json"
	"testing"
)

// Спецификация «характер исполнения»: SubmitParams отправляет temperature/cfg
// воркеру только если они заданы (0 = «по умолчанию», ключа нет — omitempty);
// Job читает temperature/cfg из ответа воркера.

func TestCharacterSubmitParamsMarshal(t *testing.T) {
	t.Parallel()
	b, err := json.Marshal(SubmitParams{Style: "rock", Lyrics: "[verse] la", Temperature: 1.15, Cfg: 2.5})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["temperature"] != 1.15 || m["cfg"] != 2.5 {
		t.Fatalf("json %s: want temperature=1.15 cfg=2.5", b)
	}
}

func TestCharacterSubmitParamsZeroOmitted(t *testing.T) {
	t.Parallel()
	b, err := json.Marshal(SubmitParams{Style: "rock", Lyrics: "[verse] la"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"temperature", "cfg"} {
		if _, ok := m[k]; ok {
			t.Fatalf("json %s: key %q must be absent at 0", b, k)
		}
	}
}

func TestCharacterJobUnmarshal(t *testing.T) {
	t.Parallel()
	var j Job
	if err := json.Unmarshal([]byte(`{"id":5,"status":"done","temperature":1.15,"cfg":2.5}`), &j); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if j.Temperature != 1.15 || j.Cfg != 2.5 {
		t.Fatalf("job %+v: want Temperature=1.15 Cfg=2.5", j)
	}
	var old Job // старый трек: полей нет → 0
	if err := json.Unmarshal([]byte(`{"id":6,"status":"done"}`), &old); err != nil {
		t.Fatalf("unmarshal old: %v", err)
	}
	if old.Temperature != 0 || old.Cfg != 0 {
		t.Fatalf("old job %+v: want 0/0", old)
	}
}
