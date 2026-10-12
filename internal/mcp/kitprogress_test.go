package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// инструмент fx_kit_progress зарегистрирован и показывает прогресс воркера (FxKitProgress).
// Воркер — kitProgressFake поверх fakeService (server_test.go).

type kitProgressFake struct {
	*fakeService
	calls int
	resp  map[string]any
	err   error
}

func (f *kitProgressFake) FxKitProgress(ctx context.Context) (map[string]any, error) {
	f.calls++
	return f.resp, f.err
}

func newKitProgressServer(t *testing.T, resp map[string]any, err error) (*Server, *kitProgressFake) {
	t.Helper()
	s, base := newTestServer(t)
	fake := &kitProgressFake{fakeService: base, resp: resp, err: err}
	s.client = fake
	return s, fake
}

// Идёт установка — вывод содержит набор, часть, done и total из ответа воркера.
func TestFxKitProgressToolShowsProgress(t *testing.T) {
	t.Parallel()
	s, fake := newKitProgressServer(t, map[string]any{
		"name": "vsco-violin", "part": "pizz", "done": float64(7), "total": float64(31), "bytes": float64(12345678),
	}, nil)
	out, ok := call(t, s, "fx_kit_progress", map[string]any{})
	if !ok {
		t.Fatalf("fx_kit_progress failed: %s", out)
	}
	if fake.calls == 0 {
		t.Error("FxKitProgress не вызван")
	}
	for _, want := range []string{"vsco-violin", "pizz", "7", "31"} {
		if !strings.Contains(out, want) {
			t.Errorf("в выводе нет %q: %s", want, out)
		}
	}
}

// Ошибка воркера — ошибка инструмента с причиной.
func TestFxKitProgressToolWorkerError(t *testing.T) {
	t.Parallel()
	s, _ := newKitProgressServer(t, nil, errors.New("воркер недоступен"))
	out, ok := call(t, s, "fx_kit_progress", map[string]any{})
	if ok {
		t.Fatalf("want ошибку, got: %s", out)
	}
	if !strings.Contains(out, "воркер недоступен") {
		t.Errorf("в ошибке нет причины: %s", out)
	}
}
