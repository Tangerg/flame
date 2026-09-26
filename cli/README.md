# Flame CLI

Flame CLI provides scriptable commands and an interactive terminal client over the public Flame Runtime bindings.

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

- `.flame.yaml` in the selected local directory (`.` or `-C`) is the project-local CLI preferences file.
- The OS user configuration directory provides the default CLI preferences file.
- `--config` selects an explicit CLI YAML file.
- `FLAME_CLI_*` variables and flags override file values.
- `$FLAME_HOME/runtime/config.yaml` is Runtime-owned configuration.
- `FLAME_RUNTIME_CONFIG_DIR` selects the sole Runtime configuration directory.

The process working directory is never an implicit Runtime configuration source. Source checkouts that use `runtime/config/config.yaml` select it explicitly with `FLAME_RUNTIME_CONFIG_DIR`.

Run subscriptions reconnect with bounded backoff until canceled.

## Connect to a shared Runtime

With no endpoint configured, CLI owns one embedded Runtime and closes it on exit.
Set `--runtime-url`, `FLAME_CLI_RUNTIME_ENDPOINT`, or `runtime.endpoint` in CLI YAML
to attach to an already running Runtime through its HTTP/SSE binding. The value is
the base URL, such as `http://127.0.0.1:17171`; the client appends `/v2/rpc`.
`FLAME_RUNTIME_TOKEN` supplies the bearer credential separately from printable
preferences. Endpoint failure or protocol incompatibility remains an error; it
never starts a replacement Runtime or falls back to local storage.

```sh
flame --runtime-url http://127.0.0.1:17171 sessions ls
flame --runtime-url http://127.0.0.1:17171 --session ses_example
flame --runtime-url https://agent.example/flame -C ./local-context \
  run --workspace /srv/project --file notes.md "review these notes"
```

For remote connections, `--workspace` names a path on the Runtime host. Omitting it
uses the Runtime's default workspace when creating a Session. `-C` remains the
client's local directory for CLI configuration and attached files. An existing
Session keeps its Runtime workspace; attaching a file never reads that workspace
through the client's filesystem. The terminal uses this same local directory for
attachments, prompt editing, imports, and exports throughout Session switches.
Remote workspace references retain their exact spelling and are interpreted and
validated by the executing Runtime, including when it uses another operating system.

Closing a terminal attached to a shared Runtime detaches its observations and saves
its authoring state. It leaves accepted Runs running. `Ctrl-C` remains an explicit
Run cancellation; one-shot `flame run` also cancels its own Run when interrupted.
Neither action shuts down the shared Runtime. Drafts, outboxes, deletion journals,
and history are stored separately for each configured endpoint beneath
`$FLAME_HOME/cli/targets/`; embedded state retains `$FLAME_HOME/cli`.
Runtime's durable idempotency namespace still guards command replay independently
of that local endpoint partition. Changing an endpoint's spelling selects a new
local partition; it does not migrate pending commands automatically.

Dynamic value completion loads the effective configuration before opening Runtime
so it cannot silently query another target. Completion-script generation and help
remain independent of Runtime startup.

Sideloaded command executables now use command protocol **2**. Requests carry
`workspace` as the Runtime's opaque reference and `localDirectory` as the client's
local authoring root. A command still executes from its discovered plugin directory;
it must choose the appropriate reference explicitly instead of assuming the Runtime
workspace exists on the client. Executables must respond with `protocol: 2` and
declare manifest `schemaVersion: 3`, which rejects older command semantics before
execution. The extension-host `apiVersion` remains `1`.

## Markdown, formulas, and diagrams

Assistant and reasoning messages render Markdown through Oolong, including fenced
code highlighting, tables, lists, quotes, and links. Display formulas use Oolong's
native terminal LaTeX layout: put `$$` on separate lines or use a `math` code fence.
Unsupported formulas keep their source and display the rendering diagnostic.

Mermaid code fences become inline diagrams after the message completes. Diagram
preparation runs asynchronously; incomplete streamed diagrams remain readable
source. Each message owns its images and pending work, which are released when
the message is discarded, its session is replaced, or the terminal closes.

