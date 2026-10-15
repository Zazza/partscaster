package yue

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// Спецификация (internal-guitar-plan-chords, условие 9 и «Устройство (контракт)»: Go):
//   - GuitarPart(ctx, id, style) — GET /jobs/{id}/guitar_part?style=N (N — манера 0…4 как есть);
//   - ответ воркера {style, bars, strikes, beats, tune_cents} доходит до вызывающего;
//   - ошибки воркера (422 «нет плана — сделайте транскрипцию», 422 нет дорожки гитары, 404) — ошибка
//     с текстом воркера, чтобы приложение и MCP показали понятную причину.
// Граница — HTTP воркера (httptest).

type gpReq struct {
	method string
	path   string
	query  url.Values
	n      int
}

func gpServer(t *testing.T, status int, resp string, got *gpReq) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		got.n++
		got.method, got.path, got.query = r.Method, r.URL.Path, r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL)
}

// gpMap — ответ клиента как JSON-объект (форма результата — по JSON-полям воркера).
func gpMap(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %v: %v", v, err)
	}
	m := map[string]any{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("ответ не объект: %s", b)
	}
	return m
}

const gpResp = `{"style":2,"bars":[{"start":0.5,"end":2.5,"chord":"G","section":"verse"}],` +
	`"strikes":[{"t":0.52,"midi":[43]},{"t":1.0,"midi":[50]}],"beats":[0.5,1.0,1.5,2.0],"tune_cents":-12.5}`

func TestGuitarPartRequest(t *testing.T) {
	t.Parallel()
	for _, style := range []int{0, 1, 2, 3, 4} {
		var got gpReq
		c := gpServer(t, 200, gpResp, &got)
		if _, err := c.GuitarPart(context.Background(), 919, style); err != nil {
			t.Fatalf("style %d: GuitarPart: %v", style, err)
		}
		if got.n != 1 || got.method != http.MethodGet || got.path != "/jobs/919/guitar_part" {
			t.Fatalf("style %d: запрос %d× %s %s, want 1× GET /jobs/919/guitar_part", style, got.n, got.method, got.path)
		}
		want := []string{"0", "1", "2", "3", "4"}[style]
		if got.query.Get("style") != want {
			t.Errorf("style %d: ?%s, want style=%s", style, got.query.Encode(), want)
		}
	}
}

func TestGuitarPartResponsePassedThrough(t *testing.T) {
	t.Parallel()
	var got gpReq
	c := gpServer(t, 200, gpResp, &got)
	res, err := c.GuitarPart(context.Background(), 5, 2)
	if err != nil {
		t.Fatalf("GuitarPart: %v", err)
	}
	m := gpMap(t, res)
	if m["style"] != float64(2) {
		t.Errorf("style %v, want 2 (ответ %v)", m["style"], m)
	}
	bars, _ := m["bars"].([]any)
	if len(bars) != 1 {
		t.Fatalf("bars %v, want 1 такт", m["bars"])
	}
	if b, _ := bars[0].(map[string]any); b["chord"] != "G" || b["section"] != "verse" {
		t.Errorf("такт %v, want chord G, section verse", bars[0])
	}
	strikes, _ := m["strikes"].([]any)
	if len(strikes) != 2 {
		t.Fatalf("strikes %v, want 2 удара", m["strikes"])
	}
	s0, _ := strikes[0].(map[string]any)
	if s0["t"] != 0.52 {
		t.Errorf("удар %v, want t 0.52", s0)
	}
	if midi, _ := s0["midi"].([]any); len(midi) != 1 || midi[0] != float64(43) {
		t.Errorf("ноты удара %v, want [43]", s0["midi"])
	}
	if m["tune_cents"] != -12.5 {
		t.Errorf("tune_cents %v, want -12.5", m["tune_cents"])
	}
	if beats, _ := m["beats"].([]any); len(beats) != 4 {
		t.Errorf("beats %v, want 4 доли", m["beats"])
	}
}

// Ошибки воркера доходят текстом: нет плана (сделайте транскрипцию), нет дорожки гитары, нет джобы.
func TestGuitarPartWorkerErrorsCarryText(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		status int
		detail string
	}{
		{422, "у трека нет плана (score.abc) — сделайте транскрипцию"},
		{422, "у трека нет дорожки гитары"},
		{404, "job not found"},
	} {
		var got gpReq
		b, _ := json.Marshal(map[string]string{"detail": tc.detail})
		c := gpServer(t, tc.status, string(b), &got)
		_, err := c.GuitarPart(context.Background(), 3, 0)
		if err == nil {
			t.Fatalf("%d %q: want ошибку", tc.status, tc.detail)
		}
		if !strings.Contains(err.Error(), tc.detail) {
			t.Errorf("%d: ошибка %q, want с текстом воркера %q", tc.status, err, tc.detail)
		}
	}
}

// Распознавание удалено (условие 9): у клиента и интерфейса Service нет GuitarNotes, есть GuitarPart.
// «Метода нет» компиляцией не проверить — проверка по набору методов через рефлексию.
func TestGuitarNotesMethodRemoved(t *testing.T) {
	t.Parallel()
	if hasMethod(New("http://127.0.0.1:1"), "GuitarNotes") {
		t.Fatal("у yue.Client остался метод GuitarNotes — распознавание нот должно быть удалено")
	}
	if !hasMethod(New("http://127.0.0.1:1"), "GuitarPart") {
		t.Fatal("у yue.Client нет метода GuitarPart")
	}
	svc := reflect.TypeOf((*Service)(nil)).Elem()
	if _, ok := svc.MethodByName("GuitarNotes"); ok {
		t.Error("в yue.Service остался GuitarNotes")
	}
	if _, ok := svc.MethodByName("GuitarPart"); !ok {
		t.Error("в yue.Service нет GuitarPart")
	}
}

func hasMethod(v any, name string) bool {
	_, ok := reflect.TypeOf(v).MethodByName(name)
	return ok
}
