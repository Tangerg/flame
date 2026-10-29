# Professional deliverables

`flame.deliverables` creates, imports, revises, inspects, renders and exports
editable documents, workbooks and presentations through one declared plugin
capability. Its workspace component and model tools use that same application.

## Ownership

An immutable revision's native DOCX, XLSX, PPTX or PDF bytes own its content.
The document aggregate owns its identity and current revision. A change requires
the exact expected current revision and creates a new revision; it never rewrites
history. Structured creation instructions are command evidence, not a second
editable document. Imported packages are edited by explicit, bounded operations
on selected parts. Unsupported edits are refused rather than round-tripped
through a lossy reconstruction.

The private SQLite archive owns revisions, derivative assets and original command
receipts. Revision publication and command settlement share one transaction.
An admitted command with no settlement remains unconfirmed after interruption;
its identity cannot execute again. Runtime continues to own runs, model work,
authorization and tool recovery. The plugin has no independent agent or job loop.

Inspection, rendered PDF, page images, checks and visual-review observations are
derived evidence tied to an exact source revision and hash. A new revision has
no inherited render or review. Structural validity, formula results, successful
rendering and visual review are distinct observations. None establishes that
the author's statements are true. The component owns selection and unsubmitted
drafts only. File saving belongs to the host and does not advance a document's
review or delivery state.

## Format boundary

`contracts/content.ts` publishes the supported content and edit vocabulary.
`backend/formats` owns native creation, package admission, inspection and local
editing. DOCX and PPTX creation use native editable Office objects. XLSX creation
retains typed values and formulas; formula caches supplied by a model are never
accepted as calculated results. Imported files retain the original bytes, and
local edits preserve untouched package parts. Macros, active embedded payloads,
unsafe relationships and packages exceeding admission limits are refused.

PDF is an import, inspection and explicit-edit format and an Office-rendering
output. Preview uses page images from the actual PDF, not a separately authored
HTML approximation. The catalog states supported operations and fidelity limits.
Template workflows import an original file and apply those local operations;
there is no claim of arbitrary Office feature support.

| Format | Native creation                                                                                        | Revision operations                                                                                    |
| ------ | ------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------ |
| DOCX   | Rich text, headings, lists, tables, PNG/JPEG images, headers, footers, page numbers and references     | Replace exact text runs; append paragraphs                                                             |
| XLSX   | Typed cells, formulas, styles, comments, tables, merges, freeze panes and editable bar/line/pie charts | Set exact cells; invalidate dependent caches; recalculate through LibreOffice; export one sheet as CSV |
| PPTX   | Editable text, shapes, tables, charts, images, notes and masters                                       | Replace exact slide/notes text; reorder the original slides                                            |
| PDF    | Produced by the actual Office renderer                                                                 | Import passive classic-xref PDFs; rotate/reorder pages; fill supported AcroForm fields                 |

The catalog is the executable schema and limitation reference. Imported PDF object
streams, encryption, signatures, attachments and active content are refused. PDF
text-body rewriting, OCR, redaction and arbitrary Office object restructuring are
outside the declared edit vocabulary. Unsupported form glyphs fail rather than
producing a falsely successful appearance update.

Content JSON uses Qimu's exact numeric codec before projection. Values that cannot
be represented faithfully are refused at authoring boundaries. Imported workbook
numbers outside that projection remain native decimal text in inspection; CSV
retains their original numeric lexemes. Formula-like string values are escaped
for spreadsheet consumers, and missing or error formula caches prevent CSV export.

## Interaction

The workspace page supplies guided document, table and slide creation, advanced
schema-based input, resumable imports, revision history, local edit forms, actual
page previews, attributed reviews and download controls. The same published
operations are available as model tools. The additional read-only `page` tool
returns an actual retained PNG so a model can examine the render it is reviewing.

