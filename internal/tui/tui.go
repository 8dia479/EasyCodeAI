package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Engine interface {
	Run(context.Context, string, io.Writer) error
	Resume(context.Context, io.Writer) error
	ShowCheckpoint(io.Writer) error
}

type outputMsg string
type programSender struct{ p *tea.Program }
type taskDoneMsg struct{ err error }

type streamWriter struct{ send func(tea.Msg) }

func (w streamWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		w.send(outputMsg(string(p)))
	}
	return len(p), nil
}

type model struct {
	engine    Engine
	sender    *programSender
	workspace string
	modelName string
	input     []rune
	cursor    int
	logs      []string
	width     int
	height    int
	scroll    int
	running   bool
	cancel    context.CancelFunc
	stopping  bool
}

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")).Padding(0, 1)
	mutedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	userStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	inputStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
)

func Run(engine Engine, workspace, modelName string) error {
	sender := &programSender{}
	m := model{
		sender: sender,
		engine: engine, workspace: workspace, modelName: modelName,
		width: 80, height: 24,
		logs: []string{
			"EasyCodeAI is ready. Describe a task or type /help.",
			"Changes are checkpointed automatically. Ctrl+C cancels the active task.",
		},
	}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	sender.p = p
	_, err := p.Run()
	return err
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case outputMsg:
		m.appendLog(string(msg))
	case taskDoneMsg:
		m.running = false
		m.cancel = nil
		m.stopping = false
		if msg.err != nil {
			if errors.Is(msg.err, context.Canceled) {
				m.appendLog("\n[interrupted] Task cancelled. Checkpoint saved when interruption was observed. Use /resume to continue.")
			} else {
				m.appendLog("\n[task stopped] " + msg.err.Error())
			}
		} else {
			m.appendLog("\n[task finished]")
		}
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.running {
				if !m.stopping {
					m.stopping = true
					m.appendLog("\n[interrupt] Cancelling task and saving progress...")
					if m.cancel != nil {
						m.cancel()
					}
				}
				return m, nil
			}
			return m, tea.Quit
		case tea.KeyCtrlD:
			if !m.running && len(m.input) == 0 {
				return m, tea.Quit
			}
		case tea.KeyEnter:
			if m.running {
				return m, nil
			}
			line := strings.TrimSpace(string(m.input))
			m.input = nil
			m.cursor = 0
			if line == "" {
				return m, nil
			}
			return m.handleInput(line)
		case tea.KeyBackspace, tea.KeyDelete:
			if m.cursor > 0 {
				m.input = append(m.input[:m.cursor-1], m.input[m.cursor:]...)
				m.cursor--
			}
		case tea.KeyLeft:
			if m.cursor > 0 { m.cursor-- }
		case tea.KeyRight:
			if m.cursor < len(m.input) { m.cursor++ }
		case tea.KeyHome:
			m.cursor = 0
		case tea.KeyEnd:
			m.cursor = len(m.input)
		case tea.KeyUp:
			if m.cursor == 0 { m.scroll++ }
		case tea.KeyDown:
			if m.cursor == 0 && m.scroll > 0 { m.scroll-- }
		case tea.KeyPgUp:
			m.scroll += max(1, m.height/2)
		case tea.KeyPgDown:
			m.scroll = max(0, m.scroll-max(1, m.height/2))
		case tea.KeyRunes:
			if !m.running && len(msg.Runes) > 0 {
				runes := append([]rune(nil), msg.Runes...)
				m.input = append(m.input[:m.cursor], append(runes, m.input[m.cursor:]...)...)
				m.cursor += len(runes)
			}
		case tea.KeySpace:
			if !m.running {
				m.input = append(m.input[:m.cursor], append([]rune{' '}, m.input[m.cursor:]...)...)
				m.cursor++
			}
		}
	}
	return m, nil
}

