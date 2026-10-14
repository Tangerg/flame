# Go backend binding

The public package `github.com/Tangerg/flame/runtime/plugin` supplies the typed MCP
method binding used by Memory, Schedules and the independent Reviews example.
It lives in the existing Runtime module and imports no Runtime internal package.
Use the ordinary MCP SDK server and transport; this package does not create a
second process supervisor or lifecycle.

`NewMethod[P, R]` is the authoring contract for typed feature methods. Its owner
declares semantic identity, query/command kind, target meaning and description
once. Scope derives immutable input/output schemas from the existing Go types.
The method binds both client and server using those schemas; callers cannot
relabel commands as queries. Server admission retains and validates original input before business execution,
so typed decoding cannot hide missing fields. Invalid output remains a protocol failure,
preserving uncertainty about an already committed effect.

`github.com/Tangerg/flame/runtime/capability` owns the immutable callable
contracts independently of transport. `capability.AdmitMethod` admits local
method metadata; typed SDK methods, live bindings, callbacks and generation
consume that owner. It provides the exact schemas, validation and canonical
identity. Descriptors are deep-copy discovery/build projections.
Existing Go authors must move method descriptors, kinds, composition metadata
and callable contracts from `runtime/plugin` to `runtime/capability`. The old
SDK definitions and constructors are removed without aliases. Rebuild affected
Go consumers; the backend wire protocol, canonical JSON and fingerprints are
unchanged by this source ownership change.
`capability.New` groups the exact required method descriptors
under the consumer's semantic reference, composition rule and scope meaning.
Pipe its capability descriptor JSON into the existing Runtime `contractgen`
with `--feature - --out <contracts-directory> --ts <typescript-directory>`.
It emits canonical `capability.generated.json`, standard self-contained Schema,
operation references, reference prose and TypeScript input/output maps with a
derived `CAPABILITY` token from that same source. The canonical descriptor keeps
the original self-contained schemas. The presentation Schema rewrites local
references for its data maps and never supplies the callable fingerprint. The
generator no longer accepts a bare method array. It reuses Scope schema
derivation and Runtime's Schema-to-TypeScript emitter; no new IDL or Go DTO copy
is introduced. Numeric/cross-field constraints remain in Schema and the business
owner where TypeScript cannot express them. Output filenames are fixed; method
identities never become filesystem paths.

`NewBackend` assembles the method exports against one ordinary MCP server.
Register typed methods against that backend, then `Publish` once before connecting
the server. Missing or duplicate exports and later registration are refused.
`Describe` returns immutable method/schema projections through the current public
backend protocol. Semantic operation names are entries in that description;
they are no longer separate MCP methods.

Installed entry admission captures one immutable set of named dependencies,
waits for required providers' existing owned attempts, and verifies the complete
live export set. It then calls `SetUp` before the connection owner publishes the
session or Tools. Register `Backend.OnSetup` before `Publish` when dependencies
are declared. The handler receives `Setup`, whose `Contributions` method returns
detached contribution/dependency/binding projections, including explicit optional
absence. Return an `io.Closer` for acquired resources. The supplied context spans
the setup/session lifetime; cancellation and cleanup are joined on failure,
retirement or `Backend.Close`. Do not perform irreversible business operations to
register an entry.

Setup callback metadata supplies a canonical Runtime RPC reference for declared
collaborators. `runtime.NewPluginCallbackCaller` and `runtime.BindPluginMethod`
create typed installed dependency calls from the supplied exact bindings. Invoked
operations use their calling-owner callback; background work uses the captured
activation reference. An invoked operation missing its call callback is refused.
Fresh installed queries/commands carry call-scoped canonical callback metadata;
receipt lookup retains its original route without acquiring new collaborators.
Managed nested calls are checked against the session's immutable bindings, not a
later selection. Actual Tool/automatic/UI consumer migration, complete point
policies and durable execution projections remain open.

`runtime.BindPluginStreamingMethod` admits a typed mutation stream from the same
immutable binding used by unary calls. Its Stream freezes input/options before lazy
delivery and returns one consumable sequence of SDK StreamValue chunks/final result.
Chunk/output schemas and strict decoding precede delivery; final publication follows
source completion. Missing/invalid final evidence stays unconfirmed. Transport-owned
refusal/uncertainty classification is preserved instead of reconstructed here.
Runtime, Client and PluginCallbackCaller implement the same PluginStreamer port.
The callback caller captures the original reference/envelope, opens its Client only
on consumption and closes it when the sequence ends or stops. Invoked work missing
its calling-owner callback cannot fall back to activation. These Go mechanisms do
not yet complete query streams, TS author parity or production model/dependency
composition. Core/backend/storage identities remain unchanged by this Go API addition.

