# Flame CLI architecture

Flame CLI consumes the public embedded and HTTP Runtime bindings. It provides one-shot Cobra commands and an interactive Oolong terminal client without creating a second product backend.

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

Ending terminal observation releases live presentation blocks while retaining the last Runtime Tool status and observed output. Only an authoritative Runtime Item updates a Tool outcome; a local observation error is displayed separately.

Steer acknowledgement preserves Runtime's exact reserved User Item ID. Acceptance means the instruction is waiting to enter model context; only a completed User Item with that identity in the same Run proves application. Pending steer signals may survive an interruption and apply in a later Segment of that Run. After the Run finishes, a separate authoritative Session read settles missing Items as not applied; an unavailable read or unknown receipt remains unconfirmed. The terminal retains Item and final-read evidence when either arrives before the acknowledgement. Replayed receipts remain bound to their original Sessions. Acknowledged receipts are presentation state for the current terminal lifetime; after another restart, durable Items remain the available application evidence. These observations neither create a durable input inbox nor authorize submission to another Run.

One-shot execution reads the complete tree interruption from Conversation after the root Segment closes. Member interrupts alone do not authorize a resume; a stream lost before the root boundary must reconnect or recover the durable snapshot first.

Terminal interrupt reviews retain each pending Item's member Run identity while the resume command addresses the root. Before staging a decision, Conversation compares its reviewed interrupts with the complete authoritative waiting set. The durable workbench preserves the exact command, member identities, and answers across restart; recovery applies the same waiting-set check.

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

The separate `localruntime` module owns product-root resolution and local deployment
handoff paths. Process composition uses it to resolve `FLAME_HOME` before constructing
the binding and authoring store; the binding adapter derives Runtime's data directory
from that root. Only those two boundaries may import it.

Domain contains no I/O interfaces and no `context.Context`. A narrow port is declared beside the Application or Delivery behavior that consumes it. Composition supplies the concrete implementation.

CLI Domain types are behavior-rich only for CLI-owned invariants. A draft, queue, replay intent, or selection owner validates its state and exposes legal transitions. Runtime snapshots, protocol values, render rows, and command inputs remain typed data; wrapping them in getters does not create a domain model.

## Runtime path

The production process selects one target and fans its binding adapter into consumer-owned ports. An empty endpoint opens one embedded Runtime; an explicit endpoint connects Runtime's public HTTP/SSE client to an existing Runtime. Both bindings enter the same server delivery Endpoint. There is no environment-selected fake, service locator, alternate product implementation, or fallback between targets. The process closes the binding once; only the embedded binding owns Runtime shutdown.

CLI preferences own endpoint selection, while process input owns its bearer credential. The endpoint is immutable after the first connection. The remote client performs transport framing, strict decoding, and schema validation at the public Runtime boundary. CLI retains one negotiation and one immutable Profile for either binding. Invalid remote replies become permanent incompatible-Runtime errors; transport loss preserves unknown mutation acknowledgement and the existing exact replay policy.

Availability and mutation certainty are separate observations. A lost connection may be reattached or retried with the retained command identity. A malformed acknowledgement is permanent for that connection but still cannot prove that its mutation was refused: durable outboxes retain the unknown intent without retrying malformed data. Authoritative Runtime problem responses remain definitive refusals. Closing and later recovering an explicitly canceled but unacknowledged opening reuses one exact cancellation identity and payload.

The remote terminal detaches on exit without canceling an observed or newly accepted Run. An explicitly requested cancellation retains its settlement and durable recovery obligations. One-shot execution retains its separate process-interruption policy: cancel the Run that invocation started, release observations, and leave the shared Runtime alive.

`runtimebinding.Connection` owns binding lifecycle, capability negotiation, exact protocol translation, and safe error classification. It does not own product state. The adapter preserves exact provider/model identity and never stores credentials in CLI state, history, frames, errors, or logs.

