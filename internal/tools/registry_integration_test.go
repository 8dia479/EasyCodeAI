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
	stateDir := t.TempDir()
	t.Setenv("EASYCODE_STATE_DIR", stateDir)
	t.Setenv("EASYCODE_SESSION", "default")
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
	result, err := allowed.Execute(context.Background(), "write_file", `{"path":"main.go","content":"new"}`)
	if err != nil {
		t.Fatal(err)
	}
	marker := "backed up to "
	index := strings.Index(result, marker)
	if index < 0 {
		t.Fatalf("write result does not report a backup path: %q", result)
	}
	backupPath := strings.TrimSpace(result[index+len(marker):])
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read safety backup: %v", err)
	}
	if string(backupData) != "old" {
		t.Fatalf("backup contains %q, want original contents", backupData)
	}
	backupInfo, err := os.Stat(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if backupInfo.Mode().Perm()&0o077 != 0 {
		t.Fatalf("backup permissions = %o, want owner-only", backupInfo.Mode().Perm())
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


func TestRunChecksReportsUnsupportedLanguageWithoutClaimingSuccess(t *testing.T) {
	registry, root := newTestRegistry(t, false, false)
	if err := os.WriteFile(filepath.Join(root, "main.rs"), []byte("fn main() {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := registry.Execute(context.Background(), "run_checks", "{}")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "[checks-unavailable]") {
		t.Fatalf("run_checks result = %q, want checks-unavailable marker", got)
	}
}

func TestRunChecksMarksDocumentationOnlyWorkspaceAsChecked(t *testing.T) {
	registry, root := newTestRegistry(t, false, false)
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := registry.Execute(context.Background(), "run_checks", "{}")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "[checks-passed]") {
		t.Fatalf("run_checks result = %q, want checks-passed marker", got)
	}
}


func TestRunChecksReportsMissingGoToolchainAsUnavailable(t *testing.T) {
	registry, root := newTestRegistry(t, true, false)
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n\ngo 1.22\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	got, err := registry.Execute(context.Background(), "run_checks", "{}")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "[checks-unavailable]") {
		t.Fatalf("run_checks result = %q, want checks-unavailable marker", got)
	}
}

func TestRunChecksReportsMissingPythonAsUnavailable(t *testing.T) {
	registry, root := newTestRegistry(t, false, false)
	if err := os.WriteFile(filepath.Join(root, "main.py"), []byte("print('ok')"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	got, err := registry.Execute(context.Background(), "run_checks", "{}")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "[checks-unavailable]") {
		t.Fatalf("run_checks result = %q, want checks-unavailable marker", got)
	}
}


func TestReadFileToolSupportsBoundedLineRanges(t *testing.T) {
	registry, root := newTestRegistry(t, false, false)
	if err := os.WriteFile(filepath.Join(root, "large.txt"), []byte("first\nsecond\nthird\nfourth\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := registry.Execute(context.Background(), "read_file", `{"path":"large.txt","start_line":2,"end_line":3}`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "second\nthird\n" {
		t.Fatalf("read_file range = %q, want second and third lines", got)
	}
}
