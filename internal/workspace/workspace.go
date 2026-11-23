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
	return &Workspace{Root: filepath.Clean(abs)}, nil
}

func (w *Workspace) Resolve(name string) (string, error) {
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}
	clean := filepath.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace")
	}
	target := filepath.Join(w.Root, clean)
	rel, err := filepath.Rel(w.Root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace")
	}
	return target, nil
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
	return os.WriteFile(path, []byte(content), 0o644)
}
