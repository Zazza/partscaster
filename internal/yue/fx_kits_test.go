package yue

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// internal-studio-engine, условие 14 (этап 5а): InstallFxKit — POST /fx/kits/install?name=
// без тела; ответ воркера {name, parts, downloaded} — как есть; 422 — ошибка с причиной.

func TestInstallFxKitPostsName(t *testing.T) {
	t.Parallel()
	var method, path, name string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, name = r.Method, r.URL.Path, r.URL.Query().Get("name")
		_, _ = w.Write([]byte(`{"name":"osdk","parts":{"kick":22,"snare":37},"downloaded":true}`))
	}))
	defer srv.Close()

	got, err := New(srv.URL).InstallFxKit(context.Background(), "osdk")
	if err != nil {
		t.Fatalf("InstallFxKit: %v", err)
	}
	if method != http.MethodPost || path != "/fx/kits/install" || name != "osdk" {
		t.Errorf("запрос: %s %s name=%q, want POST /fx/kits/install name=osdk", method, path, name)
	}
	parts, _ := got["parts"].(map[string]any)
	if got["name"] != "osdk" || got["downloaded"] != true || parts["kick"] != float64(22) {
		t.Errorf("ответ: %v", got)
	}
}
