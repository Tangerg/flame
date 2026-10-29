# Plugin author SDK

The public package `github.com/Tangerg/flame/packages/go-sdk/plugin` supplies
Flame's model roles, Tool authoring helpers, captured host collaborators and Tool
receipt contract. Memory, Schedules and the independent Reviews example combine
these product contracts with Qimu's generic backend authoring API.
It lives in the independent `packages/go-sdk` module; its production closure
contains no Runtime host or private business implementation.

This guide uses the following package names:

| Name        | Import                                               | Responsibility                                                            |
| ----------- | ---------------------------------------------------- | ------------------------------------------------------------------------- |
| `plugin`    | `github.com/Tangerg/flame/packages/go-sdk/plugin`    | Flame model, Tool and product receipt contracts                           |
| `qimumcp`   | `github.com/Tangerg/flame/qimu/mcp`                  | Typed methods, MCP backend/setup, callbacks, streams and generic receipts |
| `portable`  | `github.com/Tangerg/flame/qimu/portable`             | Immutable method, capability and value-point contracts                    |
| `extension` | `github.com/Tangerg/flame/packages/go-sdk/extension` | Flame invocation origins and call/installation scope policy               |

Use the ordinary MCP SDK server and transport. Qimu's process driver and runner
own installed process execution; the Flame author package creates no supervisor.
React components use [`component-sdk`](../../component-sdk/README.md).
TypeScript authors use [`@qimu/node`](../../../qimu/node/README.md) for generic
backend behavior and [`@flame/backend-sdk`](../../backend-sdk/README.md) for Flame
collaborators and Tool receipts. These packages do not re-export one another's APIs.

## MCP metadata

The SDK's `go.mod` selects Scope and native MCP SDK releases. Native SDK `Meta`
values are encoded JSON: write typed
values with `Meta.Set`, check its error, and decode a consumed entry directly into
its declared type. Do not round-trip metadata through `map[string]any`.

Transport method names and callback/stream metadata keys come from
`qimumcp.ProtocolJSON`, which projects compiled Qimu constants and descriptors
into `@qimu/node`. `flame-sdk backend-protocol` separately projects Flame's Tool
command prefix, Tool receipt contract, declaration policy and managed Node build
policy. Neither generator inspects another source tree or a changed checkout
after the command was built.

Scope owns detached request metadata snapshots. Qimu's invocation and callback
bindings retain strict decoding, calling-owner identity, target and callback
authority validation. Stream creation freezes native JSON bytes before observation;
each iteration adds only its own captured delivery callback. Opaque values retain
numeric lexemes, null and empty containers. Rebuild backends and Go consumers
against the pinned Go SDK and Qimu modules and regenerate their exact contracts.
The Qimu cutover requires rebuilt backends and fresh host storage.

The fixed published model observer, input, context and Tool proposal contracts are immutable
process-wide values. Their schemas are compiled once by this package; checkpoint
validation and callers share the admitted contract. `Descriptor` still returns
owned schema bytes, so one consumer cannot alter another consumer's validation.
Caller-authored and received contracts retain ordinary strict admission.

`qimumcp.NewMethod[P, R]` is the authoring contract for typed feature methods. Its owner
declares semantic identity, query/command kind, target meaning and description
once. Scope derives immutable input/output schemas from the existing Go types.
The method binds both client and server using those schemas; callers cannot
relabel commands as queries. Server admission retains and validates original input before business execution,
so typed decoding cannot hide missing fields. Invalid output remains a protocol failure,
preserving uncertainty about an already committed effect.

An implementation of an existing consumer contract uses
`qimumcp.ImplementMethod[P, R](contract, name)` or
`qimumcp.ImplementStreamingMethod[P, C, R](contract, name)`. These bindings retain the
published method's exact identity and validators. Local types supply codecs;
they do not derive another Schema or relax the consumer's constraints. Missing
methods and mismatched unary/streaming delivery are construction errors. Typed
input admission, original command evidence, output validation and receipts use
the same dispatch path as author-defined methods.

[`memory`](memory/README.md) publishes the Memory contract consumed by Runtime.
Independent implementations import that pinned contract instead of regenerating
it from their own module names. Go definition names inside Schema are opaque
contract data; structural similarity never substitutes for exact identity.