The Runtime delivery endpoint is the single authority for the wire contract: both bindings validate request parameters, responses, and events with the generated protocol validators before a result reaches the CLI. The adapter therefore does not revalidate a Runtime result, and CLI projections do not restate Runtime rules such as identity syntax, closed enums, conditional field presence, numeric bounds, or lifecycle field agreement. What the adapter still checks is what Runtime cannot answer for it: a present and complete response, that the response answers the exact request it made, and that a catalog page keeps its requested filter, order, cursor continuity, and identity uniqueness. CLI-authored input is validated before it is sent, and a CLI value object validates its own construction.

The rule covers the error path too: the Runtime validates a problem before projecting it and substitutes an internal failure when it cannot, so a malformed problem is not a shape the endpoint can emit. It applies to the value, not the package — a helper in Domain or Application that takes a Runtime result and runs a generated validator on it is the same restatement as a direct call in the adapter, and is where the last of them survived. What legitimately looks similar is a CLI projection validating itself: reconstructing a wire value out of the CLI's own shape asks whether the translation was faithful, which nothing upstream can answer.

Tool presentation reads Runtime's decoded argument and result values directly. Complete JSON is encoded once for generic viewers and argument editing; it is not decoded again into a parallel material schema. Unknown tool fields remain available without having to match a built-in presentation shape.

Management, catalog, and workspace queries transfer fresh Runtime results to their consumer. The adapter does not clone them again. Synchronous calls borrow inputs; a component retaining mutable data acquires its own copy at that boundary, including immutable profiles and live event projections. Test bindings follow the same ownership contracts as Runtime.

The connection shares its immutable request metadata with synchronous binding calls. Runtime takes the snapshot retained by each operation or stream. The process owner snapshots configuration directories when it is constructed; Runtime copies them when resolving its configuration.

MCP management consumes Runtime server, tool, probe, and authorization values directly. Runtime also owns MCP error identities; the adapter applies the shared problem formatter without adding synonymous CLI errors. CLI retains form drafts and write intent and projects the server Runtime returns; the terminal formats tool schemas when building the displayed document. An editor takes ownership of its fresh server query result.

The binding adapter's immutable `Profile` retains the validated `protocol.DiscoverResponse` and client capability declaration. Command and terminal delivery declare their own read interfaces; only process composition imports the concrete binding. Runtime owns the wire constraints and feature-negotiation rule; CLI adds only its supported-surface checks and local command-replay policy. The mutation application projects the negotiated replay limits and owns the live replay clock and admission policy; Domain owns the immutable capability and guard values. Readers receive owned protocol values. Command delivery renders `runtime info --json` under `discovery` and `clientCapabilities`, using the Runtime field names and limit representations directly.

The terminal model catalog aggregates Runtime's per-provider discovery results. A provider discovery failure remains visible beside successfully discovered models; the CLI never invents fallback models. Cancellation, Runtime closure, and invalid protocol responses abort the aggregate read.

## Commands

Each command tree is built by a factory. A command declares syntax and flags, converts input to a typed request, calls one CLI use case with `cmd.Context()`, and writes through Cobra streams. Runtime failures return through `RunE`; the process boundary chooses the exit status once.

Viper is only a configuration input. It resolves defaults, files, environment, and flags into typed CLI preferences before use-case execution. Business logic does not import Cobra or Viper.

## Terminal

The terminal package owns an explicit UI state tree and cohesive feature controllers. Rendering is a projection of state and terminal dimensions; it does not repair Runtime facts or start hidden workflows.

Oolong owns terminal mode, input decoding, cell measurement, and low-level editing. Flame owns product interaction, including focus, keymaps, mouse press/release matching, overlays, composer behavior, attention signals, and stable stream presentation.

Oolong's scroll owner supplies the queue drawer's frame-local layout and commits it
with the complete root frame. Flame preserves selection by the queued entry's
identity when a fresh snapshot arrives and requests that entry remain visible as
the drawer resizes. Queue action hit regions are projections of that layout, not a
separately calculated viewport.

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

The terminal requires an explicit workbench factory and owns the returned Store until shutdown. Process composition supplies the same lazy factory to terminal startup and command-side Session deletion. It selects in-memory persistence when no state directory is configured, and otherwise opens the rooted state-file adapter. Delivery never chooses persistence or interprets a state directory. Help and completion-script generation do not invoke the factory.

