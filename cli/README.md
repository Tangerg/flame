# Flame CLI

Flame CLI provides a scriptable command surface and an interactive terminal client over the public in-process Flame Runtime. Runtime owns durable product state and execution; the CLI owns process behavior, terminal interaction, rendering, and local prompt authoring.

## Run the CLI

Run from `cli`:

```sh
go run .
go run . run "explain the failing test"
go run . run --json "summarize this workspace"
go run . run --output-format streaming-json "trace this change"
go run . sessions ls
go run . config show
```

The interactive command opens the Oolong TUI. `run` supports human-readable text, one-result JSON, and newline-delimited streaming JSON.

## Configuration

CLI preferences and Runtime configuration have separate owners.

CLI preferences configure optional provider/model overrides, run limits, approvals, UI, plugins, and key bindings. With no override, new Runs inherit the active Session's Runtime-owned durable selection:

- `./.flame.yaml` is the project-local CLI preferences file
- without it, Flame reads `flame/config.yaml` below the OS user config directory
- `--config` selects an explicit CLI preferences file
- `FLAME_CLI_*` variables and CLI flags override file values
- `flame config path` and `flame config show` report the selected file and merged result

[`config/config.example.yaml`](config/config.example.yaml) is a valid strict-schema example. Provider credentials, custom endpoints, utility/embedding roles, server settings, sandboxing, MCP, and LSP remain Runtime-owned:

- `$FLAME_HOME/runtime/config.yaml` is the user configuration
- `runtime/config/config.yaml` is the worktree development fallback
- `FLAME_RUNTIME_CONFIG_DIR` selects an explicit absolute configuration directory
- `FLAME_RUNTIME=mock` selects the scripted adapter for deterministic tests and demos

The worktree development file contains a live DeepSeek credential. Production code may load it for an explicitly requested live test. Commands, tests, logs, errors, snapshots, and documentation must never print or copy the credential.

Provider and model identity is exact. An override is either a complete provider/model pair or absent; absence is not a DeepSeek default and does not overwrite a Session. Configuration and output preserve the pair and model-owned options, and the CLI never infers a provider from a model name. Runtime deployment variables keep the `FLAME_*` namespace; CLI consumer preferences use `FLAME_CLI_*`, so one process cannot interpret the same environment variable as two owners' settings.

## Runtime ownership

The process lazily opens one module-root `runtime.Runtime` and closes it before exit. The Runtime boundary imports public contracts; CLI application and terminal packages consume CLI-owned narrow interfaces and values.

Runtime remains authoritative for:

- Sessions, Runs, Segments, Items, Goals, Plans, and Interrupts
- provider and model catalogs, credentials, and execution options
- tools, workspace facts, persistence, context compaction, and recovery
- idempotency, replay, revisions, and protocol errors

The CLI treats streaming deltas as previews. Completed Items and authoritative snapshots win after reconnect or cold recovery.

## Interactive use

The TUI keeps the conversation and composer stable while exposing Runtime-owned Goal, Plan, Run, tool, approval, question, provider, and workspace state. Searchable overlays provide Sessions, commands, models, queued prompts, files, tools, Skills, Model Context Protocol (MCP) servers, schedules, memory, knowledge, hooks, and full-content inspection.

Use `/help` or the command palette for the current command and shortcut catalog. Keymaps are context-sensitive and remain the source of truth for the shortcut guide.

Grok Build is the primary visual and interaction reference. Flame follows its information density, streaming stability, composer behavior, and Goal/Plan placement while preserving Flame's own Runtime semantics.

## Architecture

[`ARCHITECTURE.md`](ARCHITECTURE.md) defines module ownership, package criteria, Runtime isolation, command construction, TUI state, provider projection, and test strategy. [`AGENTS.md`](AGENTS.md) contains the mandatory editing rules.

The current package tree contains several historical micro-packages. Refactoring should merge only packages proved to be forwarding-only or ownerless. Do not flatten by directory count, and do not replace the tree with a broad service package.

## Verify changes

Run the standalone module gates:

```sh
GOWORK=off go test ./...
GOWORK=off go vet ./...
GOWORK=off go build ./...
GOWORK=off go mod tidy
```

Run focused real in-process Runtime scenarios for changes to Goal, Plan, steering, human input, compaction, provider behavior, or recovery. Use the real pseudo-terminal suite only when terminal protocol, resize, input, focus, or restoration changes. Run targeted race tests only when concurrent ownership changes.
