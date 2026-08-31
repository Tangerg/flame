# Flame CLI architecture

This document defines the CLI's stable ownership and dependency boundaries. It does not inventory every command, key binding, Runtime method, or completed implementation batch.

## Product role

The CLI provides two delivery modes over one Runtime product model:

- one-shot commands for scripts and pipelines
- an interactive Oolong terminal user interface (TUI)

Both modes use the public in-process Runtime binding. They share configuration, Session and Run projections, error classification, rendering vocabulary, and process ownership. They do not create an HTTP loopback or copy Runtime protocol models.

## Ownership

| Owner | Responsibility |
| --- | --- |
| Runtime | Durable Session, Run, Segment, Item, Goal, Plan, Interrupt, provider, tool, compaction, recovery, and workspace facts |
| CLI domain | Validated consumer values, presentation classifications, exact identities, and CLI-local invariants |
| CLI application | One-shot orchestration, Session opening, prompt queue, reconnect, cold recovery, and local authoring workflows |
| Command delivery | Cobra routing, flags, environment, streams, exit status, and shell completion |
| Terminal delivery | Input, focus, navigation, interaction dialogs, layout, rendering, and attention signals |
| Runtime adapter | Translation between public Runtime contracts and CLI-owned values |
| Workbench | Durable drafts, history, queue, stash, recent workspaces, and replay intent |
| Composition root | Concrete dependencies, signal lifetime, streams, one Runtime instance, and shutdown |

Runtime data never becomes CLI-owned merely because the CLI caches it for rendering. A stream preview is replaceable; a completed Runtime Item or authoritative snapshot wins.

## Dependency direction

```text
main
 ├── cmd ────────────────┐
 ├── terminal ───────────┤
 └── runtimebinding ─────┤
                         v
             CLI application and domain
```

The arrows show source dependencies toward CLI policy. Delivery may depend on
Application, Domain, and explicit adapters; Adapter may depend on Application
and Domain; Application may depend on Domain. Cobra and Viper remain in
`delivery/cmd`, Oolong remains in `delivery/terminal`, and the public Runtime
module remains in `adapter/runtimebinding`.

`internal/adapter/runtimebinding.Connection` owns the public binding lifecycle,
negotiated profile, command metadata, and protocol translation. The profile is
part of that boundary rather than a second package because it has no producer or
lifecycle outside connection negotiation. The binding does not own
product state or define a second Runtime model. `main` is the only place that
fans one Connection out into the narrow interfaces defined by CLI consumers;
commands receive only `agent.Runtime` plus the immutable profile, and terminal
construction receives explicit ports. There is no backend service bag or
service locator.

## Package shape

The physical tree makes the four dependency rings visible without adding
umbrella Go packages:

```text
internal/
├── domain/       CLI-owned values, aggregates, and invariants
├── application/  use cases, local workflows, and durable Workbench state
├── adapter/      Runtime and filesystem translation
├── delivery/     Cobra, terminal interaction, rendering, and plugin delivery
├── exactint/     cross-ring exact integer mechanism
├── testsupport/  test-only runtime fixture
└── arch/         dependency fitness tests
```

The ring directories are navigation and import boundaries, not facades. A leaf
package earns independence by owning at least one of these concerns:

- a stable consumer vocabulary or invariant
- a durable CLI-local aggregate
- a workflow lifecycle with cancellation or settlement
- an external boundary such as Runtime, filesystem, PTY, or plugin process
- a reusable mechanism with multiple real peer consumers

Single-concept packages that only rename a value, expose one forwarding function, or avoid placing another file in a cohesive package should merge into their owner. Merge proof comes from consumers and invariants, not directory count.

Do not collapse the tree into a broad `service` or `backend` layer. Prefer cohesive packages with several responsibility-named files.

`internal/application/run` owns the CLI application workflow for unattended execution,
segment reattachment, and durable steering settlement. `internal/application/session` owns
Session opening, updates, deletion, rollback, and their local settlement. These
packages orchestrate Runtime commands and CLI-local records; they do not own a
second Run or Session state machine. `internal/application/retry` owns the shared retry
mechanism and transport-recovery policy: `Backoff` is the only exponential
schedule, while `ReconnectPolicy` adds classified admission and a finite
attempt budget for Run, Runtime invalidation, workspace inspection, and MCP
management. Do not split retry admission from the schedule it consumes.

