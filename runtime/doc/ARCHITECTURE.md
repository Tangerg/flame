# Flame Runtime architecture

Flame Runtime is the product backend and sole authority for durable agent semantics. Its center is the user-visible `Run`, not a provider call, framework process, transport connection, or database row.

## Product model

Runtime owns Session, Run, Segment, Item, Goal, Plan, Interrupt, execution, persistence, recovery, provider/model selection, and compaction. Each fact has one Domain or Application owner. Protocol objects, storage records, streams, and client caches encode or project those facts without advancing them independently.

The main aggregate relationships are:

- A Session owns the durable workspace and selection context for Runs.
- A Run owns one logical execution lifecycle.
- A Segment is one active interval of a Run; interruption and resume create another Segment without creating another logical Run.
- Items are durable observable history. Streaming deltas are replaceable previews.
- Goals and Plans are distinct product concepts with their own invariants and Application workflows.

An aggregate validates its complete initial state, keeps mutable representation private, and exposes intention-revealing queries and legal transitions. Application code coordinates aggregates instead of reproducing their rules with switches over exported fields. Wire payloads, storage records, configuration values, provider messages, and read projections remain strict data because they translate facts rather than own them.

A Goal outcome report requires the immutable Goal incarnation that admitted its Run. Application validates that origin before loading or changing the current Goal; a superseded Run cannot complete its replacement.

## Dependency direction

| Ring | Responsibility |
| --- | --- |
| Domain | Aggregates, values, invariants, legal transitions, and pure policy |
| Application | Use-case ordering, transactions, workflow lifetime, and consumer-owned ports |
| Adapter | Translation to Scope, providers, MCP, persistence, and other external systems |
| Infra | Filesystem, SQLite, Git, process, LSP, telemetry, and other technical mechanisms |
| Delivery | Binding-neutral endpoint, dispatch, HTTP/SSE transport, and protocol projection |
| Bootstrap | Concrete construction, ownership transfer, startup, recovery, and shutdown |

Domain does not depend on Application or outer rings. Application depends on Domain and declares narrow interfaces for effects it consumes. Adapters and Infra satisfy those interfaces. Delivery invokes Application through one endpoint. Bootstrap is the only concrete composition root.

External SDK types do not cross their adapter. Protocol types do not enter Domain or Application. Persistence records are decoded into valid Domain values before use.

## Execution boundary

Scope's Agent Framework is the only process, strategy, child-tree, tool-loop, and checkpoint execution engine. Runtime does not copy its scheduler or interpret private framework state.

`adapter/agentexec` is the anti-corruption boundary. It maps Runtime commands and values to public Scope contracts, observes framework outcomes, and maps them back to Runtime facts. Application owns product admission, transaction ordering, cancellation intent, durable waiting state, and terminal outcome selection.

Framework observations are wake-ups, not durable commits. Runtime reconciles authoritative framework state into an Application write set before publishing durable product facts. A completed durable Item or snapshot wins over a missing or duplicated preview event.

A Delegate retains its admitted child across a human-input barrier. Each continuation opens fresh Segments, so the executor observation reopens the parent Tool attempt before forwarding child results. Application reuses the durable Tool Item identity; continuation does not admit another child or repeat its completed work.

Tool continuation uses the executor's stable call identity. Edited approval arguments change the execution input while preserving that identity; a new call with the same name or arguments receives its own Item. One remaining-call index owns whether a suspended Item still needs to resume or settle.

A Question owns its completed prompt Item and answer schema. Its unfinished Tool suspends and resumes through the ordinary Tool continuation path; the prompt retains the handler's semantic input without copying its execution identity.

A child's terminal projection precedes the waiting barrier even when an earlier sibling still needs input. Only parent Tool results wait for the model's declared call order. Completed children leave the product continuation set; the restored Scope tree retains their pending parent Tool results until that order can advance. Application supplies each waiting member's drained Tool identities so restoration does not reopen a parent result that was already committed.

Canceling the final waiting child also opens a continuation Segment. Its observer reopens only surviving parent Tool attempts. The executor supplies the exact model-visible cancellation result during preparation; Application commits that value with the canceled Items and resulting checkpoint before the tree can advance.

Continuation state retains only unfinished Tool identities. A canceled child's settled parent Item and model-context result remain in their durable owners; the executor retires that call when cancellation applies, so later continuations do not carry a separate result acknowledgment.

