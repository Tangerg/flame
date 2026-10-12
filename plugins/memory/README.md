# Memory

The backend owns durable memory items, human review and edits, rejected fact
history, the extraction ledger, curation watermark, publication and derived
embedding cache. Runtime owns committed conversation, compaction and model
configuration. It supplies a captured conversation and the actual compaction fact;
the backend decides whether extraction and curation are due. Publication uses the
same watermark compare-and-set and human-review invariants as every other write.

The installed stdio process opens only `PLUGIN_DATA/memory.db`, under an exclusive
process lease. A release declares the closed `memory` service against its own
stdio server. Runtime admits at most one enabled owner and connects its human
`agentMemory.*` operations, Run context reads and source-qualified `search_memory`
Tool to that exact generation. Closing a view cannot stop this process.

Optional model work uses an invocation-scoped, unguessable loopback capability
issued only to this backend. Its two-call budget covers extraction plus curation
or query plus corpus embedding. It accepts only bounded Memory operations, carries
no Runtime bearer or provider credential, and is canceled and joined when the
invocation or backend generation ends. Standard MCP sampling is not used. A
missing or failed optional embedder preserves keyword ranking; a storage failure
remains an error.

Human commands commit their mutation and reply in one private transaction. A
same-key redelivery returns the original reply; changing the method or input is an
idempotency conflict. Runtime stores no competing command outcome. An unavailable
owner returns `plugin_unavailable` for human operations; Run context reads omit
this optional feature when no owner is enabled. An enabled owner's failures remain
explicit. An admitted command with an unconfirmed transport reply invalidates human
reads without claiming success; the next read consults the backend. Revoked curation
leaves committed extraction pending and resumes its fold after restart. Human edits
and review remain authoritative throughout.

Disable, revoke and uninstall withdraw the generation and its capabilities. They
retain its private data. Reenable uses the same installation directory; a new
installation receives a different directory. Restart restores private facts from
the same database. Old Runtime memory tables are not read or migrated.

Build packages using [the local application workflow](../README.md). Backend
source lives in [`runtime/internal/plugins/memory`](../../runtime/internal/plugins/memory) as part
of the local Runtime application module. Verify from `runtime` with the workspace
enabled: `go test ./...`, `go vet ./...`, and `go build ./...`.

## Shared presentation

The standalone resource is generated from `src/memory.tsx`. It uses the same `@flame/ui` components and document stylesheet as Desktop. Theme selection remains with the trusted Host; the v3 carrier applies its initial projection and later updates without replacing the page or requesting memory again. Filters and pagination remain read-only, and trusted authoring stays outside the guest.

Build `icons` and `ui`, run `npm ci --prefix plugins/memory`, then `npm run check --prefix plugins/memory`. Rebuild the tracked HTML after changing shared UI or this source. The resource check enforces the Runtime size bound and byte-for-byte build reproducibility.
