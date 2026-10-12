package yue

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Тесты карточки internal-own-track, этап 20, условие 164 (тест-кейс ТК194, клиент Go).
// Написаны по карточке («Договор интерфейсов этапа 20»), без чтения реализации:
//   - yue.Service: InstallEngineModel(ctx, name) (map[string]any, error) →
//     POST /engine/models/install?name=<имя>; ответ воркера — как есть;
//     статус ≥ 400 → ошибка, в тексте — деталь воркера;
//   - EngineModelProgress(ctx) (map[string]any, error) → GET /engine/models/progress:
//     {name, file, done, total} во время загрузки или {} без неё.
// Воркер — фейковый HTTP-сервер (httptest), как в client_test.go.

// engineReq — запрос, который получил фейковый воркер.
type engineReq struct {
	Method, Path, Name string
}

func engineWorker(t *testing.T, status int, body string) (*httptest.Server, *[]engineReq) {
	t.Helper()
	var mu sync.Mutex
	got := &[]engineReq{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*got = append(*got, engineReq{r.Method, r.URL.Path, r.URL.Query().Get("name")})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

// Методы входят в интерфейс yue.Service (паритет условия 164), а не только в *Client.
func TestEngineModelMethodsInService(t *testing.T) {
	t.Parallel()
	var s Service = New("http://127.0.0.1:1")
	_ = s.InstallEngineModel
	_ = s.EngineModelProgress
}

func TestInstallEngineModelPostsName(t *testing.T) {
	t.Parallel()
	srv, got := engineWorker(t, http.StatusOK,
		`{"name":"q8","file":"YuE2-3B-Q8_0.gguf","installed":true,"size":3810232064}`)
	out, err := New(srv.URL).InstallEngineModel(context.Background(), "q8")
	if err != nil {
		t.Fatalf("InstallEngineModel: %v", err)
	}
	if len(*got) != 1 {
		t.Fatalf("воркеру ушло %d запросов, want 1: %+v", len(*got), *got)
	}
	r := (*got)[0]
	if r.Method != http.MethodPost || r.Path != "/engine/models/install" || r.Name != "q8" {
		t.Fatalf("запрос %+v, want POST /engine/models/install?name=q8", r)
	}
	if out["name"] != "q8" || out["file"] != "YuE2-3B-Q8_0.gguf" || out["installed"] != true {
		t.Fatalf("ответ воркера потерян: %v", out)
	}
}

// Другое имя — другое имя в запросе (имя не зашито).
func TestInstallEngineModelOtherName(t *testing.T) {
	t.Parallel()
	srv, got := engineWorker(t, http.StatusOK, `{"name":"q5"}`)
	if _, err := New(srv.URL).InstallEngineModel(context.Background(), "q5"); err != nil {
		t.Fatalf("InstallEngineModel: %v", err)
	}
	if len(*got) != 1 || (*got)[0].Name != "q5" {
		t.Fatalf("запросы %+v, want name=q5", *got)
	}
}

func TestInstallEngineModelWorkerErrorCarriesDetail(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		status int
		detail string
	}{
		{http.StatusBadGateway, "sha256 mismatch"},
		{http.StatusUnprocessableEntity, "неизвестная модель q4"},
	} {
		srv, _ := engineWorker(t, tc.status, `{"detail":"`+tc.detail+`"}`)
		out, err := New(srv.URL).InstallEngineModel(context.Background(), "q8")
		if err == nil {
			t.Fatalf("статус %d: ошибки нет, ответ %v", tc.status, out)
		}
		if !strings.Contains(err.Error(), tc.detail) {
			t.Fatalf("статус %d: деталь воркера %q потеряна: %v", tc.status, tc.detail, err)
		}
	}
}

func TestEngineModelProgressDuringDownload(t *testing.T) {
	t.Parallel()
	srv, got := engineWorker(t, http.StatusOK,
		`{"name":"q8","file":"YuE2-3B-Q8_0.gguf","done":1000,"total":3810232064}`)
	out, err := New(srv.URL).EngineModelProgress(context.Background())
	if err != nil {
		t.Fatalf("EngineModelProgress: %v", err)
	}
	if len(*got) != 1 || (*got)[0].Method != http.MethodGet || (*got)[0].Path != "/engine/models/progress" {
		t.Fatalf("запросы %+v, want GET /engine/models/progress", *got)
	}
	if out["name"] != "q8" || out["file"] != "YuE2-3B-Q8_0.gguf" {
		t.Fatalf("ответ потерян: %v", out)
	}
	if d, _ := out["done"].(float64); d != 1000 {
		t.Fatalf("done = %v, want 1000", out["done"])
	}
	if tot, _ := out["total"].(float64); tot != 3810232064 {
		t.Fatalf("total = %v, want 3810232064", out["total"])
	}
}

// Нет загрузки — воркер отдаёт {} → пустой ответ без ошибки.
func TestEngineModelProgressIdleIsEmpty(t *testing.T) {
	t.Parallel()
	srv, _ := engineWorker(t, http.StatusOK, `{}`)
	out, err := New(srv.URL).EngineModelProgress(context.Background())
	if err != nil {
		t.Fatalf("EngineModelProgress: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("без загрузки ответ не пуст: %v", out)
	}
}
