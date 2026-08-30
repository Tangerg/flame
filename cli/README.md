# Flame CLI

Flame CLI provides a scriptable command surface and an interactive terminal client over the public embedded Flame Runtime. Runtime owns durable product state and execution; the CLI owns process behavior, terminal interaction, rendering, and local prompt authoring.

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

The CLI uses the same Runtime configuration and data model as other Flame clients:

- `$FLAME_HOME/runtime/config.yaml` is the user configuration
- `runtime/config/config.yaml` is the worktree development fallback
- `FLAME_RUNTIME_CONFIG_DIR` selects an explicit absolute configuration directory
- `FLAME_RUNTIME=mock` selects the scripted adapter for deterministic tests and demos

The worktree development file contains a live DeepSeek credential. Production code may load it for an explicitly requested live test. Commands, tests, logs, errors, snapshots, and documentation must never print or copy the credential.

Provider and model identity is exact. Configuration and output preserve the provider/model pair and model-owned options; the CLI never infers a provider from a model name.

## Runtime ownership

The process lazily opens one `runtime/embedded.Runtime` and closes it before exit. `internal/runtimeembedded` is the only package that imports Runtime public contracts. CLI application and terminal packages consume CLI-owned narrow interfaces and values.

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

Run focused real embedded Runtime scenarios for changes to Goal, Plan, steering, human input, compaction, provider behavior, or recovery. Use the real pseudo-terminal suite only when terminal protocol, resize, input, focus, or restoration changes. Run targeted race tests only when concurrent ownership changes.