Optional terminal state is asked about once, where the choice is made. A dialog, a draft writer or a pane that has not been opened is absent from the application state, and the code that reads it says so; the type itself assumes it exists rather than returning a zero answer that reads the same as "open, but empty". A presentation block that cannot render fails where it renders instead of drawing nothing.

## Local authoring

Runtime workspace references and local authoring directories have separate owners. In remote mode, the Runtime validates and canonicalizes its filesystem paths, while a fixed client-local directory anchors attachment resolution, external editors, imports, and exports. Session switching never turns a remote workspace reference into a local file capability. Embedded composition may resolve the local working directory before sending it as a workspace. Recent Runtime workspace references are persisted unchanged, including foreign path syntax.

Sideloaded command protocol 2 carries both the Runtime workspace reference and the local authoring directory. The executable remains rooted in its discovered plugin directory. Manifest schema version 3 declares these semantics so an older executable is rejected before invocation; responses must also declare command protocol 2. Extension-host API version 1 is unchanged.

`application/extensions` owns contributed command identity, argument cardinality, availability, request snapshots, results, and the typed command contribution point. `adapter/sideload` translates manifests and bounded process I/O into that contract without importing terminal delivery. Oolong block, tool, and custom-event presentation points remain in terminal delivery.

Each contribution point has one definition identity. A separately constructed point with the same name cannot publish to or read the original point, including by weakening its capability. Keyed points derive identity only through their declared key policy; registration options do not override it. Plugin and dependency names must already have their canonical spelling at admission.

Remote authoring persistence is partitioned by the configured endpoint; embedded state keeps its existing directory. The terminal and command-side session deletion workflow use the same partition. Endpoint identity scopes local drafts and journals, while Runtime's advertised idempotency namespace and retention independently decide whether an exact command may replay. Neither a new process instance ID nor an endpoint spelling change authorizes migration of a pending mutation.

Workbench persistence contains only CLI-authored facts. The workbench Application owner owns record names, the strict current shape, and recovery semantics; its narrow persistence port carries opaque bytes while the filesystem adapter owns rooted paths, regular-file checks, and atomic replacement. Records fail closed on unknown, malformed, oversized, truncated, or trailing content.

The authoring Domain owns prompt values, deterministic FIFO and reservation transitions, and exact replay identities and guards. Workbench owns the live queue's transaction with the durable outbox: enqueue transfers the draft before dispatch, edits roll back the complete FIFO state on a failed write, and acknowledgement commits history and outbox retirement before releasing the opening reservation. A partial history commit is retried under the same command identity. A definitive refusal replaces that identity durably before returning the command to the FIFO. Terminal controllers issue these Workbench commands and render detached queue snapshots; they do not persist queue ordering, restore transactional state, or settle the outbox themselves.

Drafts and queued prompts keep attachment paths editable. Before the first mutation dispatch, the binding adapter materializes those paths into Runtime content under the existing per-file size and encoding limits. Start binds that content when it leaves the queue; Steer and a Resume carrying additional input bind it before staging. Every later attempt uses the same prepared content, command identity, and replay guard without reopening the original paths. Plain text already lives completely in the immutable command and needs no external materialization. The terminal uses its existing operation owner to prepare and save content in the background. Only a current UI callback can publish the small session journal before starting delivery; cancellation or editing the source draft discards the prepared result. Large file writes hold their own pinned directory handle without holding the authoring or filesystem-store mutex. A live filesystem Store stays bound to its original physical state directory: replacing that directory requires reopening the workbench as a new owner, with the original directory retained for reconciliation. Temporarily blocking the path and then restoring the same directory allows the original owner to continue.

