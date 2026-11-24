# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.knowledgebase/delete.v1` | command | installation | Delete a document and its index at the inspected revision. |
| `flame.knowledgebase/import.v1` | command | installation | Import or replace a user document and its embedding index at the inspected revision; use zero for a new document. A document with a project is visible only within that Project. |
| `flame.knowledgebase/list.v1` | query | installation | List the indexed user documents visible to the caller, shared documents and its Project's own, with their revisions. |
| `flame.knowledgebase/read.v1` | query | installation | Read one complete stored document visible to the caller with its exact source, revision and index identity without embedding or provider I/O. |
| `flame.knowledgebase/reindex.v1` | command | installation | Rebuild an existing document's embedding index from its stored original text at the inspected revision through the configured embedding role. |
| `flame.knowledgebase/search.v1` | query | installation | Retrieve cited chunks from the indexed user documents visible to the caller, shared documents and its Project's own, through the configured embedding role. |

Capability `flame.knowledgebase.v1` exposes the declared methods. Required backend protocol: `io.github.tangerg.frame/portable/1`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.
