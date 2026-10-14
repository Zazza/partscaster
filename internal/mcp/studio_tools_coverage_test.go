package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"partscaster/internal/config"
)

// Спецификация инструментов MCP:
//   - dsp_variant_delete — деструктивный: без confirm=true вариант не удаляется (воркер не
//     вызывается, ошибка просит подтверждения); с confirm — удаляется ровно указанный файл трека;
//   - config_set: server_url — адрес воркера (без хвостового «/»), сохраняется в настройки
//     приложения; ollama_url/ollama_model/stems_model/fx_engine уходят воркеру как заданы
//     (fx_engine=false — тоже значение, не «не задано»); незаданное не трогается;
//     воркер отказал — ошибка;
//   - make_stems: job_id и разовая model доходят до воркера; после дорожек кэш превью этого
//     трека устаревает (следующее превью качает новые дорожки); ошибка воркера — ошибка.

type studioCovFake struct {
	*fakeService
	dspDeleted []string // "<id>/<file>"
	stems      []stemsCall
	stemsErr   error
	workerCfg  []map[string]any
	cfgErr     error
}

type stemsCall struct {
	ID    int64
	Model string
}

func (f *studioCovFake) DspVariantDelete(ctx context.Context, id int64, fname string) (bool, error) {
	f.dspDeleted = append(f.dspDeleted, fmt.Sprintf("%d/%s", id, fname))
	return true, nil
}

func (f *studioCovFake) MakeStemsWith(ctx context.Context, id int64, model string) (map[string]any, error) {
	f.stems = append(f.stems, stemsCall{id, model})
	if f.stemsErr != nil {
		return nil, f.stemsErr
	}
	return map[string]any{"stems": []string{"stem-vocals.flac"}}, nil
}

func (f *studioCovFake) WorkerConfig(ctx context.Context) (map[string]any, error) {
	return map[string]any{}, nil
}

func (f *studioCovFake) SetWorkerConfig(ctx context.Context, cfg map[string]any) error {
	f.workerCfg = append(f.workerCfg, cfg)
	return f.cfgErr
}

func newStudioCovServer(t *testing.T) (*Server, *studioCovFake) {
	t.Helper()
	// настройки приложения и кэш превью — во временных каталогах
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("AppData", dir) // Windows: os.UserConfigDir читает %AppData%
	t.Setenv("PARTSCASTER_URL", "")
	t.Setenv("TMPDIR", t.TempDir())
	fake := &studioCovFake{fakeService: &fakeService{url: "http://w:8091",
		fetch: map[string]string{"stem-vocals.flac": "STEM-v1"}}}
	s := NewServer(fake, t.TempDir())
	RegisterWorkflowTools(s)
	RegisterStudioTools(s)
	RegisterLibraryTools(s)
	RegisterInstallTools(s)
	return s, fake
}

func TestDspVariantDeleteWithoutConfirmDoesNothing(t *testing.T) {
	for name, args := range map[string]map[string]any{
		"нет confirm":   {"job_id": float64(5), "file": "dsp-warm.flac"},
		"confirm=false": {"job_id": float64(5), "file": "dsp-warm.flac", "confirm": false},
	} {
		t.Run(name, func(t *testing.T) {
			s, fake := newStudioCovServer(t)
			out, ok := call(t, s, "dsp_variant_delete", args)
			if ok {
				t.Fatalf("без подтверждения удалено: %s", out)
			}
			if !strings.Contains(out, "подтверждение") {
				t.Fatalf("ошибка не просит подтверждения: %s", out)
			}
			if len(fake.dspDeleted) != 0 {
				t.Fatalf("воркер вызван без confirm: %v", fake.dspDeleted)
			}
		})
	}
}

func TestDspVariantDeleteWithConfirmDeletesExactFile(t *testing.T) {
	s, fake := newStudioCovServer(t)
	out, ok := call(t, s, "dsp_variant_delete", map[string]any{
		"job_id": float64(5), "file": "dsp-warm.flac", "confirm": true})
	if !ok {
		t.Fatalf("с confirm не удалено: %s", out)
	}
	if !reflect.DeepEqual(fake.dspDeleted, []string{"5/dsp-warm.flac"}) {
		t.Fatalf("удалено %v, want [5/dsp-warm.flac]", fake.dspDeleted)
	}
}

func TestConfigSetServerURLAppliedAndSaved(t *testing.T) {
	s, fake := newStudioCovServer(t)
	out, ok := call(t, s, "config_set", map[string]any{"server_url": "http://gpu-host:8091/"})
	if !ok {
		t.Fatalf("config_set: %s", out)
	}
	if fake.url != "http://gpu-host:8091" {
		t.Fatalf("адрес клиента %q, want без хвостового /", fake.url)
	}
	if got := config.Load().YueURL; got != "http://gpu-host:8091" {
		t.Fatalf("в настройках приложения %q — после перезапуска адрес потеряется", got)
	}
	if len(fake.workerCfg) != 0 {
		t.Fatalf("только адрес — воркеру настройки не шлются: %v", fake.workerCfg)
	}
	out, ok = call(t, s, "config_get", map[string]any{})
	if !ok || !strings.Contains(out, "http://gpu-host:8091") {
		t.Fatalf("config_get не видит адрес: %s", out)
	}
}

