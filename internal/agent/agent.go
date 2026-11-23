package agent

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/8dia479/EasyCodeAI/internal/nim"
	"github.com/8dia479/EasyCodeAI/internal/workspace"
)

type Options struct {
	MaxRounds int
}

type Agent struct {
	client *nim.Client
	ws     *workspace.Workspace
	opts   Options
}

func New(client *nim.Client, ws *workspace.Workspace, opts Options) *Agent {
	if opts.MaxRounds <= 0 {
		opts.MaxRounds = 8
	}
	return &Agent{client: client, ws: ws, opts: opts}
}

func (a *Agent) Run(ctx context.Context, task string, out io.Writer) error {
	cp := Checkpoint{Task: task, Phase: "starting", History: []string{}}
	messages := []nim.Message{
		{Role: "system", Content: systemPrompt(a.ws.Root)},
		{Role: "user", Content: task},
	}
	for round := 1; round <= a.opts.MaxRounds; round++ {
		cp.Phase = fmt.Sprintf("model_round_%d", round)
		cp.History = append(cp.History, cp.Phase)
		if err := saveCheckpoint(cp); err != nil {
			return fmt.Errorf("save checkpoint: %w", err)
		}
		fmt.Fprintf(out, "\n\x1b[36m[EasyCodeAI · round %d/%d]\x1b[0m\n", round, a.opts.MaxRounds)
		var response strings.Builder
		text, err := a.client.Complete(ctx, messages, func(delta string) {
			response.WriteString(delta)
			_, _ = io.WriteString(out, delta)
		})
		if err != nil {
			cp.Phase = "interrupted"
			cp.History = append(cp.History, "error: "+err.Error())
			_ = saveCheckpoint(cp)
			return err
		}
		if response.Len() == 0 {
			response.WriteString(text)
		}
		messages = append(messages, nim.Message{Role: "assistant", Content: response.String()})
		if strings.Contains(response.String(), "<easycode-done>") {
			cp.Phase = "completed"
			cp.History = append(cp.History, "completed")
			return saveCheckpoint(cp)
		}
		if round == a.opts.MaxRounds {
			cp.Phase = "round_limit"
			return saveCheckpoint(cp)
		}
		// Until structured tool execution is enabled, continue with one concise
		// self-directed iteration rather than silently claiming the task is done.
		messages = append(messages, nim.Message{Role: "user", Content: "Continue the same task. Review your previous response, identify the next concrete implementation or verification step, and do it. If you cannot actually modify or test files with the tools currently available, state that limitation. Finish only when the task is genuinely complete; then include <easycode-done>."})
		time.Sleep(150 * time.Millisecond)
	}
	return nil
}

func (a *Agent) ShowCheckpoint(out io.Writer) error {
	cp, err := loadCheckpoint()
	if err != nil {
		return fmt.Errorf("no saved checkpoint: %w", err)
	}
	fmt.Fprintf(out, "Task: %s\nPhase: %s\nUpdated: %s\n", cp.Task, cp.Phase, cp.UpdatedAt.Format(time.RFC3339))
	for _, item := range cp.History {
		fmt.Fprintf(out, " - %s\n", item)
	}
	return nil
}

func systemPrompt(root string) string {
	return "You are EasyCodeAI, a persistent coding agent running in a Termux CLI. Workspace: " + root + ". Be honest about capabilities. This Alpha0.1 build has no model tool-calling bridge yet, so do not claim to have edited files, run commands, or tested code. Produce concrete patches or commands only when asked, and use <easycode-done> only when the requested task is actually complete. Prefer concise, actionable progress."
}
