package workspace

import (
	"fmt"
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
	path, err := w.Resolve(name)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (w *Workspace) WriteFile(name, content string) error {
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
	return os.WriteFile(path, []byte(content), 0o644)
}