`BindCapability(export, contract)` associates a named export with those existing
handlers and checks its complete required set before publication. Several exports
may share a handler; there is still one semantic method and business receipt
namespace. Export identity describes routing, not another mutable business fact.
`capability.Parse` admits generated local metadata without executing code or fetching
schemas. `ResolveCapability` validates a concrete export and all its methods against
the consumer's exact contract. Missing, duplicate or mismatched exports are unavailable.
`Method.Bind` returns a typed `BoundMethod` over that concrete binding. It uses the
same structural admission, invocation and outcome decoding as ordinary methods.

Each bound invocation checks export name, capability fingerprint, membership and
method fingerprint before business execution. A capability member cannot omit its
export and fall back to a bare call. Unassociated method primitives remain useful
for ordinary backend utilities and host callbacks. `NewCapabilityBinding` restores
the original export and complete contract without rediscovery; its methods also
retain that identity during read-only receipt lookup. Lookup checks membership
before entering the existing receipt owner and validates the original output schema.
It cannot switch exports or invoke the mutation. The host must retain installation,
release and contribution bindings through its existing command routing owner;
a direct backend session does not supply those installed guarantees.

Composition and scope here describe the callable contract. This binding does not
yet resolve collection membership, provider selections or remote decorator chains.
Full point applicability, optionality and ordering still belong to the forthcoming
point/composition consumer rather than being inferred from an export name.

`RegisterServer[P, R]` binds a query and `RegisterCommand[P, R]` binds a command
against the same backend. `NewMethod.RegisterServer` preserves its declared kind,
target and description. `RegisterClient` installs the one invocation/discovery
transport, independent of the business schemas. `Call[P, R]` and
`CallCommand[P, R]` decode concrete types through that transport. Missing input,
unknown envelope/input/result members and malformed command
identities are refused. A reply contains exactly one value or business refusal;
an explicitly nullable value remains distinct from a missing reply.

`Resolve` discovers a method for a new call and returns a `Binding` against that
concrete session. `NewBinding` restores an already captured descriptor without
discovering another implementation. The binding validates raw input, reply and
original receipt before typed decoding. Its immutable `Contract` includes the
semantic name, query/command kind, target meaning and exact input/output schemas;
description prose is excluded. Every invocation carries that contract's derived
fingerprint. A backend refuses a different contract before calling the feature.
This exact binding is independent of private build revisions and does not make
two methods with equal shapes interchangeable.

A method's classifier returns a nonempty code only for a known business refusal.
Returning an empty code leaves the error as an MCP protocol failure. Storage,
commit, transport and cancellation errors must not be relabeled as definite
business refusals. Callers must not automatically resend a possibly committed
mutation. A feature may instead return its own typed outcome union, including
conflict evidence, as Reviews does.

The backend owns its business database and mutation receipt. The SDK neither
commits nor caches a competing result. The same logical identity and input must
return the owner's original outcome according to that operation's contract;
different input with the same identity must be refused. Retention, capacity and
transaction boundaries belong to the business owner.

`github.com/Tangerg/flame/runtime/jsoninput` owns the immutable admitted JSON.
Its standard-library parser normalizes object ordering, whitespace and string
spelling while preserving numeric lexemes and array order. Structs, maps and raw
objects agree when they encode the same request. A raw `1.0` remains distinct
from `1`, and integers beyond JavaScript's exact range are not rounded.
The former DTO-hashing `InputDigest` function is removed.

`Command.RequestInput` and `RequestInput(ctx)` expose the input received at the
typed method or Tool boundary. Private receipt owners save this value and derive
its digest; they never recreate it from a decoded DTO. An unbound context or
manually constructed command cannot supply received evidence. Runtime's pending
command store also retains the original JSON and uses it for dispatch and lookup.
It also retains the original complete method contract. Network discovery happens
outside installation admission; the concrete provider is rechecked before
saving the route. Recovery uses the retained contract rather than current method
metadata. The former route schema without that contract is refused without
conversion; retain the old directory and use a fresh Runtime data directory.

