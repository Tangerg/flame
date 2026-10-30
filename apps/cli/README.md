# Flame CLI

Flame CLI provides scriptable commands and an interactive terminal client over the public Flame Runtime bindings.

```sh
# From the repository root
make setup
make build-cli
./out/cli/darwin-arm64/flame
./out/cli/darwin-arm64/flame run "explain the failing test"
./out/cli/darwin-arm64/flame sessions ls
```

The build includes the complete `out/cli/<platform>/plugins` companion distribution required by
embedded Runtime initialization. Keep that directory beside the executable when
packaging. An HTTP-only client does not initialize a local Runtime.

Running without a subcommand opens the Oolong terminal interface. Runtime owns durable product state and execution; CLI owns process behavior, terminal interaction, rendering, and local authoring state.

MCP forms preserve command and directory bytes, and encode arguments as JSON
arrays and environment/header replacements as JSON objects. Empty and whitespace
values remain explicit process material; Runtime owns their final admission.

Workspace `/diff --raw` displays the original patch, including file headers and
line-termination markers. `/diff --rows` renders Runtime's per-file rows with
old/new line numbers, rename paths, binary changes and truncation feedback.
The client does not parse raw patches into a second diff model.

## Configuration

CLI preferences and Runtime configuration have separate owners.

- `.flame.yaml` in the selected local directory (`.` or `-C`) is the project-local CLI preferences file.
- The OS user configuration directory provides the default CLI preferences file.
- `--config` selects an explicit CLI YAML file.
- `FLAME_CLI_*` variables and flags override file values.
- `$FLAME_HOME/runtime/config.yaml` is Runtime-owned configuration.
- `FLAME_RUNTIME_CONFIG_DIR` selects the sole Runtime configuration directory.

`FLAME_HOME` is an absolute product root, resolved through `localruntime` with the
same rules as Runtime and Desktop. An empty value selects `~/.flame`. Explicit
paths retain significant whitespace; they are never trimmed into another deployment.

The process working directory is never an implicit Runtime configuration source. Source checkouts that use `runtime/config/config.yaml` select it explicitly with `FLAME_RUNTIME_CONFIG_DIR`.

Run subscriptions reconnect with bounded backoff until canceled.

`flame run` reads nonterminal stdin through EOF before opening Runtime. On Linux and macOS,
canceling that read interrupts its descriptor poll without closing the borrowed
input. Embedded callers may provide finite memory or regular-file readers; a
blocking reader must implement `ReadContext(context.Context, []byte) (int, error)`
and interrupt its own reads. Other platforms require that context-aware input
contract for raw pipes; an unsupported descriptor backend is refused before reading.

## Connect to a shared Runtime

CLI defaults to remote mode at `http://127.0.0.1:17171`. Use `--runtime-mode embedded`
to exclusively own the local Runtime data directory until shutdown completes.
An empty or unreachable remote endpoint never selects an embedded Runtime.
Set `--runtime-url`, `FLAME_CLI_RUNTIME_ENDPOINT`, or `runtime.endpoint` in user-level
or explicitly selected `--config` YAML to attach to an already running Runtime
through its HTTP/SSE binding. The value is the base URL, such as
`http://127.0.0.1:17171`; the client appends `/v2/rpc`.
`FLAME_RUNTIME_TOKEN` supplies the bearer credential separately from printable
preferences. Endpoint failure or protocol incompatibility remains an error; it
never starts a replacement Runtime or falls back to local storage.

An automatically discovered project `.flame.yaml` cannot declare `runtime.endpoint`.
Such a file is rejected before Runtime discovery, including dynamic completion;
it cannot select the destination of a process-owned bearer. Remove that setting
from project preferences and choose the endpoint through one of the explicit or
user-level sources above. Passing `--config` explicitly authorizes the chosen file.
The automatic selector loads one file, not a merged user/project pair. When
project preferences are present, choose the endpoint with the environment or
flag, or select a trusted file explicitly with `--config`.

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

Session export reports the selected file path even if durability confirmation or
staging cleanup fails after publication. An unconfirmed notice means the file is
visible but its crash durability was not established; it is not an instruction
to export another copy. Export never overwrites different existing content.

Closing a terminal attached to a shared Runtime detaches its observations and saves
its authoring state. It leaves accepted Runs running. `Ctrl-C` remains an explicit
Run cancellation; one-shot `flame run` also cancels its own Run when interrupted.
Neither action shuts down the shared Runtime. Drafts, outboxes, deletion journals,
and history are stored separately for each configured endpoint beneath
`$FLAME_HOME/cli/targets/`; embedded state retains `$FLAME_HOME/cli`. One writable Workbench owns each physical
partition from loading through Close; a second terminal targeting it is refused.
Runtime's durable idempotency namespace still guards command replay independently
of that local endpoint partition. Changing an endpoint's spelling selects a new
local partition; it does not migrate pending commands automatically.

