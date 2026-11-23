package nim

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCompleteEmitsNonStreamingContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost { t.Errorf("method = %s, want POST", r.Method) }
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" { t.Errorf("authorization = %q", got) }
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"plain response"}}]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-model")
	var streamed strings.Builder
	msg, err := client.Complete(context.Background(), []Message{{Role:"user", Content:"hello"}}, nil, func(s string) { streamed.WriteString(s) })
	if err != nil { t.Fatal(err) }
	if msg.Content != "plain response" { t.Fatalf("message content = %q", msg.Content) }
	if streamed.String() != msg.Content { t.Fatalf("callback got %q, want %q", streamed.String(), msg.Content) }
}


func TestCompleteRetriesRateLimitedRequest(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "queued", http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"recovered"}}]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-model")
	msg, err := client.Complete(context.Background(), []Message{{Role: "user", Content: "hello"}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "recovered" {
		t.Fatalf("content = %q, want recovered", msg.Content)
	}
	if attempts != 2 {
		t.Fatalf("request attempts = %d, want 2", attempts)
	}
}

func TestRetryDelayHonorsRetryAfterAndCapsIt(t *testing.T) {
	if got := retryDelay("0", 2); got != 0 {
		t.Fatalf("zero Retry-After = %s, want 0", got)
	}
	if got := retryDelay("120", 0); got != 30*time.Second {
		t.Fatalf("capped Retry-After = %s, want 30s", got)
	}
	if isRetryableStatus(http.StatusBadRequest) {
		t.Fatal("400 must not be retried")
	}
}
