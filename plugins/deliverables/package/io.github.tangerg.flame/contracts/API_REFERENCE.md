# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.deliverables/append.v1` | command | installation | Append one canonical base64 chunk at the exact acknowledged upload offset. Reuse the original command receipt after transport loss, never invent a retry identity. |
| `flame.deliverables/assessment.v1` | query | installation | Read one exact render or review report. List headers are projections and do not carry another mutable assessment. |
| `flame.deliverables/catalog.v1` | query | installation | Read the exact supported native creation/edit schemas, format limits and configured rendering engines before creating or editing deliverables. |
| `flame.deliverables/create.v1` | command | installation | Create an editable DOCX, XLSX or PPTX from the catalog's content schema. Author provenance is not independently verified evidence. Formula results require explicit recalculation. |
| `flame.deliverables/discard.v1` | command | installation | Discard the specified staging upload. This does not remove a published deliverable. |
| `flame.deliverables/download.v1` | query | installation | Read bounded bytes from an exact immutable asset. Verify the complete size and SHA-256 before saving; file saving is a separate host operation. |
| `flame.deliverables/edit.v1` | command | installation | Apply catalog-defined local native edits to the exact current revision. Unaffected package parts are preserved; stale revisions are refused. A new revision has no inherited render or review. |
| `flame.deliverables/export.v1` | command | installation | Prepare the exact native revision, its existing rendered PDF, one XLSX sheet as CSV (sheetPart required), or a delivery ZIP with native source, current derivatives and evidence manifest. CSV refuses missing/error formula caches and protects formula-like strings. This never sends or publishes a file. |
| `flame.deliverables/history.v1` | query | installation | Read immutable revision history; previous previews and reviews do not apply to later revisions. |
| `flame.deliverables/import.v1` | command | installation | Import the completed, hash-verified native upload, preserving its admitted original bytes. Unsafe or unsupported packages are refused without silent feature stripping. |
| `flame.deliverables/list.v1` | query | installation | List retained deliverables newest first. Content and evidence remain owned by exact immutable native revisions. |
| `flame.deliverables/read.v1` | query | installation | Inspect a deliverable and exact native revision, its editable item identities and source-bound checks. Omitting revision reads the current head once. |
| `flame.deliverables/recalculate.v1` | command | installation | Recalculate an XLSX through confined LibreOffice into a new native revision. Conversion may change Office formatting; the exact original is retained. Inspect resulting formula errors and missing caches. |
| `flame.deliverables/remove.v1` | command | installation | Remove one deliverable and its native/derived bytes only if the expected current revision still matches. Original receipts remain and cannot resurrect removed content. |
| `flame.deliverables/render.v1` | command | installation | Render the exact native revision through configured confined LibreOffice/Poppler. Record actual engine versions, PDF/page hashes and extraction checks; successful rendering does not establish visual correctness. |
| `flame.deliverables/review.v1` | command | installation | Record the caller's visual review of enumerated pages from an exact render. This is attributed review evidence, not an automatic quality certification or approval to publish. |
| `flame.deliverables/upload.v1` | command | installation | Reserve a bounded native-file upload for the exact size and SHA-256. No host path or URL is read. |
| `flame.deliverables/uploads.v1` | query | installation | Read admitted staging uploads and their current acknowledged offsets. Reselect only bytes matching the declared SHA-256 to resume. |

Capability `flame.deliverables.v1` uses `collection` composition in `installation` scope. Required backend protocol: `io.github.tangerg.qimu/portable/1`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.
