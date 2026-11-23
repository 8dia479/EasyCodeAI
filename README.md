# EasyCodeAI

A Termux-first, OpenAI-compatible coding agent powered by NVIDIA NIM.

**Alpha0.1** establishes a native Go CLI, streamed NIM chat-completions transport, model tool-calling, workspace-scoped file tools, a restricted Go verification tool, and persistent task checkpoints.

## Design goals

- Run directly in Termux; no desktop runtime or hosted agent server.
- Use NVIDIA NIM's OpenAI-compatible API.
- Stream each inference response, batch multiple tool calls into one model turn where supported, and retry transient 429/502/503/504 responses with bounded backoff.
- Persist the full model conversation, assistant tool calls, and tool results so interrupted tasks can be resumed.
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

Resume the most recently saved task in the same workspace:

```sh
easycode --workspace . --model "$NIM_MODEL" --max-rounds 40 --resume
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

Use `--max-rounds N` (1–200) to set the model-round budget for each invocation; `/resume` or `--resume` continues the same task with a fresh budget.\n\nThe shell is not exposed to the model in Alpha0.1. The verification tool only runs fixed Go commands, each with a five-minute timeout and a 1 MiB output cap.\n\nIn interactive mode, `/resume` restores the saved conversation and continues from its last completed tool result; `/status` displays the checkpoint. Checkpoints are stored under `~/.easycode/checkpoint.json` (or `EASYCODE_STATE_DIR`) with owner-only file permissions. Only one active checkpoint is retained, and a saved task can only resume in its original workspace.

## OpenCode reference

The CLI interaction and agent architecture are informed by the public OpenCode project. EasyCodeAI is an independent Go implementation. OpenCode is MIT-licensed; any future direct code adaptation must retain the applicable copyright and license notices.

## Development status

Alpha0.1 is an early foundation. Recent hardening adds symlink-aware workspace path checks and regression tests, plus delivery of non-streaming NIM response text. Remaining work includes cross-language project checks, stronger filesystem race resistance (the current path checks cannot eliminate filesystem TOCTOU races), explicit approval controls, a richer terminal UI, and queue-aware retry/backoff based on observed NIM behavior. CI is configured for race-enabled tests, vet, and build; the latest run has not yet been independently verified.
