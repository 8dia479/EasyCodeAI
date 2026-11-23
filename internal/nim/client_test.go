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

func TestRetryDelayHonorsRetryAfterAndSkipsLongWaits(t *testing.T) {
	if got, ok := retryDelay("0", 2); !ok || got != 0 {
		t.Fatalf("zero Retry-After = (%s, %v), want (0, true)", got, ok)
	}
	if got, ok := retryDelay("120", 0); !ok || got != 120*time.Second {
		t.Fatalf("120-second Retry-After = (%s, %v), want (120s, true)", got, ok)
	}
	if _, ok := retryDelay("3600", 0); ok {
		t.Fatal("one-hour Retry-After should not be retried early")
	}
	if isRetryableStatus(http.StatusBadRequest) {
		t.Fatal("400 must not be retried")
	}
}
