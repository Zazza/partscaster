package mcp

import (
	"strings"
	"testing"
)

// Карточка internal-vram-context, усл. 5: config_get показывает «песня до ~N мин» по max_song_sec воркера
// (N — округление max_song_sec/60); воркер без поля — подсказка как раньше, без длины.
// Внешняя граница — HTTP воркера (фейк engineFakeWorker), инструментный слой не мокается.

func vramConfigGet(t *testing.T, cfg map[string]any) string {
	t.Helper()
	s, fw := newEngineServer(t)
	fw.mu.Lock()
	fw.cfg = cfg
	fw.mu.Unlock()
	out, ok := call(t, s, "config_get", map[string]any{})
	if !ok {
		t.Fatalf("config_get: %s", out)
	}
	return out
}

func vramCfg(extra map[string]any) map[string]any {
	cfg := map[string]any{
		"cpp_model": "q8", "cpp_low_vram": true, "cpp_available": true,
		"cpp_recommended": map[string]any{"model": "q8", "low_vram": true, "vram_mib": 8188},
	}
	for k, v := range extra {
		cfg[k] = v
	}
	return cfg
}

func TestVramContextConfigGetSongLength(t *testing.T) {
	cases := []struct {
		sec  float64
		want string
	}{
		{615.36, "песня до ~10 мин"}, // экономный q8, 8 ГБ
		{871.36, "песня до ~15 мин"}, // полный контекст: 14,5 → 15
		{216, "песня до ~4 мин"},     // контекст 8192: 3,6 → 4
	}
	for _, c := range cases {
		out := vramConfigGet(t, vramCfg(map[string]any{"max_song_sec": c.sec, "cpp_context": 18176}))
		if !strings.Contains(out, c.want) {
			t.Errorf("max_song_sec=%v: нет %q:\n%s", c.sec, c.want, out)
		}
		// поле воркера доходит до агента как есть (паритет: клиент не теряет поля /config)
		if !strings.Contains(out, "max_song_sec") || !strings.Contains(out, "cpp_context") {
			t.Errorf("config_get потерял max_song_sec/cpp_context воркера:\n%s", out)
		}
		// подсказка о модели осталась
		if !strings.Contains(strings.ToLower(out), "рекоменд") {
			t.Errorf("подсказка о рекомендуемой модели пропала:\n%s", out)
		}
	}
}

func TestVramContextConfigGetWithoutSongLength(t *testing.T) {
	out := vramConfigGet(t, vramCfg(nil))
	if strings.Contains(out, "песня до") {
		t.Errorf("старый воркер без max_song_sec — длины быть не должно:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "рекоменд") {
		t.Errorf("подсказка о модели пропала у воркера без max_song_sec:\n%s", out)
	}
}