`flame plugins get-command --request '<JSON>'` inspects the Core-owned original
request using its retained `namespace` and `key`. It returns the exact installation,
release, contribution, target, contract and admitted input, including when that
producer has been uninstalled. It does not allocate a mutation identity or execute
the plugin. The original backend receipt remains the owner of the business
outcome. A different store namespace is refused; selection of a new provider cannot
replace the retained binding.

`flame plugins retire-command --request '<JSON>'` uses that same original
`namespace` and `key` to retire work only while Runtime can prove it was never
dispatched. Retirement preserves the identity and releases captured executable
dependencies. Dispatched unknown work cannot retire and remains receipt-only.
Runtime and CLI must use the same generated protocol contract. Incompatible host
storage requires a fresh data directory.

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

Terminal slash commands consume command text only after the command entrance accepts
the invocation. Unknown, unavailable and argument-cardinality failures preserve the
editor, attachments and cursor. Contributed commands accept after their cancellable
session operation starts; subsequent execution failure is reported through that
operation. Approval choices may lose a remembered scope, but keep their allow or deny
decision; unknown choices cannot create an answerable approval form.

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

`/preview [--lines N] <path>` reads from line 1 through the same Runtime file-read
operation as `/read`. The default is 80 lines; an explicit count is honored within
the shared read byte budget. The reader reports the accepted window, whole-file
line count and any truncation. It does not impose a separate preview cap.

Omitted search, read-byte and diff-row limits remain absent on the Runtime
request. Runtime owns their defaults and resource caps; CLI only forwards an
explicit positive choice. `/grep` therefore uses Runtime's current default
instead of the former client-owned 200-match default, and reports the actual
returned and total match counts.

Provider selection is either an exact provider/model pair or absent. Absence means that Runtime applies the active Session selection; CLI never infers a provider from a model name.

## Architecture