`modelprovider` publishes `flame.model-provider.v1` for an installed model
adapter. Provider profiles own authentication, endpoint and catalog facts. Open
captures one immutable client shared by ordinary chat, its actual optional
streaming and complete-request counter, or embedding execution. Handles and
original command receipts survive backend restart. Closing fences future work
and joins accepted exchanges. Reopening constructs a fresh handle from the
original confirmed open receipt; it never reads current credentials or grants
permission to repeat generation. Unknown receipts never authorize redispatch.
Runtime retains exact selection, credentials, admission, endpoint evidence,
accounting and recovery. Connection snapshots are private backend inputs, with
credential-safe formatting. Chat uses complete native Scope values. Embedding
retains Scope's complete encoded envelopes and its strict native codec, which
does not publish a derivable JSON schema; it is never reduced to vectors or
reconstructed from usage projections.

`qimumcp.Binding.ValidateInput(ctx, input)` checks the exact backend's complete structural
contract without a command identity or business dispatch. Schema cannot express
every Go decoder constraint, including integer token spelling and custom JSON
decoding. Typed decoders must therefore be deterministic, bounded, side-effect-free
functions of their input. The same decoder admits validation and execution;
validation preserves the original JSON and cannot call a business handler.
`qimumcp.ErrInvalidInput` identifies only a definite structural refusal; transport and
backend failures retain their own errors.

Runtime validates fresh unary and streaming inputs before saving a command route
or recording dispatch. A definite structural refusal leaves the identity unused.
Structurally accepted business refusals still own their original receipts.
Original-command recovery does not run new admission: it only queries the captured
owner's receipt. The compiled `portable.Protocol` requires this validation
operation. Rebuild contracts, backend and consumers together. Older backend
protocols are refused without negotiation or a fallback. Flame's package metadata
uses the separate `plugin.DeclarationProtocol`. Keep old data separately and
complete old waiting work with its original binary.

`github.com/Tangerg/flame/qimu/portable` owns immutable callable contracts
independently of transport. `portable.AdmitMethod` admits local method metadata;
typed methods, live bindings, callbacks and generation consume that owner. It
provides exact schemas, validation and canonical identity. Descriptors are
deep-copy discovery/build projections. Method, capability and portable value-point
contracts share this owner. Flame's `extension` package supplies only its product
origin and scope policy; it does not define another contract engine.
Consumers import the owners directly, without forwarding aliases. Schema
identities include the defining Go namespace, so publisher and consumer
projections must be regenerated together when ownership changes.
`portable.NewCapability` groups the exact required method descriptors
under the consumer's semantic reference, composition rule and scope meaning.
Pipe its capability descriptor JSON into
`go run github.com/Tangerg/flame/qimu/cmd/qimu contract`
with `--feature - --out <contracts-directory> --ts <typescript-directory>`.
It emits canonical `capability.generated.json`, standard self-contained Schema,
operation references, reference prose and TypeScript input/output maps with a
derived `CAPABILITY` token from that same source. The canonical descriptor keeps
the original self-contained schemas. The presentation Schema rewrites local
references for its data maps and never supplies the callable fingerprint. The
generator accepts a complete capability descriptor. `qimu/contractgen` owns its
Schema-to-TypeScript emitter and consumes Scope-derived schemas; no new IDL or Go DTO copy
is introduced. Numeric/cross-field constraints remain in Schema and the business
owner where TypeScript cannot express them. Output filenames are fixed; method
identities never become filesystem paths.

`qimumcp.NewBackend` assembles the method exports against one ordinary MCP server.
Register typed methods against that backend, then `Publish` once before connecting
the server. Missing or duplicate exports and later registration are refused.
`qimumcp.Describe` returns immutable method/schema projections through the current public
backend protocol. Semantic operation names are entries in that description;
they are no longer separate MCP methods.

Installed entry admission captures one immutable set of named dependencies,
waits for required providers' existing owned attempts, and verifies the complete
live export set. It then calls `qimumcp.SetUp` before the connection owner publishes the
session or Tools. Register `qimumcp.Backend.OnSetup` before `Publish` when dependencies
are declared. The handler receives `qimumcp.Setup`, whose `Contributions` method returns
detached contribution/dependency/binding projections, including explicit optional
absence. Return an `io.Closer` for acquired resources. The supplied context spans
the setup/session lifetime; cancellation and cleanup are joined on failure,
retirement or `qimumcp.Backend.Close`. Do not perform irreversible business operations to
register an entry.

