package main

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"

	"partscaster/internal/welcome"
)

// ТК5: YuePlayWelcomeClip играет встроенный клип окна «Что это» по имени: Load под
// отдельным id окна (не джоба и не круг фраз) с байтами клипа и длительностью durSec,
// затем старт; startSec>0 — Seek(startSec). Имя не из набора — ошибка, плеер не тронут.
// Воркер не нужен: у fakeService ни один метод не реализован.

// wcName — настоящее имя клипа из welcomeClips.json (первая пара, «было»).
func wcName(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("frontend/src/welcomeClips.json")
	if err != nil {
		t.Fatalf("welcomeClips.json: %v", err)
	}
	var data struct {
		Steps []struct {
			Pairs []struct {
				Before string `json:"before"`
			} `json:"pairs"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("welcomeClips.json: %v", err)
	}
	for _, s := range data.Steps {
		for _, p := range s.Pairs {
			if p.Before != "" {
				return p.Before
			}
		}
	}
	t.Fatal("в welcomeClips.json нет ни одной пары")
	return ""
}

func TestWelcomeClipIDSeparate(t *testing.T) {
	t.Parallel()
	if welcomeClipID == phraseLoopID {
		t.Fatalf("welcomeClipID = phraseLoopID = %d: окно и круг фраз неразличимы в плеере", welcomeClipID)
	}
	if welcomeClipID <= math.MaxInt32 {
		t.Fatalf("welcomeClipID = %d: может совпасть с id джобы", welcomeClipID)
	}
}

func TestPlayWelcomeClipFromStart(t *testing.T) {
	t.Parallel()
	name := wcName(t)
	want, err := welcome.Clip(name)
	if err != nil {
		t.Fatalf("welcome.Clip(%q): %v", name, err)
	}
	p := &recPlayer{}
	a := newTestApp(&fakeService{}, p)
	if err := a.YuePlayWelcomeClip(name, 0, 12.02); err != nil {
		t.Fatalf("YuePlayWelcomeClip: %v", err)
	}
	if len(p.loaded) != 1 || p.loaded[0] != welcomeClipID {
		t.Fatalf("Load: ids=%v, want один раз welcomeClipID=%d", p.loaded, welcomeClipID)
	}
	if !bytes.Equal(p.data[0], want) {
		t.Errorf("в плеер загружены не байты клипа %q (%d байт против %d)", name, len(p.data[0]), len(want))
	}
	if d := p.durs[0]; math.Abs(d.Seconds()-12.02) > 0.001 {
		t.Errorf("длительность в Load = %v, want 12.02s", d)
	}
	if p.plays == 0 {
		t.Error("Play не вызван: клип не заиграл")
	}
	for _, s := range p.seeked {
		if s != 0 {
			t.Errorf("startSec=0, а Seek(%v)", s)
		}
	}
}

func TestPlayWelcomeClipSeeksToStart(t *testing.T) {
	t.Parallel()
	name := wcName(t)
	p := &recPlayer{}
	a := newTestApp(&fakeService{}, p)
	if err := a.YuePlayWelcomeClip(name, 7.3, 12.02); err != nil {
		t.Fatalf("YuePlayWelcomeClip: %v", err)
	}
	if len(p.loaded) != 1 || p.loaded[0] != welcomeClipID {
		t.Fatalf("Load: ids=%v, want один раз welcomeClipID=%d", p.loaded, welcomeClipID)
	}
	if len(p.seeked) == 0 {
		t.Fatal("startSec=7.3, а Seek не вызван")
	}
	if got := p.seeked[len(p.seeked)-1]; math.Abs(got.Seconds()-7.3) > 0.001 {
		t.Errorf("Seek(%v), want 7.3s", got)
	}
}

func TestPlayWelcomeClipUnknownNameIsError(t *testing.T) {
	t.Parallel()
	for _, n := range []string{"", "no-such-clip.mp3", "../x", "clips/x", "/etc/passwd"} {
		p := &recPlayer{}
		a := newTestApp(&fakeService{}, p)
		if err := a.YuePlayWelcomeClip(n, 0, 10); err == nil {
			t.Errorf("YuePlayWelcomeClip(%q): want ошибку, got nil", n)
		}
		if len(p.loaded) > 0 || p.plays > 0 || len(p.seeked) > 0 {
			t.Errorf("имя %q: плеер тронут: loaded=%v plays=%d seeked=%v", n, p.loaded, p.plays, p.seeked)
		}
	}
}

// Встроенные клипы окна плеер сохраняет как .mp3 (связка ТК5 и ТК6): иначе
// заголовок клипа не распознан и файл уходит в плеер под чужим расширением.
func TestWelcomeClipsLoadAsMP3(t *testing.T) {
	t.Parallel()
	for _, n := range welcome.Names() {
		b, err := welcome.Clip(n)
		if err != nil {
			t.Fatalf("welcome.Clip(%q): %v", n, err)
		}
		p := &player{volume: 0.8}
		if err := p.Load(welcomeClipID, b, time.Second); err != nil {
			t.Fatalf("Load(%q): %v", n, err)
		}
		got := p.tmpFile
		p.Stop()
		if len(got) < 4 || got[len(got)-4:] != ".mp3" {
			t.Errorf("клип %q: временный файл %q, want суффикс .mp3", n, got)
		}
	}
}
