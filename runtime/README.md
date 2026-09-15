# Flame Runtime

Flame Runtime is the local product backend for Flame. It owns durable agent semantics and exposes the same behavior through an in-process Go binding and the Runtime Protocol.

Runtime is not another agent framework. Scope owns process execution, strategies, tools, and provider libraries; Runtime adapts those capabilities to Flame's Session, Run, Goal, Plan, persistence, recovery, and protocol model.

## Public surfaces

- The module-root `runtime.Runtime` is the concrete in-process binding.
- `protocol` contains binding-neutral requests, responses, events, errors, and validation.
- `contract` contains generated machine-readable protocol artifacts and the generated API reference.
- `localruntime` owns the local deployment layout and the strict credential-file handoff.
  It resolves the data directory beneath a product root and names the database and local
  token inside it, so the Runtime process and a trusted desktop client read one layout
  rather than each composing a path.

All Runtime operations enter one delivery endpoint. The Go binding avoids JSON and HTTP encoding but uses the same admission, capability, idempotency, Application, error, and event semantics as the HTTP binding.

## Open an in-process Runtime

```go
rt, err := runtime.Open(ctx, runtime.Config{
	DataDirectory:        dataDirectory,
	DefaultWorkspacePath: workspace,
})
if err != nil {
	return err
}
defer rt.Close()

session, err := rt.CreateSession(ctx, protocol.CreateSessionRequest{
	Workspace: &protocol.WorkspaceRef{Path: workspace},
}, runtime.CommandOptions{IdempotencyKey: requestID + ":session"})
if err != nil {
	return err
}
```

Hosts must close each Runtime they open. Protocol errors support `errors.Is` against public sentinel errors and `errors.As` to `protocol.ProblemError` for structured recovery information.

Cancellation failures returned by the Go binding also preserve `context.Canceled` or `context.DeadlineExceeded` for `errors.Is`. Request cancellation causes are local to that invocation; wire problems and persisted replay outcomes retain their protocol category and safe details.

The standalone Runtime and CLI interpret `FLAME_HOME` as Flame's local product root. Runtime-owned state lives under `$FLAME_HOME/runtime`; the default is `~/.flame/runtime`.

## Develop

```sh
GOWORK=off go test ./...
GOWORK=off go vet ./...
GOWORK=off go build ./...
go generate ./...
```

