package mcp

// «Голос альбома»: прослушать кусок — инструмент voice_preview (internal-album-voice-draft, условие 5; ТК4).

import (
	"context"
	"strings"
	"testing"

	"partscaster/internal/yue"
)

type voicePreviewCall struct {
	jobID int64
	p     yue.VoicePreviewParams
}

type voicePreviewFake struct {
	*fakeService
	calls []voicePreviewCall
	ret   *yue.VoicePreview
	err   error
}

func (f *voicePreviewFake) VoicePreview(ctx context.Context, jobID int64, p yue.VoicePreviewParams) (*yue.VoicePreview, error) {
	f.calls = append(f.calls, voicePreviewCall{jobID: jobID, p: p})
	if f.err != nil {
		return nil, f.err
	}
	return f.ret, nil
}

func newVoicePreviewServer(t *testing.T, ret *yue.VoicePreview, err error) (*Server, *voicePreviewFake) {
	t.Helper()
	fake := &voicePreviewFake{fakeService: &fakeService{url: "http://w:8091"}, ret: ret, err: err}
	s := NewServer(fake, t.TempDir())
	RegisterWorkflowTools(s)
	RegisterStudioTools(s)
	RegisterLibraryTools(s)
	RegisterInstallTools(s)
	return s, fake
}

func TestVoicePreviewToolAllParams(t *testing.T) {
	t.Parallel()
	s, fake := newVoicePreviewServer(t, &yue.VoicePreview{File: "preview-vc-ab12cd34.flac", From: 2, To: 8.5}, nil)
	out, ok := call(t, s, "voice_preview", map[string]any{
		"job_id": float64(214), "ref_job_id": float64(135), "from": float64(2), "to": 8.5,
		"ref_from": 12.5, "ref_dur": float64(20), "steps": float64(60),
	})
	if !ok {
		t.Fatalf("voice_preview failed: %s", out)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("calls %d", len(fake.calls))
	}
	c := fake.calls[0]
	want := yue.VoicePreviewParams{RefJobID: 135, From: 2, To: 8.5, RefFrom: 12.5, RefDur: 20, Steps: 60}
	if c.jobID != 214 || c.p != want {
		t.Fatalf("args job=%d %+v, want 214 %+v", c.jobID, c.p, want)
	}
	if !strings.Contains(out, "preview-vc-ab12cd34.flac") {
		t.Fatalf("out must contain preview file name: %s", out)
	}
}

func TestVoicePreviewToolFromZeroAndDefaults(t *testing.T) {
	t.Parallel()
	// from = 0 — начало трека, это не «не задано»; необязательные параметры не заданы — нули
	s, fake := newVoicePreviewServer(t, &yue.VoicePreview{File: "preview-vc-1.flac", From: 0, To: 6}, nil)
	out, ok := call(t, s, "voice_preview", map[string]any{
		"job_id": float64(5), "ref_job_id": float64(9), "from": float64(0), "to": float64(6),
	})
	if !ok {
		t.Fatalf("voice_preview failed: %s", out)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("calls %d", len(fake.calls))
	}
	c := fake.calls[0]
	if c.jobID != 5 || c.p != (yue.VoicePreviewParams{RefJobID: 9, From: 0, To: 6}) {
		t.Fatalf("unset params must be zero: job=%d %+v", c.jobID, c.p)
	}
	if !strings.Contains(out, "preview-vc-1.flac") {
		t.Fatalf("out must contain preview file name: %s", out)
	}
}

func TestVoicePreviewToolServiceError(t *testing.T) {
	t.Parallel()
	s, fake := newVoicePreviewServer(t, nil, &yue.StatusError{Code: 503, Msg: "yue /jobs/5/voice_preview: 503: Seed-VC не установлен"})
	out, ok := call(t, s, "voice_preview", map[string]any{
		"job_id": float64(5), "ref_job_id": float64(9), "from": float64(1), "to": float64(7),
	})
	if ok {
		t.Fatalf("expected tool error, got %s", out)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("service must be called once: %d", len(fake.calls))
	}
	if !strings.Contains(out, "Seed-VC не установлен") {
		t.Fatalf("service error text must reach caller: %s", out)
	}
}

func TestVoicePreviewToolRequiresArgs(t *testing.T) {
	t.Parallel()
	full := map[string]any{"job_id": float64(5), "ref_job_id": float64(9), "from": float64(1), "to": float64(7)}
	without := func(k string) map[string]any {
		m := map[string]any{}
		for kk, v := range full {
			if kk != k {
				m[kk] = v
			}
		}
		return m
	}
	with := func(k string, v any) map[string]any {
		m := without(k)
		m[k] = v
		return m
	}
	cases := map[string]map[string]any{
		"no job_id":     without("job_id"),
		"no ref_job_id": without("ref_job_id"),
		"no from":       without("from"),
		"no to":         without("to"),
		"to == from":    with("to", float64(1)),
		"to < from":     with("to", float64(0.5)),
		"empty":         {},
	}
	for name, args := range cases {
		s, fake := newVoicePreviewServer(t, &yue.VoicePreview{File: "preview-vc-x.flac"}, nil)
		out, ok := call(t, s, "voice_preview", args)
		if ok {
			t.Fatalf("%s %v: expected error, got %s", name, args, out)
		}
		if len(fake.calls) != 0 {
			t.Fatalf("%s %v: service must not be called: %+v", name, args, fake.calls)
		}
	}
}
