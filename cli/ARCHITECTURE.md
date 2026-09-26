# Flame CLI architecture

Flame CLI is a consumer of the public in-process Runtime binding. It provides one-shot Cobra commands and an interactive Oolong terminal client without creating a second product backend.

## Ownership

Runtime remains authoritative for Session, Run, Segment, Item, Goal, Plan, Interrupt, execution, provider selection, persistence, compaction, and recovery.

CLI owns only command and presentation concerns:

- process signals, streams, diagnostics, and exit status;
- typed CLI preferences and command input;
- terminal focus, selection, layout, navigation, overlays, and rendering;
- drafts, attachments, prompt history, queue intent, stash, and replay intent;
- projections used to present Runtime facts.

A Runtime event may update a preview, but completed Items and authoritative snapshots win after reconnect, gaps, and cold recovery.

Run observation retries classified transient transport failures until its caller cancels, with bounded backoff and no attempt quota. Subscription and rendering failures do not authorize a Run cancellation. Explicit user cancellation remains a separate Runtime command; permanent observation errors remain visible without manufacturing a product terminal. Mutation acknowledgement recovery separately honors Runtime's advertised replay retention and exact command identity.

Steer acknowledgement preserves Runtime's exact reserved User Item ID. Acceptance means the instruction is waiting to enter model context; only a completed User Item with that identity in the same Run proves application. Pending steer signals may survive an interruption and apply in a later Segment of that Run. After the Run finishes, a separate authoritative Session read settles missing Items as not applied; an unavailable read or unknown receipt remains unconfirmed. The terminal retains Item and final-read evidence when either arrives before the acknowledgement. Replayed receipts remain bound to their original Sessions. Acknowledged receipts are presentation state for the current terminal lifetime; after another restart, durable Items remain the available application evidence. These observations neither create a durable input inbox nor authorize submission to another Run.

One-shot execution reads the complete tree interruption from Conversation after the root Segment closes. Member interrupts alone do not authorize a resume; a stream lost before the root boundary must reconnect or recover the durable snapshot first.

Terminal interaction reviews retain each pending Item's member Run identity while the resume command addresses the root. Before staging a decision, Conversation compares its reviewed interactions with the complete authoritative waiting set. The durable workbench preserves the exact command, member identities, and answers across restart; recovery applies the same waiting-set check.

Cold recovery reads the root's authoritative status first. A running root is recovered with `runs.subscribe(snapshot: true)`, whose material and successor tail share one Runtime observation boundary. CLI installs that material and the opaque head cursor before consuming the tail; it never replaces the material with an independent read after attachment. Waiting and finished roots install the authoritative snapshot without subscribing. Independently owned Session metadata keeps its existing stability check around the snapshot subscription, and an unstable attempt releases its tail before retrying.

## Dependency direction

The CLI uses four responsibility rings:

| Ring | Responsibility |
| --- | --- |
| Domain | Pure CLI-owned values and local aggregate invariants |
| Application | CLI workflows and consumer-owned ports |
| Adapter | Translation to Runtime and filesystem boundaries |
| Delivery | Cobra routing, terminal interaction, and rendering |

Application depends on Domain. Adapters and Delivery depend inward on the consumer contracts they satisfy. Cobra and Viper remain in command delivery; Oolong remains in terminal delivery. The concrete Runtime binding remains in `adapter/runtimebinding`; public Protocol values may cross consumer ports directly so CLI does not create synonymous DTOs.

Domain contains no I/O interfaces and no `context.Context`. A narrow port is declared beside the Application or Delivery behavior that consumes it. Composition supplies the concrete implementation.

CLI Domain types are behavior-rich only for CLI-owned invariants. A draft, queue, replay intent, or selection owner validates its state and exposes legal transitions. Runtime snapshots, protocol values, render rows, and command inputs remain typed data; wrapping them in getters does not create a domain model.

## Runtime path

The production process opens at most one concrete Runtime, fans its binding adapter into consumer-owned ports, and closes the Runtime once. There is no environment-selected fake, loopback HTTP client, service locator, or alternate product implementation.

`runtimebinding.Connection` owns binding lifecycle, capability negotiation, exact protocol translation, and safe error classification. It does not own product state. The adapter preserves exact provider/model identity and never stores credentials in CLI state, history, frames, errors, or logs.

