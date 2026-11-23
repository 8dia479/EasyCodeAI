# EasyCodeAI

A Termux-first, OpenAI-compatible coding agent powered by NVIDIA NIM.

**Alpha0.1** establishes a native Go CLI, streamed NIM chat-completions transport, model tool-calling, workspace-scoped file tools, a restricted Go verification tool, and persistent task checkpoints.

## Design goals

- Run directly in Termux; no desktop runtime or hosted agent server.
- Use NVIDIA NIM's OpenAI-compatible API.
- Stream each inference response and batch multiple tool calls into one model turn where supported.
- Persist progress so interrupted tasks can be inspected and resumed.
- Keep file access inside the selected workspace.
- Iterate through inspect → plan → edit → test → repair.

## Queueing strategy and limitations

EasyCodeAI batches all tool calls returned in one assistant response, executes them locally, then sends the collected tool results together in the next inference request. This avoids a separate model request for every individual tool. It does **not** bypass NVIDIA's queue: each new inference request can be queued again. A completed response is not treated as a persistent server-side session, and streaming is not described as queue-free execution.

## Requirements

- Go 1.22+
- Termux: `pkg install golang git`
- NVIDIA NIM API key and an OpenAI-compatible NIM endpoint

## Build and run

```sh
git clone https://github.com/8dia479/EasyCodeAI.git
cd EasyCodeAI
git switch Alpha0.1
go build -o easycode ./cmd/easycode
```

Set the API configuration:

```sh
export NIM_API_KEY="..."
export NIM_BASE_URL="https://integrate.api.nvidia.com/v1"
export NIM_MODEL="your/nim-model-id"
```

Run inside the project you want the agent to work on:

```sh
/path/to/EasyCodeAI/easycode
```

Or give a one-shot task:

```sh
easycode --workspace . --model "$NIM_MODEL" --prompt "Inspect this project, fix the failing tests, and verify the changes."
```

## Environment variables

- `NIM_API_KEY` — required; keep it out of source control
- `NIM_BASE_URL` — defaults to `https://integrate.api.nvidia.com/v1`
- `NIM_MODEL` — required unless passed with `--model`
- `EASYCODE_WORKSPACE` — workspace path; defaults to current directory
- `EASYCODE_STATE_DIR` — checkpoint directory; defaults to `~/.easycode`

## Current tools

- `list_files` — list workspace entries
- `read_file` — read a workspace file
- `write_file` — create or replace a workspace file
- `run_checks` — run Go test, vet, and build

The shell is not exposed to the model in Alpha0.1. The verification tool only runs fixed Go commands.

## OpenCode reference

The CLI interaction and agent architecture are informed by the public OpenCode project. EasyCodeAI is an independent Go implementation. OpenCode is MIT-licensed; any future direct code adaptation must retain the applicable copyright and license notices.

## Development status

Alpha0.1 is an early foundation. Remaining work includes cross-language project checks, stronger symlink-aware workspace isolation, tool-call recovery after partial streams, explicit approval controls, a richer terminal UI, and queue-aware retry/backoff based on observed NIM behavior.