func (m model) handleInput(line string) (tea.Model, tea.Cmd) {
	if strings.HasPrefix(line, "/") {
		parts := strings.Fields(line)
		command := parts[0]
		switch command {
		case "/help":
			m.appendLog("Commands:\n  /help       Show this help\n  /resume     Resume the saved task\n  /status     Show checkpoint and task status\n  /model      Show the configured model\n  /workspace  Show the active project directory\n  /clear      Clear the visible conversation\n  /exit       Exit EasyCodeAI\n\nCtrl+C cancels an active task; when idle it exits.")
		case "/model":
			m.appendLog("Model: " + m.modelName + "\nChange NIM_MODEL before starting EasyCodeAI to use another model.")
		case "/workspace":
			m.appendLog("Workspace: " + m.workspace)
		case "/clear":
			m.logs = []string{"Visible conversation cleared. Saved task checkpoints are unchanged."}
			m.scroll = 0
		case "/status":
			return m, m.runCommand(func(ctx context.Context, out io.Writer) error {
				return m.engine.ShowCheckpoint(out)
			})
		case "/resume":
			return m, m.startTask("", true)
		case "/exit", "/quit":
			return m, tea.Quit
		default:
			m.appendLog("Unknown command: " + command + " — type /help.")
		}
		return m, nil
	}
	return m, m.startTask(line, false)
}

func (m *model) startTask(prompt string, resume bool) tea.Cmd {
	if m.running {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.running = true
	m.stopping = false
	if resume {
		m.appendLog("\n> /resume\n")
	} else {
		m.appendLog("\n" + userStyle.Render("You") + "\n" + prompt + "\n")
	}
	return func() tea.Msg {
		writer := streamWriter{send: func(msg tea.Msg) { m.sender.p.Send(msg) }}
		var err error
		if resume {
			err = m.engine.Resume(ctx, writer)
		} else {
			err = m.engine.Run(ctx, prompt, writer)
		}
		return taskDoneMsg{err: err}
	}
}

func (m *model) runCommand(run func(context.Context, io.Writer) error) tea.Cmd {
	if m.running { return nil }
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.running = true
	return func() tea.Msg {
		writer := streamWriter{send: func(msg tea.Msg) { m.sender.p.Send(msg) }}
		return taskDoneMsg{err: run(ctx, writer)}
	}
}

func (m *model) appendLog(value string) {
	if value == "" { return }
	parts := strings.Split(value, "\n")
	if len(m.logs) == 0 { m.logs = append(m.logs, "") }
	for i, part := range parts {
		if i == 0 {
			m.logs[len(m.logs)-1] += part
		} else {
			m.logs = append(m.logs, part)
		}
	}
	if len(m.logs) > 3000 {
		m.logs = append([]string(nil), m.logs[len(m.logs)-3000:]...)
	}
	m.scroll = 0
}

func (m model) View() string {
	width := max(40, m.width)
	header := headerStyle.Width(width).Render("EasyCodeAI  ·  " + m.modelName)
	meta := mutedStyle.Width(width).Render(m.workspace)
	bodyHeight := max(4, m.height-7)
	wrapped := make([]string, 0, len(m.logs))
	for _, line := range m.logs {
		wrapped = append(wrapped, wrapLine(line, width-4)...)
	}
	end := len(wrapped) - m.scroll
	if end < 0 { end = 0 }
	start := max(0, end-bodyHeight)
	if end-start > bodyHeight { start = end-bodyHeight }
	body := strings.Join(wrapped[start:end], "\n")
	if body == "" { body = " " }
	status := "READY"
	if m.running {
		status = "WORKING"
		if m.stopping { status = "CANCELLING" }
	}
	footer := mutedStyle.Width(width).Render(fmt.Sprintf("%s  ·  %d lines  ·  ↑/↓ scroll  ·  Ctrl+C interrupt/exit", status, len(wrapped)))
	prompt := "› " + string(m.input[:m.cursor]) + "█" + string(m.input[m.cursor:])
	if m.running { prompt = "› Working… (Ctrl+C to cancel)" }
	input := inputStyle.Width(width).Render(prompt)
	return lipgloss.JoinVertical(lipgloss.Left, header, meta, strings.Repeat("─", width), body, strings.Repeat("─", width), input, footer)
}

func wrapLine(line string, width int) []string {
	if width < 1 { width = 1 }
	if utf8.RuneCountInString(line) <= width { return []string{line} }
	runes := []rune(line)
	out := make([]string, 0, len(runes)/width+1)
	for len(runes) > width {
		out = append(out, string(runes[:width]))
		runes = runes[width:]
	}
	return append(out, string(runes))
}