The Runtime delivery endpoint is the single authority for the wire contract: both bindings validate request parameters, responses, and events with the generated protocol validators before a result reaches the CLI. The adapter therefore does not revalidate a Runtime result, and CLI projections do not restate Runtime rules such as identity syntax, closed enums, conditional field presence, numeric bounds, or lifecycle field agreement. What the adapter still checks is what Runtime cannot answer for it: a present and complete response, that the response answers the exact request it made, and that a catalog page keeps its requested filter, order, cursor continuity, and identity uniqueness. CLI-authored input is validated before it is sent, and a CLI value object validates its own construction.

The rule covers the error path too: the Runtime validates a problem before projecting it and substitutes an internal failure when it cannot, so a malformed problem is not a shape the endpoint can emit. It applies to the value, not the package — a helper in Domain or Application that takes a Runtime result and runs a generated validator on it is the same restatement as a direct call in the adapter, and is where the last of them survived. What legitimately looks similar is a CLI projection validating itself: reconstructing a wire value out of the CLI's own shape asks whether the translation was faithful, which nothing upstream can answer.

Tool presentation reads Runtime's decoded argument and result values directly. Complete JSON is encoded once for generic viewers and argument editing; it is not decoded again into a parallel material schema. Unknown tool fields remain available without having to match a built-in presentation shape.

Management, catalog, and workspace queries transfer fresh Runtime results to their consumer. The adapter does not clone them again. Synchronous calls borrow inputs; a component retaining mutable data acquires its own copy at that boundary, including immutable profiles and live event projections. Test bindings follow the same ownership contracts as Runtime.

The connection shares its immutable request metadata with synchronous binding calls. Runtime takes the snapshot retained by each operation or stream. The process owner snapshots configuration directories when it is constructed; Runtime copies them when resolving its configuration.

MCP management consumes Runtime server, tool, probe, and authorization values directly. Runtime also owns MCP error identities; the adapter applies the shared problem formatter without adding synonymous CLI errors. CLI retains form drafts, write intent, and acknowledgement checks; the terminal formats tool schemas when building the displayed document. An editor takes ownership of its fresh server query result.

The binding adapter's immutable `Profile` retains the validated `protocol.DiscoverResponse` and client capability declaration. Runtime owns the wire constraints and feature-negotiation rule; CLI adds only its supported-surface checks and local command-replay policy. The mutation application owns the live replay clock and admission policy; Domain owns the immutable capability and guard values. Readers receive owned protocol values. `runtime info --json` publishes these values under `discovery` and `clientCapabilities`, using the Runtime field names and limit representations directly.

The terminal model catalog aggregates Runtime's per-provider discovery results. A provider discovery failure remains visible beside successfully discovered models; the CLI never invents fallback models. Cancellation, Runtime closure, and invalid protocol responses abort the aggregate read.

## Commands

Each command tree is built by a factory. A command declares syntax and flags, converts input to a typed request, calls one CLI use case with `cmd.Context()`, and writes through Cobra streams. Runtime failures return through `RunE`; the process boundary chooses the exit status once.

Viper is only a configuration input. It resolves defaults, files, environment, and flags into typed CLI preferences before use-case execution. Business logic does not import Cobra or Viper.

## Terminal

The terminal package owns an explicit UI state tree and cohesive feature controllers. Rendering is a projection of state and terminal dimensions; it does not repair Runtime facts or start hidden workflows.

Oolong owns terminal mode, input decoding, cell measurement, and low-level editing. Flame owns product interaction, including focus, keymaps, mouse press/release matching, overlays, composer behavior, attention signals, and stable stream presentation.

Oolong also owns Markdown parsing and streaming, syntax highlighting, LaTeX layout,
and Mermaid's bounded browser execution. Flame supplies appearance and message
lifetime: completed Markdown schedules diagram preparation, the terminal owner
accepts neutral PNG results, and immutable content snapshots receive distinct image
handles. A message becomes finished only after its images or diagnostics settle.
Discard, session replacement, and shutdown cancel workers and release image data;
the terminal transport stays open until application cleanup finishes.

`/Users/tangerg/Desktop/grok-build` is the visual benchmark for information hierarchy, spacing, presentation density, stable streaming, and immediate interaction feedback. Flame keeps its own vocabulary, state ownership, and Oolong primitives. Compare deterministic renders at representative terminal dimensions so visual changes have reviewable evidence instead of subjective claims.

Long-lived terminal features own their cancellation and settlement locally. The application root coordinates them but does not mirror every feature field or become a general service bag.

Whether the connected Runtime offers an optional surface is the composition root's question, answered once against the negotiated `Profile` and expressed by leaving that consumer port unset. A binding accessor therefore returns a usable value and never reports absence with a nil pointer: assigned into a consumer's interface-typed port, a nil pointer arrives as a non-nil interface, so the consumer's own "is this wired?" check cannot fire and its first call dereferences nil. Absence is the absent field, not a present value that fails on use.