Setup callback metadata supplies a canonical Runtime RPC reference for declared
collaborators. `runtimeclient.NewPluginCallbackCaller` and `runtimeclient.BindPluginMethod`
create typed installed dependency calls from the supplied exact bindings. Invoked
operations use their calling-owner callback; background work uses the captured
activation reference. An invoked operation missing its call callback is refused.
Fresh installed queries/commands carry call-scoped canonical callback metadata;
receipt lookup retains its original route without acquiring new collaborators.
Managed nested calls are checked against the session's immutable bindings, not a
later selection. Point resolution checks consumer-owned declaration, target and
applicability contracts; execution and pending commands retain exact activation
and transitive release dependencies.

`runtimeclient.BindPluginStreamingMethod` admits a typed query or mutation stream from the same
immutable binding used by unary calls. Its Stream freezes input/options before lazy
delivery and returns one consumable sequence of `qimumcp.StreamValue` chunks/final result.
Chunk/output schemas and strict decoding precede delivery; final publication follows
source completion. Missing/invalid final evidence stays unconfirmed. Transport-owned
refusal/uncertainty classification is preserved instead of reconstructed here.
Runtime, Client and PluginCallbackCaller implement the same PluginStreamer port.
The callback caller captures the original reference/envelope, opens its Client only
on consumption and closes it when the sequence ends or stops. Invoked work missing
its calling-owner callback cannot fall back to activation. Query streams use no
command journal or receipt; command streams retain original-route recovery.
Go and TypeScript bindings preserve this distinction.

`qimumcp.Backend.BindCapability(export, contract)` associates a named export with those existing
handlers and checks its complete required set before publication. Several exports
may share a handler; there is still one semantic method and business receipt
namespace. Export identity describes routing, not another mutable business fact.
`portable.ParseCapability` admits generated local metadata without executing code or fetching
schemas. `qimumcp.ResolveCapability` validates a concrete export and all its methods against
the consumer's exact contract. Missing, duplicate or mismatched exports are unavailable.
`qimumcp.Method.Bind` returns a typed `qimumcp.BoundMethod` over that concrete binding. It uses the
same structural admission, invocation and outcome decoding as ordinary methods.

Each bound invocation checks export name, capability fingerprint, membership and
method fingerprint before business execution. A capability member cannot omit its
export and fall back to a bare call. Unassociated method primitives remain useful
for ordinary backend utilities and host callbacks. `qimumcp.NewCapabilityBinding` restores
the original export and complete contract without rediscovery; its methods also
retain that identity during read-only receipt lookup. Lookup checks membership
before entering the existing receipt owner and validates the original output schema.
It cannot switch exports or invoke the mutation. The host must retain installation,
release and contribution bindings through its existing command routing owner;
a direct backend session does not supply those installed guarantees.

Composition and scope here describe the callable contract. The installed Runtime
consumer resolves collection membership, explicit selections and ordered chains;
the backend binding validates the resulting exact export rather than inferring
membership or precedence from an export name.

`qimumcp.NewMethod` admits the contract once. Its `RegisterServer`, `Call` and
`CallCommand` operations preserve that declared kind, target and schema.
`qimumcp.RegisterClient` installs the invocation/discovery transport without originating
business contracts. `qimumcp.ReceiptQuery` and `plugin.ToolReceiptQuery` are the generic command
and Flame Tool receipt contracts respectively; backends register handlers against those same methods.
Missing input,
unknown envelope/input/result members and malformed command
identities are refused. A reply contains exactly one value or business refusal;
an explicitly nullable value remains distinct from a missing reply.

`qimumcp.Resolve` discovers a method for a new call and returns a `qimumcp.Binding` against that
concrete session. `qimumcp.NewBinding` restores an already captured descriptor without
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

`github.com/Tangerg/flame/qimu/jsonvalue` owns the immutable admitted JSON.
Its standard-library parser normalizes object ordering, whitespace and string
spelling while preserving numeric lexemes and array order. Structs, maps and raw
objects agree when they encode the same request. A raw `1.0` remains distinct
from `1`, and integers beyond JavaScript's exact range are not rounded.
The former DTO-hashing `InputDigest` function is removed.

