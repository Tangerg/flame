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

[`ARCHITECTURE.md`](ARCHITECTURE.md) defines ownership, dependency direction, Runtime isolation, command construction, terminal state, and local authoring. Mandatory module rules live in [`AGENTS.md`](AGENTS.md).

## Verify

```sh
GOWORK=off go test ./...
GOWORK=off go vet ./...
GOWORK=off go build ./...
```

Use real Runtime scenarios for changed product flows and real PTY tests only when terminal behavior is the contract.
