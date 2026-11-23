# EasyCodeAI

A Termux-first, OpenAI-compatible coding agent powered by NVIDIA NIM.

> Alpha0.1 — early development. The first milestone establishes a portable Go CLI, NIM streaming client, resumable task state, and a guarded local tool runner.

## Goals

- Run natively in Termux without a desktop runtime.
- Use NVIDIA NIM chat-completions-compatible endpoints.
- Keep each model response streaming for as long as practical.
- Persist task checkpoints so interrupted work can resume.
- Execute repository tools locally with explicit workspace boundaries.
- Iterate through inspect → plan → edit → test → repair, while making every model round-trip visible.

## Important queueing note

EasyCodeAI does not assume that a NIM request can be kept alive after the model has completed its response. Tool execution that requires sending tool results back to the model necessarily starts another inference request and may encounter the provider queue again. The agent therefore supports batched tool planning and durable checkpoints to reduce unnecessary round-trips without pretending that one completed response is a persistent server session.

## Requirements

- Go 1.22+
- Termux packages: `pkg install golang git`
- NVIDIA NIM API key and an OpenAI-compatible NIM endpoint

## Quick start

```sh
go build -o easycode ./cmd/easycode
./easycode --help
```

Set credentials using environment variables:

```sh
export NIM_API_KEY="..."
export NIM_BASE_URL="https://integrate.api.nvidia.com/v1"
export NIM_MODEL="nvidia/llama-3.3-nemotron-super-49b-v1"
```

Then run:

```sh
./easycode
```

## Configuration

Environment variables are preferred for secrets:

- `NIM_API_KEY` — required
- `NIM_BASE_URL` — defaults to NVIDIA's hosted NIM endpoint
- `NIM_MODEL` — required; model IDs vary by NIM catalog
- `EASYCODE_WORKSPACE` — defaults to the current directory
- `EASYCODE_STATE_DIR` — defaults to `~/.easycode`

## OpenCode reference

The CLI interaction and agent architecture are informed by the public OpenCode project. EasyCodeAI is an independent Go implementation, not a copy of OpenCode source. OpenCode is MIT-licensed; if code is later adapted directly, its copyright and license notice must be preserved.

## Status

Alpha0.1 currently contains the initial CLI, NIM streaming transport, checkpoint persistence, and safe workspace file/shell tools. Tool-calling orchestration, richer terminal UI, model discovery, and queue-aware scheduling are planned next.
