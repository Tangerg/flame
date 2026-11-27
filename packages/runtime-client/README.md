# TypeScript Runtime client

`@flame/runtime-client` owns HTTP/SSE transport, typed methods, pagination,
mutation identity, uncertain-response reconciliation and persistent command
journals. Runtime's registry and protocol own operation shapes and semantics;
`make generate` emits the package's `src/generated` projection.

```ts
import { createFlameClient, createHttpTransport } from "@flame/runtime-client";
const client = createFlameClient(createHttpTransport({ baseUrl: "http://127.0.0.1:17171" }));
const discovery = await client.runtime.discover();
```

A successful streaming call requires a 200 SSE response with exactly one valid
acknowledgement followed by notifications. The request state correlates both
document and stream responses with the original call. A second acknowledgement
ends the observation with a protocol error. For a Run, the acknowledgement
also binds the observation to its root Segment. A transport
reports the end of a stream once through `streamEnd`. The request state
determines whether that end rejects the pending response or settles the active
stream. A 200 JSON document may carry a streaming call's refusal.
Every notification must use the method bound to the original observation.
SSE frames use the default message name and carry a JSON-RPC envelope.
Replayable Run notifications carry an SSE id matching their admitted JSON
event ID. A conflicting or missing frame ID ends the observation before it can
advance the replay position.
Both RPC and sidecar clients bound ordinary response documents with
`maximumResponseBytes` (128 MiB by default), including error bodies without a length.
HTTP transport captures its endpoint, credentials and resource limits on creation;
later edits to the supplied configuration cannot change an admitted connection.
HTTP errors use the response's `Request-Id` for correlation. A matching
problem document can supply diagnostic detail.
Every received message completes transport consumption, including a response
rejected for a mismatched RPC identity. The original request is refused without
settling another request or requiring cancellation to release stream backpressure.
Response-body cancellation has one promise per reader. Abort, parsing failure and
transport shutdown join that original cancellation before releasing the lock.
Document failures retain their primary cause and secondary cleanup failure;
HTTP shutdown also retains cleanup failures after a caller already observed
cancellation. Repeated close returns the same settlement, with each original
cancellation operation reported once, including equal non-Error rejection values
from independent readers. Expected aborts
alone remain successful cleanup.

After capability discovery, `runtimeEvents.subscribe` can share one connection
for negotiated topics. Each subscriber's request is validated before sharing
the connection. A subscriber's abort settles its own call while the
shared stream is still awaiting acknowledgement. Canceling a subscription or
returning its iterator discards that subscriber's unread events. The last
subscriber to leave closes the shared connection. File watch subscriptions use
dedicated connections. The SDK checks the source sequence before filtering
events and ends the observation on a discontinuity. Shared subscribers retain
the Runtime's sequence numbers, including gaps between their selected topics.
A Run stream completes when its acknowledged root Run
and Segment publish `segment.finished`. An event matching just one of those
identities fails the observation before delivery. The active observer can drain
unread events through the terminal event.

Capability checks use the request's wire fields. An explicit `isolated: false`
requires isolation because it changes the Session's isolation setting. A
descendant query requires `childRuns` only when `includeDescendants` is `true`.
The SDK checks capabilities before reserving a new command. On retry, Runtime
applies its live policy to the original request, metadata and idempotency key.
JSON-RPC errors carry the problem in `error.data.problem`. For a command with
a stored response, `error.data.recordedOutcome` confirms that the failure came
from the matching durable receipt. The SDK retains a previously dispatched
command when a replay is refused without that evidence.

HTTP credentials and metadata must be ASCII field values without surrounding
whitespace. Caller-chosen command keys are 1–255 visible ASCII characters
without spaces and are retained exactly for receipt lookup. A caller-chosen key
selects its exact command, even if another pending command has identical input.
The HTTP transport refuses unrepresentable header values before opening Fetch.

Consumers use explicit public entries: the main API, `wire`, `methods`,
`validation`, `testing`, and `testing/samples/*`. Internal implementation paths
are not exports. `testing` supplies the deliberate memory-transport
fixture and sample index. Its raw sample JSON is an exact generated copy of
`runtime/contract/samples`; edit canonical samples at that Runtime owner.
Their protocol version is supplied by contract generation.