The default suite is offline. Module rules live in [Module instructions](#module-instructions) below; current boundaries live in [`doc/ARCHITECTURE.md`](doc/ARCHITECTURE.md).

## Module instructions

Flame Runtime is the product backend and sole owner of durable agent semantics. It exposes one in-process Go binding and one Runtime Protocol for CLI, Desktop, and other hosts.

Read [`../AGENTS.md`](../AGENTS.md), [`../DEVELOPMENT.md`](../DEVELOPMENT.md), and the ordered Runtime baseline in [`doc/README.md`](doc/README.md) before changing this module.

- Runtime is a product backend, not another Agent Framework. Scope owns Process, strategy, tree, provider, and effect execution contracts; Runtime adapts them to Flame product semantics.
- The module root owns the public in-process `Runtime` lifecycle and typed operation methods. `protocol` owns public request, response, event, error, version, and validation values. Do not define synonymous public models or a forwarding binding package.
- The Go binding and HTTP/JSON-RPC binding must enter the same delivery endpoint before capability admission, idempotency, invocation lifetime, Application use cases, and event or error projection.
- Domain packages own behavior-rich aggregates, value objects, invariants, and pure transitions. Aggregates validate construction, hide mutation, and expose legal domain operations; protocol, storage, configuration, provider, and projection values remain strict data. Application packages own cohesive use-case families and cross-aggregate ordering. Neither depends on protocol, storage, provider SDKs, or delivery types.
- Keep a Domain or Application package only when it protects a real aggregate vocabulary, invariant, or workflow lifecycle. Merge helper, identity, reference, and one-concept packages into their proven owner; do not replace them with one god package.
- Organize each large ring as `ring/context/package`. Context directories are non-package namespaces backed by multiple related packages; direct ring packages are limited to ring-wide mechanisms or aggregate packages that also name their context. Do not create facade parents, single-child namespaces, or deeper decorative nesting.
- Group external code by the system it translates: provider, MCP, SQLite, filesystem, Git, execution, LSP, or transport. Delete adapter-to-infra forwarding layers that own no policy or lifecycle.
- Bootstrap is the only concrete composition and shutdown owner. It constructs one delivery endpoint and one resource graph; do not maintain parallel Assembly, Host, Instance, Server, or optional-service lifecycles.
- Interfaces belong to consuming Application or delivery code and contain only invoked operations. Keep a cohesive single implementation concrete.
- Run is the product execution center. Conversation, Transcript, WorkingContext, checkpoint, stream observation, durable Item, and recovery remain distinct facts with explicit owners.
- Provider/model selection is exact and durable. Provider SDK types, credentials, endpoints, request lowering, and provider-specific errors stay behind provider adapters. Optional provider capabilities use separate narrow consumer contracts; never inflate the ordinary chat client or approximate an unavailable capability in Runtime.
- Protocol operations express product behavior, not internal functions. Catalog metadata is machine truth; generated contracts and typed Go methods are checked projections of it.
- Public API, wire, storage, and generated-contract changes are replaced completely. Migrate all in-scope consumers and delete the former shape without aliases, fallback decoding, dual persistence, or compatibility packages.
- Prefer real single-Runtime end-to-end scenarios for Goal, Plan, steer, HITL, interruption, compaction, long context, long execution, provider failure, restart, and recovery. Test concurrency only where Runtime owns concurrent lifecycle; do not invent multi-client or multi-server scenarios without a product obligation.
- Decide compaction only at an imminent model call from that complete request's token footprint. Do not trigger it from protocol message counts or Run completion. Pre-release SQLite installs the current schema directly; do not add hand-maintained epochs or a migration graph without an explicit migration requirement.

## Model invocation history

`modelInvocations.list` reads recorded provider attempts for one exact Run, newest first, with an opaque cursor and a maximum page size of 100. Child Run reads require the subagent capability, just like `runs.get`. The Go binding exposes the same operation as `ListModelInvocations`.

The invocation journal owns call identity, Segment identity, observed state, and timestamps. These records now survive Run completion and restart; deletion of their Run cascades to the records. Schema installation replaces the former pruning trigger on existing databases. Attempts already deleted by older versions cannot be reconstructed from Run token totals or transcript text.

`started` has no settlement timestamp. `completed` means Runtime accepted the complete model response. `failed` records an observed failure at the model boundary, including an invalid response or interrupted stream; it does not imply that the provider performed no work. `unknown` means execution or recovery could not establish the outcome: its `settledAt` is when that uncertainty was recorded, not a provider completion time. Consumers must not infer a measured model duration or throughput from an unknown outcome.

The timestamps come from Runtime's call lifecycle. `startedAt` is captured while reducing the start fact, before its durable commit and provider dispatch. Completion is captured after response validation and the stream projection barrier, before the completion commit. For completed and failed calls, their difference measures this Runtime lifecycle interval, including local work and waiting; it is not isolated provider latency. Neither lifecycle timestamp records the first output. Decode throughput cannot be reconstructed from this interval, token totals, or transcript timestamps.

The optional `firstOutputLatencyMillis` is measured with the monotonic clock at the streaming model boundary: from entering the provider stream to arrival of the first valid text, visible reasoning, refusal, media, or tool-call delta. It includes provider-client and network work, but excludes Run admission and the start commit. Metadata, usage, finish markers, citation attachments, and opaque reasoning state do not count. The measurement is committed with a completed or failed attempt; a stream that fails after producing output retains it. Nonstreaming calls, failures before output, unknown recovery outcomes, and historical attempts leave it absent. A measured zero is valid. It is not a token-generation rate or isolated model compute time. Existing databases receive a nullable column in the schema transaction without backfilling estimates.

The optional `usage` records provider-reported tokens for that call before Run aggregation. Missing usage means it was not reported or the attempt predates usage recording; an explicit zero remains zero. Prompt inspection is not present in this read. Aggregate Run accounting remains separate. Existing databases receive the nullable usage column in one schema transaction; historical values are not reconstructed.

## Auxiliary model observation

Compaction, memory extraction and curation, skill mining, and title generation emit an `auxiliary model` OpenTelemetry span through the existing Runtime exporter. `auxiliary.operation` identifies the caller's purpose. The span covers selection resolution, the bounded model request, and response acceptance; its duration is not isolated provider latency. The live resolver records the exact provider/model selection, and a valid response contributes its finish reason and reported token usage. Optional cache and reasoning counts remain absent when unreported, including the distinction between absence and an explicit zero. A valid but incomplete response retains its reported usage even though its text is rejected.

Failed attempts record the `resolve`, `call`, or `response` stage and distinguish cancellation from deadline expiry. These spans do not record prompt text, output text, or raw provider errors. They are diagnostic telemetry, not durable `modelInvocations.list` records or additional Run accounting. Persistence and retention depend on the configured telemetry exporter; they do not survive restart through the invocation journal.

Title generation is nested under `run segment maintenance`, with `run.id`, `gen_ai.conversation.id`, `maintenance.operation`, and `run.parked` identifying its boundary. A parked Run can generate its initial Session title while waiting for user input; this span does not mean the Run has completed. Workspace checkpoints use the same span name and run only at a terminal boundary.

## Background shell lifetime

Background commands remain addressable after they exit until `read_shell_output` consumes their final output. That final read reports completion and releases the shell handle and retained buffer; later reads report that the shell is absent. Compaction reminders preserve these retained handles, including commands that have finished with unread output. Reads while a command is running keep its handle available. Stopping a command preserves its unread output for the final read. Session teardown and Runtime shutdown also reclaim owned commands.

## Knowledge document paths

Knowledge reads and accepted writes return the Runtime-resolved absolute `path` alongside scope, content, and revision. Desktop displays that path and preserves it when projecting a saved document into its query cache. Clients must not infer user storage or project roots from a scope label. Writes still target scope and workspace with revision checks; the returned path is descriptive, not a file-write argument.

## Scope execution settlement

Scope alone propagates root, subtree, and owner cancellation to execution contexts. Runtime submits cancellation intent and projects Scope’s immutable termination; a late cancellation or owner deadline cannot replace an already settled failure. Provider diagnostics enrich only the model-failure stop acknowledged by Scope.

Model allowance admission runs during request preparation, before a provider call, and its serialized turn belongs to the entire Effect attempt. A rejected admission therefore has a definite failed settlement and retains the Run's `maxSteps` or `maxBudget` outcome. The turn is released even if later request preparation fails.

Canceling a delegated Run still cancels that child. If its in-flight model attempt returns no definite result, Scope retains the unknown Effect and fails the parent instead of delivering a normal delegate result. Runtime projects that parent outcome as `lost`, preserving the unresolved-effect diagnostic; it does not manufacture a settlement or retry the attempt. This intentionally replaces the earlier behavior that allowed the parent to continue despite the child's unresolved Effect. Cancellation before external dispatch and terminal children with definite results remain distinct cases.

Every known Tool and Delegate result, including rejected calls, uses Scope's `ResultCommitter` boundary. Only the Interaction dispatcher binds the session committer. Scope retains a complete model round in call order and publishes it through a separate Effect before model continuation or direct completion. Runtime atomically stores the exact model-visible results, product Items, invocation journal, and a receipt binding the Effect ID to its content digest. Repeating the same publication is idempotent; changed content conflicts, and a replaced Segment cannot use an old receipt. Each callback first verifies the durable receipt through the ordered Segment event stream, before accessing pending product metadata. An existing exact receipt completes a repeated callback even after the metadata is retired or the projection host is replaced. An ambiguous COMMIT response is reconciled against that same receipt without replaying tools or conversation writes.

Plain executor errors, lost responses, cancellation, and host failures do not prove a business outcome. They remain unknown and cannot produce normal model feedback. A concrete executor may return an explicit Scope Tool failure with its known output, including partial success; validation and authorization owners may reject calls they have not executed. Runtime's generic Tool observer never converts arbitrary errors into known failures. Product cancellation closes abandoned Items without inventing Tool results.

A direct Tool completion ends the Run after its ordered results commit. It adds no assistant answer and makes no further model call; a delegated direct completion supplies those same results to its parent.

Tool concurrency declarations describe the effective executable boundary. Wrappers that can change arguments through hooks, authorization, or approval continuation declare exclusive execution, because an inner resource key computed from the original arguments is no longer safe. Immutable invocation paths preserve the inner declaration; Scope remains the scheduler.

Scope owns the model-visible payload. Runtime derives presentation from that payload and never reconstructs it from UI text or independently formats Delegate diagnostics. Pending product metadata, including effective arguments, mutation paths, and offload references, accompanies the Scope checkpoint until its complete model round is published. Delegate admission retains its invocation metadata independently of the currently active child batch, so an earlier Delegate result survives when a later batch waits for input. Resuming that tree preserves the completed work.

Rejected calls retain their original input as `argumentsText`; parsed `arguments` remains an empty object. Transcript storage, history artifacts, CLI, and Desktop preserve this distinction, including malformed JSON. Existing records need no schema migration; previously omitted results cannot be reconstructed from the database alone. Run aborts persist the first causal diagnostic instead of replacing it with a generic internal error.

The Interaction snapshot shape changes with this contract. Waiting checkpoints from an older build must be completed before upgrade or discarded under the existing build-identity rule; no dual-schema reader is provided. Runtime still uses an in-memory Scope engine and restores only committed waiting checkpoints. It has no mid-run crash recovery: a restart with unfinished execution retains the existing `RunLost` policy. Durable Scope Tree hosts must additionally fence publication by Tree incarnation; Runtime rejects such a binding until a TreeDurability integration supplies that guarantee.

Waiting checkpoints declare the offloaded result IDs required by their continuation. SQLite saves these references atomically with the checkpoint, verifies Session ownership, and releases them when the checkpoint is replaced or consumed. Startup and failed-write cleanup delete only bodies held by neither a checkpoint nor an Item. Restoring an executor verifies that the declared references exactly match its pending result metadata; old checkpoints missing this ownership information are rejected under the existing recovery policy, without a compatibility reader.

Unknown-effect observations retain the Effect IDs and the first available local failure diagnostic. The RunLost record preserves these details while keeping the outcome unknown; diagnostic text is never evidence that an external operation succeeded or failed.