Workbench stores prepared content in separate SHA-256-addressed files so the supported 20 MiB attachment size does not exceed the 16 MiB authoring-record budget after base64 encoding. It commits content first, then atomically publishes its digest with the command identity and replay guard in the session record. Failed publication leaves the command editable and may leave an unreferenced content file. Content-addressed files are deduplicated and retained after cancellation or retirement: opening another Store cannot prove that an unreferenced file is not awaiting publication by a live owner, so opening never collects them. Referenced files are checked against their digest before replay. Missing or damaged content and older dispatched records containing attachment paths alone remain recoverable local records with an unknown mutation outcome. They are never filled from current files, automatically reidentified, or treated as a definitive refusal. The original session must be inspected before an explicit user decision about an unavailable command. Runtime command conflicts and local pre-dispatch failures likewise cannot prove an earlier attempt failed.

## JSON

The CLI speaks one JSON vocabulary, `encoding/json/v2`. Duplicate members, trailing documents, and unknown members are refused by the decoder itself rather than by a hand-written validating pass, and `omitzero` marks the fields whose absence is a fact, so a present-but-empty value survives the round trip. Bytes that are hashed, compared against a second projection, persisted, or piped to another program are encoded deterministically; only the durable workbench records also keep a nil collection as `null`, because a reloaded record has to distinguish an unanswered interrupt from an empty answer. The two exceptions that still decode with `encoding/json` decode a JSON number into an `any`: only that package can preserve an identifier outside float64's exact range, which is what keeps a reviewed tool argument the same value when it executes.

## Package shape

A package must own a coherent CLI vocabulary, local aggregate, workflow lifecycle, external translation, or terminal mechanism. Related behavior stays in responsibility-named files inside one package. Context namespace directories exist only for several peer packages and contain no facade Go files. A package does not earn a boundary merely because its type has an interface or its workflow has one action.

| Owner | Package | Responsibility |
| --- | --- | --- |
| Prompt authoring | `domain/authoring/prompt` | Message and attachment values, authored model options, Start and Steer intents, frozen input |
| FIFO authoring | `domain/authoring/queue` | Pure entry identity, editing holds, ordering and dispatch reservations |
| Exact replay | `domain/authoring/replay` | Stable command identity, capability and replay-guard values |
| Conversation | `domain/conversation` | Transcript folding, stream deduplication, interrupt review and Runtime projections |
| Workbench | `application/workbench` | Local authoring lifetime, durable records, draft transfers, queue transactions and outbox settlement |
| Mutation admission | `application/mutation` | Identity generation, replay clocks and exact acknowledgement policy shared by workflows |
| Agent workflows | `application/agent/run`, `application/agent/session` | Run observation and command workflows over consumer-owned Runtime ports |
| Extensions | `application/extensions` | Plugin activation, command contracts, contribution ownership and cleanup |
| Runtime binding | `adapter/runtimebinding` | Embedded/HTTP binding lifecycle, negotiation and projection translation |
| Local storage | `adapter/filesystem` | Scoped filesystem adapters for attachments, editors, session artifacts and opaque state records |
| Sideload execution | `adapter/sideload` | Manifest translation, confined executable invocation and command-protocol validation |
| User delivery | `delivery/cmd`, `delivery/terminal` | Cobra commands and Oolong interaction, rendering and view lifetimes |
| Process composition | CLI root package | Immutable Runtime target, lazy Workbench factory, port wiring and shutdown |

`authoring`, `agent`, `integration` and `filesystem` are sparse namespace directories. They contain no facade package. The Workbench Store retains clocks, randomness and persistence in Application; moving its pure queue transitions to Domain does not move filesystem or process lifetime there. Conversation is a CLI projection owner and does not become a second Runtime Session or Run state machine.

Do not create packages for individual actions, interfaces, or DTOs. Merge a single-consumer forwarding package into the consumer unless the package owns an external translation or reusable technical mechanism. Do not create broad `service`, `manager`, `backend`, `common`, or `helpers` packages.

## Verification

Use fresh-root in-memory tests for command syntax and streams, Application tests for workflow ordering, deterministic terminal state/render tests for interaction behavior, and a real PTY only for terminal mode, input decoding, resize, focus, or restoration. Architecture checks enforce dependency direction and framework isolation without freezing private fields, filenames, or package inventories.