`qimumcp.Command.RequestInput` and `qimumcp.RequestInput(ctx)` expose the input received at the
typed method or Tool boundary. Private receipt owners save this value and derive
its digest; they never recreate it from a decoded DTO. An unbound context or
manually constructed command cannot supply received evidence. Runtime's pending
command store also retains the original JSON and uses it for dispatch and lookup.
It also retains the original complete method contract. Network discovery happens
outside installation admission; the concrete provider is rechecked before
saving the route. Recovery uses the retained contract rather than current method
metadata. The former route schema without that contract is refused without
conversion; retain the old directory and use a fresh Runtime data directory.

`qimumcp.EncodeReply` seals a success or explicitly classified refusal into an immutable
`jsonvalue.Value`; its `JSON()` projection supplies storage bytes without another encoder;
`qimumcp.DecodeReply` validates the same outcome when it is read from private storage.
An empty acknowledgement and other zero values remain present successes.

`qimumcp.ReceiptMethod`, `qimumcp.ReceiptRequest` and `qimumcp.Receipt` define a read-only lookup by
original command identity, method and input digest. `qimumcp.ReadReceipt` never dispatches
the mutation. Missing, malformed or unreachable evidence remains unconfirmed;
only a valid original response supplies a known outcome. A private backend must
implement this lookup against its own receipt store and commit known refusals as
well as successes. Unknown storage/commit failures do not become stored refusals.

`RegisterTools` reuses Scope's frozen Tool schemas and execution. `InvocationID`
reads Runtime's logical Tool call identity from MCP metadata, never from model
arguments or transport IDs. `ToolCommandID` qualifies that identity for receipt
owners that separate Tool and human command namespaces. These identities are not
credentials or standing approval grants.

`ToolReceiptMethod` is an optional read of an original model Tool result. Register
it with `qimumcp.Method.RegisterServer` and accept `qimumcp.ReceiptRequest`: its `Method` field is the original
remote Tool name, `ID` is its `ToolCommandID`, and `InputDigest` identifies the
admitted Tool JSON before business translation. Read only the matching private
receipt, then use the same Tool codec and `NewToolReceipt` to project its original
output or known failure. No second result table is needed. `ReadToolReceipt` binds
the exact public query contract and validates Scope output without narrowing its
JSON. Query errors, missing methods and absent/malformed evidence remain unknown;
a valid failed Tool receipt is distinct from a failure to read it.

Qimu requires the compiled `portable.Protocol` on invocation and description. It is
a public wire contract identity, separate from internal build or data-directory
versions. Only this current protocol is interpreted. Installed admission validates
the declared contract, complete live exports and exact activation before dispatch.

The [Reviews backend](../../../examples/plugins/reviews/README.md) binds Tool
and typed method entrances to the same private owner. Its tests prove shared
receipts, business conflicts, identity conflicts and uncertain storage failures.
The repository workspace builds the current public module sources. Independent
consumers select the pinned Go SDK and Qimu module artifacts; author projects do
not require Runtime implementation source.

Qimu supplies backend wire and setup primitives. The public Runtime
`CallPlugin` entrance and typed `BindPluginMethod` proxy retain exact installed
routing and reuse the existing receipt owner. Canonical Core callbacks use the
ordinary typed Runtime Client and Delivery Endpoint. A direct backend session
does not supply those installed guarantees. Point resolution, frozen composition
and component bindings use the installed owners rather than bypassing admission.

## Unary host callbacks

`qimumcp.Callbacks` owns one lazily opened loopback endpoint for a headless Runtime
instance. Its owner admits each concrete MCP connection generation and revokes
that generation before closing its transport. `Open` binds an owning call context,
explicit request/response capacities and immutable `qimumcp.Callback` operations. It
returns an ephemeral `qimumcp.CallbackRef` and an idempotent release function. The same
endpoint serves concurrent calls; activation credentials and call references
cannot be exchanged between them.

`qimumcp.NewCallback` binds an existing typed `qimumcp.Method` to its actual use-case handler.
`qimumcp.CallHost` uses that method's exact structural contract and the same value/refusal
envelope as backend invocation. Input admission, output validation, original JSON
and query/command identity rules share the existing implementation. The endpoint
does not discover undeclared operations or assign a model-call budget.

Handlers receive the owning context's target, origin and parent projections.
Network input cannot replace them. No callback-owner lock is held across user
code or nested I/O. Call release, owner cancellation, connection retirement and
Runtime closure withdraw new admission, cancel active requests and join them.
Retirement cancels every sibling before waiting for any one sibling. Released
references return `qimumcp.ErrStaleCall`; unknown transport or handler failures never
become a business refusal or authorize automatic retry.