`internal/domain/commandreplay` owns the pure, durable capability/guard/policy model
for one Runtime replay store. `internal/application/mutation` consumes that model together
with Agent error classification and retry to settle acknowledgement; combining
them would make the persisted value owner depend on I/O behavior. Likewise,
`internal/application/promptqueue` remains a CLI application aggregate rather than Terminal
state: it owns per-Session FIFO order, stable identities, hold/edit rules,
dispatch reservation, durable restore, and rollback. Terminal only presents
and commands that aggregate.

`internal/application/session` also owns the portable Session document value, export/import
requests, and the consumer-owned Runtime transfer port. These facts share the
Session identity and lifecycle; they are not a separate bounded context.
`internal/adapter/sessionartifact` remains a distinct outbound filesystem adapter: it
owns path resolution, bounded reads, conflict-safe publication, and exact-byte
transfer without owning the document semantics.

`internal/domain/identity` owns the CLI domain's admission policy for exact foreign
Runtime resource and model-selection identities. It keeps Session, Run,
Segment, Item, Event, provider, model, and reasoning rules in
responsibility-named files, but does not wrap those foreign strings in a second
set of value types when no CLI aggregate consumes such types. It never creates
identities, infers providers, normalizes values, or owns Runtime lifecycle.
Independent mechanisms such as exact integer advancement remain separate when
they have their own vocabulary and peer consumers.

`internal/domain/agent` owns the cohesive CLI projection of the Agent Session and Run
context. Session, Run, Goal, feedback, usage, and Agent Memory values use
semantic type names inside that package and share its consumer-owned Runtime
ports. Runtime keeps Agent Memory as an independent product domain; the CLI does
not reproduce that server-side package graph for a projection with the same two
consumers and no independent resource lifecycle. New CLI packages require a
different lifecycle, external boundary, or bounded context rather than another
`Service` interface.

`internal/domain/workspace` owns the CLI projection of the selected filesystem and
project context. Resolved workspaces, authored agent documents and recipes,
knowledge documents, discoverable and managed Skills, lifecycle Hook policy,
and direct read-only diagnostics all derive their scope or authority from that
context. They use responsibility-named files and semantic type names inside one
package; there is no aggregate service facade. Runtime translation and terminal
workflow lifetimes stay outside this package, while independent filesystem or
plugin-process boundaries keep their own packages.

## Command model

The root command is built by a factory for every process or test. Commands:

1. declare syntax, flags, completion, and help
2. parse inputs into typed CLI values
3. call one application capability with `cmd.Context()`
4. write results through Cobra's configured output streams

Commands do not import Oolong, inspect Runtime protocol DTOs, mutate workbench records directly, or contain provider-specific behavior. Runtime and application failures return through `RunE`; the process boundary chooses diagnostics and exit status once.

Viper is a configuration input, not application state. The command boundary resolves defaults, config files, environment, and flags before constructing a use-case request.

One-shot `run` owns its external stdin byte-stream envelope before building an authoring Message. It reads at most 4 MiB plus one overflow byte, requires exact UTF-8, and rechecks the argument + stdin composition against the same 4 MiB limit before creating a Session or Run. This is a CLI process-memory boundary, not a duplicate model-context/token policy; Runtime仍按 selected model 的完整请求预算决定可执行性。

## Runtime boundary

The process lazily opens at most one module-root Runtime and fully closes it before exit. The Runtime boundary:

- sends the exact current protocol metadata
- preserves idempotency, replay, Run, Segment, Item, and revision identities
- maps typed Runtime errors to CLI-owned classifications without string matching
- converts protocol DTOs to CLI values without leaking DTOs inward
- treats Runtime events as invalidations or previews according to their contract
- re-reads authoritative state after a gap, reconnect, or cold recovery
- preserves exact provider/model identity and model-owned options

The production binary has no Runtime implementation selector. Scripted Runtime behavior is test support, not a product capability; PTY coverage re-executes the Go test binary and composes the fixture from test code. Ordinary unit tests inject narrow consumer interfaces directly.

The CLI can share a compatible Runtime data directory with another process because Runtime already owns that storage contract. CLI code does not add a second global lock, heartbeat, leader election, or compatibility reader.

CLI run settings hold an optional exact provider/model override, not a product default. The zero pair means “use the active Session selection”; one-shot and TUI commands preserve that omission on the wire. Presentation may combine the omitted override with the current Session projection to show the exact effective model, but that derived label never becomes command state. Explicit configuration or `--provider` + `--model` still sends an override. CLI preference environment variables use `FLAME_CLI_*`; Runtime deployment/configuration keeps `FLAME_*`. Root persistent preference flags are bound from the root flag set, so a subcommand-local flag such as `sessions update --model` cannot mutate unrelated Run settings.

