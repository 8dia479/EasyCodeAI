package nim

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
