# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.knowledgebase/accept-claim.v1` | command | installation | Accept an exact research statement and reason with validated citations in one Project. |
| `flame.knowledgebase/accept-work.v1` | command | installation | Accept a reusable method version, current claim revisions, and exact native Project deliverable bytes. |
| `flame.knowledgebase/delete.v1` | command | installation | Delete a document and its index at the inspected revision. |
| `flame.knowledgebase/import.v1` | command | installation | Import or replace a user document and its embedding index at the inspected revision; use zero for a new document. A document with a project is visible only within that Project. |
| `flame.knowledgebase/list-claims.v1` | query | installation | List a Project's latest research decisions and current evidence status. |
| `flame.knowledgebase/list-work.v1` | query | installation | List saved Work identities and immutable method versions in one Project. |
| `flame.knowledgebase/list.v1` | query | installation | List the indexed user documents visible to the caller, shared documents and its Project's own, with their revisions. |
| `flame.knowledgebase/move.v1` | command | installation | Move an inspected material to another Project without re-embedding its content. This withdraws access from the original Project. |
| `flame.knowledgebase/read-claim.v1` | query | installation | Read a retained research decision with exact page and line citations, checking current Project access and changes to each cited passage. |
| `flame.knowledgebase/read-work.v1` | query | installation | Revalidate the Work's original claim revisions and native bytes against current Project access and the latest chart revision. |
| `flame.knowledgebase/read.v1` | query | installation | Read a stored document or one numbered page at the current or retained revision. Current Project access is checked before returning source metadata or text. |
| `flame.knowledgebase/reindex.v1` | command | installation | Rebuild an existing document's embedding index from its stored original text at the inspected revision through the configured embedding role. |
| `flame.knowledgebase/search.v1` | query | installation | Retrieve cited chunks from the indexed user documents visible to the caller, shared documents and its Project's own, through the configured embedding role. |

Capability `flame.knowledgebase.v1` exposes the declared methods. Required backend protocol: `io.github.tangerg.frame/portable/1`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.
