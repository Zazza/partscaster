package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"partscaster/internal/yue"
)

// Спецификация audioProxy (ассет-сервер webview → воркер, тот же origin):
//   - /audio/{id}/{file} отдаёт файл воркера как есть (200, тип по файлу, Accept-Ranges: bytes);
//   - Range от <audio> доходит до воркера; ответ 206 с Content-Range/Content-Length и
//     нужным куском — без этого не работает перемотка;
//   - не-/audio пути, без номера или без файла — 404, к воркеру не идут;
//   - имя с «..»/«/» до воркера не доходит (не выйти из папки трека);
//   - воркер не отдал файл/недоступен — не 2xx.

const proxyBody = "0123456789abcdefghij" // 20 байт

type proxyWorker struct {
	mu   sync.Mutex
	reqs []*http.Request
	srv  *httptest.Server
}

func newProxyWorker(t *testing.T) *proxyWorker {
	t.Helper()
	w := &proxyWorker{}
	w.srv = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		w.mu.Lock()
		w.reqs = append(w.reqs, r.Clone(r.Context()))
		w.mu.Unlock()
		if r.URL.Path != "/audio/7/audio.flac" {
			http.NotFound(rw, r)
			return
		}
		rw.Header().Set("Content-Type", "audio/flac")
		http.ServeContent(rw, r, "audio.flac", time.Unix(0, 0), strings.NewReader(proxyBody))
	}))
	t.Cleanup(w.srv.Close)
	return w
}

func (w *proxyWorker) requests() []*http.Request {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]*http.Request(nil), w.reqs...)
}

func proxyGet(t *testing.T, h http.Handler, path, rng string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "http://wails.localhost"+path, nil)
	if rng != "" {
		req.Header.Set("Range", rng)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAudioProxyFullFile(t *testing.T) {
	w := newProxyWorker(t)
	h := &audioProxy{client: yue.New(w.srv.URL)}
	rec := proxyGet(t, h, "/audio/7/audio.flac", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != proxyBody {
		t.Fatalf("body %q", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "audio/flac" {
		t.Fatalf("Content-Type %q", got)
	}
	if got := rec.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Fatalf("Accept-Ranges %q — webview не будет перематывать", got)
	}
}

func TestAudioProxyRangePassedThrough(t *testing.T) {
	w := newProxyWorker(t)
	h := &audioProxy{client: yue.New(w.srv.URL)}
	rec := proxyGet(t, h, "/audio/7/audio.flac", "bytes=5-9")
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status %d, want 206", rec.Code)
	}
	if rec.Body.String() != "56789" {
		t.Fatalf("body %q, want кусок 5-9", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Range"); got != "bytes 5-9/20" {
		t.Fatalf("Content-Range %q", got)
	}
	if got := rec.Header().Get("Content-Length"); got != "5" {
		t.Fatalf("Content-Length %q", got)
	}
	reqs := w.requests()
	if len(reqs) != 1 || reqs[0].Header.Get("Range") != "bytes=5-9" {
		t.Fatalf("Range до воркера не дошёл: %v", reqs)
	}
}

func TestAudioProxyOpenEndedRange(t *testing.T) {
	w := newProxyWorker(t)
	h := &audioProxy{client: yue.New(w.srv.URL)}
	rec := proxyGet(t, h, "/audio/7/audio.flac", "bytes=15-")
	if rec.Code != http.StatusPartialContent || rec.Body.String() != "fghij" {
		t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Range"); got != "bytes 15-19/20" {
		t.Fatalf("Content-Range %q", got)
	}
}

func TestAudioProxyBadPathsNotForwarded(t *testing.T) {
	w := newProxyWorker(t)
	h := &audioProxy{client: yue.New(w.srv.URL)}
	for _, p := range []string{
		"/", "/index.html", "/audio/", "/audio/x/audio.flac", "/audio/7/", "/audio/7",
	} {
		rec := proxyGet(t, h, p, "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", p, rec.Code)
		}
	}
	if n := len(w.requests()); n != 0 {
		t.Fatalf("к воркеру ушло %d запросов на плохие пути", n)
	}
}

func TestAudioProxyTraversalNeverReachesWorker(t *testing.T) {
	w := newProxyWorker(t)
	h := &audioProxy{client: yue.New(w.srv.URL)}
	for _, p := range []string{
		"/audio/7/../8/audio.flac",
		"/audio/7/..%2F..%2Fyue.db",
		"/audio/7/sub/audio.flac",
		"/audio/7/..",
	} {
		rec := proxyGet(t, h, p, "")
		if rec.Code >= 200 && rec.Code < 300 {
			t.Errorf("%s: status %d", p, rec.Code)
		}
	}
	for _, r := range w.requests() {
		if strings.Contains(r.URL.Path, "..") || strings.Count(r.URL.Path, "/") != 3 {
			t.Errorf("до воркера дошёл путь вне папки трека: %q", r.URL.Path)
		}
	}
}

func TestAudioProxyWorkerMissingOrDown(t *testing.T) {
	w := newProxyWorker(t)
	h := &audioProxy{client: yue.New(w.srv.URL)}
	rec := proxyGet(t, h, "/audio/7/nope.flac", "")
	if rec.Code >= 200 && rec.Code < 300 {
		t.Fatalf("нет файла на воркере: status %d", rec.Code)
	}
	down := httptest.NewServer(http.NotFoundHandler())
	url := down.URL
	down.Close()
	h = &audioProxy{client: yue.New(url)}
	rec = proxyGet(t, h, "/audio/7/audio.flac", "")
	if rec.Code >= 200 && rec.Code < 300 {
		t.Fatalf("воркер недоступен: status %d", rec.Code)
	}
	if b, _ := io.ReadAll(rec.Body); bytes.Contains(b, []byte(proxyBody)) {
		t.Fatal("тело при ошибке")
	}
}
