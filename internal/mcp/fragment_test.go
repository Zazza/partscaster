package mcp

// Тесты карточки internal-own-track, условие 123 «Честный „проверить кусок“» (ТК150):
// MCP continue_job с to_sec ставит кусок (Service.FragmentJob), без to_sec — продолжение до конца
// (Service.ContinueJob), FragmentJob при этом не зовётся.
// Написаны по карточке, без чтения реализации.

import (
	"context"
	"strings"
	"testing"
)

type fragmentCall struct {
	JobID    int64
	From, To float64
	Seed     int64
	Abc      string
}

// fragmentFake — fakeService + FragmentJob, записывающий вызовы; id новой джобы — 900.
type fragmentFake struct {
	*fakeService
	fragments []fragmentCall
}

func (f *fragmentFake) FragmentJob(ctx context.Context, jobID int64, fromSec, toSec float64, seed int64, abc, styleAdd string) (int64, error) {
	f.fragments = append(f.fragments, fragmentCall{jobID, fromSec, toSec, seed, abc})
	return 900, nil
}

func newFragmentServer(t *testing.T) (*Server, *fragmentFake) {
	t.Helper()
	fake := &fragmentFake{fakeService: &fakeService{url: "http://w:8091"}}
	s := NewServer(fake, t.TempDir())
	RegisterWorkflowTools(s)
	RegisterStudioTools(s)
	RegisterLibraryTools(s)
	RegisterInstallTools(s)
	return s, fake
}

func TestContinueJobWithToSecCallsFragmentJob(t *testing.T) {
	t.Parallel()
	s, fake := newFragmentServer(t)
	out, ok := call(t, s, "continue_job", jsonArgs(t, `{"job_id":216,"from_sec":40,"to_sec":52.5}`))
	if !ok {
		t.Fatalf("continue_job failed: %s", out)
	}
	if len(fake.fragments) != 1 {
		t.Fatalf("FragmentJob calls %+v, want 1", fake.fragments)
	}
	f := fake.fragments[0]
	if f.JobID != 216 || f.From != 40 || f.To != 52.5 {
		t.Fatalf("FragmentJob = %+v, want job 216, 40–52.5", f)
	}
	if len(fake.continued) != 0 {
		t.Fatalf("ContinueJob must not be called for a fragment: %+v", fake.continued)
	}
	if !strings.Contains(out, "900") {
		t.Fatalf("в ответе нет id поставленной джобы: %s", out)
	}
}

func TestContinueJobWithToSecPassesSeedAndAbc(t *testing.T) {
	t.Parallel()
	s, fake := newFragmentServer(t)
	fake.planCheckOut = samplePlanCheck()
	out, ok := call(t, s, "continue_job", jsonArgs(t, `{"job_id":7,"from_sec":10,"to_sec":20,"seed":31,"abc":"X:1\nK:C"}`))
	if !ok {
		t.Fatalf("continue_job failed: %s", out)
	}
	if len(fake.fragments) != 1 || fake.fragments[0].Seed != 31 || fake.fragments[0].Abc != "X:1\nK:C" {
		t.Fatalf("FragmentJob = %+v, want seed 31 и тот же abc", fake.fragments)
	}
}

func TestContinueJobWithoutToSecCallsContinueJob(t *testing.T) {
	t.Parallel()
	s, fake := newFragmentServer(t)
	out, ok := call(t, s, "continue_job", jsonArgs(t, `{"job_id":216,"from_sec":40}`))
	if !ok {
		t.Fatalf("continue_job failed: %s", out)
	}
	if len(fake.fragments) != 0 {
		t.Fatalf("FragmentJob must not be called without to_sec: %+v", fake.fragments)
	}
	if len(fake.continued) != 1 || fake.continued[0].JobID != 216 {
		t.Fatalf("ContinueJob = %+v, want один вызов для 216", fake.continued)
	}
}

// Кросс-ревью s15 r1, условие 123е (ТК155): to_sec передан, даже 0, — это кусок (окно проверит воркер),
// а не продолжение до конца песни.
func TestContinueJobWithToSecZeroCallsFragmentJob(t *testing.T) {
	t.Parallel()
	s, fake := newFragmentServer(t)
	call(t, s, "continue_job", jsonArgs(t, `{"job_id":216,"from_sec":40,"to_sec":0}`))
	if len(fake.continued) != 0 {
		t.Fatalf("to_sec 0 ушёл в ContinueJob (продолжение до конца): %+v", fake.continued)
	}
	if len(fake.fragments) != 1 {
		t.Fatalf("FragmentJob calls %+v, want 1", fake.fragments)
	}
	if f := fake.fragments[0]; f.JobID != 216 || f.From != 40 || f.To != 0 {
		t.Fatalf("FragmentJob = %+v, want job 216, 40–0", f)
	}
}
