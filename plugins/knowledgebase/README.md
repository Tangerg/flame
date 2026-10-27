# Knowledge base

Knowledge base owns imported user documents, revisions, deterministic Unicode-safe
chunks and their vector index in `PLUGIN_DATA/knowledgebase.db`. Runtime owns model
construction, embedding dispatch/evidence, execution, approval and accounting.
There is no Session store or provider client in the backend.

Build this package with the ordinary first-party builder documented in
[`../README.md`](../README.md). Installation requires the built package, including
its executable. Enable it and configure Runtime's embedding role before importing
documents. The package has no graphical component.

## Operations and tools

The `knowledgebase-operations` contribution exports the Go-owned
`flame.knowledgebase.v1` collection contract. Its generated descriptors and typed
TypeScript bindings are in `io.github.tangerg.flame/contracts`. Use ordinary
`plugins.call` or the generic CLI `plugins call --request ...` entrance with the
installed contribution's exact release and contract.

- `flame.knowledgebase/import.v1` accepts `content` (`id`, `title`, `source`, `text`)
  and `expectedRevision`. Zero creates a new identity; replacement requires the
  revision returned by list/import. Example input:
  `{"content":{"id":"guide-1","title":"Guide","source":"docs/guide.md","text":"Flame owns durable Runs."},"expectedRevision":0}`.
- `flame.knowledgebase/list.v1` takes `{}` and reads document headers.
- `flame.knowledgebase/read.v1` accepts `id` and returns the complete original text
  with its source, revision and index identity, without embedding or provider I/O.
- `flame.knowledgebase/reindex.v1` accepts `id` and `expectedRevision`, rebuilds the
  index from that exact stored revision and advances the document revision. A
  concurrent edit or deletion refuses publication without replacing its result.
- `flame.knowledgebase/search.v1` accepts `query` and `limit` (1–8) and returns
  chunks with the document header, one-based chunk number and cosine score.
- `flame.knowledgebase/delete.v1` accepts `id` and `expectedRevision`. Deletion
  removes content/index and retires the identity permanently. Import a new ID to
  recreate a document; an old inspected revision cannot authorize a later incarnation.

`search_knowledge` calls the exact same retrieval use case as the typed query,
including its host callback and explicit space-mismatch refusal.
`list_knowledge_documents` reads the same private headers. Tools cannot change the
corpus. `read_knowledge_document` reads the same complete stored original without
requiring the embedding role. Import, reindex and delete remain explicit durable
commands.

The initial corpus admits 32 active documents, each up to 16,384 Unicode code
points, split into chunks of 1,024 code points. One import uses at most one
embedding exchange over at most 16 chunks. Search uses one query embedding and a
bounded scan of the private index. There is no automatic provider retry.

## Model context

Add `knowledgebase-context` to the ordered `flame.model-context.v2` composition
through `plugins.setComposition`. Enablement alone does not select a composition.
It retrieves four chunks using the final 4,096 code points of the host-captured
original `userMessage`. Host-added user-role catalogs cannot replace that input.
A text-free input or empty corpus contributes nothing.
The system contribution marks document material as untrusted reference data and
retains source/document/chunk citations. Authored messages and Session history
remain unchanged. Callback/storage failures refuse preparation explicitly.

Every embedding reply carries the actual provider/model/endpoint-derived vector
space. Index publication captures that identity with its vectors. Search refuses
`embedding_space_changed` after a role/endpoint change, even when dimensions match.
Reindex existing documents at their current revisions to rebuild them from the
original content already owned by the library; reads remain available throughout.

## Durable publication

Import first retains the original admitted JSON in the private command table.
Reindex shares this exact admission and publication workflow. Its original ID and
revision bind the captured stored text; recovery never captures a later revision
or repeats embedding. A missing or stale document refuses before model dispatch.
Only its first admission may invoke embedding. The document/index and success or
known refusal receipt commit atomically after that exchange. Reconciliation reads
the original receipt without embedding again, including after restart or a later
revision. Changed original JSON conflicts with the retained identity. An uncertain
embedding/commit remains unconfirmed; absence of a receipt cannot authorize replay.
Database corruption is a protocol failure, not a business refusal.

Generate contracts from the repository workspace with
`make generate`. Run the backend's tests from
the workspace with `go test ./plugins/knowledgebase/backend/... -count=1`.

## Contract publication

The plugin's `contractgen/` program owns its Portable definition. `make generate`
from the repository root refreshes `contracts/contract.generated.go` before the
package and client projections. Handwritten DTOs and schema hooks in
`internal/wire` remain independent of that generated descriptor. The backend and
exported author implementation bind the published contract with `ImplementMethod`;
an author's local `go generate` refreshes projections without redefining identity
from relocated Go types.
