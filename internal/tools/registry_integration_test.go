package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/8dia479/EasyCodeAI/internal/workspace"
)

func newTestRegistry(t *testing.T, allowExecution, allowOverwrite bool) (*Registry, string) {
	t.Helper()
	root := t.TempDir()
	ws, err := workspace.New(root)
	if err != nil {
		t.Fatal(err)
	}
	return New(ws, allowExecution, allowOverwrite), root
}

func TestInspectProjectDetectsLanguagesAndManifests(t *testing.T) {
	registry, root := newTestRegistry(t, false, false)
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "app.py"), []byte("print('ok')"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := registry.Execute(context.Background(), "inspect_project", "{}")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Go", "Python", "go.mod"} {
		if !strings.Contains(got, want) {
			t.Fatalf("inspect_project output %q does not contain %q", got, want)
		}
	}
}

func TestWriteFileRequiresOptInToReplaceExistingFiles(t *testing.T) {
	registry, root := newTestRegistry(t, false, false)
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Execute(context.Background(), "write_file", `{"path":"main.go","content":"new"}`); err == nil {
		t.Fatal("write_file replaced an existing file without permission")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old" {
		t.Fatalf("existing file changed to %q", data)
	}

	allowed, _ := newTestRegistry(t, false, true)
	allowed.ws = registry.ws
	if _, err := allowed.Execute(context.Background(), "write_file", `{"path":"main.go","content":"new"}`); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("file contents = %q, want new", data)
	}
}

func TestSearchFilesFindsTextAndSkipsDependencyDirectories(t *testing.T) {
	registry, root := newTestRegistry(t, false, false)
	if err := os.MkdirAll(filepath.Join(root, "node_modules", "fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n// ImportantMarker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "fake", "ignored.js"), []byte("ImportantMarker"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := registry.Execute(context.Background(), "search_files", `{"query":"importantmarker"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "main.go:2") {
		t.Fatalf("search output %q does not contain source match", got)
	}
	if strings.Contains(got, "node_modules") {
		t.Fatalf("search traversed dependency directory: %q", got)
	}
}

func TestRunChecksDoesNotExecuteGoProjectWithoutPermission(t *testing.T) {
	registry, root := newTestRegistry(t, false, false)
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n\ngo 1.22\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Execute(context.Background(), "run_checks", "{}"); err == nil || !strings.Contains(err.Error(), "--allow-project-execution") {
		t.Fatalf("run_checks error = %v, want explicit permission error", err)
	}
}
