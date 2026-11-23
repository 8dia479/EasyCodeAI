package agent

import (
	"strings"
	"testing"

	"github.com/8dia479/EasyCodeAI/internal/nim"
)

func TestCompactMessagesPreservesTaskAndSafeTurnBoundary(t *testing.T) {
	messages := []nim.Message{
		{Role: "system", Content: "system instructions"},
		{Role: "user", Content: "original task"},
	}
	for i := 0; i < 45; i++ {
		messages = append(messages,
			nim.Message{Role: "assistant", ToolCalls: []nim.ToolCall{{ID: "call-" + string(rune('A'+i%26)), Type: "function", Function: nim.ToolFunction{Name: "read_file", Arguments: "{}"}}}},
			nim.Message{Role: "tool", ToolCallID: "call-result", Content: "file content"},
			nim.Message{Role: "assistant", Content: "intermediate analysis"},
			nim.Message{Role: "user", Content: "continue"},
		)
	}

	got := compactMessages(messages)
	if len(got) >= len(messages) {
		t.Fatalf("message count was not reduced: got %d, original %d", len(got), len(messages))
	}
	if got[0].Role != "system" || got[0].Content != "system instructions" {
		t.Fatalf("system message was not preserved: %+v", got[0])
	}
	if got[1].Role != "user" || got[1].Content != "original task" {
		t.Fatalf("original task was not preserved: %+v", got[1])
	}
	if !strings.Contains(got[2].Content, "Context note") {
		t.Fatalf("missing compaction note: %+v", got[2])
	}
	if got[3].Role != "user" && !(got[3].Role == "assistant" && len(got[3].ToolCalls) == 0) {
		t.Fatalf("compaction started inside a tool-call turn: %+v", got[3])
	}
}

func TestCompactMessagesLeavesSmallConversationUntouched(t *testing.T) {
	messages := []nim.Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: "task"},
		{Role: "assistant", Content: "answer"},
	}
	got := compactMessages(messages)
	if len(got) != len(messages) {
		t.Fatalf("small conversation changed: %d messages", len(got))
	}
}