`workspace.diff.get` returns a discriminated read projection. Browser review
requests `format: "visual"` and accepts `baseline` plus `visual.patch`,
`visual.manifest`, `visual.truncated` and `visual.budget` together. Do not combine
that manifest with a later rows or changes read. Empty truncated visual results
are capacity results, not a clean workspace. Complete `raw` and bounded `rows`
remain distinct consumers' projections. Migrate former flat response fields to
the matching tagged payload; the decoder does not translate old responses.

Transport receives JSON losslessly before method validation. Schema-declared
numeric DTO fields project into JavaScript numbers and reject values they cannot
represent exactly. Opaque JSON, including Tool schemas, retains exact numbers.
The inbound envelope admits one response or notification shape and refuses
undeclared envelope members. Its schema also supplies the client-side message
type, preserving exact payload values for method validation.
For evidence such as accounting archives, `client.queryJSON(method,
params, { signal })` returns canonical JSON with the original numeric lexemes,
wide integers, null and missing members retained. Both result formats capture
request metadata once and share request validation, capability admission and
cancellation. Exact JSON accepts unary queries. Use `parseJSON`,
`stringifyJSON`, `bigint` and `ExactNumber` from `@frame/sdk/json` to inspect or return exact data.
That Frame entry owns the pure data mechanism. Its dependency closure
cannot enter Runtime calls, protocol records or validation; neutral application
input and presentation code may consume it without importing the client API.

Mutation handles retain the original key and receipt authority after uncertain
responses. An unresolved retry continues the original command. After
confirmation, retries return the observed result or refusal directly. To open a
fresh observation of an acknowledged Run, use `runs.subscribe`.
Opening a keyed mutation validates and stages the command synchronously. A
staging failure throws its original error before a mutation handle is issued.
`createMutationSettler` reports that error through its settlement promise.
The client reports local journal maintenance through
`onMutationJournalCleanupFailure`, with console diagnostics by default.
`MutationJournalCleanupError` carries the confirmed outcome and the local
storage failure. Its `retryCleanup()` retries removal of the record. The original
handle also attempts removal again when returning its confirmed result or refusal.
Presentation components
must not rebuild settlement or invent a replacement identity. Streaming reads
and durable commands retain their registry-classified distinction.

Each new Runtime command without a supplied key gets a fresh command identity.
Retry the original handle or select the pending command by its retained identity
after reconnect. For ordinary unary commands, `runtime.pendingCommands()` lists
the original method, namespace and idempotency key. Pass those values and the
original input to `runtime.retryCommand(method, identity, params)`. Admission
verifies the retained command using its captured metadata. The journal stores a
fingerprint for ordinary command inputs, so credentials and other inputs stay
with the caller. The stable-intent plugin API can recover the original command
through its authored intent.

`approval.forgetRule` accepts `{ id, expectedDecision }`. Supply the decision
when withdrawing an observed approval or denial. Omit it when deliberately
forgetting the rule at that identity regardless of its decision.

`createMutationSettler().settle(open, options)` observes the mutation returned by
`open`. Pass a fresh command factory to author a command. To recover an existing
command, pass a factory that retries its original handle or journal identity.
`onCommand(command)` supplies the original handle for callers that expose recovery.
The consumer owns any observation returned by a settled mutation and closes it
when the consumer's view retires.
Deadlines must be positive whole milliseconds within the platform timer range;
invalid deadlines are refused before opening the mutation. A deadline permits
one further observation through the same handle. If that observation cannot be
opened or also reaches its deadline, `MutationLookupError` preserves the unknown
outcome and the journal remains available for recovery.