`Reply` encodes a success or explicitly classified refusal;
`DecodeReply` validates the same outcome when it is read from private storage.
An empty acknowledgement and other zero values remain present successes.

`ReceiptMethod`, `ReceiptRequest` and `Receipt` define a read-only lookup by
original command identity, method and input digest. `ReadReceipt` never dispatches
the mutation. Missing, malformed or unreachable evidence remains unconfirmed;
only a valid original response supplies a known outcome. A private backend must
implement this lookup against its own receipt store and commit known refusals as
well as successes. Unknown storage/commit failures do not become stored refusals.

`RegisterTools` reuses Scope's frozen Tool schemas and execution. `InvocationID`
reads Runtime's logical Tool call identity from MCP metadata, never from model
arguments or transport IDs. `ToolCommandID` qualifies that identity for receipt
owners that separate Tool and human command namespaces. These identities are not
credentials or standing approval grants.

The current SDK requires `capability.Protocol` on invocation and description. It is
a public wire contract identity, separate from internal build or data-directory
versions. Only this current protocol is interpreted. Installed declaration/token
matching remains part of the continuing refactor.

The [Reviews backend](../../examples/plugins/reviews/backend/server.go) binds Tool
and typed method entrances to the same private owner. Its tests prove shared
receipts, business conflicts, identity conflicts and uncertain storage failures.
The current repository workspace builds the evolving SDK source. Independent
published consumers need a Runtime version containing this package; the older
version still named by application module metadata does not contain it.

This package supplies backend wire and setup primitives. The public Runtime
`CallPlugin` entrance and typed `BindPluginMethod` proxy retain exact installed
routing and reuse the existing receipt owner. Canonical Core callbacks use the
ordinary typed Runtime Client and Delivery Endpoint. A direct backend session
does not supply those installed guarantees. Full point values/applicability/order,
actual all-consumer migration, component bindings and decorators remain unfinished
parts of the unified-plugin refactor.

## Unary host callbacks

`Callbacks` owns one lazily opened loopback endpoint for a headless Runtime
instance. Its owner admits each concrete MCP connection generation and revokes
that generation before closing its transport. `Open` binds an owning call context,
explicit request/response capacities and immutable `Callback` operations. It
returns an ephemeral `CallbackRef` and an idempotent release function. The same
endpoint serves concurrent calls; activation credentials and call references
cannot be exchanged between them.

`NewCallback` binds an existing typed `Method` to its actual use-case handler.
`CallHost` uses that method's exact structural contract and the same value/refusal
envelope as backend invocation. Input admission, output validation, original JSON
and query/command identity rules share the existing implementation. The endpoint
does not discover undeclared operations or assign a model-call budget.

Handlers receive the owning context's target, origin and parent projections.
Network input cannot replace them. No callback-owner lock is held across user
code or nested I/O. Call release, owner cancellation, connection retirement and
Runtime closure withdraw new admission, cancel active requests and join them.
Retirement cancels every sibling before waiting for any one sibling. Released
references return `ErrStaleCall`; unknown transport or handler failures never
become a business refusal or authorize automatic retry.

The reference contains live connection credentials. Do not log it, put it in a
package or expose its loopback address to browser components. Call `Close` from
the Runtime's teardown owner, after operation admission has stopped; handlers
must not synchronously join their own callback lifetime.

`CallbackMetadata` validates and projects a reference into MCP request metadata.
`Binding.InvokeWithMetadata` leaves this ephemeral collaborator outside admitted
business input and its identity. The receiving method validates it before business
execution and exposes it through `CallbackFromContext`. Changing the reference
for a later observation does not change the original command input or receipt.
This projection supplies no durable authority of its own; the enclosing callback
owner and actual managed connection retain call and installation authority.

Memory search and maintenance now use this public unary transport. Memory owns
its two-call policy and captured model collaborators. Real-process tests prove
MCP → callback → nested MCP reentrancy without a Desktop relay. This slice does
not yet prove full installed activation/dependency acceptance or remote decorator
continuations. Canonical Runtime RPC/SSE callbacks now share the ordinary
Endpoint/Client, and Schedules uses them for durable Run acceptance. Complete
installed dependency binding and streaming decorators remain
required parts of the complete plugin refactor.

