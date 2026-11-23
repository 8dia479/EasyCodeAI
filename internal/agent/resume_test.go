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


func TestCanCompleteRequiresChecksAfterModifications(t *testing.T) {
	cases := []struct {
		name string
		cp   Checkpoint
		want bool
	}{
		{name: "no changes", cp: Checkpoint{}, want: true},
		{name: "modified but not checked", cp: Checkpoint{Modified: true, Verification: "not-run"}, want: false},
		{name: "checks failed", cp: Checkpoint{Modified: true, Verification: "failed"}, want: false},
		{name: "checks passed", cp: Checkpoint{Modified: true, Verification: "passed"}, want: true},
		{name: "checker unavailable", cp: Checkpoint{Modified: true, Verification: "unavailable"}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := canComplete(tc.cp); got != tc.want {
				t.Fatalf("canComplete() = %v, want %v", got, tc.want)
			}
		})
	}
}


func TestRunDoesNotOverwriteUnfinishedSession(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EASYCODE_STATE_DIR", dir)
	t.Setenv("EASYCODE_SESSION", "default")
	projectDir := t.TempDir()
	ws, err := workspace.New(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	cp := Checkpoint{
		Task: "important unfinished task",
		Workspace: ws.Root,
		Phase: "interrupted",
		Messages: []nim.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "important unfinished task"},
		},
	}
	if err := saveCheckpoint(cp); err != nil {
		t.Fatal(err)
	}
	engine := New(nil, ws, Options{})
	err = engine.Run(context.Background(), "new task", &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "unfinished task") {
		t.Fatalf("Run() error = %v, want unfinished-session protection", err)
	}
	saved, err := loadCheckpoint()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Task != "important unfinished task" {
		t.Fatalf("saved task was overwritten: %q", saved.Task)
	}
}