Portable Session exports cross the Runtime/filesystem boundary as one immutable `session.Document`. The Document owns format, UTF-8/JSON validity, canonical body, and the 64 MiB complete encoded limit; adapters may reject a file before reading it but cannot publish or import a larger second representation. Filesystem publication writes the exact Document bytes, so the write path cannot create an artifact that the next CLI process rejects by construction.

## Terminal model

The terminal application owns one explicit state tree. Domain state changes before rendering; render functions are pure projections of that state and terminal dimensions.

The visual hierarchy follows Grok Build as the primary reference:

1. compact Session and workspace identity
2. scrollable conversation with stable streaming blocks
3. current Goal, Plan, Run, and attention state near the composer
4. multiline composer with contextual controls and exact model identity
5. transient overlays for search, selection, confirmation, and full content

Optional regions yield space before the transcript or composer becomes unusable. Resizing must preserve draft, focus, selection, scroll intent, open interaction, and stream ownership.

Key and mouse behavior belongs to named keymaps and interaction states, not scattered switch branches. An overlay captures only its own keys; closing it restores the prior focus owner. Mouse actions require matching press and release targets so selection drags remain safe.

Oolong owns low-level terminal mode, cell width, grapheme editing, and input decoding. Flame should upgrade or fix the library when those primitives are wrong instead of maintaining a forked local copy.

## Local authoring state

Workbench persists only CLI-authored facts that Runtime does not own:

- per-Session drafts and attachments
- bounded prompt history
- queued and stashed prompts
- recent workspaces
- durable command replay intent

Each record uses an explicit current shape. Invalid or old local shapes fail closed; active development does not keep migration or compatibility readers.

Every Workbench file is one complete JSON document with a 16 MiB encoded limit shared by reads and writes. Its versioned envelope and value use a closed field vocabulary, including custom rich-state codecs such as pending HITL resume; an older process must reject state it does not understand instead of silently dropping fields on its next save. A read must reject an oversized file, trailing value, truncated document, unknown field, or invalid aggregate value instead of accepting a valid prefix. Stash and recent-workspace values own their canonical identity/path, timestamp, prompt validity, and collection uniqueness; those invariants run both before durable replacement and after decode, before capacity trimming. A write must fail before durable replacement and in-memory mutation when its next process could not reopen the result.

Authored attachments are dynamic local path references, not frozen file snapshots: selection owns stable UI identity and declared kind, while dispatch reopens the path and reads the current bytes under the 20 MiB envelope. That does not permit lossy projection. An image attachment rich value must carry a valid `image/*` MIME before filesystem I/O; a text attachment's complete dispatch-time bytes must be valid UTF-8 without NUL before conversion to a Protocol text block. CLI does not add mtime/hash locking, content sniffing, or a second provider capability policy.

Queue and replay are rich aggregates. They own identity, ordering, selection, dispatch state, acknowledgement policy, and legal edits. Terminal code issues commands to those aggregates instead of mutating slices and flags independently.

## Provider presentation

The Runtime catalog is the only provider and model fact source. The CLI preserves:

- exact provider and model identity
- reasoning or other model-owned options
- context, input, and output limits
- input and output modalities
- tool and structured-output capability
- credential and endpoint diagnostics that are safe to display

Credentials remain write-only. Configuration views use masked Runtime projections and never retain submitted secrets in Workbench or terminal frames.

OpenCode is a reference for precedence and capability projection. Flame keeps Runtime as the provider owner and does not copy OpenCode's plugin or JavaScript provider runtime.

## Verification

Use the narrowest evidence that proves the changed contract:

- root-command tests for syntax, streams, and exit behavior
- application tests for queue, replay, recovery, and interaction ownership
- real in-process Runtime tests for Session, Run, Goal, Plan, steer, HITL, compaction, and typed errors
- deterministic render tests for layout and state projection
- real PTY tests for input decoding, resize, terminal mode, focus, and restoration
- architecture tests for Runtime/Cobra/Viper/Oolong import isolation and retired package paths

Run race tests only when stream, goroutine, cancellation, shutdown, or shared mutable state changes. Do not create speculative multi-client or multi-server matrices.