Accepting an approval settles its verdict; its Tool Item remains open until execution settles or the Run ends. Reported and synthesized terminal outcomes share the same Tool cleanup. Definite Runtime preparation failures and rejected argument edits use ordinary Tool settlement to commit their exact model-visible results before the executor advances. Input waits, cancellation, and uncertain effects retain their framework control semantics; restart and later Runs retain committed failures.

Unknown external effects fail closed. Runtime does not guess whether an unconfirmed model or tool effect succeeded and does not silently replay it.

## Persistence and recovery

SQLite stores current Application and Domain state, not live framework objects, goroutines, contexts, SDK clients, or transport connections. Aggregate decoding is strict: unknown fields, invalid states, truncated values, and trailing content are rejected.

Checkpoint and waiting facts commit in the Application order required to recover the same logical Run. Terminalization and checkpoint cleanup preserve one durable winner. Recovery reconstructs from durable Runtime state and public framework checkpoints; it does not infer state from event delivery or client caches.

Bootstrap creates one file-lease set from the persistence bundle's data directory and supplies it to Session admission, Goal driving, and ordered Run-then-Goal recovery. Each use case requires its ownership backend at construction, and recovery requires both reconcilers.

Lease acquisition distinguishes contention from operational failure. Only a contended lease proves another owner is live; filesystem and lock errors abort admission or reconciliation with their cause. The persistence and authored-file adapters report the first failure in each outage through Runtime logging, including when the embedding host has not configured tracing, and retry without inventing a change notification. A successful resample resumes observation and permits a later outage to be reported again. Filesystem reconciliation reads the backend's current registrations so deleted and recreated directories regain their watches. Git observation retains its last successful HEAD/index snapshot across read failures and logs those failures without publishing a change. Watcher event loss schedules a fresh semantic read.

Background recovery reports the first consecutive sweep failure through the same logging channel and continues retrying. Schedule scanning and dispatch failures remain visible while durable pending occurrences retain their retry path. Goal driver failures and rejected Run starts are logged with their Session and Goal incarnation identities; durable Goal pause reasons keep stable cause codes rather than diagnostic errors. These diagnostics do not require tracing configuration.

The active development contract has one current storage shape. SQLite installs that shape directly and does not maintain a schema-version or migration graph. A breaking schema change replaces the old shape completely; incompatible development state is reset explicitly unless the user authorizes a real migration requirement.

Executor restore compatibility belongs to the exact BuildID and framework Deployment references. Checkpoint payloads, policy, context sources, and Tool-input continuations encode the current shape without independent hand-maintained schema counters. Decoding still validates complete identities, capabilities, budgets, prompt digests, and structural relationships before restoring execution.

## Provider and integration boundaries

MCP configuration requires its durable registry, live connection ports, tool catalog, and shared tool policy at construction. An empty registry represents no configured servers; it does not remove any of these use cases or turn missing implementations into empty query results. Startup and background connection failures produce diagnostics with their server identity even when tracing is unconfigured. A failed startup connection or tool catalog leaves that server unavailable while independently configured servers remain usable.

Provider identity is the exact provider/model pair plus model-owned options. Credential precedence, endpoints, SDK construction, request lowering, capability mapping, and provider-specific failures remain inside provider adapters. Product and delivery code do not infer a provider from a model name.

Auxiliary text generation has one live-role completion boundary. It validates the request envelope before resolving the provider and accepts only a natural stop with non-empty text. Truncated, filtered, refused, or otherwise incomplete output remains a failure; compaction and background curation never persist it as a completed artifact.

The ordinary chat contract owns complete and streaming calls. Complete-request token counting and other provider-specific capabilities remain separate narrow contracts discovered at the provider boundary. Runtime advertises only exact implemented behavior; it does not guess from a provider name, approximate unavailable behavior, or add optional methods to every client.

A provider declares whether its model identities come from the bundled catalog or its endpoint. Endpoint discovery is authoritative even when empty, and failures remain errors; missing required endpoint or credential configuration is a parameter error. Bundled metadata may enrich discovered identities without supplying replacement results. Catalog slice results transfer ownership to the caller.

