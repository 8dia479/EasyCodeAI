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

The installer builds the CLI and installs it to `$PREFIX/bin/easycode`. Run `easycode --doctor` to check the local Go/Python/Node tools and NIM configuration without sending an API request. For a manual build:

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
- `EASYCODE_SESSION` — optional named task session; can also be set with `--session`

## Run the agent

Set your NVIDIA NIM credentials and model ID:

```sh
export NIM_API_KEY="your-api-key"
export NIM_MODEL="your-nim-model-id"
export NIM_BASE_URL="https://integrate.api.nvidia.com/v1"
```

Start the OpenCode-inspired full-screen terminal UI in the project directory:

```sh
cd /path/to/your/project
easycode --workspace . --session my-project --allow-overwrite --allow-project-execution
```

Or submit a one-shot task:

```sh
easycode --workspace . --session my-project --allow-overwrite --allow-project-execution --max-rounds 40 --prompt "Inspect the project, implement the requested change, and verify it."
```

Resume that same task later with the same session name:

```sh
easycode --workspace . --session my-project --allow-overwrite --allow-project-execution --max-rounds 40 --resume
```

Omit `--allow-overwrite` to prevent replacement of existing files. Omit `--allow-project-execution` to keep Go tests/build disabled; Python and JavaScript syntax-only checks remain available.

## Current tools

- `list_files` — list workspace entries
- `read_file` — read a workspace file (up to 1 MiB) or a bounded line range in larger files
- `write_file` — create or replace a workspace file (existing-file replacement requires `--allow-overwrite`)
- `inspect_project` — detect project languages and common manifests without executing project code
- `search_files` — bounded case-insensitive source search, skipping dependency/build folders
- `run_checks` — Python AST and JavaScript syntax checks; Go test/vet/build only when explicitly enabled

Use `--max-rounds N` (1–200) to set the model-round budget for each invocation; `/resume` or `--resume` continues the same task with a fresh budget.

The model does not receive arbitrary shell access. Existing files are backed up under the EasyCodeAI state directory before an allowed replacement (up to 16 MiB per source file). Large files can be inspected in line ranges of up to 1000 lines per call. Python and JavaScript checks are syntax-only; Go tests/build can execute project code and require `--allow-project-execution`. All verification subprocesses have a five-minute timeout and a 1 MiB output cap.

The default interactive mode is a full-screen terminal UI: the header shows the active model and workspace, the main pane streams agent responses and tool activity, and the footer shows task state. Type a task and press Enter. Commands: `/help`, `/resume`, `/status`, `/model [ID]`, `/workspace`, `/clear`, and `/exit`. Press Tab to complete slash commands. While a task is running, Ctrl+C cancels it and preserves its checkpoint; while idle, Ctrl+C exits. Arrow keys edit the prompt or scroll the log. Checkpoints are stored with owner-only file permissions. The default session uses `checkpoint.json`; named sessions use isolated files under `sessions/`, allowing separate projects/tasks to be resumed independently. Select the same `--session` name to resume a task later. A saved task can only resume in its original workspace.

## OpenCode reference

The CLI interaction and agent architecture are informed by the public OpenCode project. EasyCodeAI is an independent Go implementation. OpenCode is MIT-licensed; any future direct code adaptation must retain the applicable copyright and license notices.

## Development status

Alpha0.1 now includes the core Termux CLI workflow: NIM streaming/tool calls, project discovery, source search, guarded file writes, resumable named sessions, bounded context compaction, and verification-state tracking. Regression tests cover the main safety and recovery paths, but they still need to be executed on a machine with GitHub access. After changes, EasyCodeAI requires a successful supported check or records that no safe checker is available before accepting completion. Remaining work includes deeper TypeScript/Java/Kotlin/Rust/C/C++/.NET checks, stronger filesystem race resistance (the current path checks cannot eliminate all filesystem TOCTOU races), session listing/deletion, richer tool cards and approval prompts, and queue telemetry/adaptive scheduling based on measured NIM behavior. CI is configured for race-enabled tests, vet, and build; a passing run has not yet been independently verified.
