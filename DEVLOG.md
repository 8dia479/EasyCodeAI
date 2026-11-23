# EasyCodeAI Development Log

## 2026-11-23 — Alpha0.1 reliability milestone

### Implemented

- Added versioned checkpoints that persist the complete NIM conversation, including assistant tool calls and tool results.
- Added interactive `/resume` and `/status` commands, plus the one-shot `--resume` flag.
- Resume validates that the saved task belongs to the current workspace and refuses to restart a completed task.
- Pending tool calls are recovered on resume; already persisted tool results are not executed again.
- A resumed task receives another bounded execution window instead of immediately hitting the previous round limit; `--max-rounds` configures each invocation (1–200).
- Added NIM retry handling for HTTP 429, 502, 503, and 504 responses, honoring short Retry-After values without retrying ahead of long server delays and using bounded exponential fallback delays.
- Made workspace writes atomic, preserved existing file permissions, and limited individual writes to 4 MiB.
- Added per-command verification timeouts and output caps, and filtered sensitive environment variables from test/build subprocesses.
- Added five-minute timeouts and 1 MiB output limits to each fixed Go verification command.
- Filtered project-check environment variables so NIM API credentials and model settings are not inherited by test/build processes.
- Added regression tests for checkpoint serialization, resume workspace isolation, retry behavior, output limits, secret filtering, atomic workspace writes, project discovery, source search, and overwrite permissions.
- Added project inspection and bounded source search, plus syntax-only checks for Python and JavaScript.
- Added isolated named sessions, session status/help commands, and protection against replacing unfinished tasks.
- Added context compaction that preserves the original task and avoids cutting through pending tool-call turns.
- Added verification-state tracking: edits must be checked or explicitly reported as uncheckable before task completion.
- Added a Termux installer, Makefile targets, secret/binary ignore rules, and a local `--doctor` diagnostic that does not contact NIM.
- Added explicit `--allow-overwrite` and `--allow-project-execution` permissions; Go tests/build are unavailable unless explicitly enabled.
- Added request/response size limits and a 15-minute overall NIM request timeout; sparse SSE tool-call indices are preserved.

### Verification state

- Regression tests were added but have **not** been executed in this environment.
- A local clone/build attempt could not reach GitHub because network name resolution was unavailable.
- The latest commit's GitHub combined status endpoint returned no status entries; this is not evidence of a passing CI run.
- No live NVIDIA NIM request has been made.
- Do not treat this milestone as build-verified until `go test -race ./...`, `go vet ./...`, and `go build ./...` complete successfully.

### Known limitations / next work

- Each named session stores one active task; session listing and deletion are not implemented yet.
- Filesystem path checks still cannot eliminate all TOCTOU races.
- Verification runs trusted-project Go tests/build code as the local user; environment filtering reduces credential exposure but is not an OS sandbox.
- File replacement and project-code execution now require explicit CLI opt-ins; this is a permission gate, not an OS sandbox.
- Add deeper TypeScript/Java/Kotlin/Rust/C/C++/.NET checks and more NIM compatibility tests.
- Add session listing/deletion, richer terminal UX, and configurable per-session execution budgets.
