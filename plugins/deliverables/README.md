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
receipts. Receipts bind the validated canonical input by digest, so retired upload
chunks leave no payload in the ledger. Business payload bytes and pending-command
admission are budgeted independently of retained outcomes. Completed identities
remain readable and cannot recreate a discarded upload or removed document. Revision publication and command settlement share one transaction.
The backend SDK owns transaction admission and fences the original connection
after an unconfirmed rollback. Reads and receipt lookup resume only after reopening.
A cancellation observed before publication can be confirmed in its original receipt. An admitted command with no observed settlement remains unconfirmed after interruption;
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

Content JSON uses Frame's exact numeric codec before projection. Values that cannot
be represented faithfully are refused at authoring boundaries. Imported workbook
numbers outside that projection remain native decimal text in inspection; CSV
retains their original numeric lexemes. Formula-like string values are escaped
for spreadsheet consumers, and missing or error formula caches prevent CSV export.

## Interaction

The workspace page supplies guided document, table and slide creation, advanced
schema-based input, resumable imports, revision history, local edit forms, actual
page previews, attributed reviews and download controls. Semantic document
operations are available as model tools. Binary staging and transport
(`upload`, `append`, `discard`, `download`) remain typed host/UI operations;
models can inspect staging metadata with `uploads` and import an already complete,
verified upload by identity. Models export immutable asset metadata instead of
moving a file through base64 tool messages. The additional read-only `page` tool
returns an actual retained PNG so a model can examine the render it is reviewing.
Every method declaration explicitly selects its `model` or `host` audience;
adding a new method cannot silently expose a new transfer operation to the model.

### Bounded model inspection

The native archive, complete typed contract and original receipts remain the
business owners. `backend/model.ts` owns only a read-only model projection; no
new evidence database or reconstructed source document is introduced. Model
text results have a 32 KiB UTF-8 limit. Structured tool results retain the complete
original value, including native identities, provenance and original reports.
The same pure command projection is used for the initial result and receipt
recovery. Actual page images remain image content, outside this text budget.

`catalog` returns a short format, engine and limit overview. Request
`{"format":"docx","section":"edit"}` for one original edit schema, or choose
`create` or `example`; `engines` and `limits` read those exact sections. The model
does not receive every native authoring schema on each discovery call.

`read` without a path returns the document/revision/hash identity, native asset
identity, inspection counts and whether the retained inspection was truncated
by its original format owner. For details, pass that exact `revision`, its returned
`sourceSha256` and a path
such as `["inspection","items"]`, `["inspection","checks"]`,
`["revision","provenance"]`, `["assessments"]` or `["processing"]` when present.
Paging without both source identities is refused, and the expected hash is
checked against the original native revision. A later edit cannot shift a
continuation to a different native source. The document header remains the
aggregate's current projection; its `head` can advance. Assessment headers are
append-ordered current records for the pinned native revision, not a frozen
historical list. Their original identities select immutable reports.
`assessment` similarly defaults to an
exact report summary; `["report","pages"]`, `["report","checks"]`,
`["report","extractedText"]`, `["report","notes"]` and `["assets"]` select
its original evidence where applicable. A rendered report still reports its
unverified font status and error/warning counts; a successful render is not
treated as a visual review.

Large selected collections return original keys/indexes, complete small entries
and explicit references for larger entries. Append an entry's `key` to the path
to read that value. `offset` continues at `nextOffset`; collection pages contain
at most 32 entries and may return fewer to fit the byte budget. Strings page by
Unicode code points, with explicit total and continuation positions, so their
original content can be reconstructed exactly. Missing paths, invalid offsets
and excessive selectors are refused. Projection pagination does not alter or
hide the format owner's separate retained-inspection truncation flag.

Create and import each have one inline disclosure entry. Closing either keeps
its local draft and native-file selection; reopening resumes the same form.
Disclosure owns expansion, while the existing forms and command/upload owners
retain their inputs and recovery state.

Document and revision-history pagination keeps its controls focusable while a
read is pending and at page boundaries. Pending controls reject repeat activation;
settlement preserves the user's current focus, including a later move to another
control. Read failures retain the accepted page and its available navigation.
The exact revision being read remains a selectable option alongside each history
page, so paging does not replace its label with a raw identity or change selection.
Opening a retained document, a completed command's revision or the Inspector's
current revision from its focused control moves focus to the stable
revision-history region before the read settles. This includes the region's
first appearance and an exact revision that is already selected, which is not
read again. Activation without that focus, or from a retired context, leaves
focus unchanged. If the pending page lacks room to reveal the reader, settlement
reveals that same region beneath the shared Panel controls only while it still
owns focus. Another control's focus cancels the opening intent, including a
return to the original catalog row. Settlement retires the intent; Refresh and
background reads do not originate it.
Refresh uses the same focusable pending boundary while the list or catalog rereads.
An unread selected revision uses its short identity until metadata is available.
The history region's existing identity disclosure keeps the full document and
revision identities available during reading and after failure. Native facts
appear only after a successful read; settlement preserves the open disclosure
and its focus.

Browsing dates follow the host locale and local time zone. Only the backend's
canonical ISO timestamps are interpreted; other recorded text stays unchanged.
The identity disclosure retains exact document and revision creation times from
accepted metadata, including milliseconds and the UTC designation. Missing
metadata does not acquire an inferred time.

