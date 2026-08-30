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
 ├── cmd ───────────────┐
 ├── terminal ──────────┤
 └── runtimeembedded ───┤
                        v
             CLI application and domain
```

The arrows show source dependencies toward CLI policy. The outer packages may import Cobra, Viper, Oolong, and Runtime public packages. Inner packages must not.

`internal/runtimeembedded` converts public protocol values once at the edge. Consumer packages define the narrow interfaces they need. `internal/backend` may group concrete capabilities for composition, but it must not become a service locator passed through the application.

## Package shape

The desired shape is domain-focused and flatter than the current tree. A package earns independence by owning at least one of these concerns:

- a stable consumer vocabulary or invariant
- a durable CLI-local aggregate
- a workflow lifecycle with cancellation or settlement
- an external boundary such as Runtime, filesystem, PTY, or plugin process
- a reusable mechanism with multiple real peer consumers

Single-concept packages that only rename a value, expose one forwarding function, or avoid placing another file in a cohesive package should merge into their owner. Merge proof comes from consumers and invariants, not directory count.

Do not collapse the tree into a broad `service` or `backend` layer. Prefer cohesive packages with several responsibility-named files.

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

The process lazily opens at most one embedded Runtime and fully closes it before exit. The adapter:

- sends the exact current protocol metadata
- preserves idempotency, replay, Run, Segment, Item, and revision identities
- maps typed Runtime errors to CLI-owned classifications without string matching
- converts protocol DTOs to CLI values without leaking DTOs inward
- treats Runtime events as invalidations or previews according to their contract
- re-reads authoritative state after a gap, reconnect, or cold recovery
- preserves exact provider/model identity and model-owned options

The CLI can share a compatible Runtime data directory with another process because Runtime already owns that storage contract. CLI code does not add a second global lock, heartbeat, leader election, or compatibility reader.

Portable Session exports cross the Runtime/filesystem boundary as one immutable `sessiontransfer.Document`. The Document owns format, UTF-8/JSON validity, canonical body, and the 64 MiB complete encoded limit; adapters may reject a file before reading it but cannot publish or import a larger second representation. Filesystem publication writes the exact Document bytes, so the write path cannot create an artifact that the next CLI process rejects by construction.

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

Every Workbench file is one complete JSON document with a 16 MiB encoded limit shared by reads and writes. Its versioned envelope and value use a closed field vocabulary, including custom rich-state codecs such as pending HITL resume; an older process must reject state it does not understand instead of silently dropping fields on its next save. A read must reject an oversized file, trailing value, truncated document, or unknown field instead of accepting a valid prefix; a write must fail before durable replacement and in-memory mutation when its next process could not reopen the result.

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
- real embedded Runtime tests for Session, Run, Goal, Plan, steer, HITL, compaction, and typed errors
- deterministic render tests for layout and state projection
- real PTY tests for input decoding, resize, terminal mode, focus, and restoration
- architecture tests for Runtime/Cobra/Viper/Oolong import isolation and retired package paths

Run race tests only when stream, goroutine, cancellation, shutdown, or shared mutable state changes. Do not create speculative multi-client or multi-server matrices.
