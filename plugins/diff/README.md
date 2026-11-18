# Workspace Diff

The optional workspace component calls canonical `workspace.diff.get` through the
public SDK. Host supplies an immutable workspace reference and file-focus revision;
Runtime owns confinement, Git baselines and read bounds. The package has no backend
process, private application import, HTML carrier or own callable contribution.

Comparison mode, file selection, layout and filters are local presentation state. A fresh
workspace/focus instance defaults to working-tree comparison. Switching comparison
cancels the prior read. Failed reads retain the accepted snapshot and label its
actual comparison. Renames, binary evidence and partial reads retain their canonical
meaning. File rows and the reader heading render the canonical Runtime file status
through the package catalogs; the accepted read keeps its wire values. Host
target/focus replacement retires the render instance and joins SDK I/O.

The general entry starts at a changed-file overview with aggregate text counts.
A file-specific entry passes its normalized path to Runtime instead of searching
a workspace-wide slice. Its independent Files command requests the workspace
overview, retaining the current comparison. Web requests only `format: "visual"`.
Runtime captures its baseline, bounded patch and authoritative manifest in one read.
The manifest owns status, rename provenance, binary evidence and known counts;
Pierre only renders the accepted patch. No second rows, raw or changes request
supplements that snapshot. File and row budgets select complete files through
Runtime's existing parser, under the byte safety bound. Responses publish the
actual budgets and truncation. An empty truncated manifest means no complete file
fit, not a clean comparison. Web deliberately removes the former full raw reread;
its Copy included patch action copies only the accepted bytes, including headers
and EOF markers. CLI retains complete raw and bounded rows for its own consumers.

The package parses each complete Git file patch through public `@pierre/diffs`
`processFile`, with explicit errors and line matching disabled, and checks it
against the manifest. Parse failures retain the prior accepted read and surface
the failure. Missing context remains missing; no reconstructed full files or
expand-to-full-source requests are supplied.
The host renders the accepted file patch through the shared `@flame/ui`
`DiffBody` slot. The package bundles no highlighter, worker or theme: the host's
Pierre renderer, its single worker pool and its reader labels serve code and
diffs alike, and a host without that slot shows the literal patch. The bounded
current-file workload renders without virtualization, so native selection covers
every presented line without a second hidden DOM. The host reader handles Select
All and projects the native composed range into parsed source lines for clipboard
text. Gutters and EOF labels are excluded; partial UTF-16 offsets, blank lines,
CRLF and the final source newline are preserved. When a selection crosses the EOF
of one side into the other, the displayed rows remain separated. Split-layout
selection follows the selected physical columns; the independent included-patch
action always copies the same accepted patch. Native composed-range support is
part of the shared Chrome 137 / Safari 26 browser floor.

File rows navigate to one reader;
they do not expand every file's code into a single report. The comparison and
refresh controls precede the list filter. In the reader, the current file, position,
previous/next commands and layout stay in the shared persistent control zone.
The accepted comparison, file/workspace scope, real baseline and read time stay
beside that identity in the reader and overview. Read time records client
acceptance, not a repository revision. A pending or failed request retains these
facts with the accepted content; only a successful response advances them.
Display-locale changes reformat labels and dates without changing those accepted
facts, selection, filters or layout. They preserve the original pending read and
issue no request. Read lifetimes follow the captured Runtime, workspace, signal,
tracking collaborator and comparison query. Actual replacement, query changes
and retirement retain their cancellation boundaries.
Failures retain their original cause. Error and RPC detail text stays literal;
a missing or unsupported cause uses the current display locale's fallback.
Full read feedback belongs to scrolling content. The persistent toolbar keeps a
compact failure cue and Refresh; verbose feedback cannot consume the reading
viewport beside a long filename at large font sizes or zoom.
The persistent filename takes at most two lines. The complete current path and
rename provenance remain selectable in the scrolling content for text, binary
and metadata-only files; complete paths also remain in the overview rows.

Returning from a list-owned reader retains its filter and scroll and focuses the
current row, including after a failed comparison. Only a return that actually
starts a workspace read waits for that response. A failed workspace read keeps
the scoped result and its Files command available for an explicit retry.
Before the first accepted overview, retiring the scoped Files action hands focus
to the existing Refresh control while the filter is unavailable. Success continues
to the selected row or filter; later outside focus or deliberate blur retires
that pending continuation.
Changing or clearing the local filter reveals the beginning of its
results while retaining search focus. Unchanged filters and refreshed reads
preserve the current reading position. A direct file entry leaves Escape to the
host's source-return owner;
its independent Files command opens the overview. Layout popups consume their own
dismissal first. Refresh retains the chosen reader when it remains in the accepted
comparison; an absent file returns to the overview with an explicit notice. Failed
reads keep the accepted comparison visibly identified beside the scope control.
The package remains a read-only current-workspace view, not a stored Tool diff.

The working-tree action's accessible name combines its localized purpose with
the same visible line totals or file count rendered inside the button. Native
labelling references those nodes rather than serializing a second count label.

The generic material key `diff` participates in the existing workspace navigation
consumer. Absent/competing candidates open the catalogue. Disablement/uninstall
withdraws registrations and package resources without changing files, Git history,
recorded Tool results or independent Runs. An evaluated release replacement requires
the normal window reload.

Build icons and UI, install the portable toolchain, then run from the repository root:

```sh
npm run build --prefix plugins
npm run check:resources --prefix plugins
```

Edit `component/component.tsx` and rebuild the tracked ESM. React/JSX/UI are host-provided
shared modules. Installation admits exact package bytes without executing code.
Existing installations must explicitly select the rebuilt package digest through
the normal resource rollout; rebuilding source does not update an admitted release.