## Model preparation host collaborators

Installed model selection, context and input handlers receive a call-scoped
`CallbackFromContext` reference through the existing managed connection. It enters
canonical Runtime RPC and the host's independently selected model callbacks while
the outer query is pending. The host retains the original invocation and supplies
the receiver activation's exact Actor and declared dependency bindings; the
reference expires after either success or failure.

Runtime authorizes its selected model pipeline separately from a plugin's declared
cross-plugin requirements. That does not grant arbitrary calls to undeclared plugin
operations. A nested model exchange that reenters a contributor still preparing its
ancestor is explicitly refused before recursively invoking that contribution;
Runtime neither bypasses the contributor nor retargets the frozen pipeline.
Preparation inside an active execution carries its original member's accounting
publisher, even before primary provider dispatch. Per-call request/output budgets
remain supplied by the operation owner. Full native composition, observer host RPC
cohosting and background effect restoration remain separate acceptance work.

## Background activation and retained caller projection

A contribution's background declaration supplies SetupContribution.Background with
its own immutable installation target. An entry declaring background work must
register OnSetup. Return owned resources as io.Closer; Backend.Close cancels and
joins setup lifetimes before private stores close. Setup establishes resources and
checks availability; it does not advance business facts merely to register them.

An activation callback keeps its authenticated Actor and declared dependency bindings.
For a retained background request, use WithInvocation on the public callback client's
context. The host verifies the original dispatched route for human/model callers and
admits fresh automatic triggers only for that installation without an invented parent.
Origin, parent and target are ephemeral transport context, not business input or
credentials. Closing the callback observation leaves independently accepted Runs with
their canonical Runtime owner. Rebuild against backend protocol 2026-10-14.2.

Known business refusals use capability.Refusal. The former plugin.Failure type is
removed; migrate external Go classifiers and errors.AsType calls to the pure
contract package. JSON refusal fields stay code/message. SetupDependency.Bindings
now uses SetupBinding, a setup transport projection validated by the existing
identity owners; the connection adapter no longer imports Core Protocol DTOs.

A background setup handler returns ActivationResource. Setup provisions resources;
Activate starts owned business work once per session only after successful setup.
The connection supervisor joins resources after failed setup/start or withdrawal.
A lost activation acknowledgement cannot issue Start twice in the same lifetime.
Managed background owners can call NotifyPluginChange for their own declared
contribution. It publishes plugins.dataChanged through the existing invalidation
owner; no new event bus or plugin business store is created.

## Captured model calls

ModelMethod publishes flame.model/call.v1 using Scope chat.Request and the complete
Scope chat.Response, under the existing authenticated callback lifetime. CallModel
takes an explicit caller-owned call ID, validates and snapshots the request before
transport, and validates detached response evidence. NewModelCallback adapts an
admitted call owner's behavior to this contract. The caller owns exact model
selection, admission, budgets and identity fencing; the SDK does not resolve a
different provider or establish another replay/result store.

An explicit admission Refusal without response is returned as known refusal.
If a valid response is already observed, the result preserves it alongside failure.
Memory is a production consumer of this contract; its original private budget and
receipt owner remain outside the SDK, and prompt-to-text policy stays with Memory.

ModelResult retains a valid known inner Response alongside ModelFailure when an
outer operation fails. Failure alone is also valid; an empty outcome is refused.
Usage, reasoning, actual model metadata and absent versus explicit zero accounting
remain Scope-owned fields. Cancellation/deadline categories retain errors.Is
semantics across the callback; other failures remain explicit. CallModel performs
one delivery with no automatic provider retry. A lost acknowledgement is not proof
that no model work occurred; the owning invocation must preserve that uncertainty.

The external-package subprocess regression consumes only public SDK/Scope APIs and
the existing callback endpoint, without a Desktop relay. This proves the unary
contract and callback path, not installation-wide model composition or streaming.
Memory's narrower completion/embedding grant and production Interaction/auxiliary
entrypoints remain actual consumers awaiting migration through the complete model
execution/streaming owner. Production stream composition, installed remote next,
frozen contribution order and durable inner evidence remain explicit work. No unary
descriptor advertises streaming before that path exists.

## Captured streams