MCP, LSP, Git, filesystem, execution, and other integrations are grouped by the external system they translate. A wrapper remains only when it owns policy, translation, confinement, authority, or resource lifecycle.

## Protocol and bindings

The Contract Registry is the method and policy source used by delivery and contract generation. Generated artifacts in `contract` are the machine truth for methods, schemas, capabilities, errors, unions, and transport endpoints. Discovery identity and capability-catalog constraints are declared there and enforced by the generated validators, so consumers do not maintain another schema.

Plan, Goal, Schedule, Knowledge, agent-memory, and file-observation use cases are present in every complete Runtime. Discovery advertises them directly; Session snapshots include the current Plan and any current Goal, and portable import restores Plan as part of the atomic Session write. Git availability remains a host fact, and repository observation failures remain errors at registration. Capability negotiation still governs optional client behavior and host-dependent integrations.

The module-root Go binding and HTTP/JSON-RPC binding enter the same delivery endpoint before:

1. request validation and capability admission;
2. idempotency and replay handling;
3. invocation lifecycle control;
4. Application execution;
5. response, error, and event projection.

That endpoint is the single authority for the wire contract. It validates every request's parameters and every response and event against the generated validators, so a consumer of either binding never has to recheck a shape Runtime already published. An invalid response or event becomes an internal error instead of reaching the caller.

A stream carries its own failure. An operation's event source reports a delivery-side defect as the stream's error rather than closing cleanly, so a consumer can distinguish a Run that stopped producing events from a Runtime that could not describe one. The JSON-RPC transport has no error frame, so it ends the stream at the first event it cannot publish and lets the client resume from its last event id; it never skips a frame and continues.

The Go binding does not serialize through HTTP, but it does not bypass product semantics. Protocol changes publish one current shape without aliases, fallback decoding, dual methods, or dual events.

## Composition and lifecycle

Bootstrap constructs one endpoint and one resource graph. One Instance lifecycle owns startup rollback and ordered shutdown: stop delivery, join accepted operations and workers, stop Application producers, drain maintenance, join execution, and release resources. A caller timeout never cancels cleanup; a settled component failure allows a later Close attempt. Construction has no separate builder lifecycle. Public `runtime.Runtime` owns that Bootstrap instance and rejects new work after closing begins.

A required collaborator is required once, where the object is built. A constructor that refuses to return a partial object makes its methods total, so they do not re-check the receiver and invent an answer for a state composition cannot produce — an empty catalog, an unknown-server error, or a successful no-op for work that never happened. A feature the user has not configured is different: it is stored state with its own query, not a missing implementation.

Optional means the user disabled it, not that composition forgot it. A durable write-set names every store it writes to, because whether a delete, fork, rollback or commit is complete cannot depend on how it was assembled; a use case that reports a committed change names its observation and invalidation channels for the same reason. Tests that do not exercise a capability supply an inert collaborator, not an absent one.

A constructed value does not re-derive its own construction. Domain identities, selections and canonical representations keep their material private and admit it through one parser, so a later `Validate` asks only whether the value was constructed at all — which is what a write-set or read boundary can actually receive.

An aggregate whose constructors and transitions all end in its own check cannot exist while invalid, so that check is unexported and no reader calls it. `transcript.Item` is the worked example: seven constructors, `RestoreItem`, and every settle, fork and approval transition close on it. What such a type still cannot prevent is its zero value, which is writable wherever the type is visible; the one question a consumer may still ask is therefore whether an aggregate was attached at all, not whether it is legal.

A value is validated where it enters the process, and read everywhere else. The boundaries that own that answer are the ones facing something untrusted: a use case turning caller input into a domain value, an adapter parsing a remote server's reply, and a store decoding its own persisted rows. An Application that re-checks identity, uniqueness or field agreement on the way back out of its own port is not defending the product — it is a second implementation of a rule the producer already owns, and it drifts. Uniqueness stated by a primary key, a lookup keyed by the identity it was asked for, and a projection whose contradictory states the producer clears at the transition are all facts to consume, not to reprove. A test that reaches this code through a fake port returning something the real one cannot produce is freezing the defense, not the contract.