The reference contains live connection credentials. Do not log it, put it in a
package or expose its loopback address to browser components. Call `Close` from
the Runtime's teardown owner, after operation admission has stopped; handlers
must not synchronously join their own callback lifetime.

`qimumcp.CallbackMetadata` validates and projects a reference into MCP request metadata.
`qimumcp.Binding.InvokeWithMetadata` leaves this ephemeral collaborator outside admitted
business input and its identity. The receiving method validates it before business
execution and exposes it through `qimumcp.CallbackFromContext`. Changing the reference
for a later observation does not change the original command input or receipt.
This projection supplies no durable authority of its own; the enclosing callback
owner and actual managed connection retain call and installation authority.

`qimumcp.Callbacks.OpenRPC` can bind call-local callbacks beside the once-published host
methods and canonical Runtime RPC on the same endpoint. Local names cannot replace
published methods and do not appear in another call's grant. They share the existing
cancellation, retirement and stale-reference owner. Its variadic source signature
requires external method-value/interface consumers to update and rebuild; no
former-signature wrapper remains. Core/backend wire and storage formats are unchanged.

Memory search and maintenance now use this public unary transport. Memory owns
its two-call policy and captured model collaborators. Real-process tests prove
MCP → callback → nested MCP reentrancy without a Desktop relay. Canonical Runtime
RPC/SSE callbacks share the ordinary Endpoint/Client, and Schedules uses them for
durable Run acceptance. Installed callbacks carry immutable dependency bindings;
remote continuations retain their original frozen chain.

## Model preparation host collaborators

Installed model selection, context and input handlers receive a call-scoped
`qimumcp.CallbackFromContext` reference through the existing managed connection. It enters
canonical Runtime RPC and the host's independently selected model callbacks while
the outer query is pending. The host retains the original invocation and supplies
the receiver activation's exact Actor and declared dependency bindings; the
reference expires after either success or failure.

Runtime authorizes its selected model pipeline separately from a plugin's declared
cross-plugin requirements. That does not grant arbitrary calls to undeclared plugin
operations. A nested model exchange that reenters a contributor still preparing its
ancestor is explicitly refused before recursively invoking that contribution;
Runtime neither bypasses the contributor nor retargets the frozen pipeline.
Installed transparent model observers now receive canonical RPC and independent
host model callbacks beside their captured `next` continuation. Ordinary callbacks
retain the observer activation's Actor and dependency bindings. `next` retains the
original caller's values and exact remaining chain while its network request can
cancel that work; it cannot borrow the observer's Actor. A fresh exchange that
reenters the still-active observer is refused explicitly. Original canonical stream
chunks, backpressure, source response and per-call accounting remain authoritative.

Preparation inside an active execution carries its original member's accounting
publisher, even before primary provider dispatch. Per-call request/output budgets
remain supplied by the operation owner. Retained background caller attribution
does not recreate an expired execution member, accounting publisher or next.

## Background activation and retained caller projection

A contribution's background declaration supplies `qimumcp.SetupContribution.Background` with
its own immutable installation target. An entry declaring background work must
register `qimumcp.Backend.OnSetup`. Return owned resources as `io.Closer`; `qimumcp.Backend.Close` cancels and
joins setup lifetimes before private stores close. Setup establishes resources and
checks availability; it does not advance business facts merely to register them.

An activation callback keeps its authenticated Actor and declared dependency bindings.
For a retained background request, use `qimumcp.WithInvocation` on the public callback client's
context. The host verifies the original dispatched route for human/model callers and
admits fresh automatic triggers only for that installation without an invented parent.
Retained commands preserve their declared resource target, including Session and
workspace targets; they are not forced into an installation target. The complete
stored invocation and original installation/release/server must match, and a merely
prepared command cannot authorize restoration. An unrecorded automatic trigger
cannot name another resource. This attribution does not restore an expired next
continuation or a stopped Scope effect's execution authority.
Origin, parent and target are ephemeral transport context, not business input or
credentials. Closing the callback observation leaves independently accepted Runs with
their canonical Runtime owner. Rebuild backends and callers against the same
compiled `portable.Protocol` and exact published contracts.

