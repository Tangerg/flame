# Memory

The backend owns durable memory items, human review and edits, rejected fact
history, the extraction ledger, curation watermark, publication and derived
embedding cache. Runtime owns committed conversation, compaction and model
configuration. It supplies a captured conversation and the actual compaction fact;
the backend decides whether extraction and curation are due. Publication uses the
same watermark compare-and-set and human-review invariants as every other write.

The installed stdio process opens only `PLUGIN_DATA/memory.db`, under an exclusive
process lease. A release declares the closed `memory` service against its own
stdio server. Multiple candidates may be enabled; product composition explicitly
selects one installation/contribution for new calls. Runtime connects its human
`agentMemory.*` operations, Run context reads and source-qualified `search_memory`
Tool to that exact generation. Closing a view cannot stop this process.

Optional model work uses the public SDK's invocation-scoped `CallbackRef`, bound
to this concrete backend generation. Runtime owns one lazily opened loopback
callback endpoint, shared by concurrent calls. Memory's RPC adapter owns its
two-call budget for extraction plus curation or query plus corpus embedding; the
public endpoint imposes no Memory policy. The reference carries no Runtime bearer
or provider credential and expires when the owning call or backend generation
ends. Withdrawal cancels and joins admitted callbacks, including incomplete
request bodies. Standard MCP sampling is not used. A
missing or failed optional embedder preserves keyword ranking; a storage failure
remains an error.

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
The private command table stores original admitted JSON in `input_json` and the
typed value/refusal envelope in `reply`. Fingerprints derive from that original
input, including its exact numeric lexemes. A null reply represents private
admission without a known outcome; it is not a successful empty result. Former
completed-only, digest-only and bare-result databases are refused without conversion;
retain that directory separately and use a fresh directory for this breaking change.

Build packages using [the local application workflow](../README.md). Backend
source lives in [`runtime/internal/plugins/memory`](../../runtime/internal/plugins/memory) as part
of the local Runtime application module. Verify from `runtime` with the workspace
enabled: `go test ./...`, `go vet ./...`, and `go build ./...`.

## Shared presentation

The standalone resource is generated from `src/view.tsx`. It uses the same `@flame/ui` components and document stylesheet as Desktop. Theme selection remains with the trusted Host; the v3 carrier applies its initial projection and later updates without replacing the page or requesting memory again. Filters and pagination remain read-only, and trusted authoring stays outside the guest.

Build `icons` and `ui`, run `npm ci --prefix plugins`, then `npm run check --prefix plugins`. Rebuild the tracked HTML after changing shared UI or this source. The resource check enforces the Runtime size bound and byte-for-byte build reproducibility.
