# EasyCodeAI

A Termux-first, OpenAI-compatible coding agent powered by NVIDIA NIM.

**Alpha0.1** provides a Termux-first Go CLI coding agent with streamed NVIDIA NIM tool-calling, resumable conversations, workspace-aware project discovery and source search, guarded file modification, and bounded verification.

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

On Termux, install the prerequisites and run the included installer:

```sh
pkg install git golang
git clone --branch Alpha0.1 https://github.com/8dia479/EasyCodeAI.git
cd EasyCodeAI
bash scripts/install-termux.sh
```

The installer builds the CLI and installs it to `$PREFIX/bin/easycode`. For a manual build:

```sh
go build -trimpath -o easycode ./cmd/easycode
```

Run `make check` to execute the race-enabled tests, vet, and build checks locally.

## Environment variables

- `NIM_API_KEY` — required; keep it out of source control
- `NIM_BASE_URL` — defaults to `https://integrate.api.nvidia.com/v1`
- `NIM_MODEL` — required unless passed with `--model`
- `EASYCODE_WORKSPACE` — workspace path; defaults to current directory
- `EASYCODE_STATE_DIR` — checkpoint directory; defaults to `~/.easycode`
- `EASYCODE_SESSION` — optional named task session; can also be set with `--session`\n- `EASYCODE_SESSION` — optional named task session; can also be set with `--session`

## Current tools

- `list_files` — list workspace entries
- `read_file` — read a workspace file
- `write_file` — create or replace a workspace file (existing-file replacement requires `--allow-overwrite`)
- `inspect_project` — detect project languages and common manifests without executing project code
- `search_files` — bounded case-insensitive source search, skipping dependency/build folders
- `run_checks` — Python AST and JavaScript syntax checks; Go test/vet/build only when explicitly enabled

Use `--max-rounds N` (1–200) to set the model-round budget for each invocation; `/resume` or `--resume` continues the same task with a fresh budget.

The model does not receive arbitrary shell access. Python and JavaScript checks are syntax-only; Go tests/build can execute project code and require `--allow-project-execution`. All verification subprocesses have a five-minute timeout and a 1 MiB output cap.

In interactive mode, `/resume` restores the saved conversation, `/status` displays the current session, and `/help` lists commands. Checkpoints are stored with owner-only file permissions. The default session uses `checkpoint.json`; named sessions use isolated files under `sessions/`, allowing separate projects/tasks to be resumed independently. Select the same `--session` name to resume a task later. A saved task can only resume in its original workspace.

## OpenCode reference

The CLI interaction and agent architecture are informed by the public OpenCode project. EasyCodeAI is an independent Go implementation. OpenCode is MIT-licensed; any future direct code adaptation must retain the applicable copyright and license notices.

## Development status

Alpha0.1 is an early foundation. Recent hardening adds symlink-aware workspace path checks and regression tests, plus delivery of non-streaming NIM response text. After changes, EasyCodeAI requires a successful supported check or records that no safe checker is available before accepting completion. Remaining work includes deeper TypeScript/Java/Kotlin/Rust/C/C++/.NET checks, stronger filesystem race resistance (the current path checks cannot eliminate all filesystem TOCTOU races), session listing/deletion and a richer terminal UI, and queue telemetry/adaptive scheduling based on measured NIM behavior. CI is configured for race-enabled tests, vet, and build; a passing run has not yet been independently verified.
