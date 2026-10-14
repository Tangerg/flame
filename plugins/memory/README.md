# Memory

The backend owns durable memory items, human review and edits, rejected fact
history, the extraction ledger, curation watermark, publication and derived
embedding cache. Runtime owns committed conversation, compaction and model
configuration. It supplies a captured conversation and the actual compaction fact;
the backend decides whether extraction and curation are due. Publication uses the
same watermark compare-and-set and human-review invariants as every other write.

The installed stdio process opens only `PLUGIN_DATA/memory.db`, under an exclusive
process lease. Its local declaration binds `memory-provider` to the `memory`
server and export with the exact generated consumer contract. Multiple candidates may be enabled; product composition explicitly
selects one installation/contribution for new calls. Runtime connects human `plugins.call` operations, Run context reads and
source-qualified `search_memory`
Tool to that exact generation. Closing a component cannot stop this process.

Optional model work uses the public SDK's invocation-scoped `CallbackRef`, bound
to this concrete backend generation. Runtime owns one lazily opened loopback
callback endpoint, shared by concurrent calls. Memory publishes its caller budget
from the Go business owner as `contracts/caller-policy.generated.json`; the same
generation emits the Core caller's compiled projection. The calling operation
allocates two calls for extraction plus curation or query plus corpus embedding.
The shared callback endpoint imposes no Memory policy. The reference carries no Runtime bearer
or provider credential and expires when the owning call or backend generation
ends. Withdrawal cancels and joins admitted callbacks, including incomplete
request bodies. Standard MCP sampling is not used. An absent embedder or failed
optional embedding request permits keyword ranking; a storage failure remains
an error. The Tool's configured collaborator-resolution failure is explicit
before dispatch rather than being treated as an absent model role. The same rule
applies to human search. Callback references travel only in invocation metadata,
never in search or maintenance business input or durable routing/receipt records.

Text embedding now uses the public SDK `flame.model/embed-texts.v1` callback.
Workspace and optional vector-space identity use `plugin.ToolContext`; callback
credentials travel only in the existing public callback metadata. The private
Memory scope codec and embedding method are deleted. Rebuild Runtime and this
backend together; no legacy metadata reader is retained. This changes neither
private data nor command receipt formats. Core constructs its operation-scoped
grant from the published caller policy and public SDK contracts, without importing
the private backend. Query collaborators inherit the same admitted invocation
as the backend query, including original origin and parent identity.

The actual Memory tool adapter captures the Run workspace, original invocation
metadata and the executable's concrete callback session before dispatch. The
generic MCP pool knows no Memory service type or model DTO. After preparation it
rechecks source/configuration/session authority and cancellation; refusal releases
the grant without calling the backend. Every dispatched call releases its grant
after settlement, including a failed or missing reply.

Human commands commit their mutation and reply in one private transaction. Known
business refusals roll back mutation and commit the refusal receipt in that same
transaction; unexpected failures leave no invented receipt. A same-key redelivery
returns the original reply; changing the method or input is an idempotency conflict.
Runtime retains immutable installation, release, method and input routing before
dispatch, and stores no competing business outcome. An unresolved command pins
its release against selection, configuration and uninstall, including after
Runtime/database restart. Disable/re-enable keeps the original binding. After
dispatch admission, retries only query that owner's receipt: a missing receipt
never authorizes resubmission. An unavailable new owner returns
`plugin_unavailable`; an unavailable original dispatched owner remains an unknown
outcome and is never replaced by a current provider. Run context reads omit
this optional feature when its selected owner is unavailable. Another enabled
candidate never silently takes over. An available selected owner's failures remain
explicit. An admitted command with an unconfirmed transport reply invalidates human
reads without claiming success; the next read consults the backend. Disabled curation
leaves committed extraction pending and resumes its fold after restart. Human edits
and review remain authoritative throughout.

Maintenance durably binds its original command before model work. An admitted
command with no final reply remains unconfirmed across backend restart and cannot
start extraction or curation again. A subsequent maintenance activity has its own
identity and may fold the retained pending ledger under the existing policy.
Publication commits watermark, items and the original maintenance reply in one
short private transaction. Receipt failure rolls publication back and preserves
the ledger. Model I/O stays outside transactions so human edits and review remain
available. A completed duplicate returns the original Published value, rather
than reevaluating whether a new generation is due. Receipt lookup after transport
loss or a backend restart performs no model work.

Disable and uninstall withdraw the generation and its capabilities. They
retain its private data. Reenable uses the same installation directory; a new
installation receives a different directory. Restart restores private facts from
the same database. Old Runtime memory tables are not read or migrated.
The private command table stores original admitted JSON in `business_input_json` and the
typed value/refusal envelope in `reply`. Fingerprints derive from that original
input, including its exact numeric lexemes. A null reply represents private
admission without a known outcome; it is not a successful empty result. Former
call-context input, completed-only, digest-only and bare-result databases are refused without conversion;
retain that directory separately and use a fresh directory for this breaking change.

