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

`Descriptor` is an independently owned discovery/build projection, not another
method authority. `NewCapability` groups the exact required method descriptors
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

`BindCapability(export, contract)` associates a named export with those existing
handlers and checks its complete required set before publication. Several exports
may share a handler; there is still one semantic method and business receipt
namespace. Export identity describes routing, not another mutable business fact.
`ParseCapability` admits generated local metadata without executing code or fetching
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

The current SDK requires `BackendProtocol` on invocation and description. It is
a public wire contract identity, separate from internal build or data-directory
versions. Only this current protocol is interpreted. Installed declaration/token
matching remains part of the continuing refactor.

The [Reviews backend](../../examples/plugins/reviews/backend/server.go) binds Tool
and typed method entrances to the same private owner. Its tests prove shared
receipts, business conflicts, identity conflicts and uncertain storage failures.
The current repository workspace builds the evolving SDK source. Independent
published consumers need a Runtime version containing this package; the older
version still named by application module metadata does not contain it.

This package currently supplies backend wire primitives. Runtime's existing
Memory/Schedules commands retain original installed routing before dispatch and
reconcile through this lookup. Generic installed-provider proxies, full point descriptors,
generic contribution declaration and component bindings remain separate unfinished
parts of the unified-plugin refactor. A direct backend session is not a public
installed-provider proxy. Canonical Core callbacks use the existing typed Runtime
Client and the same Delivery Endpoint as ordinary host calls.

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
not yet provide installed dependency/entry rendezvous or remote decorator
continuations. Canonical Runtime RPC/SSE callbacks now share the ordinary
Endpoint/Client, and Schedules uses them for durable Run acceptance. Complete
installed dependency binding and streaming decorators remain
required parts of the complete plugin refactor.