Known business refusals use `portable.Refusal`; classifiers and `errors.AsType`
consume that contract directly. JSON refusal fields are `code` and `message`.
`qimumcp.SetupDependency.Bindings` uses `qimumcp.SetupBinding`, a setup transport
projection validated by the existing identity owners. It contains no Flame
Protocol DTOs.

A background setup handler returns `qimumcp.ActivationResource`. Setup provisions
resources; `qimumcp.Activate` starts owned business work once per session only
after successful setup.
The connection supervisor joins resources after failed setup/start or withdrawal.
A lost activation acknowledgement cannot issue Start twice in the same lifetime.
Managed background owners can call NotifyPluginChange for their own declared
contribution. It publishes plugins.dataChanged through the existing invalidation
owner; no new event bus or plugin business store is created.

Workspace observation sources use the ordinary Runtime client's `SubscribeRuntime`
with `files.changed` and exact `WatchSpec` paths. Start the subscription from the
activation resource, after setup; cancel and join it during retirement. Runtime
admits workspace/path identity, owns bounded OS observation and reports stream
failure or `resync`. It does not turn a notification into a Run. A source backend
owns its trigger rules and durably captured intent, then uses `GetRunAcceptance`
and `StartRun` with the original request and identities. Connection-local sequences
need a new generation on reconnect; coalesced notifications and resync cannot be
represented as an exactly-once filesystem event log. See
[`File watch`](../../../plugins/filewatch/README.md) for an ordinary installed example.

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
contract and callback path. Installed Interaction and auxiliary model composition
use the same captured source, frozen contribution order and durable inner evidence.
Memory allocates its narrower grant from its published caller policy. A unary
descriptor never advertises streaming.

## Captured streams

`qimumcp.NewStreamingMethod` describes one input, a stream chunk schema and a distinct final
output schema. Stream schema is part of immutable method identity; feature generation
projects MethodChunks/Go chunk types and documents the final result.
`qimumcp.NewStreamingCallback` and `qimumcp.StreamHost` use the same authenticated callback endpoint, raw input admission,
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

`qimumcp.CallbackLimits.MaxResponseBytes` bounds every frame and final result. The model aggregate
also uses it as a conservative cumulative encoded-delta byte budget, including
metadata, before admitting deltas into Scope's accumulator. This budget can be stricter
than the final serialized response; it is explicit buffering policy, not an arbitrary
chunk-count limit. Completed inner response/usage survives a subsequent outer failure.
Partially admitted chunks remain observable even when no complete response exists.

An external executable consumes a callback capturing a frozen remaining Scope
stream chain and verifies its order, transformed input and single provider invocation.
Installed decorators retain this selection/order and exact activation; Go and TS
stream consumers preserve the same cancellation and terminal rules. Ordinary unary binding
and installed unary admission refuse stream methods before delivery/route persistence.
Only implemented streaming entrances advertise stream support.

## Backend streams

`qimumcp.StreamingMethod.RegisterServer` binds a real SDK backend stream under the same
method/capability publication checks. `qimumcp.StreamingMethod.StreamWithMetadata`
and `qimumcp.Binding.StreamWithMetadata` keep one original MCP request for input/final result.
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
SDK transport. Runtime admits installation-qualified query and command streams.
The exact method contract selects query or owner-committed command policy before
identity admission at the same `plugins.stream` delivery endpoint. Queries carry
no command key, durable command route or private receipt. Early query consumption
cancels its original source, and missing terminal evidence stays a read failure.
Commands retain their original route and receipt-only recovery; chunks have no
replay cursor. Explicit owner refusal is final evidence, while missing command
terminal delivery remains unconfirmed. The empty opening conveys no business result.

Public Runtime/Client.StreamPlugin and BindPluginStreamingMethod support both
kinds with `PluginCallOptions`; query calls carry no command identity. The TypeScript client exposes
`plugins.queryStream` beside the durable `plugins.stream` command entrance.
Component bindings expose `queryStream(method, input, consume, signal)` and keep
command `stream(intent, method, input, consume, signal)` under the mutation journal.
Both reuse the existing receiver/backpressure/cancellation owner. Ordinary
`plugins.call` remains unary. Runtime discovery and the generated bindings own the
exact protocol revision required by each consumer.

Core `CallPluginRequest.Input`, `CallPluginResponse.Output`, stream values and
point attributes/values are JSON text strings. Contribution, requirement and
resolved-binding contracts retain their canonical JSON strings too. Typed Go
bindings encode and decode the same original source through `jsonvalue` and the
feature contract, preserving full-width integers and receipt lexemes. Rebuild
public Go/CLI/TS consumers together; former inline Core payloads are refused.
The backend's existing MCP value/receipt envelopes are unchanged.