NewStreamingMethod describes one input, a stream chunk schema and a distinct final
output schema. Stream schema is part of immutable method identity; feature generation
projects MethodChunks/Go chunk types and documents the final result. NewStreamingCallback
and StreamHost use the same authenticated callback endpoint, raw input admission,
captured context and cancellation owner as unary calls. SSE framing uses the existing
released SSE reader/writer; there is no new endpoint, event queue or JSON parser.

StreamModel and NewModelStreamCallback carry Scope response deltas and ModelResult.
Provider work starts only on iteration, and request evidence is frozen before it.
Scope ResponseAccumulator owns the aggregate. A final result is published only after
the source ends; an error after a tentative result cannot become success. Stopping
iteration cancels transport and joins through the existing callback lifetime. Writes
are synchronous with production, so a failed/stalled transport stops advancing the
source rather than accumulating an unbounded event queue. No reconnection or provider
retry occurs after partial output; missing/truncated terminal evidence remains unknown.

Callback MaxResponseBytes bounds every frame and final result. The model aggregate
also uses it as a conservative cumulative encoded-delta byte budget, including
metadata, before admitting deltas into Scope's accumulator. This budget can be stricter
than the final serialized response; it is explicit buffering policy, not an arbitrary
chunk-count limit. Completed inner response/usage survives a subsequent outer failure.
Partially admitted chunks remain observable even when no complete response exists.

An external executable consumes a callback capturing a frozen remaining Scope
stream chain and verifies its order, transformed input and single provider invocation.
This proves the model callback stream and remaining-chain substrate; actual installed
decorator selection/retention, TS author parity and
Interaction/auxiliary/Memory production migration remain open. Ordinary unary binding
and installed unary admission refuse stream methods before delivery/route persistence.
Only implemented streaming entrances advertise stream support.

## Backend streams

StreamingMethod.RegisterServer binds a real SDK backend stream under the same
method/capability publication checks. StreamingMethod.StreamWithMetadata and
Binding.StreamWithMetadata keep one original MCP request for input/final result.
The backend sends ordered chunks synchronously to a captured Runtime callback,
whose acknowledgement follows actual consumer demand. That existing second
direction supplies backpressure without another listener, standard-pipe reverse
request assumptions, notification queue or business result store.

The receiving reference belongs to the MCP session and call; original callback
metadata remains available for a decorator's model next. Concurrent/out-of-order
chunk delivery is refused, bytes/schema are checked before consumption and final
schema/capacity checks precede completion. Early stop cancels and joins the control
request and receiver. A failed stream retains observed chunks and an unconfirmed
final outcome; it is delivered once without retry. Typed stream input is captured
before lazy delivery. Metadata is also captured as original JSON before iteration;
per-key raw values avoid narrowing numeric lexemes through float64. A caller's
replacement callback/parent metadata cannot retarget the retained stream. Unary
raw/typed invocation still refuses stream contracts.

The external-package stdio subprocess publishes an exact decoration export, calls
captured model next within its original request, transforms input/outer chunks and
preserves known inner response/usage with outer failure. This proves real bidirectional
SDK transport. Runtime now admits installation-qualified mutation streams and
retains their original Commands route. Public Runtime/Client.StreamPlugin and
plugins.stream enter that owner through the same delivery endpoint. Chunks have
no replay cursor; retry reads only the original private receipt. Explicit owner
refusal is final evidence, while missing terminal delivery remains unconfirmed.
The empty opening response conveys no business result. Ordinary plugins.call and
component bindings remain unary; query/TS streams and production model/
dependency composition still require their owning migration. Core is
2026-10-14.21; backend protocol/private formats are unchanged.

## Primary Interaction context composition

`ModelContextContract` publishes the ordered `flame.model-context.v1` context
contribution point. An installed backend registers `ModelContextMethod`, binds
its declared capability export and returns `ModelContextResult`. The query
receives the captured provider/model and the complete current Scope request;
its invocation metadata identifies the original model call and parent effect.
It has no command identity, execution continuation or model-routing authority.

Only valid system messages may be contributed. Later contributors see earlier
material in the explicit captured order. Runtime validates exact installation,
release, contribution, export and contract before each invocation. A failed,
withdrawn or changed contributor refuses preparation before provider dispatch.

