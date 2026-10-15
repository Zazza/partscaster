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

// Спецификация (internal-guitar-plan-chords, условие 9 и «Устройство (контракт)»: Go / MCP):
//   - инструмент MCP guitar_part зарегистрирован, есть в tools/list; job_id обязателен, style (0…4) — нет;
//   - вызов доходит до воркера через yue.Service.GuitarPart: GET /jobs/{job_id}/guitar_part?style=N;
//   - ответ воркера (такты с аккордами и частями, удары с нотами, манера) — в тексте ответа инструмента:
//     по нему ИИ отвечает на «что за звук на N секунде»;
//   - нет job_id / job_id ≤ 0 — ошибка, воркер не вызывается; ошибка воркера (422 «сделайте транскрипцию»,
//     422 нет дорожки гитары) — ошибка инструмента с текстом воркера;
//   - инструмента guitar_notes (распознавание) больше нет.
// Граница — HTTP воркера (httptest); сервис — настоящий yue-клиент.

type gpWorker struct {
	mu    sync.Mutex
	reqs  []string // "METHOD path"
	query []url.Values
}

func (w *gpWorker) calls() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.reqs...)
}

// gpServerFor — MCP-сервер с yue-клиентом на фейковом воркере; на /jobs/*/guitar_part отвечает status/resp.
func gpServerFor(t *testing.T, status int, resp string) (*Server, *gpWorker) {
	t.Helper()
	w := &gpWorker{}
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if strings.HasSuffix(r.URL.Path, "/guitar_part") || strings.HasSuffix(r.URL.Path, "/guitar_notes") {
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

const gpPart = `{"style":3,"bars":[{"start":24.0,"end":26.0,"chord":"F#m","section":"chorus"}],` +
	`"strikes":[{"t":26.13,"midi":[42]},{"t":26.51,"midi":[54,57,61,66]}],"beats":[24.0,24.5],"tune_cents":7.0}`

func gpToolNames(t *testing.T, s *Server) []string {
	t.Helper()
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
	var out []string
	for _, tl := range list.Result.Tools {
		out = append(out, tl.Name)
	}
	return out
}

func TestGuitarPartToolSchema(t *testing.T) {
	t.Parallel()
	s, _ := gpServerFor(t, 200, `{}`)
	s.mu.RLock()
	tool, ok := s.tools["guitar_part"]
	s.mu.RUnlock()
	if !ok {
		t.Fatal("инструмент guitar_part не зарегистрирован")
	}
	if strings.TrimSpace(tool.Description) == "" {
		t.Error("guitar_part: нет описания")
	}
	pr, _ := tool.InputSchema["properties"].(map[string]any)
	for _, k := range []string{"job_id", "style"} {
		if _, ok := pr[k]; !ok {
			t.Errorf("guitar_part: нет аргумента %s в схеме %v", k, pr)
		}
	}
	req, _ := tool.InputSchema["required"].([]string)
	hasJob, hasStyle := false, false
	for _, r := range req {
		hasJob = hasJob || r == "job_id"
		hasStyle = hasStyle || r == "style"
	}
	if !hasJob || hasStyle {
		t.Errorf("required %v, want job_id обязателен, style — нет", req)
	}
}

func TestGuitarPartInToolsListNotesGone(t *testing.T) {
	t.Parallel()
	s, _ := gpServerFor(t, 200, `{}`)
	names := gpToolNames(t, s)
	has := map[string]bool{}
	for _, n := range names {
		has[n] = true
	}
	if !has["guitar_part"] {
		t.Error("guitar_part нет в tools/list")
	}
	if has["guitar_notes"] {
		t.Error("guitar_notes остался в tools/list — распознавание нот удалено")
	}
	s.mu.RLock()
	_, notes := s.tools["guitar_notes"]
	s.mu.RUnlock()
	if notes {
		t.Error("инструмент guitar_notes всё ещё зарегистрирован")
	}
}

func TestGuitarPartToolCallsWorker(t *testing.T) {
	t.Parallel()
	for _, style := range []int{0, 1, 2, 3, 4} {
		s, w := gpServerFor(t, 200, gpPart)
		out, ok := call(t, s, "guitar_part", map[string]any{"job_id": float64(919), "style": float64(style)})
		if !ok {
			t.Fatalf("style %d: guitar_part: %s", style, out)
		}
		if got := w.calls(); len(got) != 1 || got[0] != "GET /jobs/919/guitar_part" {
			t.Fatalf("style %d: запросы воркеру %v, want [GET /jobs/919/guitar_part]", style, got)
		}
		want := []string{"0", "1", "2", "3", "4"}[style]
		if got := w.query[0].Get("style"); got != want {
			t.Errorf("style %d: ?%s, want style=%s", style, w.query[0].Encode(), want)
		}
	}
}

func TestGuitarPartToolDefaultStyleAuto(t *testing.T) {
	t.Parallel()
	// style не задан — манера «авто» (0): запрос со style=0 или без параметра (воркер считает 0)
	s, w := gpServerFor(t, 200, gpPart)
	out, ok := call(t, s, "guitar_part", map[string]any{"job_id": float64(7)})
	if !ok {
		t.Fatalf("guitar_part: %s", out)
	}
	if got := w.calls(); len(got) != 1 || got[0] != "GET /jobs/7/guitar_part" {
		t.Fatalf("запросы воркеру %v", got)
	}
	if v := w.query[0].Get("style"); v != "" && v != "0" {
		t.Errorf("style без аргумента = %q, want 0 (авто)", v)
	}
}

func TestGuitarPartToolAnswerHasPart(t *testing.T) {
	t.Parallel()
	// по ответу видно, что звучит около 26 с: аккорд такта, часть песни, момент удара и его ноты
	s, _ := gpServerFor(t, 200, gpPart)
	out, ok := call(t, s, "guitar_part", map[string]any{"job_id": float64(5), "style": float64(3)})
	if !ok {
		t.Fatalf("guitar_part: %s", out)
	}
	for _, want := range []string{"F#m", "chorus", "26.13", "42", "66"} {
		if !strings.Contains(out, want) {
			t.Errorf("ответ без %q: %s", want, out)
		}
	}
}

func TestGuitarPartToolBadArgs(t *testing.T) {
	t.Parallel()
	for _, args := range []map[string]any{
		{}, {"job_id": float64(0)}, {"job_id": float64(-3)}, {"style": float64(1)},
		{"job_id": float64(5), "style": float64(5)}, {"job_id": float64(5), "style": float64(-1)},
	} {
		s, w := gpServerFor(t, 200, gpPart)
		out, ok := call(t, s, "guitar_part", args)
		if ok {
			t.Errorf("%v: want ошибку, ответ %s", args, out)
		}
		if got := w.calls(); len(got) != 0 {
			t.Errorf("%v: воркер вызван %v, want нет", args, got)
		}
	}
}

func TestGuitarPartToolWorkerErrorText(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		status int
		detail string
	}{
		{422, "у трека нет плана (score.abc) — сделайте транскрипцию"},
		{422, "у трека нет дорожки гитары"},
	} {
		b, _ := json.Marshal(map[string]string{"detail": tc.detail})
		s, _ := gpServerFor(t, tc.status, string(b))
		out, ok := call(t, s, "guitar_part", map[string]any{"job_id": float64(5)})
		if ok {
			t.Fatalf("%d: want ошибку, ответ %s", tc.status, out)
		}
		if !strings.Contains(out, tc.detail) {
			t.Errorf("%d: ошибка %q, want с текстом воркера %q", tc.status, out, tc.detail)
		}
	}
}
