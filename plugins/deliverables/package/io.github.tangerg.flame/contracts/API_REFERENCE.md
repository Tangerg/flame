# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.deliverables/append.v1` | command | installation | Append one canonical base64 chunk at the exact acknowledged upload offset. Reuse the original command receipt after transport loss, never invent a retry identity. |
| `flame.deliverables/assessment.v1` | query | installation | Read one exact render or review report. List headers are projections and do not carry another mutable assessment. |
| `flame.deliverables/blocks.v1` | query | installation | Project a DOCX revision's body blocks from its native bytes: paragraphs with style, list and alignment facts and their text runs, tables and other blocks, each with the block id and exact-XML expect that block edits name. Omitting revision reads the current head once. |
| `flame.deliverables/catalog.v1` | query | installation | Read the exact supported native creation/edit schemas, format limits and configured rendering engines before creating or editing deliverables. |
| `flame.deliverables/comment.v1` | command | installation | Open a comment thread anchored to an exact revision and optionally an object in it, with the object's text then. |
| `flame.deliverables/copy.v1` | command | installation | Start a new deliverable from an exact revision of another, such as a template. Its first revision carries that revision's native bytes, including masters, layouts, theme, styles and sample content, and records the source revision and its hash as processing evidence; later changes to the source do not affect it. |
| `flame.deliverables/create.v1` | command | installation | Create an editable DOCX, XLSX or PPTX from the catalog's content schema. Author provenance is not independently verified evidence. Formula results require explicit recalculation. |
| `flame.deliverables/discard.v1` | command | installation | Discard the specified staging upload. This does not remove a published deliverable. |
| `flame.deliverables/download.v1` | query | installation | Read bounded bytes from an exact immutable asset. Verify the complete size and SHA-256 before saving; file saving is a separate host operation. |
| `flame.deliverables/edit.v1` | command | installation | Apply catalog-defined local native edits to the exact current revision. Unaffected package parts are preserved; stale revisions are refused. A new revision has no inherited render or review. |
| `flame.deliverables/export.v1` | command | installation | Prepare the exact native revision, its existing rendered PDF, one XLSX sheet as CSV (sheetPart required), or a delivery ZIP with native source, current derivatives and evidence manifest. CSV refuses missing/error formula caches and protects formula-like strings. This never sends or publishes a file. |
| `flame.deliverables/history.v1` | query | installation | Read immutable revision history; previous previews and reviews do not apply to later revisions. |
| `flame.deliverables/import.v1` | command | installation | Import the completed, hash-verified native upload, preserving its admitted original bytes. Unsafe or unsupported packages are refused without silent feature stripping. |
| `flame.deliverables/list.v1` | query | installation | List retained deliverables newest first, only the caller's Project's own when it works in a Project. Content and evidence remain owned by exact immutable native revisions. |
| `flame.deliverables/read.v1` | query | installation | Inspect a deliverable and exact native revision, its editable item identities and source-bound checks. Omitting revision reads the current head once. |
| `flame.deliverables/recalculate.v1` | command | installation | Recalculate an XLSX through confined LibreOffice into a new native revision. Conversion may change Office formatting; the exact original is retained. Inspect resulting formula errors and missing caches. |
| `flame.deliverables/remove.v1` | command | installation | Remove one deliverable and its native/derived bytes only if the expected current revision still matches. Original receipts remain and cannot resurrect removed content. |
| `flame.deliverables/render.v1` | command | installation | Render the exact native revision through configured confined LibreOffice/Poppler. Record actual engine versions, PDF/page hashes and extraction checks; successful rendering does not establish visual correctness. |
| `flame.deliverables/reopen.v1` | command | installation | Reopen a resolved comment thread. |
| `flame.deliverables/reply.v1` | command | installation | Reply to a comment thread, optionally naming the revision that addresses it. Replying never resolves a thread; the person who asked decides. |
| `flame.deliverables/resolve.v1` | command | installation | Resolve an open comment thread. |
| `flame.deliverables/restore.v1` | command | installation | Make an earlier revision of the same deliverable current again as a new revision with its exact native bytes. The exact current revision is required; history is kept, and the new revision has no inherited render or review. |
| `flame.deliverables/review.v1` | command | installation | Record the caller's visual review of enumerated pages from an exact render. This is attributed review evidence, not an automatic quality certification or approval to publish. |
| `flame.deliverables/settle.v1` | command | installation | Record that the person adopted a pending suggestion into their draft or dismissed it. |
| `flame.deliverables/sheets.v1` | query | installation | Project an XLSX revision's sheets and cells from its native bytes: each cell's exact setCell expect, shown value and reproducing setCell value, plus locked ranges setCell refuses. Omitting revision reads the current head once. |
| `flame.deliverables/slides.v1` | query | installation | Project a PPTX revision's slides, element identities, frames, text runs, notes, chart data and the slide layouts insertSlide accepts from its native bytes. Omitting revision reads the current head once. Use these identities with expect values in catalog-defined presentation edits. |
| `flame.deliverables/suggest.v1` | command | installation | Propose catalog-defined edits to an exact revision for the person to review in its open editor. Nothing changes until they adopt the edits into their draft and save it. A suggestion holds only edits an editor stages: PPTX replaceText, setFrame, setChartData and deleteElement; XLSX setCell; DOCX replaceText, insertParagraph, deleteBlock and setParagraphStyle. The edits must apply to that revision. Use edit to change the document directly. |
| `flame.deliverables/suggestions.v1` | query | installation | List a deliverable's suggested edits newest first, with the revision each was made against and whether the person adopted or dismissed it. |
| `flame.deliverables/threads.v1` | query | installation | List a deliverable's comment threads, newest first: each anchor (the revision, object identity and quoted text it was made on), status and messages. A thread whose object no longer exists keeps its anchor and quote. |
| `flame.deliverables/upload.v1` | command | installation | Reserve a bounded native-file upload for the exact size and SHA-256. No host path or URL is read. |
| `flame.deliverables/uploads.v1` | query | installation | Read admitted staging uploads and their current acknowledged offsets. Reselect only bytes matching the declared SHA-256 to resume. |

Capability `flame.deliverables.v1` exposes the declared methods. Required backend protocol: `io.github.tangerg.frame/portable/1`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.
