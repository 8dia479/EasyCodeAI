package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/8dia479/EasyCodeAI/internal/nim"
	"github.com/8dia479/EasyCodeAI/internal/workspace"
)

func TestResumeRejectsDifferentWorkspace(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("EASYCODE_STATE_DIR", stateDir)
	projectDir := t.TempDir()
	ws, err := workspace.New(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	otherWorkspace := filepath.Join(stateDir, "another-project")
	cp := Checkpoint{
		Task: "continue work",
		Workspace: otherWorkspace,
		Phase: "interrupted",
		Messages: []nim.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "continue work"},
		},
	}
	if err := saveCheckpoint(cp); err != nil {
		t.Fatal(err)
	}

	engine := New(nil, ws, Options{})
	err = engine.Resume(context.Background(), &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "belongs to workspace") {
		t.Fatalf("Resume() error = %v, want workspace mismatch", err)
	}
}

func TestResumeRejectsCompletedTask(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("EASYCODE_STATE_DIR", stateDir)
	projectDir := t.TempDir()
	ws, err := workspace.New(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	cp := Checkpoint{
		Task: "finished task",
		Workspace: ws.Root,
		Phase: "completed",
		Messages: []nim.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "finished task"},
		},
	}
	if err := saveCheckpoint(cp); err != nil {
		t.Fatal(err)
	}
	engine := New(nil, ws, Options{})
	err = engine.Resume(context.Background(), &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "already completed") {
		t.Fatalf("Resume() error = %v, want completed-task error", err)
	}
}
