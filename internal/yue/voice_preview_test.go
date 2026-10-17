package yue

// «Голос альбома»: прослушать кусок (internal-album-voice-draft, условия 4–5; ТК4).
// Клиент VoicePreview: POST /jobs/{id}/voice_preview → {file, from, to}.

import (
	"context"
	"net/http"
	"testing"
)

func TestVoicePreviewFullParams(t *testing.T) {
	t.Parallel()
	var got updateReq
	c := updateServer(t, 200, `{"file":"preview-vc-ab12cd34.flac","from":2,"to":8.5}`, &got)
	vp, err := c.VoicePreview(context.Background(), 214, VoicePreviewParams{
		RefJobID: 135, From: 2, To: 8.5, RefFrom: 12.5, RefDur: 20, Steps: 60,
	})
	if err != nil {
		t.Fatalf("VoicePreview: %v", err)
	}
	if got.method != http.MethodPost || got.path != "/jobs/214/voice_preview" {
		t.Fatalf("request: %s %s", got.method, got.path)
	}
	want := map[string]any{"ref_job_id": float64(135), "from": float64(2), "to": 8.5,
		"ref_from": 12.5, "ref_dur": float64(20), "steps": float64(60)}
	if len(got.body) != len(want) {
		t.Fatalf("body %v, want %v", got.body, want)
	}
	for k, v := range want {
		if got.body[k] != v {
			t.Fatalf("body[%s]=%v, want %v (body %v)", k, got.body[k], v, got.body)
		}
	}
	if vp == nil || vp.File != "preview-vc-ab12cd34.flac" || vp.From != 2 || vp.To != 8.5 {
		t.Fatalf("answer %+v", vp)
	}
}

func TestVoicePreviewZeroOptionalOmittedFromZeroSent(t *testing.T) {
	t.Parallel()
	// from = 0 — начало трека, законное значение: поле уходит всегда; ref_from/ref_dur/steps — только не нули
	var got updateReq
	c := updateServer(t, 200, `{"file":"preview-vc-1.flac","from":0,"to":6}`, &got)
	vp, err := c.VoicePreview(context.Background(), 5, VoicePreviewParams{RefJobID: 9, To: 6})
	if err != nil {
		t.Fatalf("VoicePreview: %v", err)
	}
	if got.method != http.MethodPost || got.path != "/jobs/5/voice_preview" {
		t.Fatalf("request: %s %s", got.method, got.path)
	}
	want := map[string]any{"ref_job_id": float64(9), "from": float64(0), "to": float64(6)}
	if len(got.body) != len(want) {
		t.Fatalf("zero optional fields must be omitted, from/to always sent: %v", got.body)
	}
	for k, v := range want {
		if gv, ok := got.body[k]; !ok || gv != v {
			t.Fatalf("body[%s]=%v, want %v (body %v)", k, gv, v, got.body)
		}
	}
	if vp == nil || vp.File != "preview-vc-1.flac" || vp.From != 0 || vp.To != 6 {
		t.Fatalf("answer %+v", vp)
	}
}

func TestVoicePreviewWorkerErrors(t *testing.T) {
	t.Parallel()
	for _, code := range []int{404, 422, 503} {
		var got updateReq
		c := updateServer(t, code, `{"detail":"Seed-VC не установлен"}`, &got)
		vp, err := c.VoicePreview(context.Background(), 5, VoicePreviewParams{RefJobID: 9, From: 1, To: 7})
		if err == nil {
			t.Fatalf("status %d: expected error, got %+v", code, vp)
		}
	}
}
