# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.knowledgebase/delete.v1` | command | installation | Delete a document and its index at the inspected revision. |
| `flame.knowledgebase/import.v1` | command | installation | Import or replace a user document and its embedding index at the inspected revision; use zero for a new document. |
| `flame.knowledgebase/list.v1` | query | installation | List indexed user documents and their revisions. |
| `flame.knowledgebase/search.v1` | query | installation | Retrieve cited chunks from indexed user documents through the configured embedding role. |

Capability `flame.knowledgebase.v1` uses `collection` composition in `installation` scope. Required backend protocol: `io.github.tangerg.flame/backend/2026-10-15.3`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.