## Primary Interaction context composition

`ModelContextContract` publishes the ordered `flame.model-context.v2` context
contribution point. An installed backend registers `ModelContextMethod`, binds
its declared capability export and returns `ModelContextResult`. The query
receives the captured provider/model and the complete current Scope request;
its invocation metadata identifies the original model call and parent effect.
It has no command identity, execution continuation or model-routing authority.
`userMessage` is the latest user message from the original caller's request,
captured before the host appends transient material such as deferred-tool catalogs.
It is absent when that source has no user message. Retrieval consumers use this
projection instead of inferring authored input from the final request's last role.
Earlier contributors cannot replace it.

Context v2 replaces the former v1 method and contract without an adapter. Rebuild
context backends and select the v2 composition explicitly. Finish waiting v1 work
with its original build; captured bindings are not retargeted.

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
Request transformations use the separate input contract below. Transparent model observers keep
their existing Scope Call/Stream contract and cannot acquire those authorities.

## Model selection before exact binding

`ModelSelectionContract` publishes the `flame.model-selection.v2` selection role.
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

The query's `context` projects the actual calling use and its captured envelope.
`interaction` has no caller byte budget at this boundary; `utility` carries text
bytes and output tokens before authoring its prompts; `independent` carries encoded
request bytes and output tokens from the selected-call budget. Limits remain with
their calling owners and cannot be changed by selection. They are neither model
capacity nor a monetary allowance. A selector must not infer use from an absent
request or treat text bytes as encoded-request bytes. Selection contributors must
rebuild and explicitly select v2; the former contract has no adapter or fallback.

Selection is per accepted execution/auxiliary capture. Input transformations and
independent routed physical calls use the separate admitted boundaries below.

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

## Tool proposal transformation

`ToolProposalContract` publishes `flame.tool-proposal.v1`, an ordered call-scoped
decoration contract. Its `ToolProposalMethod` binds
`flame.tool-proposal/transform.v1` as a typed query from `chat.ToolCall` to
`chat.ToolCall`. An ordinary installed backend registers the method and binds the
contract to its declared export. Configure its order with the existing composition
operation; publisher origin supplies no special path.

The input uses Runtime's logical Tool call identity, original model origin and
parent Run. Contributors may change arguments, preserving call ID and Tool name.
Their queries use the existing call-bound RPC link and captured installation
authority. A failed or withdrawn contributor fails preparation; it is never
silently skipped. After the ordered transformations, trusted workspace hooks and
the original Tool owner admit final argument structure and approval policy.
Approval, execution and history consume those same effective arguments.

Execution freezes the bindings, activation identities and transitive release
dependencies with its other call contributors. Approval restoration uses the
stored final arguments and never reruns transformations. A pending backend
handshake defers startup's read-only waiting probe instead of blocking management.
The required private checkpoint field is now `tool_proposals`; earlier captures
are refused, never interpreted as an empty chain. Preserve old data and finish
old waiting executions with their original binary. This additive SDK contract
does not change existing Core/backend wire epochs or private business receipts.

`TestInstalledToolProposalIsApprovedExecutedAndRetainedWithoutTransformReplay`
builds an independent public-SDK stdio author, installs it with Reviews, verifies
rewritten approval/execution/history and the private owner after Runtime restart,
and proves malformed arguments cannot reach approval or mutation. The test's
provider omits usage: recovery preserves that absence instead of inventing tokens
or a known price. No graphical component is loaded.

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
command before its state transition.
Canonical media the selected model cannot accept is never passed to its provider
counter; compaction uses its local projection and final transformation/admission
remains authoritative. Independent calls retain their own identities, captured
limits and physical source evidence.

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
ref, err := qimumcp.CallbackFromContext(ctx)
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
contracts. `qimumcp.Callbacks.PublishRPC` publishes them beside canonical RPC on the existing
endpoint; per-reference transport limits, captured invocation and activation lifetime
still apply. The existing physical endpoint journal fences production dispatch and
retains full evidence. The SDK does not invent a model receipt store or fake Run.

