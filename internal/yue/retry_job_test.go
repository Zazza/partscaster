package yue

import (
	"context"
	"net/http"
	"testing"
)

func TestRetryJobSendsPost(t *testing.T) {
	t.Parallel()
	var got updateReq
	c := updateServer(t, 200, `{"retried":true}`, &got)
	if err := c.RetryJob(context.Background(), 42); err != nil {
		t.Fatalf("RetryJob: %v", err)
	}
	if got.method != http.MethodPost || got.path != "/jobs/42/retry" {
		t.Fatalf("request: %s %s", got.method, got.path)
	}
	if len(got.body) != 0 {
		t.Fatalf("retry needs no body fields: %v", got.body)
	}
}