[`ARCHITECTURE.md`](ARCHITECTURE.md) defines ownership, dependency direction, Runtime isolation, command construction, terminal state, and local authoring. Mandatory module rules live in [Module instructions](#module-instructions) below.

The directory map follows ownership. `domain/authoring` holds
pure local intent, while `domain/conversation` folds Runtime facts for presentation.
`application/workbench` owns authoring, outbox transactions and the complete
installed-command intent lifecycle; terminal code
commands that owner instead of coordinating durable and live queue state itself.
Typed Memory entries also retain original decision, patch and target admission;
terminal delivery consumes pending readouts and renders retry instructions.
`application/mutation` shares exact replay admission across Run and Session
workflows. Root composition opens Workbench lazily and supplies consumer-owned
Runtime ports. Executable plugins live in `adapter/sideload`, with command contracts
in `application/extensions`. Built-in Oolong block and tool presentations stay private
to terminal; command output renderers stay private to `delivery/cmd`.

## Verify

```sh
go test ./...
go vet ./...
go build ./...
```

Use real Runtime scenarios for changed product flows and real PTY tests only when terminal behavior is the contract.
These commands use the repository's `go.work` and validate coordinated Runtime and CLI
changes without publishing modules. Before an independent CLI module release, run the
same checks with `GOWORK=off` against the matching published Runtime versions.

## Module instructions

Flame CLI owns command routing, process behavior, terminal interaction, rendering, and CLI-local authoring state. It consumes Runtime; it is not a second Runtime.

Read [`../AGENTS.md`](../../AGENTS.md), [`../DEVELOPMENT.md`](../../DEVELOPMENT.md), and [`ARCHITECTURE.md`](ARCHITECTURE.md) before changing this module.

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

Tool policies use Runtime's source references. MCP commands address a server as
`<name>` for a user server or `<installation-id>/<name>` for an installation server; the
CLI parses that argument once into Runtime's structured `MCPServerID` and renders server
identities back in the same form; a tool source reads `builtIn/<name>`, `a2a/<endpoint>`
or `mcp/<server>/<tool>` with the server in that form. `/mcp-tool <server> <tool>
<enable|disable|allow|deny>` changes exposure or creates a global standing rule.
`/rules` shows source identity, explicit subject match type, and stale authority; `/rule-delete` forgets a
specific rule. Connection forms contain connection configuration only. Setting
an allow or deny replaces the existing decision for the same scope, source, and
subject type and value; distinct matching patterns of equal specificity still favor deny.
Approval changes do not wait for MCP connection or authorization operations.
MCP secret forms validate their set/clear shape through the generated protocol
contract. Runtime owns environment and HTTP header admission. Environment keys
and values reach that owner unchanged, including empty and whitespace values;
an empty value sets `KEY=` rather than removing the variable.
`flame approvals ls` lists global rules; `--session` adds that session's visible
rules. `/mcp-tools` includes disabled tools across configured servers, and tool
listings explain name collisions that exclude connected tools from the model.
A server's inline status problem is rendered from its closed category; a category
outside the one its state allows is reported as a Runtime contract violation rather
than printed as raw text. Runtime and clients must use the same protocol revision, and obsolete Runtime
approval databases require a fresh data directory rather than data migration.

`/utility default` uses the Runtime default model for subsequent utility calls;
`/utility <provider/model>` assigns an explicit model. An installed model selector
may change the final route. The former `/utility inherit` command is removed.

## Runtime plugin commands

`flame plugins providers` reads explicit provider choices. Use
`flame plugins select-provider --request '{"role":"memory","installationId":"…","contribution":"memory"}'`
to select a declared installation/contribution for future calls. Enablement does
not select a provider; several candidates may be enabled. Disabling or removing
the selected provider leaves the choice unavailable rather than selecting another
one. Selection preserves private data and the original route of pending commands.

`flame plugins compositions` lists consumer-selected decorator chains in their
explicit outer-to-inner order. `flame plugins set-composition --request
'{"contract":"<canonical capability JSON>","members":[{"installationId":"…","contribution":"…"}]}'`
sets that order through the Runtime command owner; retain its printed command ID
for uncertain acknowledgement recovery. `flame plugins resolve-composition
--request '{"contract":"<canonical capability JSON>"}'` reports current exact
bindings in that same order. Unavailable members are errors, never silently
removed or replaced. These commands do not activate backends or create another
composition store.

`flame plugins list` inspects admitted releases, each installation's desired `enabled` value, Skill provenance, typed admission diagnostics, the
realization Runtime observes now (release availability and unavailable backends) and each
declared input's state. A configured secret input, including every header and
authorization input, reports `configured` and never carries its text.
Installation and explicit release selection are the trust decisions. Selection preserves
desired enablement and refuses a candidate missing required retained inputs before mutation.
`install`, `stage`, `select`, `configure`, `set-enablement`, and `uninstall`
accept their exact Runtime JSON request through `--request`. Source paths refer to the
Runtime machine. Each mutation prints its command identity to stderr and the resulting installation as
JSON; `uninstall` is an acknowledgement and prints nothing on success. Retain the identity and use
`--command-id` with the original request to recover an uncertain acknowledgement.
Configuration requests include the selected release's `digest` as well as `installationId`,
and are deltas: `valueChanges` sets or clears named inputs, and `serverChanges` and
`skillChanges` map component names to `enable` or `disable`. Unnamed inputs and components
keep their current value or enablement.

Session naming uses the existing `flame session rename <session-id> <title>` command,
with the same inspected revision and original command identity as canonical Session
editing. Public plugin components call that same `sessions.update` owner. The former
`plugins rename-session` wrapper is removed without an alias; do not translate an
unresolved old request into another method or identity.

Terminal Rename captures the selected Session's revision when its form opens.
Neither a catalog refresh nor a cold read can replace that precondition. Admission
keeps a busy form intact; a refused write restores the original form and draft with
its error. Cancel and reopen Sessions, then Rename, to inspect a fresh title and revision.
Favorite actions and `/rename` also submit their explicitly captured revision.

Runtime rejects a stale digest without applying inputs or component enablement changes;
do not replace the digest in an unresolved request with a newly selected release.
Authored JSON uses Runtime's strict request decoder. Explicit typed nulls cannot become
false enablement or empty change maps; use the declared clear variant. Include required
fields explicitly, including `enabled: false` and empty `serverChanges: {}`.
Invalid requests and replay identities are rejected before the CLI opens Runtime.

Installed MCP Tools use Runtime's existing Run/Interrupt path. Human plugin business
operations use the public installed call entrance; Core operations use their canonical
commands. The CLI owns terminal interaction, while approval, execution and receipts
retain their Runtime owners.

Installed operation and point requests carry feature input/attributes and complete
contracts as JSON strings. Returned feature output/values are JSON strings too.
Typed Go readers decode the original source, preserving full-width integers and
receipt lexemes. Rebuild Runtime and CLI together; inline payloads are refused.
Private receipts and local pending input records retain their original JSON and
owner.

Schedule catalog reads capture the selected installation/contribution and full
required callable contract once, then page through `plugins.call`. The CLI's Go
request/output projection is generated from the same publisher contracts as TS;
no private backend package is imported. Selection changes cannot mix providers
inside one traversal. Cursor/request/row bounds and duplicate ID rejection remain.
Every Schedule read and mutation uses public installed operations.
Before dispatch, the CLI workbench persists the original installation, release,
contribution, contract, target, input bytes and command ID. After a lost
acknowledgement or restart, `/schedule-create`, `/schedule-delete <original-full-id>`
`/schedule-edit <original-full-id>` or `/schedule-run <original-full-id>` reconciles that request before current
provider discovery. An unresolved creation reuses the original draft instead of
opening a fresh form. Startup reports pending intents. An unavailable original
owner, expired replay guarantee or changed Runtime store leaves intent unconfirmed;
it cannot resubmit the command to another installation. Only a decoded owner
acknowledgement or business refusal retires it. This record contains no result. If local publication becomes visible but directory
synchronization fails, the original intent remains pending and delivery is fenced.
Close and reopen the workbench to confirm the actual records before retrying;
retries cannot create a replacement command identity.

Forms and the binding adapter use the same generated publisher Go projection in
`internal/domain/schedule`; no private backend import or handwritten create DTO
is needed. Workspace resolution precedes initial capture and is never repeated for
pending intent. The publisher still owns schedule validation, creation and receipt.
Manual execution returns the existing Core Session/Run reference; a settled command
permits a later new intent. Editing and enablement retain the originally inspected
revision and patch; workspace reset is an explicit empty CWD, while omission keeps
the binding. Pending edits are reconciled before reading another revision. Readers
use the generated publisher snapshot directly, without a Core Schedule projection
or extra Enabled state. No Schedule Core API, DTO or feature flag remains; the
client negotiates the generic plugins entrance and resolves the selected contract.

Memory catalog reads use the publisher's generated Go contract and public installed
query. The selected installation, release, contribution and complete contract are
captured once for all pages. Project workspace resolution precedes capture; each
page keeps that owner and target. Capacity/request/page bounds, repeated IDs/cursors,
empty continuations and scope mismatches are refused without partial results.
The former ListAgentMemory/agentMemory.list and workspace list wrapper are removed.
CLI Memory deletion uses the generated public operation and stages its complete
binding, raw input, command identity and replay guard in the existing workbench
before dispatch. An uncertain deletion resumes before workspace or catalog reads,
including after client restart and provider replacement. The retry accepts a full
ID or an unambiguous prefix among pending deletions; an invalid acknowledgement
retains the original intent. Desktop deletion now also uses the installed operation
and its existing client journal restores the original route before any new provider
preparation. Core DeleteAgentMemory/agentMemory.delete and their DTO are removed.
CLI review also uses the published operation; the installed graphical component
uses that same backend owner. Approve and reject share
one logical intent per item; an unconfirmed decision must be reconciled before an
opposite decision can be submitted. Pending full/prefix IDs resolve before current
workspace/catalog reads, and original decision/request/key survive restart.
Core ReviewAgentMemory/agentMemory.review, its request and global decision enum
are removed. Editing and pin/unpin now use the generated public update operation.
They share one item intent and retain the original nullable patch before I/O.
An edited retry cannot confirm a different draft; /memory-edit first resumes the
original pending patch without current workspace/catalog reads. Public snapshot
decoding precedes intent retirement. Core UpdateAgentMemory/agentMemory.update and
its request are removed. Add now uses the generated public operation and one
authoring intent. The workbench retains the original scope, captured workspace,
content, binding and command identity. /memory-add reconciles that intent before
reading the current workspace or opening another editor. Changed content or target
cannot replace an uncertain addition. The backend alone resolves the captured path
to its private project root and reports whether it changed the business state.
Snapshot validation, including original scope, precedes retirement. Core
AddAgentMemory/agentMemory.add, its DTO and the Memory-specific Core feature are
removed. Clients negotiate plugins and resolve the concrete published capability.
Uncertain requests from retired Core contracts are not translated into fresh
public commands. The publisher owns Memory read models and closed values.
Terminal presentation owns its compact read projection; generated publisher
snapshots remain the transport contract. Visible-state and timestamp checks precede
presentation, while publisher schemas own identity/content admission. Complete
readers consume the publisher-owned capacity without a Core Memory export. Complete readers obtain the
publisher's schema-bound Capacity from the first page, require it to remain stable
under the captured binding and reject cumulative overflow or empty continuation.
Page width uses the backend default, and the publisher alone derives row/request
limits from its business constants.

Memory and Schedule invalidations use `plugins.dataChanged`.
The CLI subscribes once when either reader exists and refreshes both through their
public owner reads. The event contains no business state or record IDs. Unknown
installed command outcomes still invalidate reads without claiming success; ordinary
resync and subscription gap/partition behavior remain unchanged.