A mode the composition root never produces is not a mode. The terminal always opens a workbench — an in-memory one when no state directory is configured — so there is no draft-less, outbox-less terminal to guard against, and code that asked anyway had to invent an answer per call site: two refused, eight silently succeeded, and one reported a run as durably dispatched without writing it.

Optional terminal state is asked about once, where the choice is made. A dialog, a draft writer or a pane that has not been opened is absent from the application state, and the code that reads it says so; the type itself assumes it exists rather than returning a zero answer that reads the same as "open, but empty". A presentation block that cannot render fails where it renders instead of drawing nothing.

## Local authoring

Workbench persistence contains only CLI-authored facts. The workbench aggregate owns record names, the strict current shape, and recovery semantics; its narrow persistence port carries opaque bytes while the filesystem adapter owns rooted paths, regular-file checks, and atomic replacement. Records fail closed on unknown, malformed, oversized, truncated, or trailing content. Queue and replay are CLI aggregates with explicit identities and legal transitions; terminal code commands them instead of mutating slices and flags independently.

Drafts and queued prompts keep attachment paths editable. Before the first mutation dispatch, the binding adapter materializes those paths into Runtime content under the existing per-file size and encoding limits. Start binds that content when it leaves the queue; Steer and a Resume carrying additional input bind it before staging. Every later attempt uses the same prepared content, command identity, and replay guard without reopening the original paths. Plain text already lives completely in the immutable command and needs no external materialization. The terminal uses its existing operation owner to prepare and save content in the background. Only a current UI callback can publish the small session journal before starting delivery; cancellation or editing the source draft discards the prepared result. Large file writes hold their own pinned directory handle without holding the authoring or filesystem-store mutex. A live filesystem Store stays bound to its original physical state directory: replacing that directory requires reopening the workbench as a new owner, with the original directory retained for reconciliation. Temporarily blocking the path and then restoring the same directory allows the original owner to continue.

Workbench stores prepared content in separate SHA-256-addressed files so the supported 20 MiB attachment size does not exceed the 16 MiB authoring-record budget after base64 encoding. It commits content first, then atomically publishes its digest with the command identity and replay guard in the session record. Failed publication leaves the command editable and may leave an unreferenced content file. Content-addressed files are deduplicated and retained after cancellation or retirement: opening another Store cannot prove that an unreferenced file is not awaiting publication by a live owner, so opening never collects them. Referenced files are checked against their digest before replay. Missing or damaged content and older dispatched records containing attachment paths alone remain recoverable local records with an unknown mutation outcome. They are never filled from current files, automatically reidentified, or treated as a definitive refusal. The original session must be inspected before an explicit user decision about an unavailable command. Runtime command conflicts and local pre-dispatch failures likewise cannot prove an earlier attempt failed.

## JSON

The CLI speaks one JSON vocabulary, `encoding/json/v2`. Duplicate members, trailing documents, and unknown members are refused by the decoder itself rather than by a hand-written validating pass, and `omitzero` marks the fields whose absence is a fact, so a present-but-empty value survives the round trip. Bytes that are hashed, compared against a second projection, persisted, or piped to another program are encoded deterministically; only the durable workbench records also keep a nil collection as `null`, because a reloaded record has to distinguish an unanswered interaction from an empty answer. The two exceptions that still decode with `encoding/json` decode a JSON number into an `any`: only that package can preserve an identifier outside float64's exact range, which is what keeps a reviewed tool argument the same value when it executes.

## Package shape

A package must own a coherent CLI vocabulary, local aggregate, workflow lifecycle, external translation, or terminal mechanism. Related behavior stays in responsibility-named files inside one package. Context namespace directories exist only for several peer packages and contain no facade Go files. A package does not earn a boundary merely because its type has an interface or its workflow has one action.

Do not create packages for individual actions, interfaces, or DTOs. Merge a single-consumer forwarding package into the consumer unless the package owns an external translation or reusable technical mechanism. Do not create broad `service`, `manager`, `backend`, `common`, or `helpers` packages.

## Verification

Use fresh-root in-memory tests for command syntax and streams, Application tests for workflow ordering, deterministic terminal state/render tests for interaction behavior, and a real PTY only for terminal mode, input decoding, resize, focus, or restoration. Architecture checks enforce dependency direction and framework isolation without freezing private fields, filenames, or package inventories.