Inline diagrams require a terminal supporting the Kitty graphics protocol and
pixel cell dimensions, plus the official Mermaid CLI (`mmdc`) and its Chromium
backend. Install the backend separately:

```sh
npm install -g @mermaid-js/mermaid-cli@11.17.0
```

Flame uses Oolong's bounded Mermaid renderer and does not download a browser at
startup. Missing terminal capabilities, a missing backend, or invalid diagram
syntax leave the source visible with a diagnostic. Scripted text and JSON output
retain the original content.

In the terminal, `/skills` lists available skills and local discovery diagnostics. `/skills <name>` reads the current resolver-selected document, including source path, revision, and instructions. Open skill readers refresh after Runtime skill-change events.

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
When validating coordinated, unpublished Runtime and CLI changes, use the repository's
`go.work` (`go test ./...`, `go vet ./...`, `go build ./...` from `cli`). The standalone
`GOWORK=off` gate requires publishing the matching Runtime modules and updating their
versions; a local checkout does not make new public Runtime APIs available to a
previously published dependency.

## Module instructions

Flame CLI owns command routing, process behavior, terminal interaction, rendering, and CLI-local authoring state. It consumes Runtime; it is not a second Runtime.

Read [`../AGENTS.md`](../AGENTS.md), [`../DEVELOPMENT.md`](../DEVELOPMENT.md), and [`ARCHITECTURE.md`](ARCHITECTURE.md) before changing this module.

- Runtime remains authoritative for Session, Run, Segment, Item, Goal, Plan, Interrupt, provider/model selection, execution, persistence, compaction, and recovery. Do not mirror their states, validation, errors, feature catalogs, or lifecycle transitions in CLI-owned types.
- Consume the public embedded or HTTP Runtime binding through narrow interfaces defined by CLI consumers. Use Runtime Protocol values at that boundary instead of translating them into a synonymous CLI data model.
- CLI-owned models are limited to presentation and interaction concerns such as Conversation folding, selection, drafts, prompt history, queue intent, stash, rendering, and terminal focus. Give a local aggregate behavior when it owns legal transitions; keep Runtime projections and rendering inputs as data.
- `main.go` selects one immutable Runtime target, owns signals and streams, and closes its binding once. It owns Runtime shutdown only for the embedded binding. Do not pass a service bag or service locator through commands, workflows, or terminal state.
- Cobra and Viper stay in `cmd` and the process composition path. Commands construct fresh trees, parse typed input, call one CLI use case with `cmd.Context()`, and write through Cobra streams.
- Oolong and terminal protocols stay in terminal delivery. Rendering reads state; it does not repair or advance Runtime state.
- Organize packages around cohesive CLI workflows and durable local owners. Prefer several responsibility-named files in `run`, `session`, `command`, `conversation`, or `terminal` over packages for each action, interface, request, or response. Merge single-consumer forwarding packages into that consumer unless they isolate an external boundary.
- Production code has one Runtime path. Keep fakes beside tests or in test-only support; do not ship a mock Runtime selected by environment configuration.
- Runtime events are observations. Completed Runtime Items and authoritative snapshots win after gaps, reconnects, and cold recovery.
- Preserve exact provider/model identity and model-owned options. Never infer a provider from a model name or retain credentials in history, frames, errors, or logs.
- Use `/Users/tangerg/Desktop/grok-build` as visual evidence for terminal hierarchy, density, streaming stability, and interaction feedback. Preserve Flame vocabulary and Oolong ownership; do not copy its internal architecture.
- Test user-visible one-shot and terminal flows. Use in-memory root-command tests for routing, deterministic render snapshots at fixed dimensions for visual regressions, and a real PTY only when terminal escape sequences, resize, focus, input decoding, or restoration are the contract.
- `internal/adapter/runtimebinding` is the single external translation boundary. Its `Connection` owns binding lifecycle, negotiation, the immutable capability `Profile`, and DTO translation but no product state machine; only the composition root may fan it out into consumer-owned ports. Keep production packages under the explicit `domain`, `application`, `adapter`, or `delivery` ring unless a cross-ring mechanism has proven peer consumers.
