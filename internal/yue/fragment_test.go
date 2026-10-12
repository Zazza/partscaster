package yue

// Client.FragmentJob(ctx, id, from, to, seed, abc) — POST /jobs/{id}/continue с from_sec, to_sec;
// seed — только если > 0, abc — только если не пусто; ответ воркера {"id": N} → N.

import (
	"context"
	"net/http"
	"testing"
)

func TestFragmentJobSendsWindowSeedAbc(t *testing.T) {
	t.Parallel()
	var got updateReq
	c := updateServer(t, 200, `{"id":77}`, &got)
	id, err := c.FragmentJob(context.Background(), 12, 10.5, 18.25, 4242, "X:1\nK:C", "")
	if err != nil {
		t.Fatalf("FragmentJob: %v", err)
	}
	if id != 77 {
		t.Fatalf("id = %d, want 77", id)
	}
	if got.method != http.MethodPost || got.path != "/jobs/12/continue" {
		t.Fatalf("request: %s %s", got.method, got.path)
	}
	if got.body["from_sec"] != 10.5 || got.body["to_sec"] != 18.25 {
		t.Fatalf("from_sec/to_sec: %v", got.body)
	}
	if got.body["seed"] != float64(4242) {
		t.Fatalf("seed: %v", got.body)
	}
	if got.body["abc"] != "X:1\nK:C" {
		t.Fatalf("abc: %v", got.body)
	}
}

func TestFragmentJobOmitsZeroSeedAndEmptyAbc(t *testing.T) {
	t.Parallel()
	for _, seed := range []int64{0, -3} {
		var got updateReq
		c := updateServer(t, 200, `{"id":5}`, &got)
		id, err := c.FragmentJob(context.Background(), 3, 1, 4, seed, "", "")
		if err != nil {
			t.Fatalf("seed %d: FragmentJob: %v", seed, err)
		}
		if id != 5 {
			t.Fatalf("seed %d: id = %d, want 5", seed, id)
		}
		if _, ok := got.body["seed"]; ok {
			t.Fatalf("seed %d must not be sent: %v", seed, got.body)
		}
		if _, ok := got.body["abc"]; ok {
			t.Fatalf("empty abc must not be sent: %v", got.body)
		}
		if got.body["from_sec"] != float64(1) || got.body["to_sec"] != float64(4) {
			t.Fatalf("seed %d: from_sec/to_sec: %v", seed, got.body)
		}
	}
}

// FragmentJob(…, abc, styleAdd) — style_add в теле только если не пусто.
func TestFragmentJobStyleAdd(t *testing.T) {
	t.Parallel()
	var got updateReq
	c := updateServer(t, 200, `{"id":8}`, &got)
	if _, err := c.FragmentJob(context.Background(), 4, 2, 9, 0, "", "tape saturation, wide stereo"); err != nil {
		t.Fatalf("FragmentJob: %v", err)
	}
	if got.body["style_add"] != "tape saturation, wide stereo" {
		t.Fatalf("style_add: %v", got.body)
	}

	var empty updateReq
	c = updateServer(t, 200, `{"id":9}`, &empty)
	if _, err := c.FragmentJob(context.Background(), 4, 2, 9, 0, "", ""); err != nil {
		t.Fatalf("FragmentJob: %v", err)
	}
	if _, ok := empty.body["style_add"]; ok {
		t.Fatalf("empty style_add must not be sent: %v", empty.body)
	}
}
