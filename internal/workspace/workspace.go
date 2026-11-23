package workspace

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Workspace struct {
	Root string
}

func New(root string) (*Workspace, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace is not a directory: %s", abs)
	}
	// Resolve the root once so later containment checks use its real location.
	realRoot, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}
	return &Workspace{Root: filepath.Clean(realRoot)}, nil
}

// Resolve rejects lexical traversal and symlink traversal outside the workspace.
// For a not-yet-created destination, it resolves the nearest existing parent
// before appending the missing path components.
func (w *Workspace) Resolve(name string) (string, error) {
	if name == "" || filepath.IsAbs(name) {
		return "", fmt.Errorf("absolute or empty paths are not allowed")
	}
	clean := filepath.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace")
	}
	target := filepath.Join(w.Root, clean)
	real, err := resolveExistingPrefix(target)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(w.Root, real)
	if err != nil || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace through a symlink")
	}
	return real, nil
}

func resolveExistingPrefix(path string) (string, error) {
	current := filepath.Clean(path)
	var missing []string
	for {
		_, err := os.Lstat(current)
		if err == nil {
			real, evalErr := filepath.EvalSymlinks(current)
			if evalErr != nil {
				return "", fmt.Errorf("resolve path: %w", evalErr)
			}
			for i := len(missing) - 1; i >= 0; i-- {
				real = filepath.Join(real, missing[i])
			}
			return filepath.Clean(real), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no existing parent for path")
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func (w *Workspace) ReadFile(name string) (string, error) {
	const maxReadSize = 1 << 20
	path, err := w.Resolve(name)
	if err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("refusing to read non-regular file: %s", name)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxReadSize+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxReadSize {
		return string(data[:maxReadSize]) + "\n[truncated at 1 MiB]", nil
	}
	return string(data), nil
}


func (w *Workspace) ReadFileLines(name string, startLine, endLine int) (string, error) {
	const (
		maxLines     = 1000
		maxLineIndex = 10_000_000
		maxBytes     = 1 << 20
	)
	if startLine < 1 || endLine < startLine || endLine-startLine+1 > maxLines || endLine > maxLineIndex {
		return "", fmt.Errorf("line range must be within 1-%d lines", maxLines)
	}
	path, err := w.Resolve(name)
	if err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("refusing to read non-regular file: %s", name)
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	var result strings.Builder
	for lineNo := 1; scanner.Scan(); lineNo++ {
		if lineNo > endLine {
			break
		}
		if lineNo < startLine {
			continue
		}
		line := scanner.Text() + "\n"
		if result.Len()+len(line) > maxBytes {
			result.WriteString("[range output truncated at 1 MiB]\n")
			break
		}
		result.WriteString(line)
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return result.String(), nil
}

func (w *Workspace) WriteFile(name, content string) error {
	const maxFileSize = 4 << 20
	if len(content) > maxFileSize {
		return fmt.Errorf("file content exceeds the %d MiB write limit", maxFileSize>>20)
	}
	path, err := w.Resolve(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Re-resolve after creating parent directories to catch a replaced symlink.
	path, err = w.Resolve(name)
	if err != nil {
		return err
	}

	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing to replace non-regular file: %s", name)
		}
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(statErr) {
		return statErr
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".easycode-write-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// Check containment once more immediately before the atomic replacement.
	verified, err := w.Resolve(name)
	if err != nil {
		return err
	}
	if verified != path {
		return fmt.Errorf("workspace path changed while writing: %s", name)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace %s: %w", name, err)
	}
	return nil
}
