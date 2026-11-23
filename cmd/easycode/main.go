package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/8dia479/EasyCodeAI/internal/agent"
	"github.com/8dia479/EasyCodeAI/internal/nim"
	"github.com/8dia479/EasyCodeAI/internal/workspace"
)

func main() {
	var prompt string
	var model string
	var baseURL string
	var root string
	var resume bool
	var maxRounds int
	var allowProjectExecution bool
	var allowOverwrite bool
	var session string
	var doctor bool
	flag.StringVar(&prompt, "prompt", "", "task to give the coding agent")
	flag.StringVar(&model, "model", os.Getenv("NIM_MODEL"), "NVIDIA NIM model ID")
	flag.StringVar(&baseURL, "base-url", os.Getenv("NIM_BASE_URL"), "OpenAI-compatible NIM base URL")
	flag.StringVar(&root, "workspace", os.Getenv("EASYCODE_WORKSPACE"), "project workspace")
	flag.BoolVar(&resume, "resume", false, "resume the saved task for this workspace")
	flag.IntVar(&maxRounds, "max-rounds", 20, "maximum model rounds per invocation (1-200)")
	flag.BoolVar(&allowProjectExecution, "allow-project-execution", false, "allow Go tests/build to execute code from the workspace")
	flag.BoolVar(&allowOverwrite, "allow-overwrite", false, "allow the agent to replace existing workspace files")
	flag.StringVar(&session, "session", os.Getenv("EASYCODE_SESSION"), "named persistent task session (letters, digits, dot, dash, underscore)")
	flag.BoolVar(&doctor, "doctor", false, "check local configuration and tool availability without contacting NIM")
	flag.Parse()

	if len(session) > 64 {
		fmt.Fprintln(os.Stderr, "--session must be at most 64 characters")
		os.Exit(2)
	}
	for _, ch := range session {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.') {
			fmt.Fprintln(os.Stderr, "--session may contain only letters, digits, dot, dash, and underscore")
			os.Exit(2)
		}
	}
	if session != "" {
		if err := os.Setenv("EASYCODE_SESSION", session); err != nil {
			fatal(err)
		}
	}

	if maxRounds < 1 || maxRounds > 200 {
		fmt.Fprintln(os.Stderr, "--max-rounds must be between 1 and 200")
		os.Exit(2)
	}
	if root == "" {
		root, _ = os.Getwd()
	}
	if baseURL == "" {
		baseURL = "https://integrate.api.nvidia.com/v1"
	}
	apiKey := os.Getenv("NIM_API_KEY")
	if doctor {
		if err := runDoctor(root, baseURL, model, apiKey != ""); err != nil {
			fatal(err)
		}
		return
	}
	if model == "" {
		fmt.Fprintln(os.Stderr, "NIM_MODEL is required (or pass --model)")
		os.Exit(2)
	}
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "NIM_API_KEY is required")
		os.Exit(2)
	}

	ws, err := workspace.New(root)
	if err != nil {
		fatal(err)
	}
	client := nim.NewClient(baseURL, apiKey, model)
	engine := agent.New(client, ws, agent.Options{MaxRounds: maxRounds, AllowProjectExecution: allowProjectExecution, AllowFileOverwrite: allowOverwrite})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if resume && prompt != "" {
		fatal(fmt.Errorf("--resume cannot be combined with --prompt"))
	}
	if resume {
		if err := engine.Resume(ctx, os.Stdout); err != nil {
			fatal(err)
		}
		return
	}

	if prompt != "" {
		if err := engine.Run(ctx, prompt, os.Stdout); err != nil {
			fatal(err)
		}
		return
	}

	fmt.Println("EasyCodeAI — /resume restores the saved task, /status shows it, /exit quits.")
	if !allowOverwrite || !allowProjectExecution {
		fmt.Println("Safe mode: existing-file replacement and Go project execution are disabled unless enabled with CLI flags.")
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("\n easycode > ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		switch {
		case line == "/exit" || line == "/quit":
			return
		case line == "":
			continue
		case line == "/resume":
			if err := engine.Resume(ctx, os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, "resume:", err)
			}
		case line == "/status":
			if err := engine.ShowCheckpoint(os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		case line == "/help":
			fmt.Println("/resume  continue the saved task\n/status  show the current session\n/help    show commands\n/exit    quit")
		default:
			if err := engine.Run(ctx, line, os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, "agent:", err)
			}
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "easycode:", err)
	os.Exit(1)
}


func runDoctor(root, baseURL, model string, hasAPIKey bool) error {
	fmt.Println("EasyCodeAI environment check")
	fmt.Printf("Go runtime: %s\n", runtime.Version())
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		fmt.Printf("NIM endpoint: invalid (%v)\n", err)
	} else {
		fmt.Printf("NIM endpoint: configured (%s)\n", baseURL)
	}
	if model == "" {
		fmt.Println("NIM model: missing (set NIM_MODEL or --model)")
	} else {
		fmt.Printf("NIM model: %s\n", model)
	}
	if hasAPIKey {
		fmt.Println("NIM API key: present (value hidden)")
	} else {
		fmt.Println("NIM API key: missing")
	}
	if _, err := workspace.New(root); err != nil {
		fmt.Printf("Workspace: invalid (%v)\n", err)
	} else {
		fmt.Printf("Workspace: accessible (%s)\n", root)
	}
	for _, name := range []string{"go", "python3", "python", "node"} {
		if path, err := exec.LookPath(name); err == nil {
			fmt.Printf("%s: %s\n", name, path)
		} else {
			fmt.Printf("%s: not found\n", name)
		}
	}
	fmt.Println("Note: this check does not send a request to NVIDIA NIM.")
	return nil
}