func TestConfigSetWorkerSettingsPassedAsGiven(t *testing.T) {
	s, fake := newStudioCovServer(t)
	out, ok := call(t, s, "config_set", map[string]any{
		"ollama_url": "http://ollama:11434", "ollama_model": "qwen3:8b",
		"stems_model": "roformer", "fx_engine": false,
	})
	if !ok {
		t.Fatalf("config_set: %s", out)
	}
	want := []map[string]any{{"ollama_url": "http://ollama:11434", "ollama_model": "qwen3:8b",
		"stems_model": "roformer", "fx_engine": false}}
	if !reflect.DeepEqual(fake.workerCfg, want) {
		t.Fatalf("воркеру ушло %v, want %v", fake.workerCfg, want)
	}
	if fake.url != "http://w:8091" {
		t.Fatalf("адрес воркера изменён без server_url: %q", fake.url)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "partscaster", "settings.json")); err == nil {
		t.Fatal("настройки приложения перезаписаны без server_url")
	}
}

func TestConfigSetOnlyGivenKeys(t *testing.T) {
	s, fake := newStudioCovServer(t)
	if out, ok := call(t, s, "config_set", map[string]any{"fx_engine": true}); !ok {
		t.Fatalf("config_set: %s", out)
	}
	want := []map[string]any{{"fx_engine": true}}
	if !reflect.DeepEqual(fake.workerCfg, want) {
		t.Fatalf("воркеру ушло %v, want %v", fake.workerCfg, want)
	}
}

func TestConfigSetEmptyTouchesNothing(t *testing.T) {
	s, fake := newStudioCovServer(t)
	if out, ok := call(t, s, "config_set", map[string]any{}); !ok {
		t.Fatalf("config_set: %s", out)
	}
	if len(fake.workerCfg) != 0 || fake.url != "http://w:8091" {
		t.Fatalf("пустой вызов что-то поменял: %v %q", fake.workerCfg, fake.url)
	}
}

func TestConfigSetWorkerRefusalIsError(t *testing.T) {
	s, fake := newStudioCovServer(t)
	fake.cfgErr = errors.New("422: stems_model")
	out, ok := call(t, s, "config_set", map[string]any{"stems_model": "nope"})
	if ok {
		t.Fatalf("отказ воркера скрыт: %s", out)
	}
	if !strings.Contains(out, "422") {
		t.Fatalf("причина потеряна: %s", out)
	}
}

func TestMakeStemsPassesJobAndModel(t *testing.T) {
	for _, tc := range []struct {
		args map[string]any
		want stemsCall
	}{
		{map[string]any{"job_id": float64(42)}, stemsCall{42, ""}},
		{map[string]any{"job_id": float64(42), "model": "roformer"}, stemsCall{42, "roformer"}},
		{map[string]any{"job_id": float64(9), "model": "htdemucs"}, stemsCall{9, "htdemucs"}},
	} {
		s, fake := newStudioCovServer(t)
		out, ok := call(t, s, "make_stems", tc.args)
		if !ok {
			t.Fatalf("%v: %s", tc.args, out)
		}
		if !reflect.DeepEqual(fake.stems, []stemsCall{tc.want}) {
			t.Fatalf("%v: до воркера дошло %v, want %v", tc.args, fake.stems, tc.want)
		}
		if !strings.Contains(out, "stem-vocals.flac") {
			t.Fatalf("ответ воркера не возвращён: %s", out)
		}
	}
}

func TestMakeStemsErrorIsError(t *testing.T) {
	s, fake := newStudioCovServer(t)
	fake.stemsErr = errors.New("503: demucs")
	out, ok := call(t, s, "make_stems", map[string]any{"job_id": float64(3)})
	if ok {
		t.Fatalf("ошибка воркера скрыта: %s", out)
	}
}

func TestMakeStemsRefreshesPreviewCache(t *testing.T) {
	s, fake := newStudioCovServer(t)
	ctx := context.Background()
	read := func() string {
		t.Helper()
		p, err := s.fxCache().Fetch(ctx, fake, 42, "stem-vocals.flac")
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if got := read(); got != "STEM-v1" {
		t.Fatalf("кэш: %q", got)
	}
	// дорожки пересобраны на воркере
	fake.fetch["stem-vocals.flac"] = "STEM-v2"
	if out, ok := call(t, s, "make_stems", map[string]any{"job_id": float64(42)}); !ok {
		t.Fatalf("make_stems: %s", out)
	}
	if got := read(); got != "STEM-v2" {
		t.Fatalf("после make_stems превью играет старую дорожку: %q", got)
	}
}

func TestMakeStemsKeepsOtherTracksCache(t *testing.T) {
	s, fake := newStudioCovServer(t)
	ctx := context.Background()
	if _, err := s.fxCache().Fetch(ctx, fake, 7, "stem-vocals.flac"); err != nil {
		t.Fatal(err)
	}
	n := len(fake.fetched)
	if out, ok := call(t, s, "make_stems", map[string]any{"job_id": float64(42)}); !ok {
		t.Fatalf("make_stems: %s", out)
	}
	if _, err := s.fxCache().Fetch(ctx, fake, 7, "stem-vocals.flac"); err != nil {
		t.Fatal(err)
	}
	if len(fake.fetched) != n {
		t.Fatalf("кэш чужого трека сброшен: скачиваний %d → %d", n, len(fake.fetched))
	}
}