`plugins.commands()` lists active claims and recoverable entries under their
original keys. Only recoverable entries expose `recover()` and `retire()`.
`plugins.subscribeCommands(listener)` observes claim and settlement changes,
and `plugins.commandsVersion()` supplies the current snapshot version.
Each change identifies the original key. A resolved change reports the
definitive recovery or retirement outcome. Availability still follows any
remaining journal cleanup.
`plugins.observeRecovery(key, route, listener, signal)` delivers the settled
result once to an observer of that exact key and route. The recovering caller's
output decoder runs after the original receipt is delivered. The general
subscription exposes the outcome category and refusal code.
Each entry exposes `matchesIntent(intent)`, a
read-only comparison against the original journal's salted intent fingerprint.
Use the same authored intent when associating pending work with a document or
action; equal bindings alone cannot identify that input. Direct calls without a
stable intent do not match. The predicate neither claims nor executes a command,
and it does not validate changed input. Original input, contract and route remain
validated by command capture and receipt recovery. No raw intent or authored
input is added to the persistent journal.

`plugins.call` and `plugins.stream` carry an admitted `PluginCallRoute` under
`route`, with exact feature JSON in `input`. A pending command exposes its
original route as `binding`. Command lookup returns that route and its input.

Each client owns one mutation journal and retires it when `close()` begins.
Closing also settles each outstanding response or stream and joins transport,
receive and consumer work. A failed error observer
cannot prevent another observer's terminal delivery or resource release; shutdown
preserves every observer and cleanup failure, including non-Error rejections.
Consumers may supply `mutationStorage` to retain unresolved commands across
client replacement. The `capabilities` reader supplies the sole namespace and
retention authority; there is no separate scope, clock, or journal lifecycle
option. Recovery handles and `createMutationSettler` retain their explicit
operation lifetimes. Every mutation, including one with an explicit key, must
reserve its immutable input and Runtime store namespace before
dispatch. Missing scope or failed persistence refuses the command without opening
transport. A successfully reserved command retains its original identity and
receipt lookup even while discovery is unavailable. Its dispatch stays bound to
the original store.

The journal captures exact JSON input and the request metadata before admission.
It owns the durable transition from reserved to dispatched. RPC delivery commits
that transition immediately before opening transport; a failed write prevents
sending. Cancellation removes a reservation only while it is known not to have
been dispatched. Dispatched identities survive cancellation and process loss,
including a canceled retry. Plugin preparation observes the same cancellation
boundary before reserving an intent. `RpcCancellationError` represents explicit
caller cancellation with the standard `AbortError` name and the original signal
reason as its `cause`.
A recovery handle can read an original receipt after another observer clears
the pending record. Dispatch requires the original reservation. Only the
handle that withdrew an unsent reservation can reopen it.
That one captured metadata value belongs to every original dispatch and explicit
retry, including stable-intent plugin commands. Replacing client defaults cannot
change an unresolved command. A replay refusal at the HTTP boundary does not
prove whether an earlier dispatch was accepted, so it retains the original
identity for receipt reconciliation. The current journal requires captured
metadata, the delivery phase and one tagged command. A Run record contains the
original request, a plugin record contains the admitted route, and other Runtime
commands name their protocol method. An invalid record stops recovery with a
storage error and remains available for inspection. Settle outstanding commands
with the previous client before upgrading, or preserve that client and its
journal separately for recovery and start with a fresh client journal.

`runs.recoverCommands()` observes the original receipts of pending Start, Resume,
and Steer commands. Each observation is `recorded`, `notRecorded`, or `unknown`.
Call `observation.acknowledge()` for a `recorded` observation after incorporating
its acceptance. Until then, later observations can still recover the same
command. A recorded recovery handle projects its captured receipt directly.
An unresolved retry continues to use its original admission and request.
A Steer recovery handle keeps the command identity it discovered. The journal
supplies the current delivery phase and nominated input, including nominations
recorded by another client after discovery.
A recorded Runtime receipt settles the original Steer even after another
observer has cleared the journal entry.

Run `npm run check --workspace @flame/runtime-client` from the root after
`make setup`. Independent author checks consume a real packed package, rather
than workspace-private paths. No former package name or wildcard client export
is provided.

Workspace `listFiles` returns `FileListPage`, including `complete`, `coverage` and
`nextCursor`. A non-recursive unfiltered request enumerates only that directory.
Native enumeration cursors are bound to the root, path, ignore policy and directory
revision. Consumers continue until `complete`, and offer explicit continuation when
the inspection budget is exhausted. A page is not a complete workspace catalog.
