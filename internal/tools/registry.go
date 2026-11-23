package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/8dia479/EasyCodeAI/internal/nim"
	"github.com/8dia479/EasyCodeAI/internal/workspace"
)

type Registry struct {
	ws *workspace.Workspace
}

func New(ws *workspace.Workspace) *Registry {
	return &Registry{ws: ws}
}

func str(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func schema(props map[string]any, required ...string) map[string]any {
	return map[string]any{
		"type": "object", "properties": props, "required": required,
		"additionalProperties": false,
	}
}

func (r *Registry) Specs() []nim.ToolSpec {
	return []nim.ToolSpec{
		{Type: "function", Function: nim.ToolFunctionSpec{
			Name: "list_files", Description: "List workspace files. Use . for root.",
			Parameters: schema(map[string]any{"path": str("relative directory")}, "path"),
		}},
		{Type: "function", Function: nim.ToolFunctionSpec{
			Name: "read_file", Description: "Read a text file inside workspace.",
			Parameters: schema(map[string]any{"path": str("relative file path")}, "path"),
		}},
		{Type: "function", Function: nim.ToolFunctionSpec{
			Name: "write_file", Description: "Create or replace a text file inside workspace.",
			Parameters: schema(map[string]any{"path": str("relative path"), "content": str("complete file content")}, "path", "content"),
		}},
		{Type: "function", Function: nim.ToolFunctionSpec{
			Name: "run_checks", Description: "Run go test ./..., go vet ./..., and go build ./.... No arbitrary commands.",
			Parameters: schema(map[string]any{}),
		}},
	}
}

func (r *Registry) Execute(ctx context.Context, name, args string) (string, error) {
	var input map[string]any
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", err
	}
	switch name {
	case "list_files":
		path, _ := input["path"].(string)
		if path == "" || path == "." {
			path = "."
		}
		dir := r.ws.Root
		if path != "." {
			var err error
			dir, err = r.ws.Resolve(path)
			if err != nil {
				return "", err
			}
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return "", err
		}
		var result []string
		for _, entry := range entries {
			kind := "file"
			if entry.IsDir() {
				kind = "dir"
			}
			result = append(result, kind+" "+entry.Name())
		}
		return strings.Join(result, "\n"), nil

	case "read_file":
		path, _ := input["path"].(string)
		value, err := r.ws.ReadFile(path)
		if err != nil {
			return "", err
		}
		if len(value) > 100000 {
			value = value[:100000] + "\n[truncated]"
		}
		return value, nil

	case "write_file":
		path, _ := input["path"].(string)
		value, _ := input["content"].(string)
		if err := r.ws.WriteFile(path, value); err != nil {
			return "", err
		}
		return "Wrote " + filepath.Clean(path), nil

	case "run_checks":
		var result strings.Builder
		checks := [][]string{{"test", "./..."}, {"vet", "./..."}, {"build", "./..."}}
		for _, args := range checks {
			checkCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			cmd := exec.CommandContext(checkCtx, "go", args...)
			cmd.Dir = r.ws.Root
			output := &limitedBuffer{max: 1 << 20}
			cmd.Stdout = output
			cmd.Stderr = output
			err := cmd.Run()
			timedOut := checkCtx.Err() == context.DeadlineExceeded
			cancel()

			fmt.Fprintf(&result, "$ go %s\n%s\n", strings.Join(args, " "), output.String())
			if err != nil {
				if timedOut {
					return result.String(), fmt.Errorf("go %s timed out after 5 minutes", strings.Join(args, " "))
				}
				return result.String(), fmt.Errorf("go %s: %w", strings.Join(args, " "), err)
			}
		}
		return result.String(), nil

	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

type limitedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	remaining := b.max - b.buf.Len()
	if remaining > 0 {
		n := len(p)
		if n > remaining {
			n = remaining
		}
		_, _ = b.buf.Write(p[:n])
	}
	if len(p) > remaining {
		b.truncated = true
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	result := b.buf.String()
	if b.truncated {
		result += "\n[output truncated at 1 MiB]"
	}
	return result
}
