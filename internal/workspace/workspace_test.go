package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	ws, err := New(root)
	if err != nil { t.Fatal(err) }
	for _, name := range []string{"../outside", "sub/../../outside", filepath.Join("..", "outside")} {
		if _, err := ws.Resolve(name); err == nil {
			t.Errorf("Resolve(%q) unexpectedly succeeded", name)
		}
	}
}

func TestResolveAllowsWorkspacePaths(t *testing.T) {
	root := t.TempDir()
	ws, err := New(root)
	if err != nil { t.Fatal(err) }
	got, err := ws.Resolve(filepath.Join("nested", "new.txt"))
	if err != nil { t.Fatal(err) }
	if !strings.HasPrefix(got, ws.Root+string(filepath.Separator)) {
		t.Fatalf("resolved path %q is outside %q", got, ws.Root)
	}
	if err := ws.WriteFile(filepath.Join("nested", "new.txt"), "hello"); err != nil {
		t.Fatal(err)
	}
	text, err := ws.ReadFile(filepath.Join("nested", "new.txt"))
	if err != nil { t.Fatal(err) }
	if text != "hello" { t.Fatalf("got %q, want hello", text) }
}

func TestResolveRejectsDirectorySymlinkEscape(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "workspace")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(root, 0o755); err != nil { t.Fatal(err) }
	if err := os.MkdirAll(outside, 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil { t.Fatal(err) }
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil { t.Skipf("symlinks unavailable: %v", err) }
	ws, err := New(root)
	if err != nil { t.Fatal(err) }
	if _, err := ws.Resolve(filepath.Join("linked", "secret.txt")); err == nil {
		t.Fatal("directory symlink escape was allowed")
	}
	if err := ws.WriteFile(filepath.Join("linked", "created.txt"), "bad"); err == nil {
		t.Fatal("write through directory symlink escape was allowed")
	}
	if _, err := os.Stat(filepath.Join(outside, "created.txt")); !os.IsNotExist(err) {
		t.Fatal("outside file was created")
	}
}

func TestResolveRejectsFileSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "workspace")
	if err := os.MkdirAll(root, 0o755); err != nil { t.Fatal(err) }
	outside := filepath.Join(base, "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil { t.Fatal(err) }
	if err := os.Symlink(outside, filepath.Join(root, "linked.txt")); err != nil { t.Skipf("symlinks unavailable: %v", err) }
	ws, err := New(root)
	if err != nil { t.Fatal(err) }
	if _, err := ws.ReadFile("linked.txt"); err == nil {
		t.Fatal("file symlink escape was allowed")
	}
}


func TestWriteFilePreservesModeAndReplacesContents(t *testing.T) {
	root := t.TempDir()
	ws, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "existing.txt")
	if err := os.WriteFile(path, []byte("old"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := ws.WriteFile("existing.txt", "new content"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new content" {
		t.Fatalf("contents = %q, want new content", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o750 {
		t.Fatalf("mode = %o, want 750", info.Mode().Perm())
	}
}

func TestWriteFileRejectsOversizedContent(t *testing.T) {
	root := t.TempDir()
	ws, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	err = ws.WriteFile("too-large.txt", strings.Repeat("x", (4<<20)+1))
	if err == nil {
		t.Fatal("oversized file write unexpectedly succeeded")
	}
	if _, statErr := os.Stat(filepath.Join(root, "too-large.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("oversized file exists or stat failed unexpectedly: %v", statErr)
	}
}


func TestReadFileCapsLargeFiles(t *testing.T) {
	root := t.TempDir()
	ws, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	data := strings.Repeat("x", (1<<20)+100)
	if err := os.WriteFile(filepath.Join(root, "large.txt"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ws.ReadFile("large.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "[truncated at 1 MiB]") {
		t.Fatalf("large file result lacks truncation marker")
	}
	if len(got) > (1<<20)+100 {
		t.Fatalf("read result unexpectedly exceeds cap: %d bytes", len(got))
	}
}