Primary Interaction captures this order with its execution configuration and
checkpoint and retains its installation dependencies. Contributions enter the
existing capacity/compaction input as fixed instructions; they reach the provider
through the same one-call preparation record as the current Session tail.
Neither enters Scope's retained WorkingContext or authored durable conversation.
The physical model journal records the complete request actually dispatched.
Waiting recovery uses captured bindings, without selecting a replacement chain.
The checkpoint now requires `model_contexts`, including an explicit empty array;
former payloads are refused rather than interpreted as an empty composition.

Auxiliary text use cases capture the same context contract and observer order for
their complete lifetime. The host captures the actual role selection and caller's
input-text byte/output-token budget before building the request; each final request
includes contributed context in that byte check. Memory extraction/curation share
one capture and retain their original per-call identities. A failure or over-budget
composition refuses before the physical endpoint, and release cancels/joins active
calls before dropping either context or observer dependencies.

Public model selection now runs before fresh Run and auxiliary role binding, as
described below. Auxiliary capacity now checks the fully composed request against
its captured selected-model limits, using that provider's counter when available.
General request transformations remain unfinished semantic boundaries. Transparent model observers keep
their existing Scope Call/Stream contract and cannot acquire those authorities.

## Model selection before exact binding

`ModelSelectionContract` publishes the `flame.model-selection.v1` selection role.
The ordinary provider-selection API chooses one installed contribution explicitly.
A backend registers `ModelSelectionMethod` and returns the complete provider,
model and optional model-owned reasoning effort. The query carries the original
calling origin/parent and a concrete call target, with no command identity or
model-next grant. Absence of an explicit role preserves the caller's proposal;
a selected-but-withdrawn, unavailable or invalid publisher refuses the use case.
Runtime never discovers an implicit winner or silently falls back from a refusal.

A fresh Run offers its composed initial Scope request before provider construction
and final media/output-capacity admission. The selected identity is validated,
passed to the executor, and frozen in the original Run opening. The Session keeps
its separately owned user model preference. Command fingerprints retain the
original user's input; accepted Start retries and waiting Resume read the original
Run/checkpoint rather than rerunning selection. Descendant execution retains the
accepted execution selection under the existing tree owner.

Auxiliary work selects before resolving the provider instance. Its request is
absent at capture because Memory can author two prompts later within one use case.
`ModelSelectionRequest.request` is therefore optional, not an invented empty
conversation. The selected identity travels with its actual resolved model into
context composition, observation and pricing. Role changes affect new captures.
No selector binding is retained as a later execution dependency after the query
has returned; its result is the immutable selection owned by the accepted use case.

Selection is presently per accepted execution/auxiliary capture. General input
transforms and multiple independently routed physical calls still require their
own admitted boundaries and final acceptance evidence.

Auxiliary captures freeze the selected catalog token envelope and optional input
counter with the same model construction. After applying captured reasoning effort
and composing context, Runtime reserves the requested output under the Domain's
published independent input/output/context facts. It counts the complete request
with the selected provider when available; counter failure, cancellation or a
non-positive count refuses dispatch. Without a provider counter, auxiliary and
primary compaction share the existing complete-request local estimate, including
Scope framing, options and metadata. This estimate is a preflight approximation,
not proof of provider-exact tokenization. A catalog miss keeps capacity unknown;
existing caller text-byte/output budgets still apply. Decorators preserve frozen
limits, selection and counter, and capture release cancels/joins counting before
releasing its dependencies. The counted composed request is the one dispatched.

## Model input transformation

`ModelInputContract` declares ordered `flame.model-input.v1` input transformers.
An installed backend registers `ModelInputMethod`, receiving the captured complete
selection and a Scope request, and returns a transformed Scope request. This query
has the original call target/origin/parent and no command identity or model-next
reference. Output must preserve model/reasoning, Tool definitions/choice and the
original presence of an output reservation; it may lower an existing output limit.
Tools are compared using canonical JSON with numeric lexemes retained. Message and
other request-option changes require final admission by the owning consumer.

Auxiliary captures now freeze context, input-transform and observer order together.
Context is composed first, then each transformer sees the prior transform's request.
The final text/output and selected-model token capacity checks run before observers
or the physical endpoint. Invalid output, failure, withdrawal or exceeded bounds
refuse generation; no fallback/retry or model routing is implied. Caller conversation
is independently retained and never adopts transformed messages. The same use-case
lifetime cancels/joins requests and retains all participating installation releases.

