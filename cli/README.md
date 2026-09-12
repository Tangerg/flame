# Flame CLI

Flame CLI provides scriptable commands and an interactive terminal client over the public in-process Flame Runtime.

```sh
cd cli
go run .
go run . run "explain the failing test"
go run . run --json "summarize this workspace"
go run . sessions ls
go run . config show
```

Running without a subcommand opens the Oolong terminal interface. Runtime owns durable product state and execution; CLI owns process behavior, terminal interaction, rendering, and local authoring state.

## Configuration

CLI preferences and Runtime configuration have separate owners.

- `.flame.yaml` in the selected workspace (`.` or `-C`) is the project-local CLI preferences file.
- The OS user configuration directory provides the default CLI preferences file.
- `--config` selects an explicit CLI YAML file.
- `FLAME_CLI_*` variables and flags override file values.
- `$FLAME_HOME/runtime/config.yaml` is Runtime-owned configuration.
- `FLAME_RUNTIME_CONFIG_DIR` selects the sole Runtime configuration directory.

The process working directory is never an implicit Runtime configuration source. Source checkouts that use `runtime/config/config.yaml` select it explicitly with `FLAME_RUNTIME_CONFIG_DIR`.

Provider selection is either an exact provider/model pair or absent. Absence means that Runtime applies the active Session selection; CLI never infers a provider from a model name.

## Architecture

[`ARCHITECTURE.md`](ARCHITECTURE.md) defines ownership, dependency direction, Runtime isolation, command construction, terminal state, and local authoring. Mandatory module rules live in [Module instructions](#module-instructions) below.

## Verify

```sh
GOWORK=off go test ./...
GOWORK=off go vet ./...
GOWORK=off go build ./...
```

Use real Runtime scenarios for changed product flows and real PTY tests only when terminal behavior is the contract.

## Module instructions

Flame CLI owns command routing, process behavior, terminal interaction, rendering, and CLI-local authoring state. It consumes Runtime; it is not a second Runtime.

Read [`../AGENTS.md`](../AGENTS.md), [`../DEVELOPMENT.md`](../DEVELOPMENT.md), and [`ARCHITECTURE.md`](ARCHITECTURE.md) before changing this module.

- Runtime remains authoritative for Session, Run, Segment, Item, Goal, Plan, Interrupt, provider/model selection, execution, persistence, compaction, and recovery. Do not mirror their states, validation, errors, feature catalogs, or lifecycle transitions in CLI-owned types.
- Consume the public Runtime Go binding through narrow interfaces defined by CLI consumers. Use Runtime Protocol values at that boundary instead of translating them into a synonymous CLI data model.
- CLI-owned models are limited to presentation and interaction concerns such as Conversation folding, selection, drafts, prompt history, queue intent, stash, rendering, and terminal focus. Give a local aggregate behavior when it owns legal transitions; keep Runtime projections and rendering inputs as data.
- `main.go` opens at most one concrete Runtime, owns signals and streams, and closes it once. Do not pass a service bag or service locator through commands, workflows, or terminal state.
- Cobra and Viper stay in `cmd` and the process composition path. Commands construct fresh trees, parse typed input, call one CLI use case with `cmd.Context()`, and write through Cobra streams.
- Oolong and terminal protocols stay in terminal delivery. Rendering reads state; it does not repair or advance Runtime state.
- Organize packages around cohesive CLI workflows and durable local owners. Prefer several responsibility-named files in `run`, `session`, `command`, `conversation`, or `terminal` over packages for each action, interface, request, or response. Merge single-consumer forwarding packages into that consumer unless they isolate an external boundary.
- Production code has one Runtime path. Keep fakes beside tests or in test-only support; do not ship a mock Runtime selected by environment configuration.
- Runtime events are observations. Completed Runtime Items and authoritative snapshots win after gaps, reconnects, and cold recovery.
- Preserve exact provider/model identity and model-owned options. Never infer a provider from a model name or retain credentials in history, frames, errors, or logs.
- Use `/Users/tangerg/Desktop/grok-build` as visual evidence for terminal hierarchy, density, streaming stability, and interaction feedback. Preserve Flame vocabulary and Oolong ownership; do not copy its internal architecture.
- Test user-visible one-shot and terminal flows. Use in-memory root-command tests for routing, deterministic render snapshots at fixed dimensions for visual regressions, and a real PTY only when terminal escape sequences, resize, focus, input decoding, or restoration are the contract.
- `internal/adapter/runtimebinding` is the single external translation boundary. Its `Connection` owns binding lifecycle, negotiation, the immutable capability `Profile`, and DTO translation but no product state machine; only the composition root may fan it out into consumer-owned ports. Keep production packages under the explicit `domain`, `application`, `adapter`, or `delivery` ring unless a cross-ring mechanism has proven peer consumers.
