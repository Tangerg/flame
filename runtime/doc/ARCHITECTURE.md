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

Unknown external effects fail closed. Runtime does not guess whether an unconfirmed model or tool effect succeeded and does not silently replay it.

## Persistence and recovery

SQLite stores current Application and Domain state, not live framework objects, goroutines, contexts, SDK clients, or transport connections. Aggregate decoding is strict: unknown fields, invalid states, truncated values, and trailing content are rejected.

Checkpoint and waiting facts commit in the Application order required to recover the same logical Run. Terminalization and checkpoint cleanup preserve one durable winner. Recovery reconstructs from durable Runtime state and public framework checkpoints; it does not infer state from event delivery or client caches.

Lease acquisition distinguishes contention from operational failure. Only a contended lease proves another owner is live; filesystem and lock errors abort admission or reconciliation with their cause. External database observation reports outages through Runtime telemetry and retries without inventing a change notification.

The active development contract has one current storage shape. SQLite installs that shape directly and does not maintain a schema-version or migration graph. A breaking schema change replaces the old shape completely; incompatible development state is reset explicitly unless the user authorizes a real migration requirement.

Executor restore compatibility belongs to the exact BuildID and framework Deployment references. Checkpoint payloads, policy, context sources, and Tool-input continuations encode the current shape without independent hand-maintained schema counters. Decoding still validates complete identities, capabilities, budgets, prompt digests, and structural relationships before restoring execution.

## Provider and integration boundaries

MCP configuration requires its durable registry, live connection ports, tool catalog, and shared tool policy at construction. An empty registry represents no configured servers; it does not remove any of these use cases or turn missing implementations into empty query results.

Provider identity is the exact provider/model pair plus model-owned options. Credential precedence, endpoints, SDK construction, request lowering, capability mapping, and provider-specific failures remain inside provider adapters. Product and delivery code do not infer a provider from a model name.

The ordinary chat contract owns complete and streaming calls. Complete-request token counting and other provider-specific capabilities remain separate narrow contracts discovered at the provider boundary. Runtime advertises only exact implemented behavior; it does not guess from a provider name, approximate unavailable behavior, or add optional methods to every client.

A provider declares whether its model identities come from the bundled catalog or its endpoint. Endpoint discovery is authoritative even when empty, and failures remain errors; bundled metadata may enrich discovered identities without supplying replacement results. Catalog slice results transfer ownership to the caller.

MCP, LSP, Git, filesystem, execution, and other integrations are grouped by the external system they translate. A wrapper remains only when it owns policy, translation, confinement, authority, or resource lifecycle.

## Protocol and bindings

The Contract Registry is the method and policy source used by delivery and contract generation. Generated artifacts in `contract` are the machine truth for methods, schemas, capabilities, errors, unions, and transport endpoints.

The module-root Go binding and HTTP/JSON-RPC binding enter the same delivery endpoint before:

1. request validation and capability admission;
2. idempotency and replay handling;
3. invocation lifecycle control;
4. Application execution;
5. response, error, and event projection.

The Go binding does not serialize through HTTP, but it does not bypass product semantics. Protocol changes publish one current shape without aliases, fallback decoding, dual methods, or dual events.

## Composition and lifecycle

Bootstrap constructs one endpoint and one resource graph. One Instance lifecycle owns startup rollback and ordered shutdown: stop delivery, join accepted operations and workers, stop Application producers, drain maintenance, join execution, and release resources. A caller timeout never cancels cleanup; a settled component failure allows a later Close attempt. Construction has no separate builder lifecycle. Public `runtime.Runtime` owns that Bootstrap instance and rejects new work after closing begins.

Production construction consumes the complete storage bundle opened by persistence. Plan, Goal, permission, Schedule, mutation recovery, and role persistence are always wired; absent user configuration is stored state, not a missing implementation. Narrow use-case tests supply their own collaborators without adding partial production configurations.

Every goroutine has one owner, stop condition, and join path. Request cancellation governs the request; accepted Run execution uses a Runtime-owned lifetime. Transport disconnect does not implicitly cancel durable execution.

Detached shells remain Runtime-owned after the Tool call and Run that launched them. Changing a Session's workspace or isolation policy stops that Session's shells and retires its derived context and isolated copy before exposing the replacement. Session deletion and rollback stop the same owned processes; a destructive working-tree restore additionally stops shells below the shared workspace across every Session before touching files. History or file rollback discards the old isolated copy so removed effects cannot reappear in a later Run. Runtime shutdown stops shells before destroying the isolated directories they may still use.

Process-local authority follows the facts that justify it. Replacing, rolling back, or compacting effective model context clears that Session's read-before-write evidence. Restoring a working tree clears such evidence for every Session that observed paths below the shared workspace.

Context compaction is decided only at an imminent main-model call from that call's complete request footprint: instructions, durable and transient messages, Tools, model options, provider limits, and provider-native counting when available. Protocol message count and Run completion are not pressure signals. The same path performs any durable rewrite and emits the observable boundary; post-Run maintenance only consumes the resulting fact.

Working-tree checkpoints are scoped by both Session and canonical workspace identity. A Session relocation may retain independent history for each workspace, but a Run checkpoint can only restore the exact workspace that produced it; the storage adapter verifies the complete persisted identity before any Git mutation.

Process-local notifications carry no product truth. They wake consumers, which reread durable projections.

## Internal value ownership

Synchronous calls borrow mutable inputs until return. A store or adapter that returns a newly decoded slice transfers it to the caller; retaining or asynchronously using mutable data requires an independent snapshot. Immutable Domain values share their private representation across containers and copy mutable inputs or projections only at their boundary. Validation protects external input and persistent decoding, rather than repeatedly reconstructing an already-valid value.

## Package shape

Large rings may use `ring/context/package` where a context contains several peer packages. Namespace directories contain no Go facade. Direct ring packages are reserved for ring-wide mechanisms or aggregates that also name their context.

Keep related behavior in responsibility-named files inside one package. Split a package only for a distinct aggregate vocabulary, workflow lifecycle, external translation, or reusable mechanism. Merge forwarding, identity-only, helper-only, and one-concept packages into the owner that gives them meaning.

## Verification

Tests protect observable protocol and binding behavior, Domain invariants, Application transaction ordering, strict persistence, recovery, execution lifecycle, and dependency direction. The primary lifecycle matrix covers Goal, Plan, steer, HITL, interruption and resume, compaction, long context, long execution, provider failure, restart, and recovery through one Runtime. Architecture tests prevent outer dependencies from leaking inward and keep public SDKs at their adapters; they do not freeze private filenames, fields, function inventories, or exact package counts. Multi-client, multi-server, and race scenarios need evidence that Runtime owns that concurrency.