Primary Interaction now captures the same transform order in its execution
configuration and required `model_inputs` checkpoint field. Restore validates
original bindings rather than selecting current ones. Canonical compaction runs
first; context and transformed complete requests remain ephemeral. The existing
single-call preparation record now carries the complete final request, including
Options, and Call/Stream consume that exact request. Final model media/token/output
admission and accounting calibration use the transformed input. An over-capacity
transform refuses rather than rerunning plugin I/O or silently compacting its output.
Former checkpoints without the captured input field are refused.

Start, Resume and Steer admit structurally valid authored input and preserve its
original bytes without deciding the selected model's media support. That support
is checked on the final complete request after canonical reduction and input
transformation, for root and delegated Call/Stream. A converter can turn unsupported
source media into supported text without replacing the original transcript. An
unconverted unsupported request fails the accepted execution before provider I/O;
there is no automatic replay. Structural MIME/empty-input errors still refuse the
command before its state transition. Core .29 publishes this failure-timing change.
Canonical media the selected model cannot accept is never passed to its provider
counter; compaction uses its local projection and final transformation/admission
remains authoritative. Full native/multiple-call acceptance is still open.

## Independent selected model calls

`SelectedModelMethod` and `SelectedModelStreamMethod` publish
`flame.model/selected-call.v1` and `flame.model/selected-stream.v1`. They use
`ModelCallRequest`: an exact `ModelSelection`, complete Scope `chat.Request`, and
`ModelCallBudget` with positive `maxRequestBytes` and `maxOutputTokens`. The output
reservation must be present and fit the budget. Serialized request size includes
messages, media, options, Tools and their schemas; the Runtime checks it again after
captured context and input conversion, before exact model capacity admission.

Use the RPC callback reference supplied to an ordinary installed invocation:

```go
ref, err := plugin.CallbackFromContext(ctx)
if err != nil { return err }
request.Options.MaxOutputTokens = new(int64(64))
input := plugin.ModelCallRequest{
    Selection: plugin.ModelSelection{Provider: "anthropic", Model: "configured-model"},
    Budget: plugin.ModelCallBudget{MaxRequestBytes: 64 << 10, MaxOutputTokens: 64},
    Request: *request.Clone(),
}
response, err := plugin.CallSelectedModel(ctx, ref, "workflow-model-call-1", input)
```

A real workflow supplies a distinct stable identity for each intended exchange,
retains its own total-work budget, and handles failure/uncertainty through its existing
owner. Another request can propose another route with another identity; there is no
SDK-wide two-call limit. `StreamSelectedModel` freezes the request before iteration,
preserves canonical deltas and `ModelResult`, and joins the provider when consumption
stops. Never retry a lost exchange automatically or reuse its ID for changed input.

`NewSelectedModelCallback` and `NewSelectedModelStreamCallback` bind an owner to these
contracts. `Callbacks.PublishRPC` publishes them beside canonical RPC on the existing
endpoint; per-reference transport limits, captured invocation and activation lifetime
still apply. The existing physical endpoint journal fences production dispatch and
retains full evidence. The SDK does not invent a model receipt store or fake Run.

The reference for an observer's `next` is a captured continuation with its original
capability. It does not grant selected-model routing. The transparent observer's
once-through contract remains specific to observation; it is not a universal rule
for composition. General durable composition still requires Scope-owned effects or
an existing durable use case. New selected capabilities are additive exact methods;
older hosts without that grant explicitly refuse them. No legacy method is relabeled.

`PublishRPC` now has the variadic signature `func(http.Handler, ...Callback) error`.
In-repository publishers are migrated. External method-value assignments requiring
the former exact Go function type must change and rebuild; there is no old-signature
wrapper. This source break does not alter the canonical RPC wire contract.

CallHost and StreamHost send the original context invocation in the existing
Flame-Invocation header. An activation-scoped background RPC grant validates it
against the retained calling-owner evidence before projecting the context. Forged
origin/parent/target is refused. Ordinary call and next grants keep their captured
owner and ignore this header; authenticated actor identity is never replaced.

Independent selected exchanges require an ID distinct from their original calling
operation and host-owned Model/Tool boundary. An existing logical Model/Tool ID
cannot be repurposed as a fresh physical caller. The host captures execution ownership;
wire invocation headers and parent fields cannot select another Run or Segment.
Endpoint observations expose the original optional Segment in Core .30.
