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
method authority. Pipe descriptor JSON into the existing Runtime `contractgen`
with `--feature - --out <contracts-directory> --ts <typescript-directory>`.
It emits standard self-contained Schema, operation descriptors, reference prose
and TypeScript input/output maps from that same source. It reuses Scope schema
derivation and Runtime's Schema-to-TypeScript emitter; no new IDL or Go DTO copy
is introduced. Numeric/cross-field constraints remain in Schema and the business
owner where TypeScript cannot express them. Output filenames are fixed; method
identities never become filesystem paths.

`RegisterServer[P, R]` binds an unkeyed method. `RegisterCommand[P, R]` passes the
bounded logical command identity and typed input to the business owner.
`RegisterClient[P, R]`, `Call[P, R]` and `CallCommand[P, R]` use those same wire
types. Missing input, unknown envelope/input/result members and malformed command
identities are refused. A reply contains exactly one value or business refusal;
an explicitly nullable value remains distinct from a missing reply.

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

The [Reviews backend](../../examples/plugins/reviews/backend/server.go) binds Tool
and typed method entrances to the same private owner. Its tests prove shared
receipts, business conflicts, identity conflicts and uncertain storage failures.
The current repository workspace builds the evolving SDK source. Independent
published consumers need a Runtime version containing this package; the older
version still named by application module metadata does not contain it.

This package currently supplies backend wire primitives. Runtime's existing
Memory/Schedules commands retain original installed routing before dispatch and
reconcile through this lookup. Generic installed-provider proxies, host callbacks, point descriptors,
generic contribution declaration and component bindings remain separate unfinished
parts of the unified-plugin refactor. A direct backend session is not a public
installed-provider proxy, and these primitives do not authorize core Runtime calls.
