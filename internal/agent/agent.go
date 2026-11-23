package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/8dia479/EasyCodeAI/internal/nim"
	"github.com/8dia479/EasyCodeAI/internal/tools"
	"github.com/8dia479/EasyCodeAI/internal/workspace"
)

type Options struct {
	MaxRounds             int
	AllowProjectExecution bool
	AllowFileOverwrite    bool
}

type Agent struct {
	client *nim.Client
	ws     *workspace.Workspace
	opts   Options
	tools  *tools.Registry
}

func New(client *nim.Client, ws *workspace.Workspace, opts Options) *Agent {
	if opts.MaxRounds <= 0 {
		opts.MaxRounds = 20
	}
	return &Agent{client: client, ws: ws, opts: opts, tools: tools.New(ws, opts.AllowProjectExecution, opts.AllowFileOverwrite)}
}

func (a *Agent) Run(ctx context.Context, task string, out io.Writer) error {
	if strings.TrimSpace(task) == "" {
		return fmt.Errorf("task cannot be empty")
	}
	if existing, err := loadCheckpoint(); err == nil {
		if existing.Phase != "completed" {
			return fmt.Errorf("session already contains an unfinished task (%s); use /resume or start with a different --session", existing.Phase)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("refusing to replace an unreadable checkpoint: %w", err)
	}
	cp := Checkpoint{
		Task:      task,
		Session:   os.Getenv("EASYCODE_SESSION"),
		Workspace: a.ws.Root,
		Phase:     "starting",
		Messages: []nim.Message{
			{Role: "system", Content: systemPrompt(a.ws.Root)},
			{Role: "user", Content: task},
		},
	}
	if err := saveCheckpoint(cp); err != nil {
		return err
	}
	return a.runLoop(ctx, cp, out)
}

// Resume restores the saved model conversation, including assistant tool calls
// and tool results. The workspace must match the one recorded by the task.
func (a *Agent) Resume(ctx context.Context, out io.Writer) error {
	cp, err := loadCheckpoint()
	if err != nil {
		return fmt.Errorf("load resumable task: %w", err)
	}
	if filepath.Clean(cp.Workspace) != filepath.Clean(a.ws.Root) {
		return fmt.Errorf("saved task belongs to workspace %q, not %q", cp.Workspace, a.ws.Root)
	}
	if cp.Phase == "completed" {
		return fmt.Errorf("the saved task is already completed")
	}
	fmt.Fprintf(out, "Resuming: %s\nLast phase: %s (round %d)\n", cp.Task, cp.Phase, cp.Round)
	cp.Phase = "resuming"
	if err := saveCheckpoint(cp); err != nil {
		return err
	}
	return a.runLoop(ctx, cp, out)
}

func (a *Agent) runLoop(ctx context.Context, cp Checkpoint, out io.Writer) error {
	specs := a.tools.Specs()

	// A process may have stopped after the model returned tool calls but before
	// all results were saved. Complete any missing calls before asking the model
	// for another response.
	if err := a.executePendingTools(ctx, &cp, out); err != nil {
		return err
	}

	maxRound := cp.Round + a.opts.MaxRounds
	for round := cp.Round + 1; round <= maxRound; round++ {
		cp.Round = round
		cp.Phase = fmt.Sprintf("model_round_%d", round)
		cp.History = append(cp.History, cp.Phase)
		if err := saveCheckpoint(cp); err != nil {
			return err
		}

		beforeCompaction := len(cp.Messages)
		cp.Messages = compactMessages(cp.Messages)
		if len(cp.Messages) != beforeCompaction {
			cp.History = append(cp.History, "conversation context compacted")
			if err := saveCheckpoint(cp); err != nil {
				return err
			}
		}
		fmt.Fprintf(out, "\n\x1b[36m[EasyCodeAI · round %d/%d]\x1b[0m\n", round, maxRound)
		reply, err := a.client.Complete(ctx, cp.Messages, specs, func(s string) {
			_, _ = io.WriteString(out, s)
		})
		if err != nil {
			cp.Phase = "interrupted"
			cp.History = append(cp.History, "request failed: "+err.Error())
			_ = saveCheckpoint(cp)
			return err
		}

		cp.Messages = append(cp.Messages, reply)
		if err := saveCheckpoint(cp); err != nil {
			return err
		}

		if len(reply.ToolCalls) > 0 {
			if err := a.executePendingTools(ctx, &cp, out); err != nil {
				return err
			}
			continue
		}

		if strings.Contains(reply.Content, "<easycode-done>") {
			if !canComplete(cp) {
				cp.Messages = append(cp.Messages, nim.Message{
					Role: "user",
					Content: "You modified project files but have not run the available verification checks successfully. Call run_checks now. If the project has no supported checks, report that limitation accurately; do not claim tests passed.",
				})
				if err := saveCheckpoint(cp); err != nil {
					return err
				}
				continue
			}
			cp.Phase = "completed"
			cp.History = append(cp.History, "completed")
			return saveCheckpoint(cp)
		}

		cp.Messages = append(cp.Messages, nim.Message{
			Role:    "user",
			Content: "Continue the task. Use available tools to inspect, edit, and verify. Do not claim completion until checks pass. Finish with <easycode-done> only when complete.",
		})
		if err := saveCheckpoint(cp); err != nil {
			return err
		}
	}

	cp.Phase = "round_limit"
	_ = saveCheckpoint(cp)
	return fmt.Errorf("reached %d-round execution budget; checkpoint saved; run /resume to continue", a.opts.MaxRounds)
}

func (a *Agent) executePendingTools(ctx context.Context, cp *Checkpoint, out io.Writer) error {
	lastAssistant := -1
	for i := len(cp.Messages) - 1; i >= 0; i-- {
		if cp.Messages[i].Role == "assistant" {
			lastAssistant = i
			break
		}
	}
	if lastAssistant < 0 || len(cp.Messages[lastAssistant].ToolCalls) == 0 {
		return nil
	}

	completed := make(map[string]bool)
	for _, msg := range cp.Messages[lastAssistant+1:] {
		if msg.Role == "tool" && msg.ToolCallID != "" {
			completed[msg.ToolCallID] = true
		}
	}

	for _, call := range cp.Messages[lastAssistant].ToolCalls {
		if call.ID == "" {
			return fmt.Errorf("model returned a tool call without an ID")
		}
		if completed[call.ID] {
			continue
		}
		if err := ctx.Err(); err != nil {
			cp.Phase = "interrupted"
			cp.History = append(cp.History, "tool execution interrupted: "+err.Error())
			_ = saveCheckpoint(*cp)
			return err
		}

		fmt.Fprintf(out, "\n\x1b[33m[tool] %s\x1b[0m\n", call.Function.Name)
		result, toolErr := a.tools.Execute(ctx, call.Function.Name, call.Function.Arguments)
		if toolErr != nil {
			result = "TOOL ERROR: " + toolErr.Error()
		}
		if call.Function.Name == "run_checks" && toolErr != nil {
			if strings.Contains(toolErr.Error(), "--allow-project-execution") {
				cp.Verification = "unavailable"
			} else {
				cp.Verification = "failed"
			}
		}
		if toolErr == nil {
			switch call.Function.Name {
			case "write_file":
				cp.Modified = true
				cp.Verification = "not-run"
			case "run_checks":
				switch {
				case strings.HasPrefix(result, "[checks-passed]"):
					cp.Verification = "passed"
				case strings.HasPrefix(result, "[checks-unavailable]"):
					cp.Verification = "unavailable"
				default:
					cp.Verification = "failed"
				}
			}
		}
		cp.Messages = append(cp.Messages, nim.Message{
			Role:       "tool",
			ToolCallID: call.ID,
			Content:    result,
		})
		cp.History = append(cp.History, "tool "+call.Function.Name+": "+truncate(result, 250))
		if err := saveCheckpoint(*cp); err != nil {
			return err
		}
		fmt.Fprintln(out, result)
	}
	return nil
}

func compactMessages(messages []nim.Message) []nim.Message {
	const (
		maxMessages = 100
		keepRecent  = 80
		maxBytes    = 8 << 20
	)
	if len(messages) <= 2 {
		return messages
	}
	total := 0
	for _, message := range messages {
		total += len(message.Role) + len(message.Content) + len(message.ToolCallID)
		for _, call := range message.ToolCalls {
			total += len(call.ID) + len(call.Type) + len(call.Function.Name) + len(call.Function.Arguments)
		}
	}
	if len(messages) <= maxMessages && total <= maxBytes {
		return messages
	}

	candidate := len(messages) - keepRecent
	if candidate < 2 {
		candidate = 2
	}
	if total > maxBytes {
		recentBytes := 0
		for i := len(messages) - 1; i >= 2; i-- {
			message := messages[i]
			size := len(message.Role) + len(message.Content) + len(message.ToolCallID)
			for _, call := range message.ToolCalls {
				size += len(call.ID) + len(call.Type) + len(call.Function.Name) + len(call.Function.Arguments)
			}
			recentBytes += size
			if recentBytes > maxBytes*3/4 {
				candidate = i
				break
			}
		}
	}
	start := -1
	for i := candidate; i < len(messages); i++ {
		if messages[i].Role == "user" || (messages[i].Role == "assistant" && len(messages[i].ToolCalls) == 0) {
			start = i
			break
		}
	}
	if start < 2 {
		return messages
	}

	compacted := make([]nim.Message, 0, len(messages)-start+3)
	compacted = append(compacted, messages[0], messages[1])
	compacted = append(compacted, nim.Message{
		Role: "user",
		Content: "[Context note: older conversation messages were compacted to fit the model context. The original task is preserved above. Re-inspect current workspace files and rely on the current source state rather than assuming omitted conversation details.]",
	})
	compacted = append(compacted, messages[start:]...)
	return compacted
}

func sessionLabel(value string) string {
	if value == "" { return "default" }
	return value
}

func canComplete(cp Checkpoint) bool {
	return !cp.Modified || cp.Verification == "passed" || cp.Verification == "unavailable"
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func (a *Agent) ShowCheckpoint(out io.Writer) error {
	cp, err := loadCheckpoint()
	if err != nil {
		return fmt.Errorf("no saved checkpoint: %w", err)
	}
	fmt.Fprintf(out, "Task: %s\nSession: %s\nWorkspace: %s\nPhase: %s\nRound: %d\nVerification: %s\nUpdated: %s\n",
		cp.Task, sessionLabel(cp.Session), cp.Workspace, cp.Phase, cp.Round, cp.Verification, cp.UpdatedAt.Format("2006-01-02 15:04:05 MST"))
	for _, item := range cp.History {
		fmt.Fprintln(out, " -", item)
	}
	return nil
}

func systemPrompt(root string) string {
	return "You are EasyCodeAI, an autonomous coding agent in Termux. Workspace: " + root +
		". Start with inspect_project and targeted search_files/read_file calls. Never assume a language, framework, command, or file that you have not inspected. Use only the provided tools; never invent shell access. Stay inside the workspace. Before changing existing files, inspect their complete relevant contents and preserve unrelated behavior. Use small coherent edits, then run_checks. Python and JavaScript checks are syntax-only; Go tests/build execute project code and are available only when the user explicitly enabled them. If a required tool is denied, explain the limitation and do not pretend it ran. Continue iteratively: inspect, plan, edit, test, diagnose, fix, retest. Do not claim success without evidence. Finish with <easycode-done> only after the task is actually complete and the available checks pass."
}