The ordinary workflow is create/import, inspect, revise, recalculate if required,
render, examine pages, record review, and export. Each edit captures the expected
current revision. Each review captures its revision, render and reviewed page set.
An old page's successful review cannot certify a later edit. A disconnected
component retains its original command handle and consults that receipt; it
cannot turn an absent receipt into permission to execute again.

## Execution and evidence

LibreOffice is the only Office rendering and workbook recalculation engine.
Poppler reads the actual PDF and produces page previews and text. Tools are
explicitly configured by absolute paths on the Runtime host. Each invocation
uses a private profile and scratch directory, a scrubbed environment, bounded
outputs and a deadline. Linux uses bubblewrap and macOS uses Seatbelt; unavailable
confinement is an explicit refusal, never an unrestricted fallback. No model
argument can name an executable, shell command, host path or remote resource.

Linux also requires `/usr/bin/prlimit`; the sandbox applies address-space, CPU,
single-file and diagnostic-output limits before executing any renderer, including
version probes. macOS applies CPU, single-file and open-file limits before execution
and uses the system Perl interpreter to sample Darwin's `proc_info` resident
memory every 25 ms, terminating the process on excess or
measurement failure. This sampled threshold allows brief overshoot; it is not a
kernel address-space limit. All bounds are published by `contracts/limits.ts`. Optional
`FLAME_DELIVERABLES_TOOL_ROOTS` is a JSON array of specific canonical installation
directories needed by tools outside the system directories. General home,
workspace and root directories, or paths overlapping plugin data, are refused.
Private plugin data must also remain outside every fixed system read root.
Fonts in user homes are not exposed. macOS requires actual executable binaries
that can run with `process-fork` denied; launchers requiring helper processes are
refused. A successful version probe is not a claim that every document or platform
installation has been qualified.

The private LibreOffice profile disables macros, active content and external-link
updates and sets `OOXMLRecalcMode=0`, the engine's always-recalculate-on-load mode.
An ordinary conversion with the engine's default profile can retain stale caches.
Each page is rasterized separately with bounded pixel dimensions; outputs must be
regular, unlinked files with admitted sizes. A process returning zero without its
required PDF, workbook or page image is a failure. Retirement terminates and joins
the confined process and any Linux descendants before removing scratch. Unconfirmed process exit retains the
private directory and prevents another engine call.

Recalculation is an explicit conversion into a new native revision, retaining its
original. The resulting workbook is inspected for missing cached results and
formula errors. Engine versions and exact source/output hashes accompany the
observation. LibreOffice's rendering and conversion can differ from Microsoft
Office; page images require actual visual review before claiming visual quality.

Native package parsing runs in a separate single-operation managed Node process
inside the same OS confinement boundary. Its invocation directory and packaged
code are read-only, and the OS boundary isolates network access. Node permissions
add a code-only filesystem grant and deny child-process creation; they do not own
network isolation. Parsing has a 256 MiB JavaScript heap, a 2 GiB memory budget
and a 60-second deadline. The external Office/PDF operation has a 4 GiB memory
budget and a 120-second deadline. Linux enforces the memory budget as address
space; macOS uses the resident-memory observation described above. Both callers use one process owner
to join cancellation with actual process exit; unconfirmed exit retains scratch,
closes the caller to subsequent operations and is not a known success.

Configure these installation inputs in Flame. Environment names are the backend
projection of those inputs, not an alternative configuration owner.

| Installation input        | Backend projection              | Value                                                                                        |
| ------------------------- | ------------------------------- | -------------------------------------------------------------------------------------------- |
| `office-executable`       | `FLAME_DELIVERABLES_OFFICE`     | Absolute LibreOffice executable path                                                         |
| `pdfinfo-executable`      | `FLAME_DELIVERABLES_PDFINFO`    | Absolute Poppler `pdfinfo` path                                                              |
| `pdftoppm-executable`     | `FLAME_DELIVERABLES_PDFTOPPM`   | Absolute Poppler `pdftoppm` path                                                             |
| `pdftotext-executable`    | `FLAME_DELIVERABLES_PDFTOTEXT`  | Absolute Poppler `pdftotext` path; optional text extraction                                  |
| `tool-installation-roots` | `FLAME_DELIVERABLES_TOOL_ROOTS` | JSON array of specific read-only installation directories when system roots are insufficient |