Production construction consumes the complete storage bundle opened by persistence. Plan, Goal, permission, Schedule, mutation recovery, role persistence, Knowledge, and agent-memory review and curation are always wired; absent user configuration is stored state, not a missing implementation. Agent-memory search may use keyword ranking without an embedding provider. Narrow use-case tests supply their own collaborators without adding partial production configurations. Hook execution and management share one resolver built from the user home and durable trust store. A project with no hooks or revoked trust still has complete inspection and trust-management use cases.

Skill discovery, library curation, and proposal review require complete implementations. Bootstrap requires an absolute user Skill directory and constructs its store, usage recorder, and maintenance workers even when the library is empty. A Skill store requires an absolute library root and a valid scope, and a maintenance component requires its sweeper at construction.

Every goroutine has one owner, stop condition, and join path. Request cancellation governs the request; accepted Run execution uses a Runtime-owned lifetime. Transport disconnect does not implicitly cancel durable execution.

Detached shells remain Runtime-owned after the Tool call and Run that launched them. Changing a Session's workspace or isolation policy stops that Session's shells and retires its derived context and isolated copy before exposing the replacement. Session deletion and rollback stop the same owned processes; a destructive working-tree restore additionally stops shells below the shared workspace across every Session before touching files. History or file rollback discards the old isolated copy so removed effects cannot reappear in a later Run. Runtime shutdown stops shells before destroying the isolated directories they may still use.

Process-local authority follows the facts that justify it. Working-context construction requires the user home and the Knowledge, agent-memory, Goal, Plan, and hook readers; absent user content is an owner-provided empty result. Session cleanup requires the working-context, tool, and shell owners at construction; compaction requires its context invalidator. Durable compaction invalidates context only after its history rewrite succeeds. Replacing, rolling back, or compacting effective model context clears that Session's read-before-write evidence. Restoring a working tree clears such evidence for every Session that observed paths below the shared workspace.

Context compaction is decided only at an imminent main-model call from that call's complete request footprint: instructions, durable and transient messages, Tools, model options, provider limits, and provider-native counting when available. Protocol message count and Run completion are not pressure signals. The same path performs any durable rewrite and emits the observable boundary; post-Run maintenance only consumes the resulting fact.

That boundary first commits completed Delegate results already present in the imminent request. Durable context comparison must observe those results even when background reconciliation has not run yet, including after canceling a waiting sibling and restoring the parent.

Required compaction resolves its current lifecycle Hook policy before calling the summary model or rewriting history. A configuration or trust-read failure stops compaction and preserves its cause. Hook command, observe-only lifecycle Hook, refetchable Tool projection, Skill usage recording, and post-Run maintenance failures produce diagnostics without requiring an active tracing span. Their best-effort policy does not revise the committed Tool result or published lifecycle boundary.

Working-tree checkpoints are scoped by both Session and canonical workspace identity. A Session relocation may retain independent history for each workspace, but a Run checkpoint can only restore the exact workspace that produced it; the storage adapter verifies the complete persisted identity before any Git mutation.

Process-local notifications carry no product truth. They wake consumers, which reread durable projections.

## Internal value ownership

Synchronous calls borrow mutable inputs until return. A store or adapter that returns a newly decoded slice transfers it to the caller; retaining or asynchronously using mutable data requires an independent snapshot. Immutable Domain values share their private representation across containers and copy mutable inputs or projections only at their boundary. Validation protects external input and persistent decoding, rather than repeatedly reconstructing an already-valid value.

## Package shape

Large rings may use `ring/context/package` where a context contains several peer packages. Namespace directories contain no Go facade. Direct ring packages are reserved for ring-wide mechanisms or aggregates that also name their context.

Keep related behavior in responsibility-named files inside one package. Split a package only for a distinct aggregate vocabulary, workflow lifecycle, external translation, or reusable mechanism. Merge forwarding, identity-only, helper-only, and one-concept packages into the owner that gives them meaning.

## Verification

Tests protect observable protocol and binding behavior, Domain invariants, Application transaction ordering, strict persistence, recovery, execution lifecycle, and dependency direction. The primary lifecycle matrix covers Goal, Plan, steer, HITL, interruption and resume, compaction, long context, long execution, provider failure, restart, and recovery through one Runtime. Architecture tests prevent outer dependencies from leaking inward and keep public SDKs at their adapters; they do not freeze private filenames, fields, function inventories, or exact package counts. Multi-client, multi-server, and race scenarios need evidence that Runtime owns that concurrency.
