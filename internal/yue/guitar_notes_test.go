package yue

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// Спецификация (internal-guitar-notes, «Устройство (контракт)»):
//   - GuitarNotes(ctx, jobID, force) — POST /jobs/{id}/guitar_notes; force=true → ?force=1, иначе force не включён
//     (воркер отдаёт сохранённое распознавание трека);
//   - ответ воркера {"notes": N, "engines": [...], "cached": bool} доходит до вызывающего;
//   - ошибки воркера (404 нет джобы, 422 нет дорожки гитары, 503 не установлено / нет видеопамяти) — ошибка
//     с текстом воркера, чтобы приложение показало понятную причину.

type gnReq struct {
	method string
	path   string
	query  url.Values
	n      int
}

func gnServer(t *testing.T, status int, resp string, got *gnReq) *Client {
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

// asMap — ответ клиента как JSON-объект (форма результата — по JSON-полям воркера).
func asMap(t *testing.T, v any) map[string]any {
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

func forceOn(q url.Values) bool {
	v := strings.ToLower(q.Get("force"))
	return v == "1" || v == "true"
}

func TestGuitarNotesRequestWithoutForce(t *testing.T) {
	t.Parallel()
	var got gnReq
	c := gnServer(t, 200, `{"notes":431,"engines":["yourmt3","basic-pitch"],"cached":true}`, &got)
	res, err := c.GuitarNotes(context.Background(), 919, false)
	if err != nil {
		t.Fatalf("GuitarNotes: %v", err)
	}
	if got.n != 1 || got.method != http.MethodPost || got.path != "/jobs/919/guitar_notes" {
		t.Fatalf("запрос %d× %s %s, want 1× POST /jobs/919/guitar_notes", got.n, got.method, got.path)
	}
	if forceOn(got.query) {
		t.Fatalf("без force запрос не должен просить пересчёт: ?%s", got.query.Encode())
	}
	m := asMap(t, res)
	if m["notes"] != float64(431) {
		t.Errorf("notes %v, want 431 (ответ %v)", m["notes"], m)
	}
	eng, _ := m["engines"].([]any)
	if len(eng) != 2 || eng[0] != "yourmt3" || eng[1] != "basic-pitch" {
		t.Errorf("engines %v, want [yourmt3 basic-pitch]", m["engines"])
	}
	if m["cached"] != true {
		t.Errorf("cached %v, want true", m["cached"])
	}
}

func TestGuitarNotesForceQuery(t *testing.T) {
	t.Parallel()
	var got gnReq
	c := gnServer(t, 200, `{"notes":12,"engines":["basic-pitch"],"cached":false}`, &got)
	res, err := c.GuitarNotes(context.Background(), 7, true)
	if err != nil {
		t.Fatalf("GuitarNotes: %v", err)
	}
	if got.method != http.MethodPost || got.path != "/jobs/7/guitar_notes" {
		t.Fatalf("запрос %s %s", got.method, got.path)
	}
	if !forceOn(got.query) {
		t.Fatalf("force=true → ?force=1, а запрос ?%s", got.query.Encode())
	}
	m := asMap(t, res)
	if m["notes"] != float64(12) {
		t.Errorf("notes %v, want 12", m["notes"])
	}
	// только basic-pitch: YourMT3+ не установлен — приложение должно это видеть
	if eng, _ := m["engines"].([]any); len(eng) != 1 || eng[0] != "basic-pitch" {
		t.Errorf("engines %v, want [basic-pitch]", m["engines"])
	}
	if cv, ok := m["cached"]; ok && cv != false {
		t.Errorf("cached %v, want false (или нет поля)", cv)
	}
}

// Ошибки воркера доходят текстом: не установлено, нет видеопамяти, нет дорожки гитары, нет джобы.
func TestGuitarNotesWorkerErrorsCarryText(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		status int
		detail string
	}{
		{503, "распознавание нот не установлено: worker/notes_install.sh"},
		{503, "не хватает видеопамяти: освободите видеопамять и повторите"},
		{422, "у трека нет дорожки гитары"},
		{404, "job not found"},
	} {
		var got gnReq
		b, _ := json.Marshal(map[string]string{"detail": tc.detail})
		c := gnServer(t, tc.status, string(b), &got)
		_, err := c.GuitarNotes(context.Background(), 3, false)
		if err == nil {
			t.Fatalf("%d %q: want ошибку", tc.status, tc.detail)
		}
		if !strings.Contains(err.Error(), tc.detail) {
			t.Errorf("%d: ошибка %q, want с текстом воркера %q", tc.status, err, tc.detail)
		}
	}
}