`runtime/internal/plugins/memory/rpc/contracts.go` is the typed method
source used by actual server registration. Query consumers use its generated
contract rather than importing the private domain or constructing another schema.
Core Run context consumes the generated Go projection in
`runtime/internal/adapter/integration/pluginservices/memorycontract`; CLI keeps its
independent generated projection from the same source. Its generated
canonical capability, Schema, method references and TypeScript data maps live
under `io.github.tangerg.flame/contracts` in this package. Regenerate from Runtime
with `go generate ./internal/plugins/memory/rpc`; the existing contractgen
projects the same methods. Generated Go operation frames are defined projection
types, so callers explicitly convert shared read shapes instead of depending on
transparent aliases. Canonical JSON and wire behavior are unchanged. The package builder copies this namespace into the
compiled package. Do not hand-edit generated artifacts or duplicate schemas.

The published snapshot owns its identity pattern, closed scope/origin/status
vocabularies and Unicode content boundary. These derive from the private Go model
and its existing constants, with canonical JSON and Go/TS consumers regenerated
together. ItemID publishes its actual patterned string contract. Core has no
Memory business record or business enum; its prompt adapter retains only the
consumed ID, content, pinned flag and ordering time. Private lifecycle admission
stays with the backend. The read adapter validates the published method/output,
active requested target and search bound before projecting prompt values; clients own their visible read projections and
reject hidden/unknown states before presentation. A schema change requires rebuilding
the ordinary ESM component against the exact generated capability.

Management pages publish Capacity as a projection of MaxVisiblePerTarget, enforced
by the generated schema const. The existing private owner alone defines target
capacity and management page width. Input Limit and output Rows bounds derive from
MaxManagementPageItems; omitting Limit uses that owner's default. Complete collectors
must retain the captured binding/capacity, reject drift/cumulative overflow and never
return partial success on continuation failure. Core exports no Memory capacity.

External read refresh uses the generic plugins.dataChanged signal. The ordinary
component retains a tracked generation-owned subscription, cancels it on retirement
and rereads its bound owner on change/resync. Private commit/curation and generic
command recovery notices carry no result or business records. A lost acknowledgement
can invalidate reads but cannot settle the original command. Subscription failure
remains visible while accepted data is retained.
The backend publishes the named `memory` capability. Runtime validates the full
contract and declared export for queries and captures the contribution/export and
complete capability for original command recovery. Human, Tool and automatic
entrances retain the same private business and receipt owners.

Build packages using [the local application workflow](../README.md). Backend
source lives in [`runtime/internal/plugins/memory`](../../runtime/internal/plugins/memory) as part
of the local Runtime application module. Verify from `runtime` with the workspace
enabled: `go test ./...`, `go vet ./...`, and `go build ./...`.

## Public consumer client

`src/client.ts` supplies headless reads and durable commands over this package's
generated capability and public Runtime client. It owns traversal validation and
acknowledgement decoding; business changes and receipts remain in the backend.
Its exported item is a consumer projection, with vocabulary derived from the
same generated contract. It imports no Desktop/Workspace implementation. Workspace
adapters translate this client into their own application ports.

## Shared presentation

The resource is ordinary prebuilt ESM from `src/component.tsx`, using shared React,
public SDK and UI provider context. Build `icons` and `ui`, install plugin dependencies,
and use `node plugins/build-components.mjs` (or `--check` for exact reproduction).
The former HTML guest/channel and separate static control build are removed.

The ordinary Memory component consumes the generated CAPABILITY_JSON and typed
method maps through the public SDK. Its exact own installation/contribution binds
reads and human mutations; no public operation selects another current provider.
The backend still owns validation, business changes and original command receipts.

The Host supplies optional frozen Session/workspace targets under client scope;
user memory remains available without a workspace. The component owns only local
scope/filter/edit and accepted page/cursor state. Unknown acknowledgement retains the
original mutation handle/key, blocks new changes and exposes same-command reconciliation.
A failed read preserves the accepted page. No optimistic item or competing result
owner is introduced. The former HTML/Memory-specific carrier/read API is removed.
The shared client journal preserves the original binding across client generation
changes. Backend/shared-client tests cover recovery and invalidation; rendered
window-reload interaction is excluded from the current acceptance scope.

## Feature boundary

Business changes expose a local `OnChange` signal within the caller's current
write-set. The transport owns publication after its private transaction commits;
no Core invalidation record or resource enum enters the feature application.
Failed writes and losing curation comparisons cannot emit a successful change.