Creation, inspection, native editing and native export do not require Office or
Poppler. Native parsing still requires bubblewrap and `prlimit` on Linux, or
Seatbelt on macOS. Office rendering needs LibreOffice, `pdfinfo` and `pdftoppm`; PDF rendering
needs the latter two. Recalculation needs LibreOffice. An unconfigured engine is
visible in the catalog and refuses only operations requiring it. Format processing
currently requires Linux or macOS; Windows does not silently run without limits.

## Delivery

Uploads and downloads use bounded, offset-checked chunks. The archive admits the
declared byte count and SHA-256 before importing an upload. Export can return the
native file, the rendered PDF or a ZIP containing the native revision, available
derivatives and a source-bound evidence manifest; XLSX also exports one inspected
sheet as CSV. Saving uses the public
`ComponentContext.saveFile` capability. Browser download requests are distinguished
from confirmed native writes and user cancellation.

Files remain in `PLUGIN_DATA/deliverables.db`; the plugin does not invent a second
workspace file identity. Sources, units, assumptions and reproducibility notes are
author-supplied provenance and remain distinguishable from observed checks.
External sharing and messaging continue to belong to their existing connectors.

## Development

Install dependencies at the repository root. Generate contracts and components
with `make generate`. Build and export through the ordinary first-party tooling:

```sh
node plugins/scripts/build-packages.mjs --plugin deliverables --output /absolute/new/packages
node plugins/scripts/build-author-kit.mjs --plugin deliverables --output /absolute/new/author --offline
```

The managed Node backend is bundled with its dependencies. Activation performs
no npm install. LibreOffice, Poppler and the platform confinement tool are external
host prerequisites; their binaries are not included in the plugin archive.

For explicit host qualification, configure those environment projections and run
`FLAME_DELIVERABLES_NATIVE_TESTS=1` with the backend test runner. The native tests
exercise the production confinement, actual DOCX/XLSX/PPTX rendering and observed
workbook formula results. Ordinary tests remain independent of host renderer
installation. macOS execution and the native save panel require qualification on
macOS; a Linux pass cannot establish those properties.

The public component SDK now requires binary `saveFile`; `saveTextFile` has been
removed from the host contract. Rebuild the host and external SDK consumers
together. Existing installations must explicitly install/select the rebuilt
release. Private archive schemas are exact: an incompatible directory is refused
and must be retained separately, with no implicit conversion or compatibility
reader.

## Design references

[Claude file creation](https://support.claude.com/en/articles/12111783-create-and-edit-files-with-claude)
motivates a complete editable-file workflow across documents, spreadsheets,
presentations and PDF. [Manus native PowerPoint](https://manus.im/blog/manus-ppt-slides)
supports authoring native editable chart/table/master objects directly.
[ChatGPT Slides](https://help.openai.com/en/articles/20001605-creating-and-collaborating-on-slides-in-chatgpt)
emphasizes revision and review of the exported result. Flame implements those
principles through its existing plugin lifecycle rather than another artifact or
agent runtime.

The native writers are [docx](https://docx.js.org/),
[ExcelJS](https://github.com/exceljs/exceljs) and
[PptxGenJS](https://gitbrent.github.io/PptxGenJS/); PDF edits use
[pdf-lib](https://pdf-lib.js.org/). They do not own Flame revisions or receipts.
Workbook calculation belongs to LibreOffice, not model-supplied formula caches.
Its [command-line interface](https://help.libreoffice.org/latest/en-US/text/shared/guide/start_parameters.html)
and [recalculation enum](https://docs.libreoffice.org/sc/html/calcconfig_8hxx_source.html)
define the engine invocation/profile behavior. Bundled dependency licenses are
retained in `package/THIRD_PARTY_NOTICES.txt`.