The reference for an observer's `next` is a captured continuation with its original
capability. It does not grant selected-model routing. The transparent observer's
once-through contract remains specific to observation; it is not a universal rule
for composition. General durable composition still requires Scope-owned effects or
an existing durable use case. New selected capabilities are additive exact methods;
older hosts without that grant explicitly refuse them. No legacy method is relabeled.

`PublishRPC` now has the variadic signature `func(http.Handler, ...qimumcp.Callback) error`.
In-repository publishers are migrated. External method-value assignments requiring
the former exact Go function type must change and rebuild; there is no old-signature
wrapper. This source break does not alter the canonical RPC wire contract.

`qimumcp.CallHost` and `qimumcp.StreamHost` send the original context invocation
under the compiled `qimumcp.InvocationHeader`. An activation-scoped background RPC grant validates it
against the retained calling-owner evidence before projecting the context. Forged
origin/parent/target is refused. Ordinary call and next grants keep their captured
owner and ignore this header; authenticated actor identity is never replaced.

Independent selected exchanges require an ID distinct from their original calling
operation and host-owned Model/Tool boundary. An existing logical Model/Tool ID
cannot be repurposed as a fresh physical caller. The host captures execution ownership;
wire invocation headers and parent fields cannot select another Run or Segment.
Endpoint observations expose the original optional Segment in the Core protocol.

## Tool context and text embeddings

`WithToolContext` projects a captured `ToolContext` into ordinary MCP Tool
metadata. `ToolContextFromContext` admits that same closed current shape. Workspace
and optional vector-space identity come from the calling owner, outside model
arguments; callback authority remains only in `qimumcp.CallbackMetadata` and
`qimumcp.CallbackFromContext`. Setting Tool context cannot mutate an earlier caller's
metadata. There is no standing model grant inside the context record.

An ordinary installed model Tool call receives the same canonical call-scoped host
grant as its public `plugins.call` entry, through the one grant owner. The grant
carries the Tool call's own invocation and the backend's frozen dependency
authority, so one business reached through either entry keeps identical host
capability, declared-dependency reach and call attribution. Memory keeps its
narrower model/embedding grant and `ToolContext` as its own two-call policy; an
ordinary backend never declares the Memory contract to reach the host.

`TextEmbeddingMethod`, `NewTextEmbeddingCallback` and `CallTextEmbedding` publish
`flame.model/embed-texts.v2` for a captured model. Input is a text array; output is
`TextEmbeddings`, containing `space` and its corresponding float32 `vectors`.
The model adapter supplies the actual provider/model/endpoint-derived space in the
same exchange. A plugin never infers it from dimensions or a separately read role.
Empty, missing, non-finite and inconsistent vector sets are refused. This is an
explicit vector projection; the provider adapter
retains its complete response, usage and endpoint evidence. Each physical exchange
has its own original child identity and retains the caller's origin/parent.
On ordinary installed Runtime callbacks, this method resolves the configured
embedding role for the exchange. An absent role returns the explicit
`model_role_unconfigured` refusal; a configured resolution failure stays an error.
Neither path selects another provider. An independent exchange cannot reuse its
calling or executing owner's logical identity.

Call admission, resource budgets and workflow counts belong to its caller; the
shared SDK has no Memory-specific two-call limit and never retries provider work.

The former vector-only v1 method is removed. `CallTextEmbedding` and
`NewTextEmbeddingCallback` now return `TextEmbeddings`; external authors must update
their handlers and consume `Vectors` with its `Space`. Rebuild Memory and Runtime
together. There is no legacy method reader. Private databases and command receipts
keep their formats. The Core wire epoch is unchanged.

## Private SQLite receipts

Independent authors can use [`plugin/sqlite`](sqlite/README.md) for original input
admission, nullable unknown settlement and atomic mutation/receipt handling. The
same implementation serves the bundled backends. Strict schema admission and
per-database transaction lifetime live in [`qimu/sqlite`](../../../qimu/sqlite/README.md).
No private Runtime store or host business schema is part of the author API.
`portable.ErrInvocationConflict` classifies changed original method/input under
one invocation identity. Host replay retention and execution lifecycle remain
inside Runtime.

## Model choices

[`model`](../model/README.md) owns the immutable provider/model/reasoning choice
and atomic patch rules shared with Runtime. [`model/catalog`](../model/catalog/README.md)
admits known effort levels and input modalities from the released Scope catalog.
Authors use these public packages directly; role configuration, endpoint evidence
and execution budgets retain their respective owners.
