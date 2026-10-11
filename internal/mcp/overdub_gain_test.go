package mcp

// Тесты карточки internal-own-track, уточнение 125а (ТК154): MCP overdub — gain не задан → 0,5;
// явный 0 → 0 (кавер без смешивания), значение доходит до Service.SubmitOverdub.
// Написаны по карточке, без чтения реализации.

import (
	"context"
	"testing"
)

type overdubCall struct {
	JobID         int64
	Style, Lyrics string
	Gain          float64
	Abc           string
	Seed          int64
}

// overdubFake — fakeService + SubmitOverdub, записывающий вызовы; id новой джобы — 901.
type overdubFake struct {
	*fakeService
	overdubs []overdubCall
}

func (f *overdubFake) SubmitOverdub(ctx context.Context, id int64, style, lyrics string, gain float64, abc string, seed int64) (int64, error) {
	f.overdubs = append(f.overdubs, overdubCall{id, style, lyrics, gain, abc, seed})
	return 901, nil
}

func newOverdubServer(t *testing.T) (*Server, *overdubFake) {
	t.Helper()
	fake := &overdubFake{fakeService: &fakeService{url: "http://w:8091"}}
	s := NewServer(fake, t.TempDir())
	RegisterWorkflowTools(s)
	RegisterStudioTools(s)
	RegisterLibraryTools(s)
	RegisterInstallTools(s)
	return s, fake
}

func TestOverdubGainCases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args string
		want float64
	}{
		{"не задан — 0.5", `{"job_id":42,"style":"jazz piano"}`, 0.5},
		{"явный 0 — кавер без смешивания", `{"job_id":42,"style":"jazz piano","gain":0}`, 0},
		{"явный 0.8 — как задан", `{"job_id":42,"style":"jazz piano","gain":0.8}`, 0.8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, fake := newOverdubServer(t)
			out, ok := call(t, s, "overdub", jsonArgs(t, tc.args))
			if !ok {
				t.Fatalf("overdub failed: %s", out)
			}
			if len(fake.overdubs) != 1 {
				t.Fatalf("SubmitOverdub calls %+v, want 1", fake.overdubs)
			}
			got := fake.overdubs[0]
			if got.JobID != 42 || got.Style != "jazz piano" {
				t.Fatalf("SubmitOverdub = %+v, want job 42, style jazz piano", got)
			}
			if got.Gain != tc.want {
				t.Fatalf("gain = %v, want %v", got.Gain, tc.want)
			}
		})
	}
}
