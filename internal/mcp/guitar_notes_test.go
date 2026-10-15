package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"partscaster/internal/yue"
)

// Спецификация (internal-guitar-notes, условие 8 и контракт «Паритет»):
//   - инструмент MCP guitar_notes зарегистрирован, есть в tools/list; job_id обязателен, force — необязателен;
//   - вызов доходит до воркера через yue.Service.GuitarNotes: POST /jobs/{job_id}/guitar_notes,
//     force=true → ?force=1, без force — без пересчёта;
//   - ответ воркера (сколько нот, какими распознавателями, из сохранённого ли) — в тексте ответа инструмента;
//   - нет job_id / job_id ≤ 0 — ошибка, воркер не вызывается; ошибка воркера (503 «не установлено»,
//     422 нет дорожки гитары) — ошибка инструмента с текстом воркера.
// Граница — HTTP воркера (httptest); сервис — настоящий yue-клиент.

type gnWorker struct {
	mu    sync.Mutex
	reqs  []string // "METHOD path?query"
	query []url.Values
}

func (w *gnWorker) calls() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.reqs...)
}

// gnServerFor — MCP-сервер с yue-клиентом на фейковом воркере; на /jobs/*/guitar_notes отвечает status/resp.
func gnServerFor(t *testing.T, status int, resp string) (*Server, *gnWorker) {
	t.Helper()
	w := &gnWorker{}
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if strings.HasSuffix(r.URL.Path, "/guitar_notes") {
			w.mu.Lock()
			w.reqs = append(w.reqs, r.Method+" "+r.URL.Path)
			w.query = append(w.query, r.URL.Query())
			w.mu.Unlock()
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(status)
			_, _ = rw.Write([]byte(resp))
			return
		}
		http.NotFound(rw, r)
	}))
	t.Cleanup(srv.Close)
	s := NewServer(yue.New(srv.URL), t.TempDir())
	RegisterWorkflowTools(s)
	RegisterStudioTools(s)
	RegisterLibraryTools(s)
	RegisterInstallTools(s)
	return s, w
}

func gnForce(q url.Values) bool {
	v := strings.ToLower(q.Get("force"))
	return v == "1" || v == "true"
}

func TestGuitarNotesToolSchema(t *testing.T) {
	t.Parallel()
	s, _ := gnServerFor(t, 200, `{}`)
	s.mu.RLock()
	tool, ok := s.tools["guitar_notes"]
	s.mu.RUnlock()
	if !ok {
		t.Fatal("инструмент guitar_notes не зарегистрирован")
	}
	if strings.TrimSpace(tool.Description) == "" {
		t.Error("guitar_notes: нет описания")
	}
	pr, _ := tool.InputSchema["properties"].(map[string]any)
	for _, k := range []string{"job_id", "force"} {
		if _, ok := pr[k]; !ok {
			t.Errorf("guitar_notes: нет аргумента %s в схеме %v", k, pr)
		}
	}
	req, _ := tool.InputSchema["required"].([]string)
	hasJob, hasForce := false, false
	for _, r := range req {
		hasJob = hasJob || r == "job_id"
		hasForce = hasForce || r == "force"
	}
	if !hasJob || hasForce {
		t.Errorf("required %v, want job_id обязателен, force — нет", req)
	}
}

func TestGuitarNotesToolInToolsList(t *testing.T) {
	t.Parallel()
	s, _ := gnServerFor(t, 200, `{}`)
	lines := rpcRoundTrip(t, s, []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	})
	if len(lines) != 2 {
		t.Fatalf("ответов %d", len(lines))
	}
	var list struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &list); err != nil {
		t.Fatal(err)
	}
	for _, tl := range list.Result.Tools {
		if tl.Name == "guitar_notes" {
			return
		}
	}
	t.Fatal("guitar_notes нет в tools/list")
}

func TestGuitarNotesToolCallsWorker(t *testing.T) {
	t.Parallel()
	s, w := gnServerFor(t, 200, `{"notes":431,"engines":["yourmt3","basic-pitch"],"cached":false}`)
	out, ok := call(t, s, "guitar_notes", map[string]any{"job_id": float64(919)})
	if !ok {
		t.Fatalf("guitar_notes: %s", out)
	}
	if got := w.calls(); len(got) != 1 || got[0] != "POST /jobs/919/guitar_notes" {
		t.Fatalf("запросы воркеру %v, want [POST /jobs/919/guitar_notes]", got)
	}
	if gnForce(w.query[0]) {
		t.Errorf("без force — без пересчёта, а запрос ?%s", w.query[0].Encode())
	}
	for _, want := range []string{"431", "yourmt3", "basic-pitch"} {
		if !strings.Contains(out, want) {
			t.Errorf("ответ без %q: %s", want, out)
		}
	}
}

func TestGuitarNotesToolForce(t *testing.T) {
	t.Parallel()
	s, w := gnServerFor(t, 200, `{"notes":12,"engines":["basic-pitch"],"cached":false}`)
	out, ok := call(t, s, "guitar_notes", map[string]any{"job_id": float64(7), "force": true})
	if !ok {
		t.Fatalf("guitar_notes: %s", out)
	}
	if got := w.calls(); len(got) != 1 || got[0] != "POST /jobs/7/guitar_notes" {
		t.Fatalf("запросы воркеру %v", got)
	}
	if !gnForce(w.query[0]) {
		t.Errorf("force=true → ?force=1, а запрос ?%s", w.query[0].Encode())
	}
	if !strings.Contains(out, "12") {
		t.Errorf("ответ без числа нот: %s", out)
	}
}

func TestGuitarNotesToolBadJobID(t *testing.T) {
	t.Parallel()
	for _, args := range []map[string]any{{}, {"job_id": float64(0)}, {"job_id": float64(-3)}, {"force": true}} {
		s, w := gnServerFor(t, 200, `{"notes":1,"engines":["basic-pitch"],"cached":false}`)
		out, ok := call(t, s, "guitar_notes", args)
		if ok {
			t.Errorf("%v: want ошибку, ответ %s", args, out)
		}
		if got := w.calls(); len(got) != 0 {
			t.Errorf("%v: воркер вызван %v, want нет", args, got)
		}
	}
}

func TestGuitarNotesToolWorkerErrorText(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		status int
		detail string
	}{
		{503, "распознавание нот не установлено: worker/notes_install.sh"},
		{422, "у трека нет дорожки гитары"},
	} {
		b, _ := json.Marshal(map[string]string{"detail": tc.detail})
		s, _ := gnServerFor(t, tc.status, string(b))
		out, ok := call(t, s, "guitar_notes", map[string]any{"job_id": float64(5)})
		if ok {
			t.Fatalf("%d: want ошибку, ответ %s", tc.status, out)
		}
		if !strings.Contains(out, tc.detail) {
			t.Errorf("%d: ошибка %q, want с текстом воркера %q", tc.status, out, tc.detail)
		}
	}
}