Inspected items show a bounded content excerpt before their native kind and
identity. Expanding an item exposes the complete retained text with its original
whitespace. Preview boundaries preserve whole graphemes; empty items retain their
identity instead of invented content. Search continues to use the full retained
text and exact native identity, with the same whitespace normalization as the
excerpt. Local page boundaries stay focusable and reject unavailable activation,
keeping keyboard reading in the inspected list.

Each rendered page thumbnail opens its original PNG in the shared image reader
for local zoom and panning. The captured revision, render and page identity remain
with the preview owner. Opening, zooming or closing the image does not mark a
review accepted or change the selected page. Existing review controls continue
to own those observations.

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
macOS also refuses execution under an incompatible inherited Seatbelt profile;
the plugin does not retry outside its required confinement.

Linux also requires `/usr/bin/prlimit` and a read-only procfs mount inside its
private PID namespace. Dynamic loaders use `/proc/self/exe` to resolve libraries
relative to their executable; an empty procfs cannot satisfy that dependency.
A host that prohibits the private mount is refused. Hosts enforcing
[AppArmor user-namespace restrictions](https://documentation.ubuntu.com/security/security-features/privilege-restriction/apparmor/#apparmor-unprivileged-user-namespace-restrictions)
must permit bubblewrap to create its private namespaces; CI loads Ubuntu's
packaged `bwrap-userns-restrict` profile. The sandbox applies address-space, CPU,
single-file and diagnostic-output limits before executing any renderer, including
version probes. Debian-family packages also require read-only access to the shared
LibreOffice configuration in `/etc/libreoffice`. macOS applies CPU, single-file and
open-file limits before execution
and samples resident memory every 25 ms, terminating the process on excess or
measurement failure. This sampled threshold allows brief overshoot; it is not a
kernel address-space limit. All bounds are published by `contracts/limits.ts`. Optional
`FLAME_DELIVERABLES_TOOL_ROOTS` is a JSON array of specific canonical installation
directories needed by tools outside the system directories. General home,
workspace and root directories, or paths overlapping plugin data, are refused.
Private plugin data must also remain outside every fixed system read root.
On Linux, configure the distribution's `libreoffice` entry point, whose startup
wrapper owns profile initialization and restart within the private PID namespace.
Homebrew Poppler on macOS requires both its `Cellar` and `opt` directories as
installation roots because its dynamic libraries use `opt` symlink paths.
Fonts in user homes are not exposed. macOS runs the pinned LibreOfficeKit native
worker directly with `process-fork` denied. The desktop `soffice` launcher depends
on desktop IPC and GUI services even with `--headless`, so it is not an accepted
macOS installation. The native worker uses the same private profile, resource
limits and process owner; no GUI, network or child-process permission is added.
Its release manifest and executable are verified before invocation, and its
report identifies the pinned LibreOffice core and native package versions.

The private LibreOffice profile disables macros, active content and external-link
updates and sets `OOXMLRecalcMode=0`, the engine's always-recalculate-on-load mode.
macOS workbook conversion also explicitly requests LibreOfficeKit recalculation.
An ordinary conversion with the engine's default profile can retain stale caches.
The macOS PDF export limits embedded images to 300 DPI; page preview dimensions
remain independently bounded.
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
| `office-executable`       | `FLAME_DELIVERABLES_OFFICE`     | Absolute staged `dsoffice` path on macOS; distribution LibreOffice entry point on Linux      |
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

### macOS Office installation

From the repository root, stage the pinned native package for the current
architecture into an absolute installation directory:

```sh
node tooling/office-toolchain.mjs --output /absolute/toolchains
```

The command prints the executable path to use for `office-executable`. Add its
package directory (the parent of `bin`) to `tool-installation-roots`, alongside
Homebrew's `Cellar` and `opt` directories when using Homebrew Poppler, or `/opt/local`
when using MacPorts. CI builds against a pinned MacPorts ports tree on both macOS architectures.
MacPorts can use a matching binary archive or build from the pinned port's
verified source. The three PDF executables are installed under `/opt/local/bin`. Homebrew
[no longer publishes new Intel bottles](https://docs.brew.sh/Support-Tiers), so
MacPorts is the supported Intel installation route. Existing
macOS installations must replace their `soffice` input and its installation root
with these values while the backend is quiescent, then select the rebuilt plugin.
No artifact data migration is required.

`backend/office-release.json` pins the Apple Silicon and Intel package URLs,
archive integrity, manifest and executable hashes. Installation verifies the
archive before extraction and every manifest file before publishing the directory.
The full package, including licenses, notices and corresponding source, is retained.
`--offline` uses the prepared archive cache; `--arch arm64` or `--arch x64` stages a
specific macOS architecture. Reusing an installation checks its files again and
refuses corruption; replace a damaged, quiescent installation as a unit. Runtime
activation does not download or repair tools.

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
installation. The workspace CI jobs install LibreOffice and Poppler and run
this qualification on Linux and both macOS architectures before the complete workspace checks, so
host prerequisites fail at the native boundary. The native save panel requires
qualification on macOS; a Linux pass cannot establish those properties.

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
