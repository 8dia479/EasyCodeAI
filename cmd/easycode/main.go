package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
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
	flag.StringVar(&prompt, "prompt", "", "task to give the coding agent")
	flag.StringVar(&model, "model", os.Getenv("NIM_MODEL"), "NVIDIA NIM model ID")
	flag.StringVar(&baseURL, "base-url", os.Getenv("NIM_BASE_URL"), "OpenAI-compatible NIM base URL")
	flag.StringVar(&root, "workspace", os.Getenv("EASYCODE_WORKSPACE"), "project workspace")
	flag.BoolVar(&resume, "resume", false, "resume the saved task for this workspace")
	flag.IntVar(&maxRounds, "max-rounds", 20, "maximum model rounds per invocation (1-200)")
	flag.BoolVar(&allowProjectExecution, "allow-project-execution", false, "allow Go tests/build to execute code from the workspace")
	flag.BoolVar(&allowOverwrite, "allow-overwrite", false, "allow the agent to replace existing workspace files")
	flag.Parse()

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
	if model == "" {
		fmt.Fprintln(os.Stderr, "NIM_MODEL is required (or pass --model)")
		os.Exit(2)
	}
	apiKey := os.Getenv("NIM_API_KEY")
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
