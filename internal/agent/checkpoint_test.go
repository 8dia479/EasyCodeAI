package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/8dia479/EasyCodeAI/internal/nim"
)

func TestCheckpointRoundTripPreservesConversation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EASYCODE_STATE_DIR", dir)
	t.Setenv("EASYCODE_SESSION", "default")

	want := Checkpoint{
		Task:      "repair the failing test",
		Workspace: filepath.Join(dir, "project"),
		Phase:     "interrupted",
		Round:     4,
		History:   []string{"starting", "model_round_1", "interrupted"},
		Messages: []nim.Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: "repair the failing test"},
			{Role: "assistant", ToolCalls: []nim.ToolCall{{
				ID: "call-1", Type: "function",
				Function: nim.ToolFunction{Name: "read_file", Arguments: `{"path":"main.go"}`},
			}}},
			{Role: "tool", ToolCallID: "call-1", Content: "package main"},
		},
	}
	if err := saveCheckpoint(want); err != nil {
		t.Fatalf("saveCheckpoint() error = %v", err)
	}
	got, err := loadCheckpoint()
	if err != nil {
		t.Fatalf("loadCheckpoint() error = %v", err)
	}
	if got.Version != checkpointVersion {
		t.Fatalf("Version = %d, want %d", got.Version, checkpointVersion)
	}
	if got.Task != want.Task || got.Workspace != want.Workspace || got.Round != want.Round {
		t.Fatalf("checkpoint metadata mismatch: got %+v", got)
	}
	if len(got.Messages) != len(want.Messages) {
		t.Fatalf("messages = %d, want %d", len(got.Messages), len(want.Messages))
	}
	if got.Messages[2].ToolCalls[0].Function.Name != "read_file" {
		t.Fatalf("tool call was not preserved: %+v", got.Messages[2].ToolCalls)
	}
	if got.Messages[3].ToolCallID != "call-1" {
		t.Fatalf("tool result was not preserved: %+v", got.Messages[3])
	}
	info, err := os.Stat(filepath.Join(dir, "checkpoint.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("checkpoint permissions = %o, want owner-only", info.Mode().Perm())
	}
}

func TestLoadCheckpointRejectsLegacyOrIncompleteState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EASYCODE_STATE_DIR", dir)
	t.Setenv("EASYCODE_SESSION", "default")
	path := filepath.Join(dir, "checkpoint.json")
	if err := os.WriteFile(path, []byte(`{"task":"old task","phase":"interrupted"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadCheckpoint(); err == nil {
		t.Fatal("loadCheckpoint() accepted a legacy non-resumable checkpoint")
	}
}


func TestLoadCheckpointMigratesLegacyModifiedTaskSafely(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EASYCODE_STATE_DIR", dir)
	t.Setenv("EASYCODE_SESSION", "default")
	legacy := `{
		"version": 1,
		"task": "update source",
		"workspace": "/tmp/project",
		"phase": "interrupted",
		"history": ["tool write_file: Wrote main.go"],
		"messages": [
			{"role":"system","content":"system"},
			{"role":"user","content":"update source"}
		]
	}`
	if err := os.WriteFile(filepath.Join(dir, "checkpoint.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	cp, err := loadCheckpoint()
	if err != nil {
		t.Fatal(err)
	}
	if cp.Version != checkpointVersion || !cp.Modified || cp.Verification != "not-run" {
		t.Fatalf("legacy checkpoint migration = %+v", cp)
	}
}


func TestNamedSessionsUseSeparateCheckpointFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EASYCODE_STATE_DIR", dir)
	t.Setenv("EASYCODE_SESSION", "alpha")
	alpha := statePath()
	t.Setenv("EASYCODE_SESSION", "beta")
	beta := statePath()
	t.Setenv("EASYCODE_SESSION", "default")
	def := statePath()
	if alpha == beta || alpha == def || beta == def {
		t.Fatalf("session paths are not isolated: alpha=%q beta=%q default=%q", alpha, beta, def)
	}
	if filepath.Dir(alpha) != filepath.Join(dir, "sessions") {
		t.Fatalf("named session path = %q, want sessions directory", alpha)
	}
	if filepath.Base(def) != "checkpoint.json" {
		t.Fatalf("default session path = %q, want checkpoint.json", def)
	}
}
