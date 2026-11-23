package tools

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/8dia479/EasyCodeAI/internal/nim"
	"github.com/8dia479/EasyCodeAI/internal/workspace"
)

const (
	maxDirectoryEntries = 500
	maxSearchResults    = 200
	maxSearchFiles      = 10000
)

type Registry struct {
	ws                     *workspace.Workspace
	allowProjectExecution  bool
	allowFileOverwrite     bool
}

func New(ws *workspace.Workspace, allowProjectExecution, allowFileOverwrite bool) *Registry {
	return &Registry{ws: ws, allowProjectExecution: allowProjectExecution, allowFileOverwrite: allowFileOverwrite}
}

func str(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func integer(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

func schema(props map[string]any, required ...string) map[string]any {
	return map[string]any{
		"type": "object", "properties": props, "required": required,
		"additionalProperties": false,
	}
}

func (r *Registry) Specs() []nim.ToolSpec {
	specs := []nim.ToolSpec{
		{Type: "function", Function: nim.ToolFunctionSpec{
			Name: "inspect_project", Description: "Inspect project manifests and detect languages, frameworks, and likely test/build entry points without executing project code.",
			Parameters: schema(map[string]any{}),
		}},
		{Type: "function", Function: nim.ToolFunctionSpec{
			Name: "list_files", Description: "List entries in a workspace directory. Use . for root.",
			Parameters: schema(map[string]any{"path": str("relative directory")}, "path"),
		}},
		{Type: "function", Function: nim.ToolFunctionSpec{
			Name: "search_files", Description: "Search text inside workspace files by case-insensitive substring. Skips dependency and build directories.",
			Parameters: schema(map[string]any{"query": str("text to find")}, "query"),
		}},
		{Type: "function", Function: nim.ToolFunctionSpec{
			Name: "read_file", Description: "Read a text file inside workspace (up to 1 MiB), or request a bounded line range for larger files.",
			Parameters: schema(map[string]any{
				"path": str("relative file path"),
				"start_line": integer("optional first line, one-based"),
				"end_line": integer("optional last line, inclusive; at most 1000 lines per call"),
			}, "path"),
		}},
		{Type: "function", Function: nim.ToolFunctionSpec{
			Name: "write_file", Description: "Create or replace a text file inside workspace. Replacing an existing file requires the user to enable --allow-overwrite.",
			Parameters: schema(map[string]any{"path": str("relative path"), "content": str("complete file content")}, "path", "content"),
		}},
		{Type: "function", Function: nim.ToolFunctionSpec{
			Name: "run_checks", Description: "Run safe static syntax checks for Python and JavaScript. Go tests/vet/build execute project code and require --allow-project-execution.",
			Parameters: schema(map[string]any{}),
		}},
	}
	return specs
}

func (r *Registry) Execute(ctx context.Context, name, args string) (string, error) {
	var input map[string]any
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", fmt.Errorf("invalid tool arguments: %w", err)
	}
	switch name {
	case "inspect_project":
		return r.inspectProject()

	case "list_files":
		path, _ := input["path"].(string)
		if path == "" {
			return "", fmt.Errorf("path is required")
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
		for i, entry := range entries {
			if i >= maxDirectoryEntries {
				result = append(result, fmt.Sprintf("[truncated: showing first %d entries]", maxDirectoryEntries))
				break
			}
			kind := "file"
			if entry.IsDir() {
				kind = "dir"
			}
			result = append(result, kind+" "+entry.Name())
		}
		return strings.Join(result, "\n"), nil

	case "search_files":
		query, _ := input["query"].(string)
		if strings.TrimSpace(query) == "" {
			return "", fmt.Errorf("query is required")
		}
		if len(query) > 512 {
			return "", fmt.Errorf("search query exceeds 512 bytes")
		}
		return r.searchFiles(ctx, query)

	case "read_file":
		path, _ := input["path"].(string)
		if path == "" {
			return "", fmt.Errorf("path is required")
		}
		if _, hasStart := input["start_line"]; hasStart {
			startLine, err := intArgument(input, "start_line", 1)
			if err != nil {
				return "", err
			}
			endLine, err := intArgument(input, "end_line", startLine+999)
			if err != nil {
				return "", err
			}
			return r.ws.ReadFileLines(path, startLine, endLine)
		}
		if _, hasEnd := input["end_line"]; hasEnd {
			endLine, err := intArgument(input, "end_line", 1000)
			if err != nil {
				return "", err
			}
			return r.ws.ReadFileLines(path, 1, endLine)
		}
		return r.ws.ReadFile(path)

	case "write_file":
		path, _ := input["path"].(string)
		value, ok := input["content"].(string)
		if path == "" || !ok {
			return "", fmt.Errorf("path and string content are required")
		}
		resolved, err := r.ws.Resolve(path)
		if err != nil {
			return "", err
		}
		backupPath := ""
		if _, err := os.Lstat(resolved); err == nil {
			if !r.allowFileOverwrite {
				return "", fmt.Errorf("refusing to replace existing file %q; rerun with --allow-overwrite to permit modifications", path)
			}
			backupPath, err = backupExistingFile(resolved)
			if err != nil {
				return "", fmt.Errorf("create safety backup before replacing %q: %w", path, err)
			}
		} else if !os.IsNotExist(err) {
			return "", err
		}
		if err := r.ws.WriteFile(path, value); err != nil {
			return "", err
		}
		if backupPath != "" {
			return "Wrote " + filepath.Clean(path) + " (previous version backed up to " + backupPath + ")", nil
		}
		return "Wrote " + filepath.Clean(path), nil

	case "run_checks":
		return r.runChecks(ctx)

	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}


func intArgument(input map[string]any, name string, defaultValue int) (int, error) {
	value, exists := input[name]
	if !exists {
		return defaultValue, nil
	}
	number, ok := value.(float64)
	if !ok || number < 1 || number > 10_000_000 || number != float64(int(number)) {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return int(number), nil
}

func (r *Registry) inspectProject() (string, error) {
	manifests := []string{
		"go.mod", "go.work", "package.json", "pnpm-lock.yaml", "yarn.lock", "package-lock.json",
		"pyproject.toml", "requirements.txt", "Pipfile", "Cargo.toml", "pom.xml", "build.gradle",
		"composer.json", "Gemfile", "Package.swift", "pubspec.yaml", "mix.exs",
		"build.gradle.kts", "settings.gradle", "settings.gradle.kts", "*.sln", "*.csproj",
		"CMakeLists.txt", "Makefile",
	}
	found := make([]string, 0)
	languages := map[string]bool{}
	visited := 0
	err := filepath.WalkDir(r.ws.Root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(r.ws.Root, path)
		if relErr != nil {
			return relErr
		}
		if entry.IsDir() {
			if path != r.ws.Root && ignoredDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == "." {
			return nil
		}
		visited++
		if visited > maxSearchFiles || len(found) >= 200 {
			return filepath.SkipAll
		}
		name := entry.Name()
		for _, manifest := range manifests {
			matched, _ := filepath.Match(manifest, name)
			if matched {
				found = append(found, rel)
				break
			}
		}
		switch strings.ToLower(filepath.Ext(name)) {
		case ".go":
			languages["Go"] = true
		case ".py":
			languages["Python"] = true
		case ".js", ".jsx", ".mjs", ".cjs":
			languages["JavaScript"] = true
		case ".ts", ".tsx":
			languages["TypeScript"] = true
		case ".java":
			languages["Java"] = true
		case ".kt", ".kts":
			languages["Kotlin"] = true
		case ".rs":
			languages["Rust"] = true
		case ".c", ".h", ".cc", ".cpp", ".hpp":
			languages["C/C++"] = true
		case ".cs":
			languages["C#"] = true
		case ".html", ".htm":
			languages["HTML"] = true
		case ".css", ".scss", ".sass":
			languages["CSS"] = true
		case ".php":
			languages["PHP"] = true
		case ".rb":
			languages["Ruby"] = true
		case ".swift":
			languages["Swift"] = true
		case ".dart":
			languages["Dart"] = true
		case ".scala":
			languages["Scala"] = true
		case ".lua":
			languages["Lua"] = true
		case ".sh", ".bash":
			languages["Shell"] = true
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(found)
	var langNames []string
	for name := range languages {
		langNames = append(langNames, name)
	}
	sort.Strings(langNames)
	if len(langNames) == 0 {
		langNames = []string{"unknown"}
	}
	if len(found) == 0 {
		found = []string{"(no known project manifest found)"}
	}
	return "Detected languages: " + strings.Join(langNames, ", ") + "\nProject manifests:\n- " + strings.Join(found, "\n- ") +
		"\nDo not assume a framework or test command that is not supported by the detected files.", nil
}

func (r *Registry) searchFiles(ctx context.Context, query string) (string, error) {
	query = strings.ToLower(query)
	results := make([]string, 0)
	visited := 0
	err := filepath.WalkDir(r.ws.Root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			if path != r.ws.Root && ignoredDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		visited++
		if visited > maxSearchFiles || len(results) >= maxSearchResults {
			return filepath.SkipAll
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			return nil
		}
		rel, err := filepath.Rel(r.ws.Root, path)
		if err != nil {
			return nil
		}
		value, readErr := r.ws.ReadFile(rel)
		data := []byte(value)
		if readErr != nil || bytes.IndexByte(data, 0) >= 0 {
			return nil
		}
		for lineNo, line := range strings.Split(string(data), "
") {
			if strings.Contains(strings.ToLower(line), query) {
				line = strings.TrimSpace(line)
				if len(line) > 1200 {
					line = line[:1200] + "..."
				}
				results = append(results, fmt.Sprintf("%s:%d: %s", rel, lineNo+1, line))
				if len(results) >= maxSearchResults {
					break
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return fmt.Sprintf("No matches for %q (searched %d files).", query, visited), nil
	}
	if visited > maxSearchFiles || len(results) >= maxSearchResults {
		results = append(results, "[results truncated]")
	}
	return strings.Join(results, "\n"), nil
}

func ignoredDirectory(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".venv", "venv", ".tox", ".gradle", "build", "dist", "target", "out":
		return true
	default:
		return false
	}
}

func (r *Registry) runChecks(ctx context.Context) (string, error) {
	var result strings.Builder
	goMod := filepath.Join(r.ws.Root, "go.mod")
	if _, err := os.Stat(goMod); err == nil {
		if !r.allowProjectExecution {
			return "", fmt.Errorf("Go tests/build can execute code from this project; rerun with --allow-project-execution to enable them")
		}
		if _, err := exec.LookPath("go"); err != nil {
			return "[checks-unavailable] Go toolchain is not installed in PATH.", nil
		}
		checks := [][]string{{"test", "./..."}, {"vet", "./..."}, {"build", "./..."}}
		for _, args := range checks {
			output, err := runBounded(ctx, r.ws.Root, "go", args...)
			fmt.Fprintf(&result, "$ go %s\n%s\n", strings.Join(args, " "), output)
			if err != nil {
				return result.String(), err
			}
		}
		return "[checks-passed]\n" + result.String(), nil
	}

	pythonFiles, jsFiles := collectSourceFiles(r.ws.Root, map[string]bool{
		".py": true, ".js": true, ".mjs": true, ".cjs": true,
	})
	unsupportedPython, unsupportedJS := collectSourceFiles(r.ws.Root, map[string]bool{
		".go": true, ".jsx": true, ".ts": true, ".tsx": true, ".java": true,
		".kt": true, ".kts": true, ".rs": true, ".c": true, ".h": true,
		".cc": true, ".cpp": true, ".hpp": true, ".cs": true, ".php": true,
		".rb": true, ".swift": true, ".dart": true, ".scala": true, ".lua": true,
		".sh": true, ".html": true, ".css": true,
	})
	if len(pythonFiles) > 0 {
		script := "import ast,pathlib,sys; files=[pathlib.Path(p) for p in sys.argv[1:]]; [(ast.parse(p.read_text(encoding='utf-8'), filename=str(p))) for p in files]"
		python := "python"
		if _, err := exec.LookPath("python3"); err == nil {
			python = "python3"
		} else if _, err := exec.LookPath("python"); err != nil {
			return "[checks-unavailable] Python interpreter is not installed in PATH.", nil
		}
		output, err := runBounded(ctx, r.ws.Root, python, append([]string{"-c", script}, pythonFiles...)...)
		fmt.Fprintf(&result, "$ python syntax check (%d files)\n%s\n", len(pythonFiles), output)
		if err != nil {
			return result.String(), fmt.Errorf("Python syntax check failed: %w", err)
		}
	}
	if len(jsFiles) > 0 {
		if _, err := exec.LookPath("node"); err != nil {
			return "[checks-unavailable] Node.js is not installed in PATH.", nil
		}
		for _, path := range jsFiles {
			output, err := runBounded(ctx, r.ws.Root, "node", "--check", "./"+filepath.ToSlash(path))
			fmt.Fprintf(&result, "$ node --check %s\n%s\n", path, output)
			if err != nil {
				return result.String(), fmt.Errorf("JavaScript syntax check failed: %w", err)
			}
		}
	}
	if len(pythonFiles) == 0 && len(jsFiles) == 0 {
		if len(unsupportedPython)+len(unsupportedJS) > 0 {
			return "[checks-unavailable] No safe checker is implemented for the detected source language(s). Do not claim automated tests passed.", nil
		}
		return "[checks-passed] No executable source files requiring syntax checks were found.\n", nil
	}
	if len(unsupportedPython)+len(unsupportedJS) > 0 {
		return "[checks-unavailable] Supported Python/JavaScript syntax checks passed, but this project also contains source languages without a safe checker. Do not claim the whole project was verified.", nil
	}
	return "[checks-passed]\n" + result.String(), nil
}

func collectSourceFiles(root string, extensions map[string]bool) ([]string, []string) {
	var pythonFiles, jsFiles []string
	visited := 0
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if path != root && ignoredDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		visited++
		if visited > maxSearchFiles {
			return filepath.SkipAll
		}
		if info, err := entry.Info(); err == nil && info.Mode().IsRegular() && info.Size() <= 1<<20 {
			ext := strings.ToLower(filepath.Ext(path))
			if extensions[ext] {
				rel, err := filepath.Rel(root, path)
				if err == nil {
					if ext == ".py" {
						pythonFiles = append(pythonFiles, rel)
					} else {
						jsFiles = append(jsFiles, rel)
					}
				}
			}
		}
		return nil
	})
	sort.Strings(pythonFiles)
	sort.Strings(jsFiles)
	return pythonFiles, jsFiles
}

func runBounded(ctx context.Context, dir, executable string, args ...string) (string, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(checkCtx, executable, args...)
	cmd.Dir = dir
	cmd.Env = safeGoEnv()
	output := &limitedBuffer{max: 1 << 20}
	cmd.Stdout = output
	cmd.Stderr = output
	err := cmd.Run()
	if checkCtx.Err() == context.DeadlineExceeded {
		return output.String(), fmt.Errorf("%s timed out after 5 minutes", executable)
	}
	if err != nil {
		return output.String(), fmt.Errorf("%s: %w", executable, err)
	}
	return output.String(), nil
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
		result += fmt.Sprintf("\n[output truncated at %d bytes]", b.max)
	}
	return result
}

func safeGoEnv() []string {
	allowed := map[string]bool{
		"PATH": true, "HOME": true, "TMPDIR": true, "TMP": true, "TEMP": true,
		"GOPATH": true, "GOMODCACHE": true, "GOCACHE": true, "GOTOOLCHAIN": true,
		"GOOS": true, "GOARCH": true,
		"CGO_ENABLED": true, "CC": true, "CXX": true, "PKG_CONFIG": true,
		"TERM": true, "LANG": true, "LC_ALL": true,
	}
	var env []string
	for _, item := range os.Environ() {
		key, _, ok := strings.Cut(item, "=")
		if ok && allowed[key] {
			env = append(env, item)
		}
	}
	return env
}


func backupExistingFile(sourcePath string) (string, error) {
	const maxBackupSize = 16 << 20
	source, err := os.Open(sourcePath)
	if err != nil {
		return "", err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("refusing to back up a non-regular file")
	}
	if info.Size() > maxBackupSize {
		return "", fmt.Errorf("existing file exceeds the %d MiB backup limit", maxBackupSize>>20)
	}

	stateDir := os.Getenv("EASYCODE_STATE_DIR")
	if stateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		stateDir = filepath.Join(home, ".easycode")
	}
	session := os.Getenv("EASYCODE_SESSION")
	sessionHash := sha256.Sum256([]byte(session))
	backupDir := filepath.Join(stateDir, "backups", hex.EncodeToString(sessionHash[:8]))
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return "", err
	}
	backup, err := os.CreateTemp(backupDir, "before-write-*.bak")
	if err != nil {
		return "", err
	}
	backupPath := backup.Name()
	ok := false
	defer func() {
		_ = backup.Close()
		if !ok {
			_ = os.Remove(backupPath)
		}
	}()
	if err := backup.Chmod(0o600); err != nil {
		return "", err
	}
	written, err := io.Copy(backup, io.LimitReader(source, maxBackupSize+1))
	if err != nil {
		return "", err
	}
	if written != info.Size() {
		return "", fmt.Errorf("source changed while creating backup")
	}
	if err := backup.Sync(); err != nil {
		return "", err
	}
	if err := backup.Close(); err != nil {
		return "", err
	}
	ok = true
	return backupPath, nil
}
